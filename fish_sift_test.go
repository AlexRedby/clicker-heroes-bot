package main

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func TestSIFTFishOnGameScreen(t *testing.T) {
	detector, err := newSIFTFishDetector()
	if err != nil {
		t.Fatal(err)
	}
	defer detector.Close()

	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	background, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if point, found, err := detector.Find(background); err != nil || found {
		t.Fatalf("fish-free screen: point=%v found=%t err=%v", point, found, err)
	}

	fish, err := png.Decode(bytes.NewReader(fishPNG))
	if err != nil {
		t.Fatal(err)
	}
	height := 75
	width := fish.Bounds().Dx() * height / fish.Bounds().Dy()
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), fish, fish.Bounds(), draw.Over, nil)
	rotated := image.NewRGBA(image.Rect(0, 0, height, width))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rotated.Set(height-1-y, x, scaled.At(x, y))
		}
	}
	for _, test := range []struct {
		name   string
		sprite image.Image
	}{
		{"upright", scaled},
		{"rotated", rotated},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := image.NewRGBA(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			position := image.Pt(850, 40)
			draw.Draw(screen, test.sprite.Bounds().Add(position), test.sprite, image.Point{}, draw.Over)

			point, found, err := detector.Find(screen)
			expected := position.Add(image.Pt(test.sprite.Bounds().Dx()/2, test.sprite.Bounds().Dy()/2))
			if err != nil || !found || math.Abs(float64(point.X-expected.X)) > 15 || math.Abs(float64(point.Y-expected.Y)) > 15 {
				t.Fatalf("fish screen: point=%v found=%t err=%v, want near %v", point, found, err, expected)
			}
		})
	}
}
