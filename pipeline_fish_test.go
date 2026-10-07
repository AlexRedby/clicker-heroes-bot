package main

import (
	"bytes"
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func TestFishInEveryTransaction(t *testing.T) {
	for _, state := range []string{"Ancients", "quantity", "Ascension", "Save", "file read", "gilds", "quests", "Heroes", "Mercenaries", "unrecognized game UI", "Explorer"} {
		t.Run(state, func(t *testing.T) {
			now := time.Now()
			frame := testPipelineFrame()
			p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }}, pipelineReaders{}, pipelineOptions{fishInterval: time.Second})
			switch state {
			case "Ancients", "quantity":
				frame.context.ancients = state == "Ancients"
				frame.context.ancientDialog = state == "quantity"
				p.ancient = ancientPlanner{active: true, pending: &gameAction{frame: frame}, deadline: now.Add(20 * time.Second)}
			case "Ascension":
				frame.context.ascension = true
				p.ascension = ascensionPlanner{active: true, lastInputFrame: frame.id, deadline: now.Add(20 * time.Second)}
			case "Save", "file read":
				frame.context.saveMenu = state == "Save"
				p.options.export = &saveExportOptions{dir: t.TempDir()}
				p.export = saveExporter{requested: true, active: true, step: exportReadFile, window: "game", deadline: now.Add(time.Minute)}
				if state == "Save" {
					p.export.step = exportCloseMenu
				}
			case "gilds":
				frame.context.modal = gildChestModal
				p.gild.active = true
			case "quests":
				frame.context.questDialog = true
				p.mercenary.pending = &mercenaryAttempt{action: gameAction{frame: frame}}
			case "Heroes":
				frame.context.heroes = true
			case "Mercenaries":
				frame.context.mercenaries = true
			case "unrecognized game UI":
				frame.context.known = false
			case "Explorer":
				frame.context.known = false
				frame.context.window = "!outside-game"
			}
			p.frame, p.layout = frame, frame.layout
			p.readers.context = func(image.Image) (gameContext, error) { return frame.context, nil }
			p.readers.window = func() string { return frame.context.window }
			jobs := make([]chan analysisJob, analysisCount)
			for kind := range jobs {
				jobs[kind] = make(chan analysisJob, 1)
			}
			if err := p.capture(context.Background(), now, jobs); err != nil {
				t.Fatal(err)
			}
			if state != "Ancients" && state != "file read" && state != "Heroes" && state != "Mercenaries" {
				if len(jobs[fishAnalysis]) != 0 {
					t.Fatal("fish scanned a covered or unknown game screen")
				}
				if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: p.frame, found: true, point: image.Pt(20, 20)}, now); err != nil {
					t.Fatal(err)
				}
				if a, ok := p.nextAction(now); ok && a.kind == collectFish {
					t.Fatal("covered fish clicked")
				}
				return
			}
			if len(jobs[fishAnalysis]) != 1 {
				t.Fatal("transaction did not schedule fish")
			}
			job := <-jobs[fishAnalysis]
			point := image.Pt(20, 20)
			if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: job.frame, found: true, point: point}, now); err != nil {
				t.Fatal(err)
			}
			a, ok := p.nextAction(now)
			if !ok || a.kind != collectFish || a.point != point {
				t.Fatal("transaction blocked fresh fish", a, ok)
			}
			clicks := 0
			p.input.click = func(got image.Point) error {
				if got != point {
					t.Fatal("wrong fish point")
				}
				clicks++
				return nil
			}
			acted, err := p.execute(context.Background(), a)
			if !acted || err != nil || clicks != 1 {
				t.Fatal("fish not collected", acted, err)
			}
			pending := p.ancient.pending
			p.actionCompleted(actionResult{action: a, acted: true}, now)
			if p.ancient.pending != pending || p.controls.isPaused() {
				t.Fatal("fish lost purchase ownership")
			}
			p.controls.toggle()
			p.controls.toggle()
			if acted, err := p.execute(context.Background(), a); acted || err != nil || clicks != 1 {
				t.Fatal("stale F8 fish clicked")
			}
		})
	}
}

