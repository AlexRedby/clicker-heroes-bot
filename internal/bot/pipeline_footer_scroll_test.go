package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestClippedFooterNeedsEndScroll(t *testing.T) {
	requireAncientOCR(t)
	f := startupFrame(t, "../../testdata/hero-footer-clipped-bottom.png")
	thumb, height, found := heroScrollbarThumb(f.image)
	t.Logf("thumb=%v height=%d gap=%d", thumb, height, f.image.Bounds().Dy()*965/1000-thumb.Y-height/2)
	if !found || !heroScrollbarAtBottom(f.image) {
		t.Fatal("fixture no longer reproduces the near-bottom false positive")
	}
	for name, phase := range map[string]startupPhase{"startup": startupUpgrades, "ordinary": noStartup} {
		t.Run(name, func(t *testing.T) {
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true})
			p.frame, p.layout, p.startup, p.startupCheck = f, f.layout, phase, false
			out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, startup: phase, upgrades: phase == noStartup})
			if out.err != nil || out.found || out.upgradesKnown || out.hero.bottom {
				t.Fatalf("clipped footer accepted as complete: bottom=%t known=%t found=%t err=%v", out.hero.bottom, out.upgradesKnown, out.found, out.err)
			}
			if err := p.accept(context.Background(), out, f.at); err != nil {
				t.Fatal(err)
			}
			p.plan(f.at)
			a, ok := p.nextAction(f.at)
			if !ok || a.kind != scrollHeroes || a.point != thumb || a.target.Y != f.context.bounds.Max.Y-1 {
				t.Fatalf("footer did not navigate to end: %+v %t", a, ok)
			}
		})
	}
}

// The footer path must consume its own settled scroll acknowledgement, rather
// than switch to numeric hero OCR or wait for the next 30-second maintenance tick.
func TestOrdinaryFooterScrollAcknowledgement(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	f := startupFrame(t, "../../testdata/hero-footer-clipped-bottom.png")
	screen := f.image
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return f.context, nil }, heroes: noStartupOCR(t),
	}, pipelineOptions{heroes: true, fishInterval: time.Second})
	p.frame, p.layout, p.startupCheck = f, f.layout, false
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	read := func(now time.Time) observation {
		t.Helper()
		p.nextCapture = time.Time{}
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[heroAnalysis]:
			if !job.upgrades {
				t.Fatal("footer scroll acknowledgement switched to hero OCR")
			}
			out := p.analyze(ctx, heroAnalysis, job)
			if out.err != nil {
				t.Fatal(out.err)
			}
			if err := p.accept(ctx, out, now); err != nil {
				t.Fatal(err)
			}
			return out
		default:
			t.Fatal("footer capture not scheduled")
			return observation{}
		}
	}
	read(f.at)
	p.plan(f.at)
	a, ok := p.nextAction(f.at)
	if !ok || a.kind != scrollHeroes {
		t.Fatal("no initial end scroll", a.kind, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, f.at)
	screen = loadTestImage(t, "../../testdata/hero-startup-zero.png")
	read(f.at.Add(listScrollSettle + time.Millisecond))
	if p.hero.pending != nil {
		t.Fatal("settled footer did not acknowledge navigation")
	}
	p.plan(p.frame.at)
	a, ok = p.nextAction(p.frame.at)
	if !ok || a.kind != buyHeroUpgrades {
		t.Fatal("fresh complete footer did not queue bulk upgrades", a.kind, ok)
	}
}

func TestFooterCompleteBottomTolerance(t *testing.T) {
	requireAncientOCR(t)
	for _, path := range []string{"../../testdata/hero-scrollbar-before.png", "../../testdata/hero-panel-max.png"} {
		f := startupFrame(t, path)
		p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
		out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, upgrades: true})
		if !out.hero.thumbFound || !out.hero.bottom || out.hero.startupScroll != (image.Point{}) {
			t.Fatal("complete bottom requested another scroll", path, out.hero.thumb, out.hero.bottom)
		}
	}
}

