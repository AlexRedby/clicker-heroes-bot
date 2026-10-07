package bot

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"os"
	"testing"
)

func TestHeroLevelButton(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/no-fish-game-screen.jpg")
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
	if heroListStable(screen, movedThumb) {
		t.Fatal("moved scrollbar thumb was treated as stable")
	}
	coveredThumb := image.NewRGBA(screen.Bounds())
	draw.Draw(coveredThumb, coveredThumb.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(coveredThumb, image.Rect(483, 500, 505, 534), image.NewUniform(color.RGBA{R: 95, G: 62, B: 12, A: 255}), image.Point{}, draw.Src)
	if !heroListStable(screen, coveredThumb) {
		t.Fatal("shortened thumb with unchanged top was mistaken for movement")
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
	if !heroListStable(screen, after) {
		t.Fatal("stationary hero list was treated as unstable")
	}
	if !heroRowNameMatches(screen, after, button, button) {
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
		{"../../testdata/hero-panel-max.png", 905, 1140, false},
		{"../../testdata/hero-owned-disabled.png", 700, 905, true},
	} {
		screen := loadTestImage(t, test.path)
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

func loadTestImage(t testing.TB, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	screen, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return screen
}

func TestHeroPurchaseGuards(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-panel-max.png")
	button, found := findHeroLevelButton(screen)
	if !found || !heroCandidateKnown(screen, button) || !heroQuantitySelected(screen, 435) || heroQuantitySelected(screen, 122) || !heroLayoutValid(screen) {
		t.Fatal("known MAX layout was rejected or x1 was falsely selected")
	}
	disabled := loadTestImage(t, "../../testdata/hero-owned-disabled.png")
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
	if heroRowNameMatches(screen, changed, button, button) {
		t.Fatal("different row name accepted")
	}
}

func TestHeroScrollbarFishRegression(t *testing.T) {
	before := loadTestImage(t, "../../testdata/hero-scrollbar-before.png")
	after := loadTestImage(t, "../../testdata/fish-over-scrollbar.png")
	for _, screen := range []image.Image{before, after} {
		thumb, height, found := heroScrollbarThumb(screen)
		if !found || absDiff(thumb.X, 1172) > 5 || absDiff(thumb.Y, 1331) > 5 || absDiff(height, 111) > 8 || !heroScrollbarAtBottom(screen) {
			t.Fatalf("fish mistaken for thumb: thumb=%v height=%d found=%t", thumb, height, found)
		}
	}
	if !heroListStable(before, after) {
		t.Fatal("fish appearance was mistaken for hero list movement")
	}
	button := image.Pt(204, 894)
	if !heroRowNameMatches(before, after, button, button) {
		t.Fatal("unchanged Tsuchi row rejected")
	}
}

func TestNonGildedHeroSuccessor(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-nongilded-successor.jpg")
	current, found := findHeroLevelButton(screen)
	if !found || absDiff(current.Y, 446) > 3 || !heroRowHasLevel(screen, current.Y) {
		t.Fatalf("owned Skogur: %v %t", current, found)
	}
	next, found := findNextHeroButton(screen, current)
	if !found || absDiff(next.Y, 563) > 3 || !heroRowUnowned(screen, next.Y) || !heroCandidateKnown(screen, current) {
		t.Fatalf("non-gilded Moeru rejected: %v %t", next, found)
	}
	b := screen.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, screen, b.Min, draw.Src)
	draw.Draw(covered, image.Rect(b.Dx()*26/100, next.Y-b.Dy()*3/100, b.Dx()*36/100, next.Y+b.Dy()*3/100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if heroCandidateKnown(covered, current) {
		t.Fatal("obscured non-gilded successor accepted")
	}
	// The same cream card must work when its HIRE button becomes available.
	available := image.NewRGBA(b)
	draw.Draw(available, b, screen, b.Min, draw.Src)
	buttonRegion := image.Rect(b.Dx()*4/100, current.Y-b.Dy()*45/1000, b.Dx()*145/1000, current.Y+b.Dy()*70/1000)
	draw.Draw(available, buttonRegion.Add(image.Pt(0, next.Y-current.Y)), screen, buttonRegion.Min, draw.Src)
	latest, found := findHeroLevelButton(available)
	if !found || absDiff(latest.Y, next.Y) > 3 || !heroCandidateKnown(available, latest) {
		t.Fatalf("available non-gilded candidate rejected: %v %t", latest, found)
	}
}