func TestAncientRecognitionRetryDoesNotWaitForFish(t *testing.T) {
	now := time.Now()
	frame := testPipelineFrame()
	frame.context.ancients = true
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	p.frame, p.layout = frame, frame.layout
	pending := &gameAction{ancient: ancientCommand{step: scrollAncients, direction: 1, before: []ancientNameAnchor{{"Argaiv", 80}}}, point: image.Pt(50, 10)}
	p.ancient = ancientPlanner{plan: &ancientPlan{Plan: ancientcalc.Plan{Owned: []ancientcalc.Level{{Name: "Argaiv"}}, Rows: []ancientcalc.Purchase{{Name: "Atman"}}}}, active: true, pending: pending, deadline: now.Add(20 * time.Second)}
	failure := observation{kind: ancientAnalysis, frame: frame, err: errors.New("scrollbar obscured")}
	if err := p.accept(context.Background(), failure, now); err != nil {
		t.Fatal(err)
	}
	if p.ancient.blocked || !p.ancient.nextRead.Equal(now.Add(time.Second)) {
		t.Fatal("transient OCR failure stopped the transaction")
	}
	// A later shared frame confirms scrolling without any completed fish scan.
	frame.id++
	confirmed := ancientObservation{frame: frame, anchors: []ancientNameAnchor{{"Argaiv", 70}}}
	if err := p.accept(context.Background(), observation{kind: ancientAnalysis, frame: frame, ancient: confirmed}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if p.ancient.pending != nil {
		t.Fatal("scroll confirmation waited for fish")
	}
	p.ancient.plan = &ancientPlan{}
	p.ancient.pending = pending
	p.ancient.deadline = now.Add(20 * time.Second)
	p.plan(now.Add(21 * time.Second))
	if !p.ancient.blocked || p.controls.isPaused() {
		t.Fatal("persistent recognition failure did not stop only the purchase batch")
	}
}

func TestFishCollectionDuringSlowAncientRead(t *testing.T) {
	frame := testPipelineFrame()
	frame.context.ancients = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, clicked := make(chan struct{}), make(chan struct{}, 1)
	var visible atomic.Bool
	visible.Store(true)
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }, click: func(image.Point) error { visible.Store(false); clicked <- struct{}{}; return nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return frame.context, nil },
		fish:    func(image.Image) (image.Point, bool, error) { return image.Pt(20, 20), visible.Load(), nil },
		ancients: func(ctx context.Context, f gameFrame) (ancientObservation, error) {
			close(started)
			<-ctx.Done()
			return ancientObservation{frame: f}, ctx.Err()
		},
	}, pipelineOptions{fishInterval: time.Millisecond})
	p.ancient = ancientPlanner{active: true, pending: &gameAction{}, deadline: time.Now().Add(time.Minute)}
	p.frame, p.layout = frame, frame.layout
	done := make(chan error, 1)
	go func() { done <- p.run(ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("Ancient reader never started")
	}
	select {
	case <-clicked:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("Ancient OCR blocked fish")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSIFTFishOverAncientScrollbar(t *testing.T) {
	background := loadTestImage(t, "testdata/ascension-ancients.png")
	thumb, _, found := listScrollbarThumb(background, 416)
	if !found {
		t.Fatal("baseline Ancient scrollbar missing")
	}
	fish, err := png.Decode(bytes.NewReader(fishPNG))
	if err != nil {
		t.Fatal(err)
	}
	height := background.Bounds().Dy() / 12
	scaled := image.NewRGBA(image.Rect(0, 0, fish.Bounds().Dx()*height/fish.Bounds().Dy(), height))
	xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), fish, fish.Bounds(), draw.Src, nil)
	screen := image.NewRGBA(background.Bounds())
	draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
	placement := thumb.Sub(scaled.Bounds().Size().Div(2))
	draw.Draw(screen, scaled.Bounds().Add(placement), scaled, image.Point{}, draw.Over)
	detector, err := newSIFTFishDetector()
	if err != nil {
		t.Fatal(err)
	}
	defer detector.Close()
	point, found, err := detector.Find(screen)
	if err != nil || !found || absDiff(point.X, thumb.X) > 15 || absDiff(point.Y, thumb.Y) > 15 {
		t.Fatalf("obstructing fish=%v found=%t error=%v, want near %v", point, found, err, thumb)
	}
}

