package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
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

func TestAutoClickerFooterStartupHandoff(t *testing.T) {
	requireAncientOCR(t)
	for _, tc := range []struct {
		name      string
		confirmed bool
		resumeF8  bool
	}{
		{name: "confirmed", confirmed: true},
		{name: "confirmed after F8", confirmed: true, resumeF8: true},
		{name: "occupied footer", confirmed: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now()
			screen := loadTestImage(t, "testdata/hero-startup-zero.png")
			c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
			available := 1
			p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
				context: func(image.Image) (gameContext, error) { return c, nil },
				autoClickers: func(context.Context, gameFrame) (autoClickerPool, error) {
					return autoClickerPool{known: true, available: available, total: 3}, nil
				},
				progression: func(context.Context, image.Image, [9]skillState, bool) (progressionState, error) {
					return progressionState{Known: true, Enabled: true}, nil
				},
			}, pipelineOptions{heroes: true, autoClickers: true, progression: true, export: &saveExportOptions{}, fishInterval: time.Second})
			p.startup, p.startupCheck, p.startupPassive = startupUpgrades, false, true
			p.export.requested = false
			jobs := make([]chan analysisJob, analysisCount)
			for i := range jobs {
				jobs[i] = make(chan analysisJob, 1)
			}
			if err := p.capture(ctx, now, jobs); err != nil {
				t.Fatal(err)
			}
			poolJob, heroJob := <-jobs[autoClickerAnalysis], <-jobs[heroAnalysis]
			if poolJob.frame.id != heroJob.frame.id {
				t.Fatal("pool and footer used different frames")
			}
			for _, result := range []observation{p.analyze(ctx, autoClickerAnalysis, poolJob), p.analyze(ctx, heroAnalysis, heroJob)} {
				if err := p.accept(ctx, result, now); err != nil {
					t.Fatal(err)
				}
			}
			p.plan(now)
			a, ok := p.nextAction(now)
			if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades {
				t.Fatalf("footer clicker not selected: %+v %t", a, ok)
			}
			p.clickers.sent(a.clicker, now)
			p.actionCompleted(actionResult{action: a, acted: true}, now)
			if tc.resumeF8 {
				p.controls.toggle()
				p.controls.toggle()
				p.reset(p.controls.snapshot())
			}
			p.plan(now.Add(250 * time.Millisecond))
			if _, queued := p.queue[buyHeroUpgrades]; queued {
				t.Fatal("ordinary upgrade click ran before placement acknowledgement")
			}
			ackAt := now.Add(time.Second)
			if tc.confirmed {
				available = 0
			} else {
				ackAt = now.Add(6 * time.Second)
			}
			if err := p.capture(ctx, ackAt, jobs); err != nil {
				t.Fatal(err)
			}
			if !tc.resumeF8 && len(jobs[heroAnalysis]) != 0 {
				t.Fatal("stale footer unexpectedly scheduled a hero read")
			}
			poolJob = <-jobs[autoClickerAnalysis]
			if err := p.accept(ctx, p.analyze(ctx, autoClickerAnalysis, poolJob), ackAt); err != nil {
				t.Fatal(err)
			}
			p.plan(ackAt)
			if tc.confirmed {
				if p.startup != startupProgression || !p.clickers.upgrades || p.export.requested {
					t.Fatal("confirmed footer clicker did not advance to fresh progression")
				}
			} else {
				if p.startup != startupUpgrades || p.clickers.upgrades || !p.clickers.footerAttempted {
					t.Fatal("unconfirmed footer clicker skipped ordinary upgrade fallback")
				}
				if _, ok := p.nextAction(ackAt); ok {
					t.Fatal("stale action ran before a fresh footer read")
				}
				readAt := ackAt.Add(time.Second)
				if err := p.capture(ctx, readAt, jobs); err != nil {
					t.Fatal(err)
				}
				heroJob = <-jobs[heroAnalysis]
				if err := p.accept(ctx, p.analyze(ctx, heroAnalysis, heroJob), readAt); err != nil {
					t.Fatal(err)
				}
				p.plan(readAt)
				a, ok = p.nextAction(readAt)
				if !ok || a.kind != buyHeroUpgrades {
					t.Fatalf("unconfirmed placement did not retry ordinary footer: %+v %t", a, ok)
				}
				p.actionCompleted(actionResult{action: a, acted: true}, readAt)
				ackAt = readAt
			}
			if p.startup != startupProgression || p.export.requested {
				t.Fatal("footer pass did not wait for progression")
			}
			modeAt := ackAt.Add(time.Second)
			if err := p.capture(ctx, modeAt, jobs); err != nil {
				t.Fatal(err)
			}
			modeJob := <-jobs[progressionAnalysis]
			if modeJob.frame.id != p.frame.id {
				t.Fatal("progression was not read from a fresh frame")
			}
			if err := p.accept(ctx, p.analyze(ctx, progressionAnalysis, modeJob), modeAt); err != nil {
				t.Fatal(err)
			}
			p.plan(modeAt)
			if p.startup != noStartup || !p.export.requested || p.controls.isPaused() {
				t.Fatal("startup did not hand off to export")
			}
		})
	}
}

