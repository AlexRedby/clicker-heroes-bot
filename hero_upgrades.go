package main

import (
	"context"
	"image"
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
	for y := b.Min.Y + h*4/10; y <= b.Min.Y+h*98/100; y++ {
		green, total := 0, 0
		for x := b.Min.X + w*28/100; x < b.Min.X+w*40/100; x += max(1, w/500) {
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
