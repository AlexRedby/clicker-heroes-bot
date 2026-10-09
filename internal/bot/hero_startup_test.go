package bot

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
	for _, path := range []string{"../../testdata/hero-startup-zero.png", "../../testdata/hero-startup-gold.png", "../../testdata/hero-startup-alexa-unreadable.png", "../../testdata/hero-startup-name-empty.png"} {
		t.Run(path, func(t *testing.T) {
			f := startupFrame(t, path)
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
			if err != nil || !out.found {
				t.Fatalf("native purchase: %+v %v", out, err)
			}
		})
	}
}

func TestStartupMissedHireFixture(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || out.owned {
		t.Fatalf("missed hire fixture: %+v %v", out, err)
	}
}

func TestStartupScrollPendingResetsStaleCursor(t *testing.T) {
	previous := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	p := heroRunner{enabled: true, sweep: startupSweep{bottom: true, boosted: true, y: 600}}
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
	if !p.sweep.bottom || !p.sweep.boosted || p.sweep.y != 0 || p.sweep.attempts != 0 {
		t.Fatal("upward scroll reset sweep phase or retained viewport cursor")
	}
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
	previous := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	current.image = startupMoveThumb(t, current.image)
	current.context.bounds = current.image.Bounds()
	if heroListStable(previous.image, current.image) {
		t.Fatal("fixture pair must represent a moved viewport")
	}
	p := heroRunner{enabled: true}
	p.startStartup()
	p.sweep.bottom, p.sweep.boosted = true, true
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
	out := startupShiftThumbBy(t, screen, delta)
	if heroListStable(screen, out) {
		t.Fatal("moved native scrollbar thumb did not change viewport stability")
	}
	return out
}

func startupShiftThumbBy(t *testing.T, screen image.Image, delta int) image.Image {
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
	return out
}
func TestStartupCompletedRowSkippedWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-fisherman-before.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1361) > 4 || !out.passiveReady {
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
	next, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || next.button != out.button || !next.found || next.owned {
		t.Fatalf("name/level pixels affected selection: %+v %v", next, err)
	}
}
func TestStartupMissingInputIsBounded(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-gold.png")
	p := heroRunner{enabled: true}
	p.startStartup()
	p.sweep.bottom, p.sweep.boosted = true, true
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
		if i < 2 && a.point.Y != firstY || i == 2 && a.point.Y >= firstY {
			t.Fatalf("did not advance after two unchanged inputs: %+v", a)
		}
		p.sent(a, f.at)
	}
	sweep := p.sweep
	p.interrupt()
	if p.sweep != sweep || !p.sweep.bottom || !p.sweep.boosted || !p.sweep.maxPending || p.sweep.attempts != 1 {
		t.Fatal("F8 lost cursor")
	}
	p.startStartup()
	if p.sweep != (startupSweep{}) {
		t.Fatal("reset retained cursor")
	}
}
func TestStartupMatureViewportSeeksBottomWithoutNames(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-tsuchi-x1.png")
	f.image = startupMoveThumbBy(t, f.image, -f.image.Bounds().Dy()/40)
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{})
	if err != nil || out.found || out.startupScroll.Y != f.image.Bounds().Max.Y-1 {
		t.Fatalf("bottom seek: %+v %v", out, err)
	}
	f.context.saveMenu = true
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
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
		base = loadTestImage(t, "../../testdata/hero-startup-bottom-hire.png")
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
	f := startupFrame(t, "../../testdata/hero-startup-zero.png")
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, f.image.Bounds().Min, draw.Src)
	b := s.Bounds()
	draw.Draw(s, image.Rect(b.Dx()*445/1000, b.Dy()*38/100, b.Dx()*495/1000, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	f.image = s
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{})
	if err != nil || out.thumbFound || !out.found || !out.sweep.bottom {
		t.Fatalf("short list: %+v %v", out, err)
	}
	if !startupHeroStable(out, f) {
		t.Fatal("short list purchase requires a nonexistent scrollbar")
	}
}

