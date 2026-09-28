package main

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestFindFish(t *testing.T) {
	fish, err := loadFish()
	if err != nil {
		t.Fatal(err)
	}
	screen := image.NewRGBA(image.Rect(0, 0, 400, 300))
	draw.Draw(screen, screen.Bounds(), &image.Uniform{C: color.RGBA{30, 80, 120, 255}}, image.Point{}, draw.Src)
	if _, found := findFish(screen, fish); found {
		t.Fatal("found fish on an empty screen")
	}

	const x, y, height = 180, 40, 65
	fb := fish.Bounds()
	width := height * fb.Dx() / fb.Dy()
	for py := 0; py < height; py++ {
		for px := 0; px < width; px++ {
			c := fish.At(fb.Min.X+px*fb.Dx()/width, fb.Min.Y+py*fb.Dy()/height)
			_, _, _, alpha := c.RGBA()
			if alpha >= 0x8000 {
				screen.Set(x+px, y+py, c)
			}
		}
	}
	point, found := findFish(screen, fish)
	if !found || point.X < x+width/4 || point.X > x+width || point.Y < y+height/4 || point.Y > y+height*3/4 {
		t.Fatalf("fish location = %v, found = %v", point, found)
	}
}
