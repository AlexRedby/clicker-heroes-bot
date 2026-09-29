package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"testing"
)

func TestHeroLevelButton(t *testing.T) {
	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	screen, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	button, found := findHeroLevelButton(screen)
	if !found || absDiff(button.X, 81) > 10 || absDiff(button.Y, 497) > 15 {
		t.Fatalf("hero level button = %v, found = %t", button, found)
	}

	otherTab := image.NewRGBA(screen.Bounds())
	draw.Draw(otherTab, otherTab.Bounds(), image.NewUniform(color.RGBA{R: 102, G: 99, B: 98, A: 255}), image.Point{}, draw.Src)
	draw.Draw(otherTab, image.Rect(55, 195, 135, 245), image.NewUniform(color.RGBA{R: 80, G: 180, B: 250, A: 255}), image.Point{}, draw.Src)
	if point, found := findHeroLevelButton(otherTab); found {
		t.Fatalf("button on a non-hero tab at %v", point)
	}
	inactiveTab := image.NewRGBA(screen.Bounds())
	draw.Draw(inactiveTab, inactiveTab.Bounds(), screen, screen.Bounds().Min, draw.Src)
	tab := image.Rect(screen.Bounds().Dx()*45/1000, screen.Bounds().Dy()*15/100, screen.Bounds().Dx()*70/1000, screen.Bounds().Dy()*21/100)
	draw.Draw(inactiveTab, tab, image.NewUniform(color.Black), image.Point{}, draw.Src)
	if point, found := findHeroLevelButton(inactiveTab); found {
		t.Fatalf("button with an inactive Heroes tab at %v", point)
	}

	after := image.NewRGBA(screen.Bounds())
	draw.Draw(after, after.Bounds(), screen, screen.Bounds().Min, draw.Src)
	if heroListMoved(screen, after) {
		t.Fatal("stationary hero list was treated as scrolled")
	}
	if heroLevelChanged(screen, after, button) {
		t.Fatal("unchanged hero level was accepted")
	}
	draw.Draw(after, image.Rect(320, button.Y-15, 330, button.Y-5), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if heroListMoved(screen, after) {
		t.Fatal("changed hero text was treated as scrolling")
	}
	if !heroLevelChanged(screen, after, button) {
		t.Fatal("changed hero level was missed")
	}
	scrolled := image.NewRGBA(screen.Bounds())
	draw.Draw(scrolled, scrolled.Bounds(), screen, screen.Bounds().Min, draw.Src)
	region := image.Rect(screen.Bounds().Dx()*17/100, screen.Bounds().Dy()*40/100, screen.Bounds().Dx()*28/100, screen.Bounds().Dy()*90/100)
	draw.Draw(scrolled, region, screen, image.Pt(region.Min.X, region.Min.Y+50), draw.Src)
	if !heroListMoved(screen, scrolled) {
		t.Fatal("scrolled hero list was missed")
	}
}