func TestStartupClippedBottomCaptionScrolls(t *testing.T) {
	for _, inset := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(inset), func(t *testing.T) {
			f := startupFrame(t, "../../testdata/hero-startup-clipped-caption.png")
			if inset > 0 {
				s := image.NewRGBA(f.image.Bounds())
				draw.Draw(s, s.Bounds(), f.image, s.Bounds().Min, draw.Src)
				b := s.Bounds()
				// The button's blue band can end before the physical viewport edge.
				draw.Draw(s, image.Rect(b.Dx()*55/1000, b.Max.Y-inset, b.Dx()*140/1000, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
				f.image = s
			}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, y: 1417})
			if err != nil || out.found || !out.thumbFound || out.startupScroll.Y <= out.thumb.Y || out.startupComplete {
				t.Fatalf("clipped caption must scroll: %+v %v", out, err)
			}
		})
	}
}

func TestStartupFullyVisibleObscuredCaptionStillBlocks(t *testing.T) {
	s := startupTranslatedCard(t, "../../testdata/hero-startup-name-empty.png", 1066, 900, false)
	b := s.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, s, b.Min, draw.Src)
	draw.Draw(covered, heroButtonCaptionRegion(s, image.Pt(204, 900)), image.NewUniform(color.RGBA{70, 170, 235, 255}), image.Point{}, draw.Src)
	f := gameFrame{image: covered, context: gameContext{known: true, heroes: true, bounds: b}}
	_, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err == nil || !strings.Contains(err.Error(), "caption is obscured") {
		t.Fatalf("fully visible obstruction should not be treated as an edge: %v", err)
	}
}

func TestStartupClippedTopCaptionIsNavigationOnly(t *testing.T) {
	s := startupTranslatedCard(t, "../../testdata/hero-startup-name-empty.png", 1066, 555, false)
	b := s.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, s, b.Min, draw.Src)
	viewport := heroListViewport(s)
	draw.Draw(covered, image.Rect(b.Dx()*55/1000, viewport.Min.Y, b.Dx()*140/1000, viewport.Min.Y+3), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	f := gameFrame{image: covered, context: gameContext{known: true, heroes: true, bounds: b}}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || out.found || out.startupComplete || out.startupScroll.Y >= out.thumb.Y {
		t.Fatalf("clipped top row must not be purchased or block navigation: %+v %v", out, err)
	}
}

func TestStartupUnlockedWithoutChecksSkipped(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, 655) > 4 {
		t.Fatalf("earlier completed Treebeast/Ivan/Brittany must be skipped: %+v %v", out, err)
	}
}

func TestStartupLockedUpgradeRequestsMAXWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || !out.owned || out.button != image.Pt(204, 655) {
		t.Fatalf("one-slot dark artwork must request levels: %+v %v", out, err)
	}
	f = startupFrame(t, "../../testdata/hero-nongilded-successor.jpg")
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, 446) > 3 {
		t.Fatalf("locked Skogur must request levels: %+v %v", out, err)
	}
}

func TestStartupClippedHirePriceScrollsWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
	if err != nil || out.found || !out.passiveReady || out.bottom || out.startupComplete || out.startupScroll.Y <= out.thumb.Y {
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
		{"complete short HIRE price", 1370, true},
		{"partial bottom", 1400, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startupTranslatedCard(t, "../../testdata/hero-startup-bottom-hire.png", 1337, tc.y, false)
			if tc.buy && tc.y >= 1354 {
				// Isolate the complete HIRE price from the extra translated lower fragment.
				isolated := image.NewRGBA(s.Bounds())
				draw.Draw(isolated, isolated.Bounds(), s, s.Bounds().Min, draw.Src)
				b := s.Bounds()
				draw.Draw(isolated, image.Rect(b.Dx()*35/1000, tc.y+54, b.Dx()*44/100, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
				s = isolated
			}
			f := gameFrame{image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
			if err != nil || out.found != tc.buy || out.owned || out.startupComplete || !tc.buy && (out.startupScroll == (image.Point{}) || tc.y < 600 && out.startupScroll.Y >= out.thumb.Y || tc.y > 1380 && out.startupScroll.Y <= out.thumb.Y) {
				t.Fatalf("HIRE price boundary: %+v %v", out, err)
			}
		})
	}
}

func TestStartupCompleteUnavailableSuccessorNeedsNoNumericalOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-skogur-hire.png")
	current, ok := findHeroLevelButton(f.image)
	if !ok {
		t.Fatal("missing owned Tsuchi")
	}
	_, ok = findNextHeroButton(f.image, current)
	if !ok {
		t.Fatal("missing Skogur HIRE")
	}
	f.image = startupThumbAtTop(t, f.image)
	complete := image.NewRGBA(f.image.Bounds())
	draw.Draw(complete, complete.Bounds(), f.image, complete.Bounds().Min, draw.Src)
	viewport := heroListViewport(f.image)
	for _, band := range findHeroButtonBands(f.image, true) {
		if band.Min.Y <= viewport.Min.Y+max(3, f.image.Bounds().Dy()/150) {
			draw.Draw(complete, band, image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
		}
	}
	f.image = complete
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, y: current.Y, attempts: 2})
	if err != nil || out.found || !out.passiveReady || !out.startupComplete {
		t.Fatalf("complete unavailable successor: %+v %v", out, err)
	}
}

func TestStartupClippedHireCannotCompleteAtScrollbarBottom(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	thumb, height, ok := heroScrollbarThumb(f.image)
	if !ok {
		t.Fatal("missing fixture thumb")
	}
	b := f.image.Bounds()
	f.image = startupMoveThumbBy(t, f.image, b.Min.Y+b.Dy()*965/1000-thumb.Y-height/2)
	if !heroScrollbarAtBottom(f.image) {
		t.Fatal("translated thumb is not at bottom")
	}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
	if err != nil || !out.bottom || !out.passiveReady || out.found || out.startupComplete {
		t.Fatalf("scrollbar cannot complete a clipped HIRE: %+v %v", out, err)
	}
}

