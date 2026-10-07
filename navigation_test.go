package main

import (
	"context"
	"errors"
	"image"
	"sync/atomic"
	"testing"
	"time"
)

func TestNavigationCancelsRecognizedOrphanModals(t *testing.T) {
	for _, test := range []struct {
		file string
		step navigationStep
	}{
		{"save-menu.png", navigationSettings},
		{"ancient-quantity.png", navigationQuantity},
		{"ancient-quantity-filled.png", navigationQuantity},
		{"ascension-confirm.png", navigationAscension},
		{"ascension-junk.png", navigationJunk},
		{"mercenary-quests.png", navigationQuest},
		{"mercenary-revive.png", navigationRecovery},
		{"mercenary-bury.png", navigationRecovery},
		{"gild-chest.png", navigationGildChest},
		{"gild-reward.png", navigationGildReward},
		{"gild-roster.png", navigationGildRoster},
	} {
		t.Run(test.file, func(t *testing.T) {
			for _, width := range []int{1280, 2560} {
				screen := exportFixture(t, test.file, width)
				c, err := recognizedGame(screen)
				if err != nil || !c.known {
					t.Fatalf("context=%+v err=%v", c, err)
				}
				c.window = "game"
				now := time.Now()
				var clicks int
				p := newGamePipeline(&pauseControl{}, heroInput{click: func(image.Point) error { clicks++; return nil }}, pipelineReaders{window: func() string { return "game" }}, pipelineOptions{heroes: true})
				p.frame = gameFrame{id: 1, layout: p.layout, at: now, image: screen, context: c}
				p.plan(now)
				a, ok := p.nextAction(now)
				if !ok || a.kind != navigateGame || a.navigation != test.step {
					t.Fatalf("action=%+v ok=%t", a, ok)
				}
				if acted, err := p.execute(context.Background(), a); err != nil || !acted || clicks != 1 {
					t.Fatalf("acted=%t clicks=%d err=%v", acted, clicks, err)
				}
				p.actionCompleted(actionResult{action: a, acted: true}, now)
				// A missed click waits, then retries on a newer observed frame.
				p.frame.id++
				p.frame.at = now.Add(600 * time.Millisecond)
				p.plan(p.frame.at)
				if _, ok := p.nextAction(p.frame.at); ok {
					t.Fatal("rapid duplicate navigation")
				}
				p.frame.id++
				p.frame.at = now.Add(1500 * time.Millisecond)
				p.plan(p.frame.at)
				a, ok = p.nextAction(p.frame.at)
				if !ok || a.navigation != test.step || p.controls.isPaused() {
					t.Fatal("missed close did not retry")
				}
				// A changed window cannot receive this close input.
				p.frame.context.window = "!outside-game"
				if p.navigationStable(a) {
					t.Fatal("stale close survived window change")
				}
			}
		})
	}
}

