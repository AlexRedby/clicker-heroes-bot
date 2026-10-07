package main

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestStartupPipelineHandoff(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	t.Chdir(t.TempDir())
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	c.window = "game"
	frame := gameFrame{id: 4, layout: 1, at: now, image: screen, context: c}
	clicks := 0
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture:   func() (image.Image, error) { return screen, nil },
		click:     func(image.Point) error { clicks++; return nil },
		move:      func(image.Point) error { return nil },
		keyToggle: func(string, string) error { t.Fatal("bulk upgrades used a modifier"); return nil },
	}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }, heroes: heroReaders{level: readHeroLevel}}, pipelineOptions{heroes: true, progression: true, export: &saveExportOptions{dir: t.TempDir()}, fishInterval: time.Second})
	p.frame, p.layout = frame, frame.layout
	p.export.requested = false
	p.ancient = ancientPlanner{plan: &ancientPlan{}, finished: true}
	p.ascension = ascensionPlanner{active: true, step: waitAscensionReset, lastInputFrame: 3}
	if err := p.accept(ctx, observation{kind: ascensionAnalysis, frame: frame, ascension: ascensionObservation{frame: frame, zone: 1}}, now); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupHeroes || p.export.requested || p.ancient.plan != nil || p.controls.isPaused() {
		t.Fatal("reset did not defer Ancient export for startup")
	}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now = now.Add(time.Second)
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	job := <-jobs[heroAnalysis]
	if job.startup != startupHeroes || len(jobs[exportAnalysis]) != 0 || len(jobs[skillAnalysis]) != 0 {
		t.Fatal("wrong startup worker scheduling")
	}
	frame.id++
	frame.at = now.Add(time.Second)
	p.frame = frame
	now = frame.at
	// Positive DPS alone must not allow export or finish the affordable sweep.
	frame = p.frame
	hero := heroObservation{frame: frame, startup: true, passiveReady: true}
	if err := p.accept(ctx, observation{kind: heroAnalysis, startup: startupHeroes, frame: frame, hero: hero}, now); err != nil {
		t.Fatal(err)
	}
	p.state[progressionAnalysis] = observation{frame: frame, progression: progressionState{Known: true, Enabled: true}}
	p.plan(now)
	if p.startup != startupHeroes || p.export.requested {
		t.Fatal("one passive hero completed startup")
	}
	frame.id++
	frame.at = now.Add(time.Second)
	p.frame = frame
	now = frame.at
	hero.frame, hero.startupComplete = frame, true
	if err := p.accept(ctx, observation{kind: heroAnalysis, startup: startupHeroes, frame: frame, hero: hero}, now); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupUpgrades || p.export.requested {
		t.Fatal("sweep skipped bulk upgrades")
	}
	// Use the real footer reader from a shared capture. No new screenshot or quantity OCR.
	now = now.Add(time.Second)
	p.nextCapture = time.Time{}
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[heroAnalysis]
	if job.startup != startupUpgrades {
		t.Fatal("wrong footer stage", job.startup)
	}
	out := p.analyze(ctx, heroAnalysis, job)
	if out.err != nil || !out.found {
		t.Fatal("native footer unreadable", out.err)
	}
	if err := p.accept(ctx, out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHeroUpgrades {
		t.Fatal("bulk input not queued", a.kind, ok)
	}
	acted, err := p.execute(ctx, a)
	if err != nil || !acted || clicks != 1 {
		t.Fatal("bulk click failed", acted, err, clicks)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if p.startup != startupProgression || p.export.requested {
		t.Fatal("bulk input skipped fresh mode observation")
	}
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	now = p.frame.at
	if err := p.accept(ctx, observation{kind: progressionAnalysis, frame: p.frame, progression: progressionState{Known: true, Enabled: true}}, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	if p.startup != noStartup || !p.export.requested || p.controls.isPaused() {
		t.Fatal("startup did not hand off to export")
	}
	// A late old hero read cannot return the finished pipeline to startup.
	late := observation{kind: heroAnalysis, startup: startupHeroes, frame: p.frame, hero: hero}
	p.accept(ctx, late, now)
	if p.startup != noStartup || !p.export.requested {
		t.Fatal("old startup read revived the stage")
	}
	// Submitted Ancient batches return to normal play without a blanket pause.
	p.export.requested = false
	previous := p.frame
	previous.id--
	p.ancient = ancientPlanner{plan: &ancientPlan{}, active: true, pending: &gameAction{kind: handleAncient, frame: previous, ancient: ancientCommand{step: returnAncientHeroes}}}
	if err := p.accept(ctx, observation{kind: ancientAnalysis, frame: p.frame, ancient: ancientObservation{frame: p.frame}}, now); err != nil {
		t.Fatal(err)
	}
	if !p.ancient.finished || p.controls.isPaused() {
		t.Fatal("finished batch paused normal play")
	}
}

func TestStartupF8FishAndModal(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	now := time.Now()
	f := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, window: "game", bounds: screen.Bounds()}}
	controls := &pauseControl{}
	p := newGamePipeline(controls, heroInput{click: func(image.Point) error { t.Fatal("stale input executed"); return nil }}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true, export: &saveExportOptions{}, fishInterval: time.Second})
	p.frame, p.layout, p.startup = f, f.layout, startupHeroes
	p.startupCheck = false
	p.export.requested = false
	p.hero.startStartup()
	p.state[heroAnalysis] = observation{frame: f, hero: heroObservation{frame: f, startup: true, startupNeedsGold: true}}
	p.plan(now)
	if _, ok := p.queue[clickMonster]; !ok {
		t.Fatal("zero-DPS startup cannot earn gold")
	}
	point := image.Pt(2000, 700)
	p.fishTarget = &point
	if a, ok := p.nextAction(now); !ok || a.kind != collectFish {
		t.Fatal("startup blocked known fish")
	}
	p.frame.context.saveMenu = true
	p.plan(now)
	if _, ok := p.nextAction(now); ok {
		t.Fatal("startup sent input through a menu")
	}
	p.frame.context = f.context
	p.startupPassive = true
	p.state[progressionAnalysis] = observation{frame: f, progression: progressionState{Known: true, Enabled: false, Zone: 1}}
	p.plan(now)
	if _, ok := p.queue[enableProgression]; !ok || p.export.requested {
		t.Fatal("early progression did not enable before export")
	}
	p.hero.sweep = startupSweep{top: true, y: 865, attempts: 2}
	p.hero.enabled = false
	controls.toggle()
	p.reset(controls.snapshot())
	if p.hero.sweep != (startupSweep{top: true, y: 865, attempts: 2}) || !p.hero.enabled || p.startup != startupHeroes || p.export.requested || len(p.queue) != 0 {
		t.Fatal("F8 lost bounded cursor or startup state")
	}
	old := gameAction{kind: buyHeroUpgrades, frame: f, point: point}
	if acted, err := p.execute(context.Background(), old); acted || err != nil {
		t.Fatal("old generation executed", acted, err)
	}
	controls.toggle()
	p.reset(controls.snapshot())
	p.startupDeadline = now.Add(-time.Second)
	p.plan(now)
	if !controls.isPaused() || controls.message() == "paused: " {
		t.Fatal("unsupported startup did not pause with a reason")
	}
}

