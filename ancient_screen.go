package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"regexp"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"

	xdraw "golang.org/x/image/draw"
)

//go:embed assets/ancient-controls.png
var ancientControlsPNG []byte
var ancientControlsImage decodedPNG
var ancientLevelLabel = regexp.MustCompile(`(?i)^[li]v[li1]\s*`)

func ancientControl(screen image.Image, which int) (image.Point, bool, error) {
	regions := [...]image.Rectangle{image.Rect(475, 265, 805, 295), image.Rect(581, 370, 695, 420)}
	refs := [...]image.Rectangle{image.Rect(0, 0, 660, 60), image.Rect(0, 60, 228, 160)}
	atlas, err := ancientControlsImage.get(ancientControlsPNG)
	if err != nil {
		return image.Point{}, false, err
	}
	found, err := matchControl(screen, regions[which], atlas, refs[which])
	r := controlRect(screen, regions[which])
	return r.Min.Add(r.Size().Div(2)), found, err
}
func ancientQuantityDialog(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	// The title belongs to this quantity window, not another cream game modal.
	r := controlRect(screen, image.Rect(410, 310, 411, 311))
	red, g, b := rgb(screen.At(r.Min.X, r.Min.Y))
	if red < 230 || g < 220 || b < 150 {
		return false
	}
	_, found, err := ancientControl(screen, 0)
	return found && err == nil
}
func ancientTabSelected(screen image.Image) bool {
	if screen == nil {
		return false
	}
	b := screen.Bounds()
	r := controlRect(screen, image.Rect(260, 129, 299, 156))
	gold, total := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, blue := rgb(screen.At(x, y))
			total++
			if red >= 160 && g >= 100 && g <= 145 && blue <= 60 {
				gold++
			}
		}
	}
	return b.Dx() >= 640 && total > 0 && gold*4 >= total
}
func ancientTabPoint(screen image.Image, heroes bool) image.Point {
	x := 286
	if heroes {
		x = 80
	}
	r := controlRect(screen, image.Rect(x, 145, x+1, 146))
	return r.Min
}
func ancientButtons(screen image.Image) []image.Point {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	var rows []image.Point
	first, last := -1, -1
	gap := max(3, h/150)
	for y := b.Min.Y + h*40/100; y < b.Min.Y+h*96/100+gap; y++ {
		blue, n := 0, 0
		if y < b.Min.Y+h*96/100 {
			for x := b.Min.X + w*65/1000; x < b.Min.X+w*70/1000; x += max(1, w/500) {
				r, g, bl := rgb(screen.At(x, y))
				n++
				if bl > 150 && bl > r+40 && bl >= g-10 && g > 90 {
					blue++
				}
			}
		}
		if n > 0 && blue*10 > n*3 {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 && y-last > gap {
			if last-first > h/40 && last-first < h/7 {
				rows = append(rows, image.Pt(b.Min.X+w*95/1000, (first+last)/2))
			}
			first = -1
		}
	}
	return rows
}
func ancientLevelRegion(screen image.Image, p image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*35/1280, p.Y-b.Dy()*100/1000, b.Min.X+b.Dx()*195/1280, p.Y-b.Dy()*58/1000)
}
func ancientNameRegion(screen image.Image, p image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*170/1280, p.Y-b.Dy()*100/1000, b.Min.X+b.Dx()*570/1280, p.Y-b.Dy()*58/1000)
}

type ancientScreenRow struct {
	name, level string
	point       image.Point
}
type ancientObservation struct {
	frame       gameFrame
	souls       string
	rows        []ancientScreenRow
	quantity    string
	okay        bool
	thumb       image.Point
	thumbHeight int
	hasThumb    bool
}