func TestFishWaitsAcrossCoveringModals(t *testing.T) {
	for _, modal := range []string{"quantity", "settings", "Ascension", "gilds", "quest"} {
		t.Run(modal, func(t *testing.T) {
			now := time.Now()
			frame := testPipelineFrame()
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
			p.frame, p.layout = frame, frame.layout
			// A scan starts on the main game and finishes after a modal opens.
			source := frame
			p.frame.id++
			p.layout++
			p.frame.layout = p.layout
			switch modal {
			case "quantity":
				p.frame.context.ancientDialog = true
			case "settings":
				p.frame.context.saveMenu = true
			case "Ascension":
				p.frame.context.ascension = true
			case "gilds":
				p.frame.context.modal = gildChestModal
			case "quest":
				p.frame.context.questDialog = true
			}
			point := image.Pt(20, 20)
			if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: source, found: true, point: point}, now); err != nil {
				t.Fatal(err)
			}
			if p.fishTarget == nil || *p.fishTarget != point {
				t.Fatal("modal discarded the known fish position")
			}
			if a, ok := p.nextAction(now); ok && a.kind == collectFish {
				t.Fatal("covered fish clicked")
			}
			// Tab changes and elapsed time do not invalidate that position.
			p.frame.context = source.context
			p.frame.context.ancients = true
			p.frame.id++
			p.frame.at = now.Add(time.Minute)
			a, ok := p.nextAction(p.frame.at)
			if !ok || a.kind != collectFish || a.point != point || a.frame.id != p.frame.id {
				t.Fatal("cached fish did not resume on the current tab", a, ok)
			}
			p.actionCompleted(actionResult{action: a, acted: true}, p.frame.at)
			if p.fishTarget != nil {
				t.Fatal("clicked fish position was not cleared")
			}
			// An already-running pre-click scan must not restore the collected fish.
			if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: source, found: true, point: point}, p.frame.at); err != nil {
				t.Fatal(err)
			}
			if p.fishTarget != nil {
				t.Fatal("pre-click scan resurrected the fish")
			}
		})
	}
}

func TestFishRetryAndModalTransition(t *testing.T) {
	now := time.Now()
	frame := testPipelineFrame()
	frame.context.ancients = true
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{fishInterval: time.Second})
	p.frame, p.layout = frame, frame.layout
	out := observation{kind: fishAnalysis, frame: frame, found: true, point: image.Pt(20, 20)}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("first fish missing")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	p.frame.id++
	out.frame = p.frame
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.nextAction(now); ok {
		t.Fatal("fish double clicked without retry delay")
	}
	now = now.Add(time.Second)
	p.fish.lastClickAt = time.Now().Add(-6 * time.Second)
	p.frame.id++
	out.frame = p.frame
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	a, ok = p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("still visible fish never retried")
	}
	calls := 0
	p.input.click = func(image.Point) error { calls++; return nil }
	p.frame.context.ancientDialog = true
	if acted, err := p.execute(context.Background(), a); acted || err != nil || calls != 0 {
		t.Fatal("fish clicked newly covered UI")
	}
	p.input.capture = func() (image.Image, error) { return frame.image, nil }
	p.readers.context = func(image.Image) (gameContext, error) { return p.frame.context, nil }
	jobs := make([]chan analysisJob, analysisCount)
	for kind := range jobs {
		jobs[kind] = make(chan analysisJob, 1)
	}
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	// Complete the UI-context transition through the actual shared capture path.
	frame.context = p.frame.context
	frame.context.ancientDialog = false
	p.readers.context = func(image.Image) (gameContext, error) { return frame.context, nil }
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if p.fishTarget == nil || len(jobs[fishAnalysis]) != 0 {
		t.Fatal("known fish was lost or scanned again after modal closed")
	}
	p.controls.toggle()
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if acted, err := p.execute(context.Background(), a); acted || err != nil {
		t.Fatal("F8 replayed the previous fish action")
	}
}

