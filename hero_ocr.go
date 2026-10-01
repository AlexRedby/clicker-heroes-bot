package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gocv.io/x/gocv"
)

var gameNumber = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(?:[eE]([0-9]+))?$`)
var levelNumber = regexp.MustCompile(`(?i)^[li]v[li]\s*([0-9]+)$`)
var tesseractExecutable = "tesseract"
var ocrTimeout = 3 * time.Second
var errUnreadableGameNumber = errors.New("unreadable game number")

const gameTextPadding = 10

func parseGameNumber(raw string) (float64, bool) {
	match := gameNumber.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return 0, false
	}
	if dot := strings.IndexByte(match[1], '.'); dot >= 0 && len(match[1])-dot-1 > 3 {
		return 0, false
	}
	mantissa, err := strconv.ParseFloat(match[1], 64)
	if err != nil || mantissa < 0 {
		return 0, false
	}
	if mantissa == 0 {
		return math.Inf(-1), true
	}
	exponent := 0
	if match[2] != "" {
		if mantissa < 1 || mantissa >= 10 {
			return 0, false
		}
		exponent, err = strconv.Atoi(match[2])
		if err != nil {
			return 0, false
		}
	}
	return math.Log10(mantissa) + float64(exponent), true
}

type cappedBuffer struct {
	b bytes.Buffer
	n int
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	if w.b.Len() < w.n {
		_, _ = w.b.Write(p[:min(len(p), w.n-w.b.Len())])
	}
	return len(p), nil
}

var ocrSlots = make(chan struct{}, 2)

func runTesseract(ctx context.Context, encoded []byte, args ...string) (string, error) {
	select {
	case ocrSlots <- struct{}{}:
		defer func() { <-ocrSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, ocrTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tesseractExecutable, append([]string{"stdin", "stdout"}, args...)...)
	// Keep crop OCR from creating competing OpenMP thread pools alongside SIFT.
	cmd.Env = append(cmd.Environ(), "OMP_THREAD_LIMIT=1")
	cmd.Stdin = bytes.NewReader(encoded)
	var stderr cappedBuffer
	stderr.n = 2048
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		message := strings.TrimSpace(stderr.b.String())
		if message != "" {
			return "", fmt.Errorf("tesseract: %w: %s", err, message)
		}
		return "", fmt.Errorf("tesseract: %w", err)
	}
	return string(output), nil
}

func readGameText(ctx context.Context, screen image.Image, region image.Rectangle, scale, psm int, whiteThreshold int, characters string, formats ...string) (string, error) {
	if screen == nil {
		return "", fmt.Errorf("nil OCR screen")
	}
	region = region.Intersect(screen.Bounds())
	if region.Empty() {
		return "", fmt.Errorf("empty OCR region %v", region)
	}
	whiteOnly := whiteThreshold != 0
	yellowThreshold := 0
	if whiteThreshold == 0 {
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				r, g, b := rgb(screen.At(x, y))
				if r-b > 40 && g-b > 25 {
					yellowThreshold = max(yellowThreshold, min(r, g))
				}
			}
		}
		// Dim disabled text needs a lower threshold to preserve thin glyph strokes.
		if yellowThreshold < 180 {
			yellowThreshold = max(60, yellowThreshold/2)
		} else {
			yellowThreshold = yellowThreshold * 2 / 3
		}
	}
	source := image.NewGray(image.Rect(0, 0, region.Dx(), region.Dy()))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			r, g, b := rgb(screen.At(region.Min.X+x, region.Min.Y+y))
			if whiteOnly {
				if min(r, g, b) > max(whiteThreshold, -whiteThreshold) && max(r, g, b)-min(r, g, b) < 55 {
					source.SetGray(x, y, color.Gray{Y: 255})
				}
			} else {
				// Price text is yellow on a blue/dark button. Exclude its gold border and icon in the crop.
				v := uint8(255)
				if min(r, g) > yellowThreshold && r-b > 40 && g-b > 25 {
					v = 0
				}
				source.SetGray(x, y, color.Gray{Y: v})
			}
		}
	}
	if whiteThreshold < 0 {
		// Remove light scenery touching the crop edge; outlined white text remains isolated.
		mask, err := gocv.NewMatFromBytes(region.Dy(), region.Dx(), gocv.MatTypeCV8UC1, source.Pix)
		if err != nil {
			return "", err
		}
		labels, stats, centroids := gocv.NewMat(), gocv.NewMat(), gocv.NewMat()
		defer mask.Close()
		defer labels.Close()
		defer stats.Close()
		defer centroids.Close()
		count := gocv.ConnectedComponentsWithStats(mask, &labels, &stats, &centroids)
		background := make([]bool, count)
		for i := 1; i < count; i++ {
			x := int(stats.GetIntAt(i, int(gocv.CC_STAT_LEFT)))
			y := int(stats.GetIntAt(i, int(gocv.CC_STAT_TOP)))
			width := int(stats.GetIntAt(i, int(gocv.CC_STAT_WIDTH)))
			height := int(stats.GetIntAt(i, int(gocv.CC_STAT_HEIGHT)))
			background[i] = x == 0 || y == 0 || x+width == region.Dx() || y+height == region.Dy()
		}
		for y := 0; y < region.Dy(); y++ {
			for x := 0; x < region.Dx(); x++ {
				if background[labels.GetIntAt(y, x)] {
					source.Pix[y*source.Stride+x] = 0
				}
			}
		}
	}
	padding := gameTextPadding
	upscaled := image.NewGray(image.Rect(0, 0, source.Bounds().Dx()*scale+2*padding, source.Bounds().Dy()*scale+2*padding))
	if !whiteOnly {
		draw.Draw(upscaled, upscaled.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	}
	dst := image.Rect(padding, padding, upscaled.Bounds().Max.X-padding, upscaled.Bounds().Max.Y-padding)
	if whiteOnly {
		xdraw.NearestNeighbor.Scale(upscaled, dst, source, source.Bounds(), draw.Src, nil)
	} else {
		xdraw.ApproxBiLinear.Scale(upscaled, dst, source, source.Bounds(), draw.Src, nil)
	}

	return readTextImage(ctx, upscaled, psm, characters, formats...)
}

func readTextImage(ctx context.Context, input image.Image, psm int, characters string, formats ...string) (string, error) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, input); err != nil {
		return "", err
	}
	args := []string{"-l", "eng", "--psm", strconv.Itoa(psm), "-c", "tessedit_char_whitelist=" + characters}
	output, err := runTesseract(ctx, encoded.Bytes(), append(args, formats...)...)
	if err != nil {
		return "", fmt.Errorf("tesseract: %w", err)
	}
	return output, nil
}

func readGameNumber(ctx context.Context, screen image.Image, region image.Rectangle, scale, psm int, threshold int) (float64, error) {
	raw, err := readGameText(ctx, screen, region, scale, psm, threshold, "0123456789.eE")
	if err != nil {
		return 0, err
	}
	value, ok := parseGameNumber(raw)
	if !ok {
		return 0, fmt.Errorf("%w %q", errUnreadableGameNumber, strings.TrimSpace(raw))
	}
	return value, nil
}

func readHeroGold(ctx context.Context, screen image.Image) (float64, error) {
	if screen == nil {
		return 0, fmt.Errorf("nil OCR screen")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*156/1000, b.Min.Y+h*25/1000, b.Min.X+w*34/100, b.Min.Y+h*12/100)
	value, err := readGameNumber(ctx, screen, region, max(1, 2048/w), 7, 180)
	if !errors.Is(err, errUnreadableGameNumber) || ctx.Err() != nil {
		return value, err
	}
	// A faint leading digit can disappear in the mask; retry only an unreadable result.
	return readGameNumber(ctx, screen, region, max(1, 2048/w), 7, 170)
}

func readHeroPrice(ctx context.Context, screen image.Image, button image.Point) (float64, error) {
	if screen == nil {
		return 0, fmt.Errorf("nil OCR screen")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	start, end, top, bottom := 52, 126, 18, 56
	if w >= 1280 {
		start, end = 74, 137
	}
	bluePixels, samples := 0, 0
	for x := b.Min.X + w*55/1000; x < b.Min.X+w*130/1000; x += max(1, w/500) {
		r, g, blue := rgb(screen.At(x, button.Y))
		if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
			bluePixels++
		}
		samples++
	}
	if bluePixels*10 < samples*3 {
		// Stop above the gold border; it otherwise raises the disabled-text mask threshold.
		top, bottom = 7, 35
	}
	region := image.Rect(b.Min.X+w*start/1000, button.Y+h*top/1000, b.Min.X+w*end/1000, button.Y+h*bottom/1000)
	// Large captures already have readable glyphs; oversizing this font can lose the entire line.
	scale := 1
	if w < 2048 {
		scale = max(3, 6144/w)
	}
	return readGameNumber(ctx, screen, region, scale, 7, 0)
}

func readHeroLevel(ctx context.Context, screen image.Image, button image.Point) (int, error) {
	if screen == nil {
		return 0, fmt.Errorf("nil OCR screen")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*26/100, button.Y-h*6/100, b.Min.X+w*38/100, button.Y+h*3/100).Intersect(b)
	// The level line is nearest the button center; the hero name can be taller.
	start, last, bestStart, bestEnd := -1, -1, -1, -1
	for y := region.Min.Y; y <= region.Max.Y+2; y++ {
		white := 0
		if y < region.Max.Y {
			for x := region.Min.X; x < region.Max.X; x++ {
				r, g, blue := rgb(screen.At(x, y))
				if min(r, g, blue) > 180 && max(r, g, blue)-min(r, g, blue) < 55 {
					white++
				}
			}
		}
		if white >= max(2, w/500) {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start >= 0 && y-last > 2 {
			if bestStart < 0 || absDiff((start+last)/2, button.Y) < absDiff((bestStart+bestEnd)/2, button.Y) {
				bestStart, bestEnd = start, last
			}
			start = -1
		}
	}
	if bestEnd-bestStart < h/50 {
		return 0, fmt.Errorf("hero level text line is missing or obscured")
	}
	region.Min.Y = max(region.Min.Y, bestStart-3)
	region.Max.Y = min(region.Max.Y, bestEnd+4)
	raw, err := readGameText(ctx, screen, region, max(1, 2048/w), 7, 180, "0123456789LVlviI")
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	match := levelNumber.FindStringSubmatch(raw)
	if match == nil {
		return 0, fmt.Errorf("ambiguous hero level %q", raw)
	}
	level, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("invalid hero level %q: %w", raw, err)
	}
	return level, nil
}

func checkHeroOCR(ctx context.Context) error {
	path, err := exec.LookPath(tesseractExecutable)
	if err != nil {
		return fmt.Errorf("tesseract not found: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, ocrTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--list-langs")
	var stderr cappedBuffer
	stderr.n = 2048
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("tesseract language check: %w: %s", err, strings.TrimSpace(stderr.b.String()))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "eng" {
			return nil
		}
	}
	return fmt.Errorf("tesseract English traineddata is unavailable")
}