func TestAutoClickerPeriodicPoolRecovery(t *testing.T) {
	now := time.Now()
	s := loadTestImage(t, "testdata/hero-startup-zero.png")
	f := gameFrame{id: 10, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, autoClickers: true})
	p.frame, p.startupCheck = f, false
	p.clickers.blocked = true
	p.clickers.footerAttempted = true
	p.clickers.rememberPool(f, autoClickerPool{known: true, available: 0, total: 3})
	p.nextClickerRecovery = now.Add(5 * time.Minute)
	f.id++
	f.at = now.Add(time.Minute)
	p.frame = f
	pool := autoClickerPool{known: true, available: 0, total: 3}
	if err := p.accept(context.Background(), observation{kind: autoClickerAnalysis, frame: f, clickerPool: pool}, f.at); err != nil {
		t.Fatal(err)
	}
	if !p.clickers.blocked || p.nextClickerRead != f.at.Add(5*time.Minute) {
		t.Fatal("recovery ran early or exhausted pool was polled rapidly")
	}
	f.id++
	f.at = now.Add(6 * time.Minute)
	p.frame = f
	pool.available = 2
	if err := p.accept(context.Background(), observation{kind: autoClickerAnalysis, frame: f, clickerPool: pool}, f.at); err != nil {
		t.Fatal(err)
	}
	if p.clickers.blocked || p.clickers.footerAttempted || p.clickerFooterUntil != f.at.Add(10*time.Second) {
		t.Fatal("fresh free pool did not start bounded recovery")
	}
	p.plan(f.at)
	a, ok := p.nextAction(f.at)
	if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerMonster {
		t.Fatal("free clicker restoration was not queued", a, ok)
	}
	p.frame.context.saveMenu = true
	p.enqueue(a, f.at)
	if _, ok := p.nextAction(f.at); ok {
		t.Fatal("restoration reached covering menu")
	}
}

func TestStartupBulkWaitsForFooterPool(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/hero-startup-zero.png")
	point, found, err := readHeroUpgradeButton(context.Background(), screen)
	if err != nil || !found {
		t.Fatal("native footer", found, err)
	}
	for _, tc := range []struct {
		name                              string
		pool                              autoClickerPool
		stale, changedMask, hold, timeout bool
	}{
		{name: "pending pool then footer", hold: true},
		{name: "stale empty pool then footer", pool: autoClickerPool{known: true, total: 3}, stale: true, hold: true},
		{name: "changed empty pool mask then footer", pool: autoClickerPool{known: true, total: 3}, changedMask: true, hold: true},
		{name: "unreadable pool bounded fallback", hold: true, timeout: true},
		{name: "fresh empty pool", pool: autoClickerPool{known: true, total: 3}},
		{name: "sole clicker", pool: autoClickerPool{known: true, available: 1, total: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			f := gameFrame{id: 3, generation: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, autoClickers: true})
			p.frame, p.layout, p.generation = f, f.layout, f.generation
			p.startup, p.startupCheck, p.startupPassive = startupUpgrades, false, true
			footer := observation{frame: f, startup: startupUpgrades, found: true, upgradesKnown: true, point: point}
			p.state[heroAnalysis] = footer
			p.plan(now)
			a, ok := p.nextAction(now)
			if !ok || a.kind != buyHeroUpgrades {
				t.Fatal("bulk purchase before pool completion", a.kind, ok)
			}
			deadline := p.startupDeadline
			poolFrame := f
			if tc.stale {
				poolFrame.generation--
			}
			if tc.changedMask {
				s := image.NewRGBA(screen.Bounds())
				draw.Draw(s, s.Bounds(), screen, s.Bounds().Min, draw.Src)
				draw.Draw(s, autoClickerCountRegion(s), image.NewUniform(color.Black), image.Point{}, draw.Src)
				poolFrame.image = s
			}
			p.state[autoClickerAnalysis] = observation{frame: poolFrame, clickerPool: tc.pool}
			p.actionCompleted(actionResult{action: a, acted: true}, now)
			if !tc.hold {
				if p.startup != startupProgression {
					t.Fatal("ineligible fresh pool delayed handoff", p.startup)
				}
				return
			}
			if p.startup != startupUpgrades || p.clickers.footerPasses != 0 {
				t.Fatal("bulk purchase released footer reservation", p.startup, p.clickers.footerPasses)
			}
			// New footer reads cannot reset the deadline or repeat the ordinary input.
			for i := 1; i <= 2; i++ {
				f.id++
				f.at = now.Add(time.Duration(i) * time.Second)
				p.frame, footer.frame = f, f
				p.state[heroAnalysis] = footer
				p.plan(f.at)
				if _, ok := p.nextAction(f.at); ok || p.startupDeadline != deadline || p.startup != startupUpgrades {
					t.Fatal("waiting pool repeated input or changed deadline")
				}
			}
			pool := autoClickerPool{known: true, available: 1, total: 3}
			if tc.timeout {
				p.plan(deadline.Add(time.Nanosecond))
				if p.startup != startupProgression || p.clickers.footerPasses != 1 {
					t.Fatal("footer fallback exceeded its bounded pass")
				}
				monster := image.Pt(screen.Bounds().Dx()*3/4, screen.Bounds().Dy()/2)
				if _, ok := p.clickers.command(f, pool, autoClickerMonster, monster); !ok {
					t.Fatal("bounded fallback retained the last spare")
				}
				return
			}
			p.state[autoClickerAnalysis] = observation{frame: f, clickerPool: pool}
			p.state[heroAnalysis] = footer
			p.plan(f.at)
			a, ok = p.nextAction(f.at)
			if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades || a.point != point {
				t.Fatal("late known pool did not place footer before handoff", a.kind, a.clicker.target, ok)
			}
		})
	}
}