func TestStartupClippedHireNoMotionAndPause(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	p := heroRunner{enabled: true, sweep: startupSweep{bottom: true}}
	for i := 0; i < 4; i++ {
		f.id++
		f.at = f.at.Add(time.Second)
		out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
		if err != nil || out.found || out.startupComplete {
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

func TestStartupClippedHireBandScrollsWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price-band.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
	if err != nil || out.found || out.startupComplete || out.startupScroll.Y <= out.thumb.Y {
		t.Fatalf("clipped HIRE band must scroll before OCR: %+v %v", out, err)
	}
}

func TestStartupRevisitsUnaffordableLockedRowOnce(t *testing.T) {
	// Compose unavailable locked Cid above affordable HIRE rows. The upward
	// cursor reaches Cid after the lower rows have already been handled.
	low := startupFrame(t, "../../testdata/hero-startup-gold.png")
	high := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	s := image.NewRGBA(low.image.Bounds())
	draw.Draw(s, s.Bounds(), low.image, s.Bounds().Min, draw.Src)
	card := image.Rect(80, 558, 1100, 760)
	draw.Draw(s, card, high.image, card.Min, draw.Src)
	for y := 560; y < 758; y++ {
		for x := 140; x < 358; x++ {
			r, g, b := rgb(s.At(x, y))
			if b > 150 && b > r+40 && b >= g-10 && g > 90 {
				s.Set(x, y, color.RGBA{45, 60, 70, 255})
			}
		}
	}
	low.image = s
	first, err := readStartupHeroObservation(context.Background(), low, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, y: 655})
	if err != nil || first.found || !first.sweep.needsLevels || first.sweep.retry != 1 {
		t.Fatalf("unaffordable earlier Cid must request one revisit: %+v %v", first, err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.frame, p.startup, p.startupCheck = low, startupHeroes, false
	deadline := low.at.Add(time.Minute)
	p.startupDeadline = deadline
	// Completion after the cheap passive hero and the remaining first sweep.
	complete := heroObservation{frame: low, startup: true, startupComplete: true, passiveReady: true, sweep: first.sweep}
	out := observation{kind: heroAnalysis, startup: startupHeroes, frame: low, hero: complete}
	if err := p.accept(context.Background(), out, low.at); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupHeroes || p.hero.sweep.retry != 2 || p.hero.sweep.bottom || p.startupDeadline != deadline {
		t.Fatal("first sweep handed off or reset its budget before revisiting")
	}
	// A retired first-pass completion cannot skip the fresh second pass.
	if err := p.accept(context.Background(), out, low.at); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupHeroes {
		t.Fatal("stale completion skipped revisit")
	}
	p.controls.toggle()
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.hero.sweep.retry != 2 {
		t.Fatal("F8 lost bounded revisit ownership")
	}
	high.id, high.generation, high.layout = low.id+1, p.generation, p.layout
	high.at = low.at.Add(time.Second)
	p.frame = high
	// The bounded revisit has reached this earlier viewport.
	p.hero.sweep.bottom, p.hero.sweep.boosted = true, true
	second, err := readStartupHeroObservation(context.Background(), high, noStartupOCR(t), nil, p.hero.sweep)
	if err != nil || !second.found || !second.owned || second.button != image.Pt(204, 655) || second.sweep.retry != 2 {
		t.Fatalf("rising gold must make earlier Cid selectable in the final pass: %+v %v", second, err)
	}
	// Even persistent unaffordability cannot schedule a third pass.
	stillLow, err := readStartupHeroObservation(context.Background(), low, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, retry: 2})
	if err != nil || stillLow.sweep.retry != 2 {
		t.Fatal("final pass rearmed a retry", err)
	}
	high.id++
	p.frame = high
	complete.frame, complete.sweep = high, stillLow.sweep
	out.frame, out.hero = high, complete
	if err := p.accept(context.Background(), out, high.at); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupUpgrades {
		t.Fatal("persistent low funds created an unbounded sweep")
	}
	p.beginStartup()
	if p.hero.sweep.retry != 0 {
		t.Fatal("Ascension retained the previous revisit")
	}
}

func TestStartupSparseHeroesNeedsNoNumbers(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-post-transcension-start.png")
	for _, y := range []int{0, 655, 865, 1076} {
		out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, y: y, attempts: 2})
		if err != nil || out.passiveReady {
			t.Fatalf("sparse row %d: %+v %v", y, out, err)
		}
		if y == 1076 && (!out.found || absDiff(out.button.Y, 865) > 4 || out.owned) {
			t.Fatalf("available Treebeast not hired: %+v", out)
		}
	}
}

func TestStartupLowerHirePrecedesUpperHire(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1260) > 4 {
		t.Fatalf("lower Brittany must be hired before upper Ivan: %+v %v", out, err)
	}
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, y: out.button.Y, attempts: 2})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1076) > 4 || !out.sweep.needsLevels {
		t.Fatalf("two missed Brittany hires must advance upward to Ivan and defer work: %+v %v", out, err)
	}
}

