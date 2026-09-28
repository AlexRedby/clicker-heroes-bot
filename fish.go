package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/png"
	"math"
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

// Sparse matching scans the full circle in 5-degree steps; limit the search area if large screens make scans too slow.
func findFish(screen, fish image.Image) (image.Point, bool) {
	sb, fb := screen.Bounds(), fish.Bounds()
	fw, fh := fb.Dx(), fb.Dy()
	if fw == 0 || fh == 0 {
		return image.Point{}, false
	}

	for degrees := 0; degrees < 360; degrees += 5 {
		angle := float64(degrees) * math.Pi / 180
		for height := max(50, sb.Dy()/30); height <= min(400, sb.Dy()/4); height += max(2, height/12) {
			template, anchors, samples := makeFishTemplate(fish, height, angle)
			if len(anchors) != 4 || len(samples) == 0 {
				continue
			}
			bounds := template.Bounds()
			w, h := bounds.Dx(), bounds.Dy()
			step := max(2, height/24)
			for y := sb.Min.Y; y+h <= sb.Max.Y; y += step {
				for x := sb.Min.X; x+w <= sb.Max.X; x += step {
					if !matchesFishSamples(screen, x, y, anchors, 100, len(anchors)) {
						continue
					}
					if matchesFishSamples(screen, x, y, samples, 100, len(samples)*17/20) {
						return image.Pt(x+w/2, y+h/2), true
					}
				}
			}
		}
	}
	return image.Point{}, false
}

func makeFishTemplate(fish image.Image, height int, angle float64) (*image.RGBA, []fishSample, []fishSample) {
	fb := fish.Bounds()
	width := height * fb.Dx() / fb.Dy()
	cos, sin := math.Cos(angle), math.Sin(angle)
	rotWidth := int(math.Ceil(math.Abs(float64(width)*cos) + math.Abs(float64(height)*sin)))
	rotHeight := int(math.Ceil(math.Abs(float64(width)*sin) + math.Abs(float64(height)*cos)))
	template := image.NewRGBA(image.Rect(0, 0, rotWidth, rotHeight))
	for y := 0; y < rotHeight; y++ {
		for x := 0; x < rotWidth; x++ {
			dx, dy := float64(x)-float64(rotWidth)/2, float64(y)-float64(rotHeight)/2
			sx := cos*dx + sin*dy + float64(width)/2
			sy := -sin*dx + cos*dy + float64(height)/2
			if sx < 0 || sy < 0 || sx >= float64(width) || sy >= float64(height) {
				continue
			}
			c := fish.At(fb.Min.X+int(sx)*fb.Dx()/width, fb.Min.Y+int(sy)*fb.Dy()/height)
			_, _, _, alpha := c.RGBA()
			if alpha >= 0x8000 {
				template.Set(x, y, c)
			}
		}
	}

	var anchors []fishSample
	for _, point := range []image.Point{{140, 220}, {400, 360}, {500, 530}, {580, 840}} {
		dx := (float64(point.X)+0.5)*float64(width)/float64(fb.Dx()) - float64(width)/2
		dy := (float64(point.Y)+0.5)*float64(height)/float64(fb.Dy()) - float64(height)/2
		x := int(math.Round(cos*dx - sin*dy + float64(rotWidth)/2))
		y := int(math.Round(sin*dx + cos*dy + float64(rotHeight)/2))
		for offsetY := -2; offsetY <= 2; offsetY++ {
			for offsetX := -2; offsetX <= 2; offsetX++ {
				c := template.At(x+offsetX, y+offsetY)
				_, _, _, alpha := c.RGBA()
				if alpha >= 0xe000 {
					anchors = append(anchors, fishSample{x + offsetX, y + offsetY, c})
					goto nextAnchor
				}
			}
		}
	nextAnchor:
	}
	var samples []fishSample
	for y := rotHeight / 20; y < rotHeight; y += max(1, rotHeight/16) {
		for x := rotWidth / 20; x < rotWidth; x += max(1, rotWidth/12) {
			c := template.At(x, y)
			_, _, _, alpha := c.RGBA()
			if alpha >= 0xe000 {
				samples = append(samples, fishSample{x, y, c})
			}
		}
	}
	return template, anchors, samples
}

func matchesFishSamples(screen image.Image, x, y int, samples []fishSample, tolerance, minimum int) bool {
	matches := 0
	for i, sample := range samples {
		if colorDifference(screen.At(x+sample.x, y+sample.y), sample.color) <= tolerance {
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
