package main

import (
	"context"
	"image"
	"os"
	"testing"
	"time"
)

func TestPipelineMercenaryDeathKeepsLivingPlan(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	roster := loadTestImage(t, "testdata/mercenary-dead.png")
	dialog := loadTestImage(t, "testdata/mercenary-quests.png")
	current := roster
	reads := 0
	readers := pipelineReaders{
		context: recognizedGame,
		window:  func() string { return "game" },
		mercenaries: func(ctx context.Context, frame gameFrame) (mercenaryObservation, error) {
			if frame.context.questDialog {
				return mercenaryObservation{frame: frame, readable: true, selected: -1,
					quests: []mercenaryQuest{{reward: "gold", duration: time.Hour, point: image.Pt(1000, 390)}}}, nil
			}
			reads++
			return readMercenaryObservation(ctx, frame)
		},
	}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) {
		return current, nil
	}}, readers, pipelineOptions{mercenaries: true, fishInterval: time.Hour})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	capture := func() {
		t.Helper()
		if err := p.capture(context.Background(), now, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[mercenaryAnalysis]:
			if err := p.accept(context.Background(), p.analyze(context.Background(), mercenaryAnalysis, job), now); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	capture()
	if !p.frame.context.mercenaries || p.frame.context.questDialog || !p.mercenary.latest.readable {
		t.Fatalf("real death roster is not actionable: context=%+v observation=%+v", p.frame.context, p.mercenary.latest)
	}
	o := p.mercenary.latest
	if len(o.collect) != 2 || len(o.start) != 0 || len(o.running) != 1 {
		t.Fatalf("dead Emma must not become a running quest or hide living rows: %+v", o)
	}
	living := append([]image.Point(nil), o.collect...)
	p.plan(now.Add(time.Millisecond))
	first, ok := p.nextAction(now.Add(time.Millisecond))
	if !ok || first.kind != handleMercenary || first.mercenary.step != claimAndOpenMercenaryQuest || first.point != living[0] {
		t.Fatalf("wanted living Collect at %v, got %+v, ok=%t", living[0], first, ok)
	}
	p.actionCompleted(actionResult{action: first, acted: true}, now)
	current = dialog
	now = now.Add(2 * time.Second)
	capture()
	p.plan(now)
	quest, ok := p.nextAction(now)
	if !ok || quest.kind != handleMercenary || quest.mercenary.step != selectMercenaryQuest {
		t.Fatalf("living mercenary did not reach quest dispatch: %+v ok=%t", quest, ok)
	}
	p.actionCompleted(actionResult{action: quest, acted: true}, now)
	current = roster
	now = now.Add(time.Second)
	capture()
	p.plan(now)
	second, ok := p.nextAction(now)
	if !ok || second.kind != handleMercenary || second.mercenary.step != claimAndOpenMercenaryQuest || second.point != living[1] {
		t.Fatalf("death interrupted remaining living Collect at %v: %+v ok=%t", living[1], second, ok)
	}
	if reads != 1 {
		t.Fatalf("dead card caused repeated roster OCR: reads=%d", reads)
	}
}