func TestStartupHireImmediatelyMAXBeforeUpperHire(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	before, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true})
	if err != nil || !before.found || before.owned || absDiff(before.button.Y, 1260) > 4 {
		t.Fatalf("before lower hire: %+v %v", before, err)
	}
	// Native LVL UP artwork proves HIRE registered even with unreadable levels.
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, s.Bounds().Min, draw.Src)
	destination := heroButtonCaptionRegion(s, before.button)
	source := heroButtonCaptionRegion(f.image, image.Pt(before.button.X, 865))
	draw.Draw(s, destination, f.image, source.Min, draw.Src)
	f.image, f.id, f.at = s, f.id+1, f.at.Add(time.Second)
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), &before, startupSweep{bottom: true, boosted: true, maxPending: true, y: before.button.Y, attempts: 2})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, before.button.Y) > f.image.Bounds().Dy()/20 || out.sweep.attempts != 0 || out.sweep.maxPending {
		t.Fatalf("newly owned row must request immediate MAX with a fresh budget: %+v %v", out, err)
	}
	p := heroRunner{enabled: true}
	p.observe(out, observation{}, f.at)
	a, ok := p.action(f.at)
	if !ok || a.kind != buyHero || !a.hero.owned || a.point != out.button {
		t.Fatalf("upper Ivan HIRE preempted newly owned Brittany MAX: %+v %t", a, ok)
	}
	p.sent(a, f.at)
	if p.sweep.maxPending || !p.sweep.boosted {
		t.Fatal("sent MAX did not clear the pending hire or establish latest-owned boost")
	}
	// Completed native skill artwork allows the same sweep to continue upward.
	unlocked := loadTestImage(t, "../../testdata/hero-startup-cid-locked.png")
	card := image.Rect(89, 1180, 1126, 1390)
	leveled := image.NewRGBA(s.Bounds())
	draw.Draw(leveled, leveled.Bounds(), s, s.Bounds().Min, draw.Src)
	draw.Draw(leveled, card, unlocked, card.Min, draw.Src)
	f.image = leveled
	f.id++
	f.at = f.at.Add(time.Second)
	next, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), &out, p.sweep)
	if err != nil || !next.found || next.owned || absDiff(next.button.Y, 1076) > 4 {
		t.Fatalf("after Brittany MAX the same sweep must continue to upper Ivan HIRE: %+v %v", next, err)
	}
}

func TestStartupShortListMissingHireStaysBounded(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-gold.png")
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, s.Bounds().Min, draw.Src)
	b := s.Bounds()
	draw.Draw(s, image.Rect(b.Dx()*45/100, b.Dy()*4/10, b.Dx()*49/100, b.Max.Y), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
	f.image = s
	if _, _, found := heroScrollbarThumb(s); found {
		t.Fatal("short list fixture still has a thumb")
	}
	p := heroRunner{enabled: true, sweep: startupSweep{bottom: true, boosted: true}}
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
			t.Fatalf("attempt %d: %+v %t", i, a, ok)
		}
		if i == 0 {
			firstY = a.point.Y
		}
		if i < 2 && a.point.Y != firstY || i == 2 && (a.point.Y >= firstY || !a.hero.sweep.needsLevels) {
			t.Fatalf("unchanged short-list HIRE did not stop at two inputs: %+v", a)
		}
		p.sent(a, f.at)
	}
}

func TestStartupExhaustedLockedRowRequestsLaterRetry(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, y: 655, attempts: 2, retry: 2})
	if err != nil || !out.sweep.needsLevels || out.sweep.retry != 2 || out.found && absDiff(out.button.Y, 655) <= 4 {
		t.Fatalf("still-locked row must yield after two inputs and remain unfinished: %+v %v", out, err)
	}
}

