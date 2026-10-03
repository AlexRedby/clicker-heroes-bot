package main

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestAutoClickerSharedPipeline(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
	captures := 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { captures++; return screen, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }, autoClickers: func(context.Context, gameFrame) (autoClickerPool, error) {
		return autoClickerPool{known: true, available: 3, total: 3}, nil
	}}, pipelineOptions{heroes: true, autoClickers: true, fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	poolJob := <-jobs[autoClickerAnalysis]
	heroJob := <-jobs[heroAnalysis]
	fishJob := <-jobs[fishAnalysis]
	if captures != 1 || poolJob.frame.image != heroJob.frame.image || poolJob.frame.id != fishJob.frame.id {
		t.Fatal("clicker recognition did not share capture")
	}
	if err := p.accept(context.Background(), p.analyze(context.Background(), autoClickerAnalysis, poolJob), now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerMonster {
		t.Fatal("startup blocked monster placement", a, ok)
	}
	// The runtime records ownership before dispatch; acknowledgement uses a newer frame.
	p.clickers.sent(a.clicker, now)
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	if err := p.accept(context.Background(), observation{kind: autoClickerAnalysis, frame: p.frame, clickerPool: autoClickerPool{known: true, available: 2, total: 3}}, p.frame.at); err != nil {
		t.Fatal(err)
	}
	if p.clickers.pending != nil || p.controls.isPaused() {
		t.Fatal("pool decrement not acknowledged")
	}
	p.plan(p.frame.at)
	next, ok := p.nextAction(p.frame.at)
	if !ok || next.kind != placeOwnedClicker {
		t.Fatal("next spare clicker not queued", next, ok)
	}
	// An open settings/quantity dialog suppresses all placement.
	p.frame.context.saveMenu = true
	p.enqueue(next, p.frame.at)
	if _, ok := p.nextAction(p.frame.at); ok {
		t.Fatal("placement reached menu")
	}
}

func TestAutoClickerFooterPriorityAndF8(t *testing.T) {
	requireAncientOCR(t)
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
	frame := gameFrame{id: 3, layout: 1, at: now, image: screen, context: c}
	point, found, err := readHeroUpgradeButton(context.Background(), screen)
	if err != nil || !found {
		t.Fatal(err, found)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, autoClickers: true})
	p.frame, p.layout, p.startup, p.startupCheck = frame, 1, startupUpgrades, false
	p.state[autoClickerAnalysis] = observation{frame: frame, clickerPool: autoClickerPool{known: true, available: 1, total: 3}}
	p.state[heroAnalysis] = observation{frame: frame, startup: startupUpgrades, found: true, point: point}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades {
		t.Fatal("plain bulk purchase displaced footer placement", a, ok)
	}
	p.clickers.sent(a.clicker, now)
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.clickers.pending == nil || p.startup != startupUpgrades {
		t.Fatal("F8 lost submitted placement")
	}
	if acted, err := p.execute(context.Background(), a); acted || err != nil {
		t.Fatal("old input executed", acted, err)
	}
}
