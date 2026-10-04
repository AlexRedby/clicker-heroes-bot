package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/exec"
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

func TestStartupMissedHireFixture(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-missed-hire.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || out.owned {
		t.Fatalf("missed hire fixture: %+v %v", out, err)
	}
}

func TestStartupScrollPendingResetsStaleCursor(t *testing.T) {
	previous := startupFrame(t, "testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "testdata/hero-startup-missed-hire.png")
	p := heroRunner{enabled: true, sweep: startupSweep{top: true, y: 1395}}
	previous.id = 1
	out, err := readStartupHeroObservation(context.Background(), previous, noStartupOCR(t), nil, p.sweep)
	if err != nil {
		t.Fatal(err)
	}
	p.observe(out, observation{}, previous.at)
	a, ok := p.action(previous.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("expected pending startup scroll: %+v %t", a, ok)
	}
	p.sent(a, previous.at)
	current.image = startupMoveThumb(t, current.image)
	current.context.bounds = current.image.Bounds()
	current.id = 2
	current.at = previous.at.Add(time.Second)
	out, err = readStartupHeroObservation(context.Background(), current, noStartupOCR(t), p.before(), p.sweep)
	p.observe(out, observation{}, current.at)
	next, ok := p.action(current.at)
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 843) > 8 || !ok || next.kind != buyHero {
		t.Fatalf("stale pre-scroll cursor skipped visible HIRE: out=%+v action=%+v ok=%t err=%v", out, next, ok, err)
	}
}

func TestStartupMissedHireAfterViewportMoveKeepsHire(t *testing.T) {
	previous := startupFrame(t, "testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "testdata/hero-startup-missed-hire.png")
	current.image = startupMoveThumb(t, current.image)
	current.context.bounds = current.image.Bounds()
	if heroListStable(previous.image, current.image) {
		t.Fatal("fixture pair must represent a moved viewport")
	}
	p := heroRunner{enabled: true}
	p.startStartup()
	p.sweep.top = true
	for i := 1; i <= 2; i++ {
		previous.id = uint64(i)
		previous.at = previous.at.Add(time.Second)
		out, err := readStartupHeroObservation(context.Background(), previous, noStartupOCR(t), p.before(), p.sweep)
		if err != nil {
			t.Fatal(err)
		}
		p.observe(out, observation{}, previous.at)
		a, ok := p.action(previous.at)
		if !ok || a.kind != buyHero {
			t.Fatalf("attempt %d did not produce buy: %+v", i, a)
		}
		p.sent(a, previous.at)
	}
	current.id = 3
	out, err := readStartupHeroObservation(context.Background(), current, noStartupOCR(t), p.before(), p.sweep)
	if err != nil || !out.found || out.owned {
		t.Fatalf("moved viewport must inspect HIRE: %+v %v", out, err)
	}
}

func startupMoveThumb(t *testing.T, screen image.Image) image.Image {
	return startupMoveThumbBy(t, screen, screen.Bounds().Dy()/40)
}

func startupMoveThumbBy(t *testing.T, screen image.Image, delta int) image.Image {
	t.Helper()
	b := screen.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, screen, b.Min, draw.Src)
	thumb, height, ok := heroScrollbarThumb(screen)
	if !ok {
		t.Fatal("fixture has no scrollbar thumb")
	}
	width := b.Dx() * 25 / 1000
	thumbRect := image.Rect(thumb.X-width/2, thumb.Y-height/2, thumb.X+width/2+1, thumb.Y+height/2+1).Intersect(b)
	clear := image.Rect(thumb.X-width, thumb.Y-height/2-3, thumb.X+width+1, thumb.Y+height/2+3).Intersect(b)
	track := clear.Add(image.Pt(width*2, 0)).Intersect(b)
	trackCopy := image.NewRGBA(clear)
	draw.Draw(trackCopy, trackCopy.Bounds(), screen, track.Min, draw.Src)
	thumbCopy := image.NewRGBA(thumbRect)
	draw.Draw(thumbCopy, thumbCopy.Bounds(), screen, thumbRect.Min, draw.Src)
	draw.Draw(out, clear, trackCopy, trackCopy.Bounds().Min, draw.Src)
	shifted := thumbRect.Add(image.Pt(0, delta)).Intersect(b)
	draw.Draw(out, shifted, thumbCopy, thumbCopy.Bounds().Min, draw.Src)
	_, _, found := heroScrollbarThumb(out)
	if !found {
		t.Fatal("moved native scrollbar thumb was not recognized")
	}
	if heroListStable(screen, out) {
		t.Fatal("moved native scrollbar thumb did not change viewport stability")
	}
	return out
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

func TestStartupUnlockedWithoutChecksSkipped(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-bottom-hire.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1337) > 4 {
		t.Fatalf("unlocked Fisherman/Betty must be skipped without checks: %+v %v", out, err)
	}
}