func TestNavigationF8AndUnknownFrameRecovery(t *testing.T) {
	menu := exportFixture(t, "save-menu.png", 1280)
	screen := menu
	window := "game"
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }, focus: func(got string) error {
		if got != "game" {
			t.Fatal(got)
		}
		return nil
	}}, pipelineReaders{context: recognizedGame, window: func() string { return window }}, pipelineOptions{heroes: true})
	jobs := mercenaryRecoveryJobs()
	now := time.Now()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.navigation != navigationSettings {
		t.Fatal("startup in Settings did not close it")
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if acted, err := p.execute(context.Background(), a); acted || err != nil {
		t.Fatal("F8 retained stale close")
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	window = "!outside-game"
	screen = &viewportImage{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), reason: "game not foreground", geometry: viewportGeometry{Scene: nativeScene{Window: window}}}
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	p.plan(now.Add(time.Second))
	a, ok = p.nextAction(now.Add(time.Second))
	if !ok || a.navigation != navigationFocus || p.controls.isPaused() {
		t.Fatal("lost focus stalled recovery")
	}
	if acted, err := p.execute(context.Background(), a); err != nil || !acted {
		t.Fatal(err)
	}
	window, screen = "game", menu
	if err := p.capture(context.Background(), now.Add(2*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	p.plan(now.Add(2 * time.Second))
	a, ok = p.nextAction(now.Add(2 * time.Second))
	if !ok || a.navigation != navigationSettings {
		t.Fatal("return from unknown frame did not close Settings")
	}
}

func TestExportRecoveryMissedCloseAndRepeatedFocusSteal(t *testing.T) {
	now := time.Now()
	menu := exportFixture(t, "save-menu.png", 1280)
	c, _ := recognizedGame(menu)
	c.window = "game"
	window := "game"
	var focuses, clicks int
	p := newGamePipeline(&pauseControl{}, heroInput{focus: func(string) error { focuses++; return nil }, click: func(image.Point) error { clicks++; return nil }}, pipelineReaders{window: func() string { return window }}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	p.export = saveExporter{requested: true, active: true, step: exportCloseMenu, before: exportSnapshot{}, window: "game", deadline: now.Add(20 * time.Second)}
	p.frame = gameFrame{id: 1, at: now, layout: p.layout, image: menu, context: c}
	send := func(at time.Time, want exportStep) {
		t.Helper()
		p.frame.id++
		p.frame.at = at
		p.plan(at)
		a, ok := p.nextAction(at)
		if !ok || a.kind != handleExport || a.export.step != want {
			t.Fatalf("step=%v action=%+v ok=%t", want, a, ok)
		}
		if acted, err := p.execute(context.Background(), a); err != nil || !acted {
			t.Fatal(err)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, at)
	}
	send(now, exportCloseMenu)
	send(now.Add(1500*time.Millisecond), exportCloseMenu)
	window = "!outside-game"
	p.frame.context = gameContext{window: window, bounds: menu.Bounds()}
	send(now.Add(3*time.Second), exportRestoreGame)
	// Focus returns nil but Explorer is still foreground: retry instead of timing out.
	send(now.Add(4500*time.Millisecond), exportRestoreGame)
	window = "game"
	p.frame.context = c
	send(now.Add(6*time.Second), exportCloseMenu)
	window = "!outside-game"
	p.frame.context = gameContext{window: window, bounds: menu.Bounds()}
	send(now.Add(7500*time.Millisecond), exportRestoreGame)
	if focuses != 3 || clicks != 3 || p.controls.isPaused() {
		t.Fatalf("focuses=%d clicks=%d paused=%t", focuses, clicks, p.controls.isPaused())
	}
	// Pause/resume preserves ownership, but cancels stale input/read jobs.
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.export.before == nil || p.export.step != exportCloseMenu {
		t.Fatal("F8 discarded acknowledged Save baseline")
	}
	window = "game"
	p.frame.context = c
	p.frame.generation = p.generation
	p.frame.layout = p.layout
	send(now.Add(9*time.Second), exportCloseMenu)
	if focuses != 3 || clicks != 4 {
		t.Fatal("resume repeated Save instead of closing owned menu")
	}
}

func TestNavigationKeepsOwnedTransactions(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{gilds: true, mercenaries: true})
	now := time.Now()
	for _, file := range []string{"ancient-quantity.png", "ascension-confirm.png", "mercenary-quests.png", "gild-reward.png"} {
		screen := exportFixture(t, file, 1280)
		c, _ := recognizedGame(screen)
		c.window = "game"
		p.frame = gameFrame{id: 1, at: now, image: screen, context: c}
		p.ancient.active = true
		p.ascension.active = true
		p.mercenary.active = true
		p.gild.active = true
		if p.planNavigation(now) {
			t.Fatal("recovery hijacked owned transaction", file)
		}
	}
}

func TestNavigationCancellationCannotAcknowledgeAncientOK(t *testing.T) {
	now := time.Now()
	screen := exportFixture(t, "ancient-quantity-filled.png", 1280)
	c, _ := recognizedGame(screen)
	c.window = "game"
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	p.frame = gameFrame{id: 2, at: now, image: screen, context: c}
	p.ancient = ancientPlanner{plan: &ancientPlan{}, active: true, started: true, selected: 0, quantity: "52", deadline: now.Add(-time.Second), done: map[int]bool{}, pending: &gameAction{frame: gameFrame{id: 1}, ancient: ancientCommand{step: confirmAncientQuantity}}}
	if !p.planNavigation(now) || !p.ancient.blocked || p.ancient.pending != nil {
		t.Fatal("cancel did not revoke pending OK")
	}
	c.ancientDialog, c.ancients = false, true
	p.ancient.observe(ancientObservation{frame: gameFrame{id: 3, context: c}}, nil, now.Add(time.Second))
	if p.ancient.done[0] || !p.ancient.blocked || p.ancient.active {
		t.Fatal("closing through X acknowledged a purchase")
	}
}

func TestNavigationBlockedBatchResumesOrdinaryAnalysis(t *testing.T) {
	screen := exportFixture(t, "hero-panel-max.png", 1280)
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{context: recognizedGame, window: func() string { return "game" }}, pipelineOptions{heroes: true, skills: true, progression: true, fishInterval: time.Second})
	p.startupCheck = false
	p.ancient = ancientPlanner{plan: &ancientPlan{}, blocked: true, started: true}
	jobs := mercenaryRecoveryJobs()
	if err := p.capture(context.Background(), time.Now(), jobs); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []analysisKind{heroAnalysis, skillAnalysis} {
		select {
		case <-jobs[kind]:
		default:
			t.Fatal("stopped Ancient batch suppressed ordinary analysis", kind)
		}
	}
	p.plan(time.Now())
	if p.controls.isPaused() {
		t.Fatal("blocked purchase batch paused free gameplay")
	}
	if _, ok := p.ancient.action(p.frame, time.Now()); ok {
		t.Fatal("stopped batch replayed a purchase")
	}
}

func TestNavigationTransientCaptureFailureRecovers(t *testing.T) {
	menu := exportFixture(t, "save-menu.png", 1280)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var captures, clicks atomic.Int32
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture: func() (image.Image, error) {
			if captures.Add(1) == 1 {
				return nil, errors.New("temporary native capture failure")
			}
			return menu, nil
		},
		click: func(image.Point) error { clicks.Add(1); cancel(); return nil },
	}, pipelineReaders{context: recognizedGame, window: func() string { return "game" }}, pipelineOptions{fishInterval: time.Second})
	if err := p.run(ctx); err != nil {
		t.Fatal(err)
	}
	if clicks.Load() != 1 || captures.Load() < 2 || p.controls.isPaused() {
		t.Fatal("temporary capture failure stopped navigation", captures.Load(), clicks.Load())
	}
}

