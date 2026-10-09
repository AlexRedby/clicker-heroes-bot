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
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
			if err != nil || !out.found {
				t.Fatalf("native purchase: %+v %v", out, err)
			}
		})
	}
}

func TestStartupMissedHireFixture(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || out.owned {
		t.Fatalf("missed hire fixture: %+v %v", out, err)
	}
}

func TestStartupScrollPendingResetsStaleCursor(t *testing.T) {
	previous := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
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
	previous := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	current := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
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
	f := startupFrame(t, "../../testdata/hero-startup-gold.png")
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
	f := startupFrame(t, "../../testdata/hero-tsuchi-x1.png")
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
			f := startupFrame(t, "../../testdata/hero-startup-clipped-caption.png")
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
	s := startupTranslatedCard(t, "../../testdata/hero-startup-name-empty.png", 1066, 900, false)
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
	s := startupTranslatedCard(t, "../../testdata/hero-startup-name-empty.png", 1066, 555, false)
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
	f := startupFrame(t, "../../testdata/hero-startup-bottom-hire.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1337) > 4 {
		t.Fatalf("unlocked Fisherman/Betty must be skipped without checks: %+v %v", out, err)
	}
}

func TestStartupLockedUpgradeRequestsMAXWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{hiresDone: true})
	if err != nil || !out.found || !out.owned || out.button != image.Pt(204, 655) {
		t.Fatalf("one-slot dark artwork must request levels: %+v %v", out, err)
	}
	f = startupFrame(t, "../../testdata/hero-nongilded-successor.jpg")
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{hiresDone: true, top: true})
	if err != nil || !out.found || !out.owned || absDiff(out.button.Y, 446) > 3 {
		t.Fatalf("locked Skogur must request levels: %+v %v", out, err)
	}
}

func TestStartupClippedHirePriceScrollsWithoutOCR(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
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
			f := gameFrame{image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
			out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
			if err != nil || out.found != tc.buy || out.owned || out.startupComplete || !tc.buy && out.startupScroll.Y <= out.thumb.Y {
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
	next, ok := findNextHeroButton(f.image, current)
	if !ok {
		t.Fatal("missing Skogur HIRE")
	}
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{hiresDone: true, top: true, y: next.Y})
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
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || !out.bottom || !out.passiveReady || out.found || out.startupComplete {
		t.Fatalf("scrollbar cannot complete a clipped HIRE: %+v %v", out, err)
	}
}

func TestStartupClippedHireNoMotionAndPause(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	p := heroRunner{enabled: true, sweep: startupSweep{top: true}}
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
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true})
	if err != nil || out.found || out.startupComplete || out.startupScroll.Y <= out.thumb.Y {
		t.Fatalf("clipped HIRE band must scroll before OCR: %+v %v", out, err)
	}
}

func TestStartupRevisitsUnaffordableLockedRowOnce(t *testing.T) {
	// Compose a real owned dark Cid card with the native affordable Treebeast
	// HIRE. Dimming only Cid's button models the initial lack of funds.
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
	first, err := readStartupHeroObservation(context.Background(), low, noStartupOCR(t), nil, startupSweep{hiresDone: true})
	if err != nil || !first.found || first.owned || first.button != image.Pt(204, 865) || first.sweep.retry != 1 {
		t.Fatalf("unaffordable Cid must yield to cheap DPS and request one revisit: %+v %v", first, err)
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
	if p.startup != startupHeroes || p.hero.sweep.retry != 2 || p.hero.sweep.top || p.startupDeadline != deadline {
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
	// The final locked-skill pass follows its own hire traversal.
	p.hero.sweep.hiresDone = true
	second, err := readStartupHeroObservation(context.Background(), high, noStartupOCR(t), nil, p.hero.sweep)
	if err != nil || !second.found || !second.owned || second.button != image.Pt(204, 655) || second.sweep.retry != 2 {
		t.Fatalf("rising gold must make earlier Cid selectable in the final pass: %+v %v", second, err)
	}
	// Even persistent unaffordability cannot schedule a third pass.
	stillLow, err := readStartupHeroObservation(context.Background(), low, noStartupOCR(t), nil, startupSweep{hiresDone: true, retry: 2})
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
		out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true, y: y, attempts: 2})
		if err != nil || out.passiveReady {
			t.Fatalf("sparse row %d: %+v %v", y, out, err)
		}
		if y == 655 && (!out.found || absDiff(out.button.Y, 865) > 4 || out.owned) {
			t.Fatalf("available Treebeast not hired: %+v", out)
		}
	}
}

func TestStartupAffordableHirePrecedesOwnedMAX(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1076) > 4 {
		t.Fatalf("Ivan must be hired before Cid/Treebeast MAX: %+v %v", out, err)
	}
	out, err = readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{top: true, y: out.button.Y, attempts: 2})
	if err != nil || !out.found || out.owned || absDiff(out.button.Y, 1260) > 4 || !out.sweep.needsLevels {
		t.Fatalf("two missed Ivan hires must advance to Brittany and request a later retry: %+v %v", out, err)
	}
}

