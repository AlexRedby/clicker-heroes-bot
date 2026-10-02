package main

import (
	"bytes"
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

func TestFishDefersObscuredAncientFailureAndKeepsDeadline(t *testing.T) {
	now := time.Now()
	frame := testPipelineFrame()
	frame.context.ancients = true
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{fish: func(image.Image) (image.Point, bool, error) { return image.Point{}, false, nil }}, pipelineOptions{fishInterval: time.Second})
	p.frame, p.layout = frame, frame.layout
	p.ancient = ancientPlanner{active: true, pending: &gameAction{frame: gameFrame{}}, deadline: now.Add(20 * time.Second)}
	failure := observation{kind: ancientAnalysis, frame: frame, err: errors.New("scrollbar obscured")}
	if err := p.accept(context.Background(), failure, now); err != nil {
		t.Fatal(err)
	}
	if p.ancient.blocked || p.deferred[ancientAnalysis].frame.id == 0 {
		t.Fatal("unchecked OCR stopped purchases")
	}
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: frame, found: true, point: image.Pt(20, 20)}, now); err != nil {
		t.Fatal(err)
	}
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("pending purchase blocked fish")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	p.plan(now.Add(25 * time.Second))
	if p.ancient.blocked || p.controls.isPaused() {
		t.Fatal("fish wait expired purchase confirmation")
	}
	p.frame.id++
	p.frame.at = now.Add(25 * time.Second)
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: p.frame}, p.frame.at); err != nil {
		t.Fatal(err)
	}
	if p.ancient.blocked || !p.ancient.deadline.Equal(now.Add(45*time.Second)) || p.deferred[ancientAnalysis].frame.id != 0 {
		t.Fatal("obstructed result replayed or deadline lost")
	}
	p.ancient.pending.ancient.step = scrollAncients
	p.ancient.pending.point = image.Pt(50, 10)
	confirmed := ancientObservation{frame: p.frame, hasThumb: true, thumb: image.Pt(50, 70)}
	if err := p.accept(context.Background(), observation{kind: ancientAnalysis, frame: p.frame, ancient: confirmed}, p.frame.at); err != nil {
		t.Fatal(err)
	}
	if p.ancient.pending != nil {
		t.Fatal("fresh scroll confirmation did not resume after fish")
	}
	// Genuine non-fish errors still stop purchases after a checked frame.
	failure.frame = p.frame
	if err := p.accept(context.Background(), failure, p.frame.at); err != nil {
		t.Fatal(err)
	}
	if !p.ancient.blocked || !p.controls.isPaused() {
		t.Fatal("genuine purchase error was hidden")
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

func TestFishBeforeQuantityOrSettingsOpenCannotClick(t *testing.T) {
	for _, kind := range []actionKind{handleAncient, handleExport} {
		t.Run(map[actionKind]string{handleAncient: "quantity", handleExport: "settings"}[kind], func(t *testing.T) {
			now := time.Now()
			frame := testPipelineFrame()
			frame.context.ancients = true
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
			p.frame, p.layout = frame, frame.layout
			action := gameAction{kind: kind, frame: frame, ancient: ancientCommand{step: openAncientQuantity}, export: &exportCommand{step: exportOpenMenu}}
			p.actionCompleted(actionResult{action: action, acted: true}, now)
			// The popup has not been captured yet; the old main-screen SIFT result must be discarded.
			if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: frame, found: true, point: image.Pt(20, 20)}, now); err != nil {
				t.Fatal(err)
			}
			if p.state[fishAnalysis].found {
				t.Fatal("pre-popup fish survived its input barrier")
			}
			if a, ok := p.nextAction(now); ok && a.kind == collectFish {
				t.Fatal("old fish clicked the new popup")
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
	p.fish.lastClickAt = now.Add(-6 * time.Second)
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
	if !p.fishObstructedAt.IsZero() || len(jobs[fishAnalysis]) != 1 {
		t.Fatal("fish worker did not resume after modal closed")
	}
	p.deferred[ancientAnalysis] = observation{frame: p.frame, err: errors.New("old purchase error")}
	p.controls.toggle()
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.deferred[ancientAnalysis].frame.id != 0 {
		t.Fatal("F8 retained unchecked purchase result")
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
