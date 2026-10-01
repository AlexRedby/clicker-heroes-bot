package main

import (
	"context"
	"image"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipelineMercenaryPlanReadsRosterOnce(t *testing.T) {
	screen := loadTestImage(t, "testdata/mercenary-collect.png")
	dialog := loadTestImage(t, "testdata/mercenary-quests.png")
	bounds := screen.Bounds()
	mercenaries := gameContext{known: true, mercenaries: true, bounds: bounds, window: "game"}
	questDialog := gameContext{known: true, mercenaries: true, questDialog: true, bounds: bounds, window: "game"}
	heroes := gameContext{known: true, heroes: true, bounds: bounds, window: "game"}
	rows := []image.Point{{700, 400}, {700, 550}, {700, 700}, {700, 850}}
	var current atomic.Value
	current.Store(mercenaries)
	var rosterReads, offerReads atomic.Int32
	readers := pipelineReaders{
		context: func(image.Image) (gameContext, error) { return current.Load().(gameContext), nil },
		window:  func() string { return "game" },
		mercenaries: func(_ context.Context, frame gameFrame) (mercenaryObservation, error) {
			if frame.context.questDialog {
				offerReads.Add(1)
				return mercenaryObservation{frame: frame, readable: true, selected: -1, quests: []mercenaryQuest{{reward: "gold", duration: time.Hour, point: image.Pt(1000, 390)}}}, nil
			}
			rosterReads.Add(1)
			return mercenaryObservation{frame: frame, readable: true, top: true, bottom: true, collect: append([]image.Point(nil), rows...)}, nil
		},
	}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) {
		if current.Load().(gameContext).questDialog {
			return dialog, nil
		}
		return screen, nil
	}}, readers, pipelineOptions{mercenaries: true, fishInterval: time.Hour})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	captureAndAccept := func(at time.Time) {
		t.Helper()
		if err := p.capture(context.Background(), at, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[mercenaryAnalysis]:
			if err := p.accept(context.Background(), p.analyze(context.Background(), mercenaryAnalysis, job), at); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	captureAndAccept(now)
	if rosterReads.Load() != 1 {
		t.Fatalf("initial capture queued %d roster reads, want 1", rosterReads.Load())
	}
	p.mercenary.active, p.mercenary.returnHeroes = true, true
	p.mercenary.topVisited, p.mercenary.bottomVisited = true, true

	for i, row := range rows {
		now = now.Add(time.Second)
		p.state[fishAnalysis] = observation{frame: p.frame, elapsed: time.Millisecond}
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.mercenary.step != claimAndOpenMercenaryQuest || a.point != row {
			t.Fatalf("row %d: wanted Collect+Start at %v, got %+v ok=%t", i, row, a, ok)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, now)

		current.Store(questDialog)
		now = now.Add(time.Second)
		captureAndAccept(now)
		if offerReads.Load() != int32(i+1) {
			t.Fatalf("row %d: offer reads=%d, want %d", i, offerReads.Load(), i+1)
		}
		p.plan(now.Add(time.Millisecond))
		selectAction, ok := p.nextAction(now.Add(time.Millisecond))
		if !ok || selectAction.mercenary.step != selectMercenaryQuest {
			t.Fatalf("row %d: wanted offer selection, got %+v ok=%t", i, selectAction, ok)
		}
		p.actionCompleted(actionResult{action: selectAction, acted: true}, now.Add(time.Millisecond))

		current.Store(mercenaries)
		now = now.Add(time.Second)
		beforeReads := rosterReads.Load()
		captureAndAccept(now)
		if rosterReads.Load() != beforeReads {
			t.Fatalf("row %d: return capture repeated roster OCR", i)
		}
		if p.mercenary.roster == nil || len(p.mercenary.roster.collect) != len(rows)-i-1 {
			t.Fatalf("row %d: saved rows=%v", i, p.mercenary.roster)
		}
		if i == 0 {
			p.actionCompleted(actionResult{action: gameAction{kind: collectFish, point: image.Pt(1, 1)}, acted: true}, now)
			if p.mercenary.roster == nil || len(p.mercenary.roster.collect) != len(rows)-1 {
				t.Fatal("fish invalidation discarded remaining mercenary plan")
			}
			captureAndAccept(now.Add(time.Millisecond))
		}
	}

	p.state[fishAnalysis] = observation{frame: p.frame, elapsed: time.Millisecond}
	p.plan(now.Add(time.Second))
	returnAction, ok := p.nextAction(now.Add(time.Second))
	if !ok || returnAction.mercenary.step != returnToHeroes {
		t.Fatalf("wanted return to Heroes, got %+v ok=%t", returnAction, ok)
	}
	p.actionCompleted(actionResult{action: returnAction, acted: true}, now.Add(time.Second))
	current.Store(heroes)
	captureAndAccept(now.Add(2 * time.Second))
	if p.mercenary.active || p.mercenary.roster != nil {
		t.Fatalf("visit remained active after Heroes transition: active=%t roster=%v", p.mercenary.active, p.mercenary.roster)
	}
	if rosterReads.Load() != 1 || offerReads.Load() != int32(len(rows)) {
		t.Fatalf("final reads: roster=%d offers=%d", rosterReads.Load(), offerReads.Load())
	}
}
