package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var gameNumber = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)(?:[eE]([0-9]+))?$`)

func parseGameNumber(raw string) (float64, bool) {
	match := gameNumber.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return 0, false
	}
	mantissa, err := strconv.ParseFloat(match[1], 64)
	if err != nil || mantissa <= 0 {
		return 0, false
	}
	exponent := 0
	if match[2] != "" {
		exponent, err = strconv.Atoi(match[2])
		if err != nil {
			return 0, false
		}
	}
	return math.Log10(mantissa) + float64(exponent), true
}

func readGameNumber(screen image.Image, region image.Rectangle, scale, psm int, whiteOnly bool) (float64, error) {
	region = region.Intersect(screen.Bounds())
	if region.Empty() {
		return 0, fmt.Errorf("empty OCR region %v", region)
	}
	upscaled := image.NewRGBA(image.Rect(0, 0, region.Dx()*scale, region.Dy()*scale))
	for y := 0; y < upscaled.Bounds().Dy(); y++ {
		for x := 0; x < upscaled.Bounds().Dx(); x++ {
			r, g, b := rgb(screen.At(region.Min.X+x/scale, region.Min.Y+y/scale))
			if whiteOnly {
				low, high := min(r, g, b), max(r, g, b)
				if low > 135 && high-low < 55 {
					upscaled.Set(x, y, color.White)
				} else {
					upscaled.Set(x, y, color.Black)
				}
			} else {
				upscaled.Set(x, y, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, upscaled); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", "eng", "--psm", strconv.Itoa(psm), "-c", "tessedit_char_whitelist=0123456789.eE")
	cmd.Stdin = &encoded
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("tesseract: %w", err)
	}
	value, ok := parseGameNumber(string(output))
	if !ok {
		return 0, fmt.Errorf("unreadable game number %q", strings.TrimSpace(string(output)))
	}
	return value, nil
}

func readHeroGold(screen image.Image) (float64, error) {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*156/1000, b.Min.Y+h*25/1000, b.Min.X+w*34/100, b.Min.Y+h*12/100)
	return readGameNumber(screen, region, max(3, 6144/w), 7, true)
}

func readHeroPrice(screen image.Image, button image.Point) (float64, error) {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*47/1000, button.Y+h*14/1000, b.Min.X+w*135/1000, button.Y+h*66/1000)
	return readGameNumber(screen, region, max(4, 12288/w), 8, false)
}