func TestStartupHireDuringLevelPassRestartsGlobalPriority(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	before, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{hiresDone: true})
	if err != nil || !before.found || before.owned {
		t.Fatalf("before hire: %+v %v", before, err)
	}
	// Native LEVEL UP artwork proves the prior HIRE registered; names, levels,
	// and gold do not participate in the purchase decision.
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, s.Bounds().Min, draw.Src)
	for _, y := range []int{before.button.Y, 1260} {
		destination := heroButtonCaptionRegion(s, image.Pt(before.button.X, y))
		source := heroButtonCaptionRegion(f.image, image.Pt(before.button.X, 865))
		draw.Draw(s, destination, f.image, source.Min, draw.Src)
	}
	f.image = s
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), &before, startupSweep{top: true, y: before.button.Y, attempts: 1})
	if err != nil || out.found || out.sweep.hiresDone || out.startupScroll.Y <= out.thumb.Y || out.startupComplete {
		t.Fatalf("successful hire must finish global hiring before earlier locked Treebeast: %+v %v", out, err)
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
	p := heroRunner{enabled: true, sweep: startupSweep{top: true}}
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
		if i < 2 && a.point.Y != firstY || i == 2 && (a.point.Y <= firstY || !a.hero.sweep.needsLevels) {
			t.Fatalf("unchanged short-list HIRE did not stop at two inputs: %+v", a)
		}
		p.sent(a, f.at)
	}
}

func TestStartupExhaustedLockedRowRequestsLaterRetry(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	out, err := readStartupHeroObservation(context.Background(), f, noStartupOCR(t), nil, startupSweep{hiresDone: true, top: true, y: 655, attempts: 2, retry: 2})
	if err != nil || !out.sweep.needsLevels || out.sweep.retry != 2 || out.found && absDiff(out.button.Y, 655) <= 4 {
		t.Fatalf("still-locked row must yield after two inputs and remain unfinished: %+v %v", out, err)
	}
}

func TestStartupHiresBelowFirstViewportBeforeOwnedMAX(t *testing.T) {
	top := startupFrame(t, "../../testdata/hero-startup-cid-locked.png")
	p := heroRunner{enabled: true}
	first, err := readStartupHeroObservation(context.Background(), top, noStartupOCR(t), nil, p.sweep)
	if err != nil || first.found || first.sweep.hiresDone || first.startupComplete || first.startupScroll.Y <= first.thumb.Y {
		t.Fatalf("owned top rows must yield to offscreen HIRE rows: %+v %v", first, err)
	}
	p.observe(first, observation{}, top.at)
	a, ok := p.action(top.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("top viewport must scroll before MAX: %+v %t", a, ok)
	}
	p.sent(a, top.at)
	lower := startupFrame(t, "../../testdata/hero-startup-bottom-hire.png")
	lower.id, lower.at = top.id+1, top.at.Add(time.Second)
	hire, err := readStartupHeroObservation(context.Background(), lower, noStartupOCR(t), p.before(), p.sweep)
	if err != nil || !hire.found || hire.owned || !hire.x1 || absDiff(hire.button.Y, 1337) > 4 || hire.sweep.hiresDone {
		t.Fatalf("lower affordable HIRE must precede old locked Cid: %+v %v", hire, err)
	}
	p.observe(hire, observation{}, lower.at)
	a, ok = p.action(lower.at)
	if !ok || a.kind != buyHero || a.hero.owned {
		t.Fatalf("lower HIRE must produce an unmodified purchase: %+v %t", a, ok)
	}
	// A fully visible disabled successor at the bottom finishes only hiring.
	end := startupFrame(t, "../../testdata/hero-skogur-hire.png")
	thumb, height, found := heroScrollbarThumb(end.image)
	if !found {
		t.Fatal("missing end fixture scrollbar")
	}
	b := end.image.Bounds()
	end.image = startupShiftThumbBy(t, end.image, b.Min.Y+b.Dy()*965/1000-thumb.Y-height/2)
	boundary, err := readStartupHeroObservation(context.Background(), end, noStartupOCR(t), nil, startupSweep{top: true, y: 1234, attempts: 2, retry: 1, needsLevels: true})
	if err != nil || !boundary.sweep.hiresDone || boundary.sweep.top || boundary.sweep.y != 0 || boundary.sweep.attempts != 0 || boundary.startupComplete || boundary.found || boundary.startupScroll.Y >= boundary.thumb.Y || boundary.sweep.retry != 1 || !boundary.sweep.needsLevels {
		t.Fatalf("hire completion must reset navigation and preserve deferred work: %+v %v", boundary, err)
	}
	levels, err := readStartupHeroObservation(context.Background(), top, noStartupOCR(t), nil, boundary.sweep)
	if err != nil || !levels.found || !levels.owned || levels.button != image.Pt(204, 655) || !levels.sweep.hiresDone {
		t.Fatalf("old locked Cid becomes eligible only after the global hire pass: %+v %v", levels, err)
	}
}