func TestStartupInitialSeekBottomBeforeBuying(t *testing.T) {
	top := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	p := heroRunner{enabled: true}
	first, err := readStartupHeroObservation(context.Background(), top, noStartupOCR(t), nil, p.sweep)
	if err != nil || first.found || first.sweep.bottom || first.startupComplete || first.startupScroll.Y != top.image.Bounds().Max.Y-1 {
		t.Fatalf("initial viewport must seek bottom before purchases: %+v %v", first, err)
	}
	p.observe(first, observation{}, top.at)
	a, ok := p.action(top.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("initial seek must produce a scroll: %+v %t", a, ok)
	}
	p.sent(a, top.at)
	lower := startupFrame(t, "../../testdata/hero-startup-bottom-hire.png")
	thumb, height, found := heroScrollbarThumb(lower.image)
	if !found {
		t.Fatal("missing lower fixture scrollbar")
	}
	b := lower.image.Bounds()
	lower.image = startupShiftThumbBy(t, lower.image, b.Min.Y+b.Dy()*965/1000-thumb.Y-height/2)
	lower.id, lower.at = top.id+1, top.at.Add(time.Second)
	hire, err := readStartupHeroObservation(context.Background(), lower, noStartupOCR(t), p.before(), p.sweep)
	if err != nil || !hire.found || hire.owned || !hire.x1 || absDiff(hire.button.Y, 1337) > 4 || !hire.sweep.bottom {
		t.Fatalf("bottom anchor must authorize lower HIRE before old Cid: %+v %v", hire, err)
	}
	p.observe(hire, observation{}, lower.at)
	a, ok = p.action(lower.at)
	if !ok || a.kind != buyHero || a.hero.owned {
		t.Fatalf("HIRE must produce x1 purchase: %+v %t", a, ok)
	}
}

func TestStartupLatestOwnedMAXEvenUnlocked(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, 1260) > 4 {
		t.Fatalf("latest owned Brittany must receive MAX before earlier locked Cid: %+v %v", out, err)
	}
}

func startupThumbAtTop(t *testing.T, screen image.Image) image.Image {
	t.Helper()
	thumb, height, ok := heroScrollbarThumb(screen)
	if !ok {
		t.Fatal("fixture has no scrollbar thumb")
	}
	b := screen.Bounds()
	return startupShiftThumbBy(t, screen, b.Min.Y+b.Dy()*420/1000+height/2-thumb.Y)
}

func TestStartupUnavailableLatestMAXRequestsLaterRetry(t *testing.T) {
	s := startupTranslatedCard(t, "../../testdata/hero-startup-cid-locked.png", 655, 900, true)
	f := gameFrame{image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true})
	if err != nil || out.found || !out.sweep.boosted || !out.sweep.needsLevels || out.sweep.retry != 1 {
		t.Fatalf("unavailable latest locked MAX must defer work before moving upward: %+v %v", out, err)
	}
}

func TestStartupExhaustedHireRetiresPendingMAX(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint("disabled=", disabled), func(t *testing.T) {
			f := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
			attempts := 2
			if disabled {
				attempts = 1
				dark := image.NewRGBA(f.image.Bounds())
				draw.Draw(dark, dark.Bounds(), f.image, dark.Bounds().Min, draw.Src)
				for _, band := range findHeroButtonBands(f.image, true) {
					if absDiff((band.Min.Y+band.Max.Y-1)/2, 843) > 8 {
						continue
					}
					for y := band.Min.Y; y < band.Max.Y; y++ {
						for x := band.Min.X; x < band.Max.X; x++ {
							r, g, b := rgb(dark.At(x, y))
							if b > 150 && b > r+40 && b >= g-10 && g > 90 {
								dark.Set(x, y, color.RGBA{45, 60, 70, 255})
							}
						}
					}
				}
				f.image = dark
			}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{bottom: true, boosted: true, maxPending: true, y: 843, attempts: attempts})
			if err != nil || out.found || out.sweep.maxPending || !out.sweep.needsLevels || out.startupScroll.Y >= out.thumb.Y {
				t.Fatalf("skipped HIRE must retire pending MAX before scrolling to earlier completed rows: %+v %v", out, err)
			}
			p := heroRunner{enabled: true}
			p.observe(out, observation{}, f.at)
			a, ok := p.action(f.at)
			if !ok || a.kind != scrollHeroes {
				t.Fatalf("missing upward scroll: %+v %t", a, ok)
			}
			p.sent(a, f.at)
			if p.sweep.y != 0 || p.sweep.attempts != 0 || !p.sweep.boosted || p.sweep.maxPending {
				t.Fatal("scroll retained failed HIRE priority")
			}
			earlier := startupTranslatedCard(t, "../../testdata/hero-startup-cid-locked.png", 1260, 900, false)
			current := gameFrame{id: f.id + 1, at: f.at.Add(time.Second), image: earlier, context: gameContext{known: true, heroes: true, bounds: earlier.Bounds()}}
			next, err := readStartupHeroObservation(context.Background(), current, noStartupOCR(t), p.before(), p.sweep)
			if err != nil || next.found || next.sweep.maxPending || !next.sweep.boosted {
				t.Fatalf("upward scroll must skip completed earlier Brittany after retired HIRE: %+v %v", next, err)
			}
		})
	}
}

