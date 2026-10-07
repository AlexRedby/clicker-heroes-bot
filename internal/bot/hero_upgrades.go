package bot

import (
	"context"
	"image"
	"strconv"
	"strings"
)

// Official client 6144 bulk entry (WASM 27529) calls the chain buyer (27528),
// which skips Ascension IDs 106 and 132 before granting. Use the footer, not skill icons.
func readHeroUpgradeButton(ctx context.Context, screen image.Image) (image.Point, bool, error) {
	if !heroQuantityBarPresent(screen) {
		return image.Point{}, false, nil
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	start, last := -1, -1
	lastRow, gap := b.Min.Y+h*98/100, max(2, h/60)
	// Virtual empty rows flush a footer that ends near the capture boundary.
	for y := b.Min.Y + h*4/10; y <= lastRow+gap+1; y++ {
		green, total := 0, 1
		for x := b.Min.X + w*28/100; y <= lastRow && x < b.Min.X+w*40/100; x += max(1, w/500) {
			r, g, blue := rgb(screen.At(x, y))
			if g > 100 && g > r+50 && g > blue+50 {
				green++
			}
			total++
		}
		if green*10 >= total*3 {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start < 0 || y-last <= max(2, h/60) {
			continue
		}
		point := image.Pt(b.Min.X+w*34/100, (start+last)/2)
		height := last - start + 1
		start = -1
		if height < h/40 || height > h/10 {
			continue
		}
		raw, err := readGameText(ctx, screen, heroUpgradeButtonRegion(screen, point), max(1, 2048/w), 7, 180, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz ")
		if err != nil {
			return image.Point{}, false, err
		}
		if strings.ReplaceAll(strings.TrimSpace(raw), " ", "") == "BuyAvailableUpgrades" {
			return point, true, nil
		}
	}
	return image.Point{}, false, nil
}

func heroUpgradeButtonRegion(screen image.Image, point image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*268/1000, point.Y-b.Dy()*3/100, b.Min.X+b.Dx()*425/1000, point.Y+b.Dy()*3/100).Intersect(b)
}

// Queue guard for a previously OCR-identified footer; a displaced/covered name
// cannot authorize a click. Caller also owns reset stage, x1 and modal isolation.
func heroUpgradeButtonStable(before, after image.Image, point image.Point) bool {
	return before != nil && after != nil && heroQuantityBarPresent(after) && heroTextStable(before, after, heroUpgradeButtonRegion(before, point))
}

func clickHeroUpgrades(ctx context.Context, input heroInput, point image.Point) error {
	return clickHeroModified(ctx, input, point, "")
}

// A disabled footer still identifies a completed upgrade check; no click is needed.
func readHeroUpgradeFooter(ctx context.Context, screen image.Image) (image.Point, bool, bool, error) {
	point, available, err := readHeroUpgradeButton(ctx, screen)
	if err != nil || available {
		return point, available, available, err
	}
	if !heroQuantityBarPresent(screen) {
		return image.Point{}, false, false, nil
	}
	b := screen.Bounds()
	region := image.Rect(b.Min.X+b.Dx()*268/1000, b.Min.Y+b.Dy()*4/10, b.Min.X+b.Dx()*425/1000, b.Min.Y+b.Dy()*98/100)
	scale := max(1, 2048/b.Dx())
	raw, err := readGameText(ctx, screen, region, scale, 11, 180, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz ", "tsv")
	if err != nil {
		return image.Point{}, false, false, err
	}
	point, known := heroFooterTextPoint(raw, region, scale, b.Min.X+b.Dx()*34/100)
	return point, known, false, nil
}

// Disabled footer text provides its position without assuming a fixed list height.
func heroFooterTextPoint(raw string, region image.Rectangle, scale, x int) (image.Point, bool) {
	type line struct {
		text        string
		top, bottom int
	}
	lines := map[string]line{}
	for _, row := range strings.Split(raw, "\n") {
		fields := strings.Split(row, "\t")
		if len(fields) != 12 || fields[0] != "5" || strings.TrimSpace(fields[11]) == "" {
			continue
		}
		y, ye := strconv.Atoi(fields[7])
		h, he := strconv.Atoi(fields[9])
		if ye != nil || he != nil || h <= 0 || y < gameTextPadding || y+h > gameTextPadding+region.Dy()*scale {
			continue
		}
		key := strings.Join(fields[1:5], "/")
		value, exists := lines[key]
		if !exists {
			value.top = y
		}
		value.top, value.bottom = min(value.top, y), max(value.bottom, y+h)
		value.text += strings.TrimSpace(fields[11])
		lines[key] = value
	}
	point, found := image.Point{}, false
	for _, value := range lines {
		if value.text != "BuyAvailableUpgrades" {
			continue
		}
		if found {
			return image.Point{}, false
		}
		point = image.Pt(x, region.Min.Y+((value.top+value.bottom)/2-gameTextPadding)/scale)
		found = true
	}
	return point, found
}