func TestFishCollectionWhileWaitingForSaveFile(t *testing.T) {
	frame := testPipelineFrame()
	frame.context.heroes = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clicked := make(chan struct{}, 1)
	var visible atomic.Bool
	visible.Store(true)
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }, click: func(image.Point) error { visible.Store(false); clicked <- struct{}{}; return nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return frame.context, nil }, window: func() string { return "game" }, fish: func(image.Image) (image.Point, bool, error) { return image.Pt(20, 20), visible.Load(), nil },
	}, pipelineOptions{fishInterval: time.Millisecond, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout = frame, frame.layout
	p.export = saveExporter{requested: true, active: true, step: exportReadFile, window: "game", deadline: time.Now().Add(time.Minute)}
	done := make(chan error, 1)
	go func() { done <- p.run(ctx) }()
	select {
	case <-clicked:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("waiting for export file blocked fish")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p.ancient.plan != nil {
		t.Fatal("missing export installed a spending plan")
	}
}

func TestAncientScrollsContinueDuringSlowFishScan(t *testing.T) {
	t.Run("wheel", func(t *testing.T) { testAncientScrollWithSlowFish(t, false) })
	t.Run("wheel-to-arrow", func(t *testing.T) { testAncientScrollWithSlowFish(t, true) })
}

func testAncientScrollWithSlowFish(t *testing.T, stalledWheel bool) {
	t.Helper()
	original := loadTestImage(t, "testdata/ascension-ancients.png")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, scrolled := make(chan struct{}), make(chan bool, 3)
	var position atomic.Int32
	frame := testPipelineFrame()
	frame.image = original
	frame.context = gameContext{known: true, ancients: true, bounds: frame.image.Bounds()}
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture: func() (image.Image, error) { return original, nil },
		move:    func(image.Point) error { return nil },
		scroll: func(point image.Point, direction int) error {
			if direction != 1 {
				t.Fatal("wheel direction", direction)
			}
			if !stalledWheel {
				position.Add(1)
			}
			scrolled <- false
			return nil
		},
		click: func(point image.Point) error {
			if point != image.Pt(1172, 1416) {
				t.Fatal("arrow recovery clicked outside its native control", point)
			}
			position.Add(1)
			scrolled <- true
			return nil
		},
	}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return frame.context, nil },
		fish: func(image.Image) (image.Point, bool, error) {
			close(started)
			<-ctx.Done()
			return image.Point{}, false, ctx.Err()
		},
		ancients: func(ctx context.Context, f gameFrame) (ancientObservation, error) {
			return ancientObservation{frame: f, namesOnly: true, anchors: []ancientNameAnchor{{"Argaiv", 900 - int(position.Load())*50}}}, nil
		},
	}, pipelineOptions{fishInterval: time.Second})
	p.frame, p.layout = frame, frame.layout
	p.ancient = ancientPlanner{active: true, selected: -1,
		plan: &ancientPlan{Plan: ancientcalc.Plan{Owned: []ancientcalc.Level{{Name: "Argaiv"}}, Rows: []ancientcalc.Purchase{{Name: "Atman"}}}}, done: map[int]bool{}}
	// A previous negative observation has expired, and the new scan cannot finish.
	old := frame
	old.at = time.Now().Add(-time.Minute)
	p.state[fishAnalysis] = observation{frame: old}
	done := make(chan error, 1)
	go func() { done <- p.run(ctx) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("fish worker never started")
	}
	inputs := 2
	if stalledWheel {
		inputs = 3
	}
	for i := range inputs {
		select {
		case arrow := <-scrolled:
			if arrow != (stalledWheel && i == 2) {
				t.Fatal("wheel-to-arrow recovery did not preserve shared input ordering", i, arrow)
			}
		case <-time.After(2 * time.Second):
			cancel()
			<-done
			t.Fatal("successful Ancient scroll waited for SIFT")
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if p.state[fishAnalysis].frame.id != old.id {
		t.Fatal("ordinary scrolling invalidated the fish observation")
	}
}

func TestFishCacheSurvivesCaptureTransitions(t *testing.T) {
	frame := testPipelineFrame()
	frame.context.window = "game"
	current := frame.context
	point := image.Pt(20, 20)
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return current, nil },
		window:  func() string { return current.window },
	}, pipelineOptions{fishInterval: time.Millisecond})
	p.frame, p.layout, p.fishTarget = frame, frame.layout, &point
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	for _, tab := range []string{"Ancients", "quantity", "settings", "Mercenaries", "Heroes"} {
		current = frame.context
		switch tab {
		case "Ancients":
			current.ancients = true
		case "quantity":
			current.ancientDialog = true
		case "settings":
			current.saveMenu = true
		case "Mercenaries":
			current.mercenaries = true
		case "Heroes":
			current.heroes = true
		}
		if err := p.capture(context.Background(), time.Now(), jobs); err != nil {
			t.Fatal(err)
		}
		if p.fishTarget == nil || *p.fishTarget != point || len(jobs[fishAnalysis]) != 0 {
			t.Fatalf("%s lost the cached fish or scheduled redundant SIFT", tab)
		}
	}
	current.window = "different game window"
	if err := p.capture(context.Background(), time.Now(), jobs); err != nil {
		t.Fatal(err)
	}
	if p.fishTarget != nil {
		t.Fatal("cached coordinates crossed game windows")
	}
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: frame, found: true, point: point}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if p.fishTarget != nil {
		t.Fatal("late scan restored coordinates from another window")
	}
}
