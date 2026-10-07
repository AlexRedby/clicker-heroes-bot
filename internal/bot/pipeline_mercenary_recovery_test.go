package bot

import (
	"context"
	"image"
	"os"
	"testing"
	"time"
)

// Use native modal pixels with the real context reader and shared-frame scheduling.
func TestPipelineMercenaryRecoveryOrphanDuringStartup(t *testing.T) {
	for _, phase := range []startupPhase{noStartup, startupHeroes, startupUpgrades, startupProgression} {
		t.Run(map[startupPhase]string{noStartup: "initial check", startupHeroes: "heroes", startupUpgrades: "upgrades", startupProgression: "export pending"}[phase], func(t *testing.T) {
			screen := loadTestImage(t, "../../testdata/mercenary-bury.png")
			p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
				context: recognizedGame, window: func() string { return "game" },
				mercenaries: func(_ context.Context, f gameFrame) (mercenaryObservation, error) {
					return mercenaryObservation{frame: f, readable: true, recovery: &mercenaryRecoveryPrompt{method: mercenaryRecoveryBury, yes: image.Pt(1040, 790), no: image.Pt(1520, 790)}}, nil
				},
			}, pipelineOptions{heroes: true, mercenaries: true, fishInterval: time.Hour})
			if phase == startupProgression {
				p.startupCheck = false
				p.export.requested = true
			} else if phase != noStartup {
				p.startupCheck = false
				p.startup = phase
			}
			p.frame.context = gameContext{bounds: screen.Bounds(), window: "game"}
			point := image.Pt(1700, 800)
			p.fishTarget = &point
			jobs := mercenaryRecoveryJobs()
			now := time.Now()
			if err := p.capture(context.Background(), now, jobs); err != nil {
				t.Fatal(err)
			}
			if !p.frame.context.mercenaryDialog || p.fishTarget == nil || gameScreenVisible(p.frame.context) {
				t.Fatalf("covered fish/context: %+v", p.frame.context)
			}
			select {
			case <-jobs[fishAnalysis]:
				t.Fatal("scanned an already known, covered fish")
			default:
			}
			select {
			case job := <-jobs[mercenaryAnalysis]:
				if err := p.accept(context.Background(), p.analyze(context.Background(), mercenaryAnalysis, job), now); err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatal("startup suppressed recovery modal reading")
			}
			p.plan(now)
			a, ok := p.nextAction(now)
			if !ok || a.kind != navigateGame || a.navigation != navigationRecovery || a.point != mercenaryPoint(screen.Bounds(), 594, 548) {
				t.Fatalf("orphan must use No, got %+v ok=%t", a, ok)
			}
			p.actionCompleted(actionResult{action: a, acted: true}, now)
			now = now.Add(time.Second)
			if err := p.capture(context.Background(), now, jobs); err != nil {
				t.Fatal(err)
			}
			p.plan(now)
			if a, ok := p.nextAction(now); !ok || a.kind != navigateGame || a.navigation != navigationRecovery {
				t.Fatalf("missed No did not retry safe navigation: %+v", a)
			}
		})
	}
}

func mercenaryRecoveryJobs() []chan analysisJob {
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	return jobs
}

func TestPipelineMercenaryRecoveryRebuildsShiftedRoster(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	current := loadTestImage(t, "../../testdata/mercenary-dead.png")
	modal := loadTestImage(t, "../../testdata/mercenary-bury.png")
	after := loadTestImage(t, "../../testdata/mercenary-after-bury.png")
	rosterReads := 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return current, nil }}, pipelineReaders{
		context: recognizedGame, window: func() string { return "game" }, mercenaries: func(ctx context.Context, f gameFrame) (mercenaryObservation, error) {
			o, err := readMercenaryObservation(ctx, f)
			if !f.context.mercenaryDialog {
				rosterReads++
				if rosterReads == 1 {
					o.collect = nil
					o.start = nil
					o.top = true
					o.bottom = true
					o.thumbFound = false
				}
			}
			return o, err
		},
	}, pipelineOptions{mercenaries: true, fishInterval: time.Hour})
	now := time.Now()
	jobs := mercenaryRecoveryJobs()
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
	action := func(step mercenaryStep) gameAction {
		t.Helper()
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != handleMercenary || a.mercenary.step != step {
			t.Fatalf("want step %d: %+v ok=%t", step, a, ok)
		}
		return a
	}
	capture()
	open := action(openMercenaryRecovery)
	if open.mercenary.recovery != mercenaryRecoveryBury {
		t.Fatalf("real Emma must be buried: %+v", open)
	}
	p.actionCompleted(actionResult{action: open, acted: true}, now)
	current = modal
	now = now.Add(time.Second)
	capture()
	confirm := action(confirmMercenaryRecovery)
	if absDiff(confirm.point.X, 1040) > 2 || absDiff(confirm.point.Y, 790) > 2 {
		t.Fatalf("wrong native Yes: %+v", confirm)
	}
	p.actionCompleted(actionResult{action: confirm, acted: true}, now)
	now = now.Add(time.Second)
	capture()
	p.plan(now)
	if a, ok := p.nextAction(now); ok {
		t.Fatalf("repeated paid/burial confirmation: %+v", a)
	}
	fish := image.Pt(1700, 800)
	p.fishTarget = &fish
	current = after
	now = now.Add(time.Second)
	capture()
	if rosterReads != 2 || p.mercenary.roster == nil || len(p.mercenary.roster.collect) != 2 {
		t.Fatalf("shifted roster was not re-read: reads=%d roster=%+v", rosterReads, p.mercenary.roster)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish || a.point != fish {
		t.Fatalf("uncovered cached fish lost priority: %+v ok=%t", a, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	now = now.Add(time.Second)
	capture()
	// The native after-burial frame is scrolled down; its new scrollbar plan
	// must take priority over old row positions, without another roster OCR.
	next := action(scrollMercenariesTop)
	if next.frame.context.mercenaryDialog || rosterReads != 2 {
		t.Fatalf("stale recovery state after roster rebuild: %+v reads=%d", next, rosterReads)
	}
}
