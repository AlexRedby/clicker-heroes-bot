package bot

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestPipelineRelicJunkSalvageAndResumeAscension(t *testing.T) {
	current := loadTestImage(t, "../../testdata/ascension-junk.png")
	heroes := loadTestImage(t, "../../testdata/hero-panel-max.png")
	confirm := loadTestImage(t, "../../testdata/ascension-confirm.png")
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return current, nil }}, pipelineReaders{
		context: recognizedGame, window: func() string { return "game" },
		ascension: func(ctx context.Context, f gameFrame) (ascensionObservation, error) {
			if f.context.relicJunk {
				return readAscensionObservation(ctx, f)
			}
			return ascensionObservation{frame: f, zone: 549, souls: 20, confirm: f.context.ascension, no: f.context.ascension}, nil
		},
	}, pipelineOptions{ascension: true, fishInterval: time.Hour})
	now := time.Now()
	p.frame.context = gameContext{bounds: current.Bounds(), window: "game"}
	p.ascension = ascensionPlanner{active: true, step: openAscension, minimumReward: 10, deadline: now.Add(20 * time.Second)}
	point := image.Pt(2000, 300)
	p.fishTarget = &point
	jobs := mercenaryRecoveryJobs()
	capture := func() {
		t.Helper()
		if err := p.capture(context.Background(), now, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[ascensionAnalysis]:
			if err := p.accept(context.Background(), p.analyze(context.Background(), ascensionAnalysis, job), now); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	next := func(step ascensionStep) gameAction {
		t.Helper()
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != handleAscension || a.ascension != step {
			t.Fatalf("want Ascension step %d: got kind=%d step=%d ok=%t; planner=%+v context=%+v", step, a.kind, a.ascension, ok, p.ascension, p.frame.context)
		}
		return a
	}
	capture()
	if !p.frame.context.relicJunk || p.frame.context.ascension || p.fishContext(p.frame.context) {
		t.Fatalf("wrong native junk context: %+v", p.frame.context)
	}
	select {
	case <-jobs[fishAnalysis]:
		t.Fatal("covered cached fish was scanned")
	default:
	}
	a := next(salvageAscensionJunk)
	if absDiff(a.point.X, 1040) > 2 || absDiff(a.point.Y, 870) > 3 {
		t.Fatalf("wrong Junk Pile Yes: %v", a.point)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	now = now.Add(time.Second)
	capture()
	p.plan(now)
	if a, ok := p.nextAction(now); ok {
		t.Fatalf("unchanged Junk Pile prompt repeated input: %+v", a)
	}
	if p.fishTarget == nil {
		t.Fatal("salvage discarded covered fish")
	}
	// Model the native client returning to the HUD after salvage. The client may
	// also directly show normal Ascension; the planner tests cover that route.
	current = heroes
	now = now.Add(time.Second)
	capture()
	p.plan(now)
	fish, ok := p.nextAction(now)
	if !ok || fish.kind != collectFish {
		t.Fatalf("uncovered fish lost priority: %+v ok=%t", fish, ok)
	}
	p.actionCompleted(actionResult{action: fish, acted: true}, now)
	now = now.Add(time.Second)
	capture()
	a = next(openAscension)
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	current = confirm
	now = now.Add(time.Second)
	capture()
	a = next(confirmAscension)
	if a.frame.context.relicJunk {
		t.Fatal("junk prompt reused as Hero Souls confirmation")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if p.ascension.step != waitAscensionReset {
		t.Fatal("normal reset acknowledgement lost")
	}
}

func TestPipelineRelicJunkOrphanNoBeforeStartup(t *testing.T) {
	current := loadTestImage(t, "../../testdata/ascension-junk.png")
	heroes := loadTestImage(t, "../../testdata/hero-panel-max.png")
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return current, nil }}, pipelineReaders{
		context: recognizedGame, window: func() string { return "game" }, ascension: readAscensionObservation,
	}, pipelineOptions{heroes: true, ascension: true, fishInterval: time.Hour})
	now := time.Now()
	jobs := mercenaryRecoveryJobs()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-jobs[ascensionAnalysis]:
		if err := p.accept(context.Background(), p.analyze(context.Background(), ascensionAnalysis, job), now); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("startup suppressed orphan dialog")
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleAscension || a.ascension != cancelAscension || absDiff(a.point.X, 1520) > 2 {
		t.Fatalf("orphan must use verified No: %+v ok=%t", a, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	current = heroes
	now = now.Add(time.Second)
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	if p.ascension.active || p.startupCheck || p.startup != startupHeroes {
		t.Fatalf("cancel closure did not resume startup: ascension=%+v startup=%v check=%t", p.ascension, p.startup, p.startupCheck)
	}
}