func TestStartupInitialCaptureWithoutZoneOCR(t *testing.T) {
	ctx := context.Background()
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }}, pipelineOptions{heroes: true, fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	if p.startupCheck || p.startup != startupHeroes || len(jobs[heroAnalysis]) != 1 || len(jobs[ascensionAnalysis]) != 0 || len(jobs[exportAnalysis]) != 0 {
		t.Fatal("initial hero setup required zone OCR or export")
	}
}

func TestStartupWithoutExportOrProgression(t *testing.T) {
	now := time.Now()
	plan := &ancientPlan{}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{ascension: readAscensionObservation}, pipelineOptions{heroes: true, ancientPlan: plan})
	if !p.startupCheck {
		t.Fatal("hero setup depends on export/progression flags")
	}
	p.frame = testPipelineFrame()
	p.frame.context.heroes = true
	p.layout = p.frame.layout
	p.beginStartup()
	if p.ancient.plan != plan {
		t.Fatal("initial setup discarded supplied plan")
	}
	p.startup, p.startupPassive = startupProgression, true
	p.plan(now)
	if p.startup != noStartup || p.export.requested || p.controls.isPaused() {
		t.Fatal("hero-only setup cannot finish")
	}
}

func TestStartupVisitsHeroesWithoutInputThroughModal(t *testing.T) {
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{ascension: readAscensionObservation}, pipelineOptions{heroes: true, export: &saveExportOptions{}})
	p.frame = testPipelineFrame()
	p.layout = p.frame.layout
	p.frame.context.ancients = true
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != navigateGame || a.navigation != navigationHeroes {
		t.Fatal("startup did not return to Heroes", a, ok)
	}
	p.frame.context.ancientDialog = true
	p.enqueue(a, now)
	if _, ok := p.nextAction(now); ok {
		t.Fatal("Heroes tab clicked through a modal")
	}
}

func TestOrdinaryHeroUpgradeMaintenance(t *testing.T) {
	requireAncientOCR(t)
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	c.window = "game"
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }}, pipelineOptions{heroes: true, fishInterval: time.Second})
	p.startupCheck = false
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	job := <-jobs[heroAnalysis]
	if !job.upgrades || job.startup != noStartup {
		t.Fatal("periodic bulk check missing")
	}
	out := p.analyze(context.Background(), heroAnalysis, job)
	if out.err != nil || !out.found {
		t.Fatal(out.err, out.found)
	}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHeroUpgrades {
		t.Fatal("bulk upgrade maintenance not queued", a, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if p.startup != noStartup || p.export.requested {
		t.Fatal("maintenance restarted setup/export")
	}
	p.nextCapture = time.Time{}
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	next := <-jobs[heroAnalysis]
	if next.upgrades {
		t.Fatal("bulk OCR repeated immediately")
	}
}

func TestStartupSeeksTopWithoutInitialFooterPass(t *testing.T) {
	f := startupFrame(t, "testdata/hero-tsuchi-x1.png")
	f.layout = 1
	c, err := recognizedGame(f.image)
	if err != nil {
		t.Fatal(err)
	}
	f.context = c
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true})
	p.frame, p.layout = f, f.layout
	p.beginStartup()
	out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, startup: p.startup})
	if out.err != nil || out.found || out.hero.startupScroll == (image.Point{}) {
		t.Fatalf("initial top seek: %+v %v", out.hero, out.err)
	}
	if err = p.accept(context.Background(), out, f.at); err != nil {
		t.Fatal(err)
	}
	p.plan(f.at)
	a, ok := p.nextAction(f.at)
	if !ok || a.kind != scrollHeroes || a.target.Y >= a.point.Y {
		t.Fatalf("initial startup must seek top, not footer: %+v %t", a, ok)
	}
}
