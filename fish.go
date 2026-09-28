package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
)

//go:embed assets/orange-fish.png
var fishPNG []byte

type fishSample struct {
	x, y  int
	color color.Color
}

func loadFish() (image.Image, error) {
	return png.Decode(bytes.NewReader(fishPNG))
}

// ponytail: sparse color matching assumes this fish art; use in-game samples if other appearances cause misses.
func findFish(screen, fish image.Image) (image.Point, bool) {
	sb, fb := screen.Bounds(), fish.Bounds()
	fw, fh := fb.Dx(), fb.Dy()
	if fw == 0 || fh == 0 {
		return image.Point{}, false
	}

	anchors := []fishSample{
		{140, 220, fish.At(fb.Min.X+140, fb.Min.Y+220)}, // leaf
		{400, 360, fish.At(fb.Min.X+400, fb.Min.Y+360)}, // eye
		{500, 530, fish.At(fb.Min.X+500, fb.Min.Y+530)}, // body
		{580, 840, fish.At(fb.Min.X+580, fb.Min.Y+840)}, // tail
	}
	var samples []fishSample
	for y := fh / 20; y < fh; y += max(1, fh/16) {
		for x := fw / 20; x < fw; x += max(1, fw/12) {
			c := fish.At(fb.Min.X+x, fb.Min.Y+y)
			_, _, _, a := c.RGBA()
			if a >= 0xe000 {
				samples = append(samples, fishSample{x, y, c})
			}
		}
	}

	for h := max(28, sb.Dy()/30); h <= min(400, sb.Dy()/4); h += max(2, h/12) {
		w := h * fw / fh
		step := max(2, h/24)
		for y := sb.Min.Y; y+h <= sb.Max.Y; y += step {
			for x := sb.Min.X; x+w <= sb.Max.X; x += step {
				if !matchesFishSamples(screen, x, y, w, h, fw, fh, anchors, 100, len(anchors)) {
					continue
				}
				if matchesFishSamples(screen, x, y, w, h, fw, fh, samples, 100, len(samples)*9/10) {
					return image.Pt(x+w*55/100, y+h/2), true
				}
			}
		}
	}
	return image.Point{}, false
}

func matchesFishSamples(screen image.Image, x, y, w, h, fw, fh int, samples []fishSample, tolerance, minimum int) bool {
	matches := 0
	for i, sample := range samples {
		if colorDifference(screen.At(x+sample.x*w/fw, y+sample.y*h/fh), sample.color) <= tolerance {
			matches++
		}
		if matches+len(samples)-i-1 < minimum {
			return false
		}
	}
	return matches >= minimum
}

func colorDifference(a, b color.Color) int {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return abs(int(ar>>8)-int(br>>8)) + abs(int(ag>>8)-int(bg>>8)) + abs(int(ab>>8)-int(bb>>8))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
