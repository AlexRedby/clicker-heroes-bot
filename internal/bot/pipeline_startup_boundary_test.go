package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestStartupClippedPriceSharedPipeline(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	screen := loadTestImage(t, "../../testdata/hero-startup-clipped-price.png")
	c, err := recognizedGame(screen)
	if err != nil || !c.heroes {
		t.Fatal("native Heroes context", c, err)
	}
	c.window = "game"
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return c, nil }, heroes: noStartupOCR(t),
	}, pipelineOptions{heroes: true, fishInterval: time.Second})
	p.startupCheck, p.startup = false, startupHeroes
	p.hero.sweep = startupSweep{top: true}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	read := func(at time.Time) gameAction {
		t.Helper()
		p.nextCapture = time.Time{}
		if err := p.capture(ctx, at, jobs); err != nil {
			t.Fatal(err)
		}
		out := p.analyze(ctx, heroAnalysis, <-jobs[heroAnalysis])
		if out.err != nil || out.hero.startupComplete || out.hero.startupScroll == (image.Point{}) {
			t.Fatalf("clipped price did not request navigation: %+v %v", out.hero, out.err)
		}
		if err := p.accept(ctx, out, at); err != nil {
			t.Fatal(err)
		}
		p.plan(at)
		a, ok := p.nextAction(at)
		if !ok || a.kind != scrollHeroes || a.point != out.hero.thumb || a.target != out.hero.startupScroll || p.startup != startupHeroes || p.hero.nextScan.After(at) {
			t.Fatalf("price blocked startup navigation: %+v %t", a, ok)
		}
		return a
	}
	a := read(now)
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if acted, err := p.execute(ctx, a); acted || err != nil {
		t.Fatal("F8 allowed stale scroll", acted, err)
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	read(now.Add(time.Second))
}

func TestStartupCoveredFooterContinues(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	screen := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	point, found, err := readHeroUpgradeButton(ctx, screen)
	if err != nil || !found {
		t.Fatal("native footer", found, err)
	}
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(covered, heroUpgradeButtonRegion(screen, point), image.NewUniform(color.RGBA{R: 40, G: 40, B: 40, A: 255}), image.Point{}, draw.Src)
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return covered, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }, progression: func(context.Context, image.Image, [9]skillState, bool) (progressionState, error) {
		return progressionState{Known: true, Enabled: true}, nil
	}}, pipelineOptions{heroes: true, progression: true, export: &saveExportOptions{}, fishInterval: time.Second})
	p.startupCheck, p.startup, p.startupPassive = false, startupUpgrades, true
	p.export.requested = false
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	out := p.analyze(ctx, heroAnalysis, <-jobs[heroAnalysis])
	if out.err != nil || out.found || out.upgradesKnown {
		t.Fatal("covered footer was treated as readable", out.err, out.found, out.upgradesKnown)
	}
	if err := p.accept(ctx, out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	if p.startup != startupUpgrades || p.controls.isPaused() {
		t.Fatal("footer pass was skipped immediately")
	}
	// F8 clears observations, but the resumed optional pass must also be bounded.
	p.controls.toggle()
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	p.plan(now.Add(time.Second))
	p.plan(now.Add(12 * time.Second))
	if p.startup != startupProgression || p.controls.isPaused() || p.export.requested || len(p.queue) != 0 {
		t.Fatal("covered footer blocked progression handoff")
	}
	p.nextCapture = time.Time{}
	if err := p.capture(ctx, now.Add(13*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if err := p.accept(ctx, p.analyze(ctx, progressionAnalysis, <-jobs[progressionAnalysis]), p.frame.at); err != nil {
		t.Fatal(err)
	}
	p.plan(p.frame.at)
	if p.startup != noStartup || !p.export.requested || p.controls.isPaused() {
		t.Fatal("optional footer failure blocked export/ordinary automation")
	}
}

func TestStartupExistingFooterClickerSkipsRead(t *testing.T) {
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.frame = testPipelineFrame()
	p.frame.context.heroes = true
	p.startupCheck, p.startup, p.startupPassive = false, startupUpgrades, true
	p.clickers.upgrades = true
	p.plan(now)
	if p.startup != startupProgression || len(p.queue) != 0 || p.controls.isPaused() {
		t.Fatal("known footer clicker waited for covered text")
	}
	p.plan(now.Add(time.Second))
	if p.startup != noStartup {
		t.Fatal("known footer clicker did not continue normal play")
	}
}

func TestStartupModeConfirmationDoesNotInventZone(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true})
	p.frame = testPipelineFrame()
	p.frame.context.heroes = true
	p.layout = p.frame.layout
	p.startupCheck, p.startup, p.startupPassive = false, startupProgression, true
	if err := p.accept(ctx, observation{kind: progressionAnalysis, frame: p.frame, progression: progressionState{Known: true}}, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.queue[enableProgression]
	if !ok || a.kind != enableProgression || a.progression.Zone != 0 {
		t.Fatalf("startup invented a zone: %+v %t", a, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	if err := p.accept(ctx, observation{kind: progressionAnalysis, frame: p.frame, progression: progressionState{Known: true, Enabled: true}}, p.frame.at); err != nil {
		t.Fatal(err)
	}
	if p.state[progressionAnalysis].progression.Zone != 0 || p.progression.pending != nil || p.progression.seen {
		t.Fatal("mode confirmation became fake combat evidence")
	}
	p.plan(p.frame.at)
	if p.startup != noStartup {
		t.Fatal("mode-only confirmation blocked handoff")
	}
}