// Quantity text is blue, unlike the white HUD text. Prefer that ink so a
// blinking black caret cannot invalidate a ready confirmation.
func ancientQuantityMask(screen image.Image) *image.Gray {
	r := controlRect(screen, image.Rect(508, 325, 772, 350)).Intersect(screen.Bounds())
	crop := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, r.Min, draw.Src)
	mask, _ := ancientQuantityInkMask(crop)
	return mask
}
func ancientQuantityInkMask(screen image.Image) (*image.Gray, bool) {
	r := screen.Bounds()
	blue := func(x, y int) bool { red, g, b := rgb(screen.At(x, y)); return b > red+40 && b > g+30 && b > 100 }
	hasBlue := false
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if blue(x, y) {
				hasBlue = true
			}
		}
	}
	mask := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(mask, mask.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, b := rgb(screen.At(x, y))
			if (hasBlue && blue(x, y)) || (!hasBlue && max(red, g, b) < 140) {
				mask.SetGray(x-r.Min.X, y-r.Min.Y, color.Gray{Y: 0})
			}
		}
	}
	return mask, hasBlue
}
func readAncientQuantity(ctx context.Context, screen image.Image) (string, error) {
	r := controlRect(screen, image.Rect(508, 325, 772, 350)).Intersect(screen.Bounds())
	field := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(field, field.Bounds(), screen, r.Min, draw.Src)
	mask, blue := ancientQuantityInkMask(field)
	// Normalize bold blue game glyphs to the UI's reference scale. Keep fine
	// black text at native resolution so decimal points are not downsampled.
	if blue {
		normalized := image.NewRGBA(image.Rect(0, 0, r.Dx()*1280/screen.Bounds().Dx(), r.Dy()*1280/screen.Bounds().Dx()))
		xdraw.CatmullRom.Scale(normalized, normalized.Bounds(), field, field.Bounds(), draw.Src, nil)
		mask, _ = ancientQuantityInkMask(normalized)
	}
	var ink image.Rectangle
	for y := 0; y < mask.Bounds().Dy(); y++ {
		for x := 0; x < mask.Bounds().Dx(); x++ {
			if mask.GrayAt(x, y).Y == 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if ink.Empty() {
		return "", nil
	}
	source := mask.SubImage(ink)
	scale := 2
	padded := image.NewGray(image.Rect(0, 0, source.Bounds().Dx()*scale+2*gameTextPadding, source.Bounds().Dy()*scale+2*gameTextPadding))
	draw.Draw(padded, padded.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.NearestNeighbor.Scale(padded, padded.Bounds().Inset(gameTextPadding), source, source.Bounds(), draw.Src, nil)
	return readTextImage(ctx, padded, 7, "0123456789.eE+-")
}

func readAncientObservation(ctx context.Context, frame gameFrame) (ancientObservation, error) {
	out := ancientObservation{frame: frame}
	screen := frame.image
	if frame.context.ancientDialog {
		_, out.okay, _ = ancientControl(screen, 1)
		raw, err := readAncientQuantity(ctx, screen)
		if err != nil {
			return out, err
		}
		out.quantity = strings.TrimSpace(raw)
		return out, nil
	}
	if !frame.context.ancients {
		return out, nil
	}
	raw, err := readGameText(ctx, screen, controlRect(screen, image.Rect(410, 172, 591, 199)), max(1, 2560/screen.Bounds().Dx()), 7, 180, "0123456789.eEabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
	if err != nil {
		return out, err
	}
	match := ascensionRewardLabel.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return out, fmt.Errorf("unreadable Ancient soul budget %q", strings.TrimSpace(raw))
	}
	out.souls = match[1]
	for _, point := range ancientButtons(screen) {
		raw, err := readGameText(ctx, screen, ancientLevelRegion(screen, point), max(1, 2560/screen.Bounds().Dx()), 7, 0, "0123456789.eElLvViI")
		if err != nil {
			return out, err
		}
		raw = strings.TrimSpace(raw)
		raw = ancientLevelLabel.ReplaceAllString(raw, "")
		if _, err := ancientcalc.Value(raw); err != nil {
			continue
		}
		name, err := readGameText(ctx, screen, ancientNameRegion(screen, point), max(1, 2560/screen.Bounds().Dx()), 7, -180, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ,'")
		if err != nil {
			return out, err
		}
		name = strings.TrimSpace(strings.Split(name, ",")[0])
		if name == "" {
			continue
		}
		out.rows = append(out.rows, ancientScreenRow{name, raw, point})
	}
	out.thumb, out.thumbHeight, out.hasThumb = listScrollbarThumb(screen, 416)
	return out, nil
}