func TestStartupLockedUpgradeRequestsMAXWithoutOCR(t *testing.T) {
	f := startupFrame(t, "testdata/hero-nongilded-successor.jpg")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, 446) > 3 {
		t.Fatalf("locked Skogur must request levels: %+v %v", out, err)
	}
}

func TestStartupClippedHirePriceScrollsWithoutOCR(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-clipped-price.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || out.found || !out.passiveReady || out.bottom || out.startupComplete || out.startupNeedsGold || out.startupScroll.Y <= out.thumb.Y {
		t.Fatalf("clipped dark HIRE price must scroll: %+v %v", out, err)
	}
}

func TestStartupHirePriceAtViewportBoundary(t *testing.T) {
	for _, tc := range []struct {
		name string
		y    int
		buy  bool
	}{
		{"complete top", 640, true},
		{"partial top", 555, false},
		{"complete bottom", 1354, true},
		{"partial bottom", 1370, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startupTranslatedCard(t, "testdata/hero-startup-bottom-hire.png", 1337, tc.y, false)
			f := gameFrame{image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
			if err != nil || out.found != tc.buy || out.owned || out.startupComplete || !tc.buy && out.startupScroll.Y <= out.thumb.Y {
				t.Fatalf("HIRE price boundary: %+v %v", out, err)
			}
		})
	}
}

func TestStartupCompleteUnavailableSuccessorUsesNativePrice(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	f := startupFrame(t, "testdata/hero-skogur-hire.png")
	current, ok := findHeroLevelButton(f.image)
	if !ok {
		t.Fatal("missing owned Tsuchi")
	}
	next, ok := findNextHeroButton(f.image, current)
	if !ok {
		t.Fatal("missing Skogur HIRE")
	}
	read := noStartupOCR(t)
	prices := 0
	read.price = func(ctx context.Context, s image.Image, p image.Point) (float64, error) {
		prices++
		if p != next {
			t.Fatalf("unexpected successor: %v, want %v", p, next)
		}
		return readHeroPrice(ctx, s, p)
	}
	read.gold = readHeroGold
	out, err := readStartupHeroObservation(context.Background(), f, read, nil, startupSweep{top: true, y: next.Y})
	if err != nil || prices != 1 || out.found || !out.passiveReady || !out.startupComplete || out.startupNeedsGold {
		t.Fatalf("complete unavailable successor: %+v price reads=%d %v", out, prices, err)
	}
}

func TestStartupClippedHireCannotCompleteAtScrollbarBottom(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-clipped-price.png")
	thumb, height, ok := heroScrollbarThumb(f.image)
	if !ok {
		t.Fatal("missing fixture thumb")
	}
	b := f.image.Bounds()
	f.image = startupMoveThumbBy(t, f.image, b.Min.Y+b.Dy()*965/1000-thumb.Y-height/2)
	if !heroScrollbarAtBottom(f.image) {
		t.Fatal("translated thumb is not at bottom")
	}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.bottom || !out.passiveReady || out.found || out.startupComplete || out.startupNeedsGold {
		t.Fatalf("scrollbar cannot complete a clipped HIRE: %+v %v", out, err)
	}
}

func TestStartupClippedHireNoMotionAndPause(t *testing.T) {
	f := startupFrame(t, "testdata/hero-startup-clipped-price.png")
	p := heroRunner{enabled: true, sweep: startupSweep{top: true}}
	for i := 0; i < 4; i++ {
		f.id++
		f.at = f.at.Add(time.Second)
		out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
		if err != nil || out.found || out.startupComplete || out.startupNeedsGold {
			t.Fatalf("stationary clipped HIRE: %+v %v", out, err)
		}
		p.observe(out, observation{}, f.at)
		a, ok := p.action(f.at)
		if i == 0 {
			if !ok || a.kind != scrollHeroes {
				t.Fatalf("missing overlap scroll: %+v %t", a, ok)
			}
			p.sent(a, f.at)
		} else if ok {
			t.Fatalf("no-motion scroll authorized another input: %+v", a)
		}
	}
	if p.pending != nil {
		t.Fatal("no-motion scroll did not reach retry wait")
	}
	sweep := p.sweep
	p.interrupt()
	if p.sweep != sweep {
		t.Fatal("pause lost the clipped-row cursor")
	}
	f.id++
	f.at = f.at.Add(time.Second)
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
	p.observe(out, observation{}, f.at)
	a, ok := p.action(f.at)
	if err != nil || !ok || a.kind != scrollHeroes || out.startupComplete {
		t.Fatalf("resume did not retry navigation: %+v %t %v", a, ok, err)
	}
}