func TestExportLateSaveCompletionRetainsOnlyFileOwnership(t *testing.T) {
	for _, acted := range []bool{false, true} {
		p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{}})
		p.export.active, p.export.window, p.export.step = true, "game", exportSave
		a := gameAction{kind: handleExport, frame: testPipelineFrame(), export: &exportCommand{step: exportSave, window: "game", before: exportSnapshot{}}}
		p.controls.toggle()
		p.reset(p.controls.snapshot())
		p.rememberCompletedSave(actionResult{action: a, acted: acted})
		p.controls.toggle()
		p.reset(p.controls.snapshot())
		if (p.export.before != nil) != acted || p.ancient.plan != nil || p.export.active {
			t.Fatal("lost completed Save or retained unexecuted/stale decisions", acted)
		}
		if acted && p.export.step != exportCloseMenu {
			t.Fatal("completed Save would be replayed")
		}
	}
}

func TestExportF8BetweenSaveRecordAndActionAcknowledgement(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{}})
	p.export.active, p.export.window, p.export.step = true, "game", exportSave
	a := gameAction{kind: handleExport, frame: testPipelineFrame(), export: &exportCommand{step: exportSave, window: "game", before: exportSnapshot{}}}
	p.rememberCompletedSave(actionResult{action: a, acted: true})
	// F8 arrives after ownership recording but before the normal acknowledgement.
	p.controls.toggle()
	if applied, err := p.controls.runClick(context.Background(), a.frame.generation, func() error { t.Fatal("stale acknowledgement applied"); return nil }); err != nil || applied {
		t.Fatal(err)
	}
	p.reset(p.controls.snapshot())
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.export.before == nil || p.export.step != exportCloseMenu || p.export.active || p.ancient.plan != nil {
		t.Fatal("F8 lost the actual Save or authorized a stale decision")
	}
}