func TestStartupFooterDeadlineWaitsForSettledScroll(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	f := startupFrame(t, "../../testdata/hero-footer-clipped-bottom.png")
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, autoClickers: true})
	p.frame, p.layout, p.startup, p.startupCheck = f, f.layout, startupUpgrades, false
	p.startupDeadline = f.at.Add(10 * time.Second)
	p.clickerFooterUntil = p.startupDeadline
	thumb, _, _ := heroScrollbarThumb(f.image)
	a := gameAction{kind: scrollHeroes, frame: f, point: thumb, hero: heroObservation{frame: f, startup: true}}
	p.hero.sent(a, p.startupDeadline.Add(-200*time.Millisecond))
	p.state[autoClickerAnalysis] = observation{frame: f, clickerPool: autoClickerPool{known: true, available: 1, total: 3}}
	p.plan(p.startupDeadline.Add(time.Millisecond))
	if p.startup != startupUpgrades || p.hero.pending == nil || p.clickers.footerAttempted {
		t.Fatal("deadline skipped the settled footer capture or released its last clicker")
	}
	f.id++
	f.at = p.startupDeadline.Add(listScrollSettle)
	f.image = loadTestImage(t, "../../testdata/hero-startup-zero.png")
	point, found, err := readHeroUpgradeButton(ctx, f.image)
	if err != nil || !found {
		t.Fatal("complete native footer", found, err)
	}
	disabled := image.NewRGBA(f.image.Bounds())
	draw.Draw(disabled, disabled.Bounds(), f.image, f.image.Bounds().Min, draw.Src)
	r := heroUpgradeButtonRegion(f.image, point)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, b := rgb(f.image.At(x, y))
			if g > 100 && g > red+50 && g > b+50 {
				disabled.Set(x, y, color.RGBA{R: 60, G: 60, B: 60, A: 255})
			}
		}
	}
	f.image = disabled
	p.frame = f
	out := p.analyze(ctx, heroAnalysis, analysisJob{frame: f, startup: startupUpgrades})
	if out.err != nil || !out.upgradesKnown || out.found {
		t.Fatal("disabled complete footer", out.err, out.upgradesKnown, out.found)
	}
	if err := p.accept(ctx, out, f.at); err != nil {
		t.Fatal(err)
	}
	p.state[autoClickerAnalysis] = observation{frame: f, clickerPool: autoClickerPool{known: true, available: 1, total: 3}}
	p.plan(f.at)
	a, ok := p.nextAction(f.at)
	if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades || p.startup != startupUpgrades {
		t.Fatal("settled footer lost its clicker at the deadline", a.kind, ok, p.startup)
	}
	// Recognition and fresh captures cannot move the absolute grace deadline.
	f.id++
	f.at = p.startupDeadline.Add(2*time.Second + time.Millisecond)
	p.frame = f
	p.state[heroAnalysis].frame = f
	p.plan(f.at)
	if p.startup != startupProgression || p.hero.pending != nil || len(p.queue) != 0 {
		t.Fatal("footer grace was not bounded")
	}
}

func TestOrdinaryFooterNoMotionRetainsReadAndBoundsRetry(t *testing.T) {
	ctx := context.Background()
	f := startupFrame(t, "../../testdata/hero-footer-clipped-bottom.png")
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return f.image, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return f.context, nil }, heroes: noStartupOCR(t),
	}, pipelineOptions{heroes: true, fishInterval: time.Second})
	p.frame, p.layout, p.startupCheck = f, f.layout, false
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	read := func(now time.Time) {
		t.Helper()
		p.nextCapture = time.Time{}
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[heroAnalysis]:
			if !job.upgrades {
				t.Fatal("no-motion footer retry switched to numeric hero OCR")
			}
			if err := p.accept(ctx, p.analyze(ctx, heroAnalysis, job), now); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("footer retry was not scheduled")
		}
	}
	read(f.at)
	deadline := p.nextUpgrades
	for failure := 1; failure <= 3; failure++ {
		now := p.frame.at
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != scrollHeroes {
			t.Fatal("no-motion footer did not retry end navigation", failure, a.kind, ok)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, now)
		for attempt := 1; attempt <= 3; attempt++ {
			read(now.Add(listScrollSettle + time.Second + time.Duration(attempt)*200*time.Millisecond))
		}
		if p.hero.pending != nil || p.hero.scrollFailures != failure || p.nextUpgrades != deadline {
			t.Fatal("no-motion retry lost its bound", failure, p.hero.scrollFailures, p.nextUpgrades)
		}
		if failure < 3 {
			read(p.hero.nextScan)
		}
	}
	if p.hero.nextScan != deadline || len(p.queue) != 0 {
		t.Fatal("no-motion footer did not fall back to the periodic cadence")
	}
}
