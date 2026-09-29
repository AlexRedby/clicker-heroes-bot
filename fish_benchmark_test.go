package main

import (
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"testing"
)

func BenchmarkFish(b *testing.B) {
	fish, err := loadFish()
	if err != nil {
		b.Fatal(err)
	}
	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		b.Fatal(err)
	}
	defer file.Close()
	background, err := jpeg.Decode(file)
	if err != nil {
		b.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		point   image.Point
		height  int
		degrees int
	}{
		{name: "no-fish"},
		{name: "upright", point: image.Pt(180, 220), height: 75},
		{name: "tilted", point: image.Pt(850, 40), height: 65, degrees: 17},
		{name: "upside-down", point: image.Pt(700, 200), height: 90, degrees: 180},
	} {
		b.Run(test.name, func(b *testing.B) {
			screen := image.NewRGBA(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			if test.height > 0 {
				template, _, _ := makeFishTemplate(fish, test.height, float64(test.degrees)*math.Pi/180)
				draw.Draw(screen, template.Bounds().Add(test.point), template, image.Point{}, draw.Over)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, found := findFish(screen, fish)
				if found != (test.height > 0) {
					b.Fatal("unexpected detection")
				}
			}
		})
	}
}
