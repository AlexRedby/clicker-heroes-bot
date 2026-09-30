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

func TestSIFTFishOnHeroScreens(t *testing.T) {
	detector, err := newSIFTFishDetector()
	if err != nil {
		t.Fatal(err)
	}
	defer detector.Close()

	for _, test := range []struct {
		path     string
		expected image.Point
	}{
		{"testdata/hero-panel-max.png", image.Pt(611, 978)},
		{"testdata/hero-owned-disabled.png", image.Pt(1143, 671)},
	} {
		t.Run(test.path, func(t *testing.T) {
			file, err := os.Open(test.path)
			if err != nil {
				t.Fatal(err)
			}
			screen, err := png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			point, found, err := detector.Find(screen)
			if err != nil || !found || math.Abs(float64(point.X-test.expected.X)) > 15 || math.Abs(float64(point.Y-test.expected.Y)) > 15 {
				t.Fatalf("fish screen: point=%v found=%t err=%v, want near %v", point, found, err, test.expected)
			}
		})
	}
}

func TestSIFTFishAtScalesAndEdge(t *testing.T) {
	detector, err := newSIFTFishDetector()
	if err != nil {
		t.Fatal(err)
	}
	defer detector.Close()

	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	background, err := jpeg.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	fish, err := png.Decode(bytes.NewReader(fishPNG))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		height   int
		position image.Point
	}{
		{"small", 50, image.Pt(30, 30)},
		{"large", 100, image.Pt(30, 30)},
	} {
		t.Run(test.name, func(t *testing.T) {
			width := fish.Bounds().Dx() * test.height / fish.Bounds().Dy()
			scaled := image.NewRGBA(image.Rect(0, 0, width, test.height))
			xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), fish, fish.Bounds(), draw.Over, nil)
			screen := image.NewRGBA(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			draw.Draw(screen, scaled.Bounds().Add(test.position), scaled, image.Point{}, draw.Over)

			point, found, err := detector.Find(screen)
			expected := test.position.Add(image.Pt(width/2, test.height/2))
			if err != nil || !found || math.Abs(float64(point.X-expected.X)) > 15 || math.Abs(float64(point.Y-expected.Y)) > 15 {
				t.Fatalf("fish screen: point=%v found=%t err=%v, want near %v", point, found, err, expected)
			}
		})
	}
}

func TestFishCenterBounds(t *testing.T) {
	size := image.Pt(1024, 577)
	for _, tc := range []struct {
		x, y   float64
		inside bool
	}{
		{1023, 576, true}, {1023.4, 576.4, true}, {1023.9, 576, false}, {1023, 576.9, false}, {-.6, 50, false}, {math.NaN(), 50, false},
	} {
		p, ok := fishCenter(tc.x, tc.y, size)
		if ok != tc.inside || (ok && !p.In(image.Rectangle{Max: size})) {
			t.Fatalf("center(%v,%v)=%v %t", tc.x, tc.y, p, ok)
		}
	}
}
