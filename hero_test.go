package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
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
	if heroQuantityBarPresent(screen) {
		t.Fatal("missing quantity bar was accepted")
	}
	button, found := findHeroLevelButton(screen)
	if !found || absDiff(button.X, 81) > 10 || absDiff(button.Y, 497) > 15 {
		t.Fatalf("hero level button = %v, found = %t", button, found)
	}
	thumb, height, found := heroScrollbarThumb(screen)
	if !found || absDiff(thumb.X, 494) > 3 || absDiff(thumb.Y, 491) > 12 || height < 60 {
		t.Fatalf("hero scrollbar thumb = %v, height = %d, found = %t", thumb, height, found)
	}
	movedThumb := image.NewRGBA(screen.Bounds())
	draw.Draw(movedThumb, movedThumb.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(movedThumb, image.Rect(483, 450, 505, 534), image.NewUniform(color.RGBA{R: 95, G: 62, B: 12, A: 255}), image.Point{}, draw.Src)
	draw.Draw(movedThumb, image.Rect(483, 250, 505, 334), screen, image.Pt(483, 450), draw.Src)
	thumb, _, found = heroScrollbarThumb(movedThumb)
	if !found || absDiff(thumb.X, 494) > 3 || absDiff(thumb.Y, 291) > 12 {
		t.Fatalf("moved hero scrollbar thumb = %v, found = %t", thumb, found)
	}
	if !heroListMoved(screen, movedThumb) {
		t.Fatal("moved scrollbar thumb was missed")
	}
	coveredThumb := image.NewRGBA(screen.Bounds())
	draw.Draw(coveredThumb, coveredThumb.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(coveredThumb, image.Rect(483, 500, 505, 534), image.NewUniform(color.RGBA{R: 95, G: 62, B: 12, A: 255}), image.Point{}, draw.Src)
	if heroListMoved(screen, coveredThumb) {
		t.Fatal("partly covered scrollbar thumb was treated as moved")
	}

	otherTab := image.NewRGBA(screen.Bounds())
	draw.Draw(otherTab, otherTab.Bounds(), image.NewUniform(color.RGBA{R: 102, G: 99, B: 98, A: 255}), image.Point{}, draw.Src)
	draw.Draw(otherTab, image.Rect(55, 195, 135, 245), image.NewUniform(color.RGBA{R: 80, G: 180, B: 250, A: 255}), image.Point{}, draw.Src)
	if point, found := findHeroLevelButton(otherTab); found {
		t.Fatalf("button on a non-hero tab at %v", point)
	}
	if point, _, found := heroScrollbarThumb(otherTab); found {
		t.Fatalf("scrollbar thumb on a non-hero tab at %v", point)
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
	if !sameHeroRow(screen, after, button, button) {
		t.Fatal("unchanged hero row was rejected")
	}
	draw.Draw(after, image.Rect(483, 450, 505, 534), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if heroListStable(screen, after) {
		t.Fatal("missing scrollbar was accepted as a stationary list")
	}

}

func TestNextLockedHero(t *testing.T) {
	for _, test := range []struct {
		path                 string
		currentY, nextY      int
		nextAlreadyHasLevels bool
	}{
		{"testdata/hero-panel-max.png", 905, 1140, false},
		{"testdata/hero-owned-disabled.png", 700, 905, true},
	} {
		file, err := os.Open(test.path)
		if err != nil {
			t.Fatal(err)
		}
		screen, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !heroQuantityBarPresent(screen) {
			t.Fatal("visible quantity bar was missed")
		}
		current, found := findHeroLevelButton(screen)
		if !found || absDiff(current.Y, test.currentY) > 80 {
			t.Fatalf("%s: deepest enabled hero = %v, found = %t", test.path, current, found)
		}
		next, found := findNextHeroButton(screen, current)
		if !found || absDiff(next.Y, test.nextY) > 80 || heroRowHasLevel(screen, next.Y) != test.nextAlreadyHasLevels {
			t.Fatalf("%s: next row = %v, found = %t, owned = %t", test.path, next, found, heroRowHasLevel(screen, next.Y))
		}
	}
}

func loadHeroScreen(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	screen, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return screen
}

func TestHeroPurchaseGuards(t *testing.T) {
	screen := loadHeroScreen(t, "testdata/hero-panel-max.png")
	button, found := findHeroLevelButton(screen)
	if !found || !heroCandidateKnown(screen, button) || !heroQuantitySelected(screen, 435) || heroQuantitySelected(screen, 122) || !heroLayoutValid(screen) {
		t.Fatal("known MAX layout was rejected or x1 was falsely selected")
	}
	disabled := loadHeroScreen(t, "testdata/hero-owned-disabled.png")
	earlier, _ := findHeroLevelButton(disabled)
	if heroCandidateKnown(disabled, earlier) {
		t.Fatal("earlier hero accepted with latest owned hero disabled")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	obscured := image.NewRGBA(b)
	draw.Draw(obscured, b, screen, b.Min, draw.Src)
	next, _ := findNextHeroButton(screen, button)
	draw.Draw(obscured, image.Rect(w*26/100, next.Y-h*3/100, w*36/100, next.Y+h*3/100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if heroCandidateKnown(obscured, button) {
		t.Fatal("obscured next row accepted as unowned")
	}
	changed := image.NewRGBA(b)
	draw.Draw(changed, b, screen, b.Min, draw.Src)
	draw.Draw(changed, image.Rect(w*28/100, button.Y-h*5/100, w*365/1000, button.Y-h*3/100), image.NewUniform(color.White), image.Point{}, draw.Src)
	if sameHeroRow(screen, changed, button, button) {
		t.Fatal("different row name accepted")
	}
}
