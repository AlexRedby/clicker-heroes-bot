package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"
)

func startupFrame(t *testing.T, path string) gameFrame {
	t.Helper()
	s := loadTestImage(t, path)
	return gameFrame{id: 1, at: time.Now(), image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
}
func noStartupOCR(t *testing.T) heroReaders {
	t.Helper()
	return heroReaders{
		level: func(context.Context, image.Image, image.Point) (int, error) {
			t.Fatal("startup read a numeric level")
			return 0, nil
		},
		gold: func(context.Context, image.Image) (float64, error) {
			t.Fatal("affordable startup read gold")
			return 0, nil
		},
		price: func(context.Context, image.Image, image.Point) (float64, error) {
			t.Fatal("affordable startup read price")
			return 0, nil
		},
	}
}
func TestStartupHireNeedsNoNameOrLevelOCR(t *testing.T) {
	for _, path := range []string{"testdata/hero-startup-zero.png", "testdata/hero-startup-gold.png", "testdata/hero-startup-alexa-unreadable.png", "testdata/hero-startup-name-empty.png"} {
		t.Run(path, func(t *testing.T) {
			f := startupFrame(t, path)
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
			if err != nil || !out.found {
				t.Fatalf("native purchase: %+v %v", out, err)
			}
		})
	}
}
func TestStartupCompletedRowSkippedWithoutOCR(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-fisherman-before.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 966) > 4 || !out.passiveReady {
		t.Fatalf("completed Brittany should be skipped: %+v %v", out, err)
	}
	// Replacing all names and numerical levels must not change startup input.
	covered := image.NewRGBA(f.image.Bounds())
	draw.Draw(covered, covered.Bounds(), f.image, covered.Bounds().Min, draw.Src)
	for _, button := range append(findHeroButtons(f.image, true), findHeroButtons(f.image, false)...) {
		b := covered.Bounds()
		region := image.Rect(b.Dx()*17/100, button.Y-b.Dy()/10, b.Dx()*365/1000, button.Y+b.Dy()/100)
		draw.Draw(covered, region, image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	}
	f.image = covered
	next, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || next.button != out.button || !next.found || next.owned {
		t.Fatalf("name/level pixels affected selection: %+v %v", next, err)
	}
}
func TestStartupMissingInputIsBounded(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-gold.png")
	p := heroRunner{enabled: true}
	p.startStartup()
	p.sweep.top = true
	firstY := 0
	for i := 0; i < 3; i++ {
		f.id++
		f.at = f.at.Add(time.Second)
		out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
		if err != nil {
			t.Fatal(err)
		}
		p.observe(out, observation{}, f.at)
		a, ok := p.action(f.at)
		if !ok || a.kind != buyHero {
			t.Fatalf("purchase%d: %+v %t", i, a, ok)
		}
		if i == 0 {
			firstY = a.point.Y
		}
		if i < 2 && a.point.Y != firstY || i == 2 && a.point.Y <= firstY {
			t.Fatalf("did not advance after two unchanged inputs: %+v", a)
		}
		p.sent(a, f.at)
	}
	p.interrupt()
	if !p.sweep.top || p.sweep.attempts != 1 {
		t.Fatal("F8 lost cursor")
	}
	p.startStartup()
	if p.sweep != (startupSweep{}) {
		t.Fatal("reset retained cursor")
	}
}
func TestStartupMatureViewportSeeksTopWithoutNames(t *testing.T) {
	f := startupFrame(t, "testdata/hero-tsuchi-x1.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{})
	if err != nil || out.found || out.startupScroll == (image.Point{}) {
		t.Fatalf("top seek: %+v %v", out, err)
	}
	f.context.saveMenu = true
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || out.found || out.startupScroll != (image.Point{}) {
		t.Fatal("menu received startup input")
	}
}

func startupTranslatedCard(t *testing.T, path string, sourceY, targetY int, dark bool) image.Image {
	t.Helper()
	s := loadTestImage(t, path)
	b := s.Bounds()
	base := s
	if b.Dx() == 2560 {
		base = loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	}
	out := image.NewRGBA(b)
	draw.Draw(out, b, base, b.Min, draw.Src)
	viewport := heroListViewport(out)
	viewport.Min.X, viewport.Max.X = b.Min.X+b.Dx()*35/1000, b.Min.X+b.Dx()*44/100
	draw.Draw(out, viewport, image.NewUniform(color.RGBA{R: 255, G: 224, B: 95, A: 255}), image.Point{}, draw.Src)
	source := image.Rect(viewport.Min.X, sourceY-b.Dy()*75/1000, viewport.Max.X, sourceY+b.Dy()*85/1000).Intersect(b)
	destination := source.Add(image.Pt(0, targetY-sourceY)).Intersect(viewport)
	draw.Draw(out, destination, s, destination.Min.Sub(image.Pt(0, targetY-sourceY)), draw.Src)
	if dark {
		for y := viewport.Min.Y; y < viewport.Max.Y; y++ {
			for x := b.Min.X + b.Dx()*55/1000; x < b.Min.X+b.Dx()*140/1000; x++ {
				r, g, blue := rgb(out.At(x, y))
				if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
					out.Set(x, y, color.RGBA{R: 45, G: 60, B: 70, A: 255})
				}
			}
		}
	}
	return out
}

func TestStartupShortListWithoutScrollbarCanBuy(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-zero.png")
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, f.image.Bounds().Min, draw.Src)
	b := s.Bounds()
	draw.Draw(s, image.Rect(b.Dx()*445/1000, b.Dy()*38/100, b.Dx()*495/1000, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	f.image = s
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{})
	if err != nil || out.thumbFound || !out.found || !out.sweep.top {
		t.Fatalf("short list: %+v %v", out, err)
	}
	if !startupHeroStable(out, f) {
		t.Fatal("short list purchase requires a nonexistent scrollbar")
	}
}

func TestStartupClippedBottomCaptionScrolls(t *testing.T) {
	for _, inset := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(inset), func(t *testing.T) {
			f := startupFrame(t, "testdata/hero-startup-clipped-caption.png")
			if inset > 0 {
				s := image.NewRGBA(f.image.Bounds())
				draw.Draw(s, s.Bounds(), f.image, s.Bounds().Min, draw.Src)
				b := s.Bounds()
				// The button's blue band can end before the physical viewport edge.
				draw.Draw(s, image.Rect(b.Dx()*55/1000, b.Max.Y-inset, b.Dx()*140/1000, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
				f.image = s
			}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true, y: 1417})
			if err != nil || out.found || !out.thumbFound || out.startupScroll.Y <= out.thumb.Y || out.startupComplete {
				t.Fatalf("clipped caption must scroll: %+v %v", out, err)
			}
		})
	}
}