func TestStartupUpwardScrollAtBottomNeedsMotion(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-bottom-hire.png")
	thumb, height, ok := heroScrollbarThumb(f.image)
	if !ok {
		t.Fatal("fixture has no scrollbar thumb")
	}
	b := f.image.Bounds()
	f.image = startupShiftThumbBy(t, f.image, b.Min.Y+b.Dy()*965/1000-thumb.Y-height/2)
	p := heroRunner{enabled: true, sweep: startupSweep{bottom: true, boosted: true, y: 600}}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, p.sweep)
	if err != nil || !out.bottom || out.found || out.startupScroll.Y >= out.thumb.Y {
		t.Fatalf("fixture must request upward navigation from bottom: %+v %v", out, err)
	}
	p.observe(out, observation{}, f.at)
	a, ok := p.action(f.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("missing upward scroll: %+v %t", a, ok)
	}
	p.sent(a, f.at)
	f.id++
	f.at = f.at.Add(time.Second)
	unchanged, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
	if err != nil {
		t.Fatal(err)
	}
	p.observe(unchanged, observation{}, f.at)
	if p.pending == nil {
		t.Fatal("bottom thumb was mistaken for a successful upward drag")
	}
	if a, ok := p.action(f.at); ok {
		t.Fatalf("unchanged upward drag authorized another input: %+v", a)
	}
}

func TestStartupUnconfirmedScrollKeepsFreshCursor(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	p := heroRunner{enabled: true, sweep: startupSweep{bottom: true, boosted: true, y: 600, needsLevels: true, retry: 1}}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, p.sweep)
	if err != nil || out.found || out.startupScroll.Y >= out.thumb.Y {
		t.Fatalf("fixture must request upward navigation: %+v %v", out, err)
	}
	p.observe(out, observation{}, f.at)
	a, ok := p.action(f.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("missing upward scroll: %+v %t", a, ok)
	}
	p.sent(a, f.at)
	fresh := p.sweep
	f.id++
	f.at = f.at.Add(time.Second)
	unchanged, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), p.before(), p.sweep)
	if err != nil || !unchanged.found || unchanged.sweep.y == 0 || absDiff(unchanged.button.Y, 843) > 8 {
		t.Fatalf("stationary viewport must compute an obsolete lower-row cursor: %+v %v", unchanged, err)
	}
	p.observe(unchanged, observation{}, f.at)
	if p.pending == nil || p.sweep != fresh || p.sweep.y != 0 {
		t.Fatal("unconfirmed scroll overwrote its fresh cursor", p.sweep)
	}
	moved := startupFrame(t, "../../testdata/hero-startup-fisherman-before.png")
	moved.id, moved.at = f.id+1, f.at.Add(time.Second)
	next, err := readStartupHeroObservation(context.Background(), moved, noStartupOCR(t), p.before(), p.sweep)
	if err != nil || !next.found || next.owned || next.button.Y <= unchanged.button.Y {
		t.Fatalf("confirmed earlier viewport must select its lower available row: %+v %v", next, err)
	}
	p.observe(next, observation{}, moved.at)
	a, ok = p.action(moved.at)
	if p.pending != nil || !ok || a.kind != buyHero || a.point != next.button || p.sweep != next.sweep {
		t.Fatalf("confirmed scroll did not adopt the current purchase cursor: %+v %t", a, ok)
	}
}