func TestStartupFullyVisibleObscuredCaptionStillBlocks(t *testing.T) {
	s := startupTranslatedCard(t, "testdata/hero-startup-name-empty.png", 1066, 900, false)
	b := s.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, s, b.Min, draw.Src)
	draw.Draw(covered, heroButtonCaptionRegion(s, image.Pt(204, 900)), image.NewUniform(color.RGBA{70, 170, 235, 255}), image.Point{}, draw.Src)
	f := gameFrame{image: covered, context: gameContext{known: true, heroes: true, bounds: b}}
	_, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err == nil || !strings.Contains(err.Error(), "caption is obscured") {
		t.Fatalf("fully visible obstruction should not be treated as an edge: %v", err)
	}
}

func TestStartupClippedTopCaptionIsNavigationOnly(t *testing.T) {
	s := startupTranslatedCard(t, "testdata/hero-startup-name-empty.png", 1066, 555, false)
	b := s.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, s, b.Min, draw.Src)
	viewport := heroListViewport(s)
	draw.Draw(covered, image.Rect(b.Dx()*55/1000, viewport.Min.Y, b.Dx()*140/1000, viewport.Min.Y+3), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	f := gameFrame{image: covered, context: gameContext{known: true, heroes: true, bounds: b}}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || out.found {
		t.Fatalf("clipped top row must not be purchased or block navigation: %+v %v", out, err)
	}
}
