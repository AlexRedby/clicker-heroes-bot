package bot

import (
	"context"
	"image"
	"path/filepath"
	"testing"
	"time"
)

// Exercise the public progression bundle through startup/export and back into
// ordinary workers, rather than enabling isolated feature flags in the test.
func TestProgressionCycleResumesAllWorkers(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	screen := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}
	options, err := configureRun(pipelineOptions{
		progression: true, mercenaries: true, fishInterval: time.Second,
		gildInterval: 5 * time.Minute, ascensionStall: 3 * time.Minute, ascensionMinGain: .25,
		export: &saveExportOptions{dir: t.TempDir(), planOutput: filepath.Join(t.TempDir(), "plan.json")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.heroes || !options.skills || !options.autoClickers || !options.gilds || !options.ascension {
		t.Fatal("progression bundle omitted a required part of the cycle")
	}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return c, nil },
		autoClickers: func(context.Context, gameFrame) (autoClickerPool, error) {
			return autoClickerPool{known: true, total: 3}, nil // Already assigned; ordinary footer click is still valid.
		},
		skills: func(context.Context, image.Image) ([9]skillState, error) { return skillsReady(1), nil },
		progression: func(context.Context, image.Image, [9]skillState, bool) (progressionState, error) {
			return progressionState{Known: true, Enabled: true, Zone: 1}, nil
		},
	}, options)
	p.frame = gameFrame{id: 4, layout: 1, at: now, image: screen, context: c}
	p.layout = 1
	p.beginStartup()
	// A completed bounded sweep still owes the footer pass and mode handoff.
	if err := p.accept(ctx, observation{kind: heroAnalysis, startup: startupHeroes, frame: p.frame,
		hero: heroObservation{frame: p.frame, startup: true, startupComplete: true, passiveReady: true}}, now); err != nil {
		t.Fatal(err)
	}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	take := func(kind analysisKind) analysisJob {
		t.Helper()
		select {
		case job := <-jobs[kind]:
			return job
		default:
			t.Fatalf("worker %d not scheduled (startup=%d, export=%t, ancient=%t)", kind, p.startup, p.export.requested, p.ancient.active)
			return analysisJob{}
		}
	}
	capture := func() {
		t.Helper()
		now = now.Add(time.Second)
		p.nextCapture = time.Time{}
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
	}
	accept := func(out observation) {
		t.Helper()
		if err := p.accept(ctx, out, now); err != nil {
			t.Fatal(err)
		}
	}
	capture()
	accept(p.analyze(ctx, autoClickerAnalysis, take(autoClickerAnalysis)))
	out := p.analyze(ctx, heroAnalysis, take(heroAnalysis))
	if out.err != nil || !out.found {
		t.Fatal("native startup footer not read", out.err)
	}
	accept(out)
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHeroUpgrades {
		t.Fatal("startup footer did not run", a.kind, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	capture()
	accept(p.analyze(ctx, progressionAnalysis, take(progressionAnalysis)))
	p.plan(now)
	if p.startup != noStartup || !p.export.requested || p.controls.isPaused() {
		t.Fatal("completed setup blocked fresh export")
	}
	p.plan(now)
	if _, ok := p.queue[handleExport]; !ok {
		t.Fatal("fresh export not started")
	}
	// Existing export UI tests cover Explorer/menu transitions. Complete this
	// owned worker with a modeled no-purchase plan; no private save is required.
	p.export.step, p.export.waiting = exportReadFile, false
	p.export.deadline = now.Add(30 * time.Second)
	capture()
	exported := take(exportAnalysis)
	accept(observation{kind: exportAnalysis, frame: exported.frame, export: exportResult{plan: &ancientPlan{}}})
	p.plan(now)
	if p.export.requested || p.export.active || !p.ancient.finished || p.controls.isPaused() {
		t.Fatal("empty fresh plan blocked ordinary automation")
	}
	if _, ok := p.queue[handleAncient]; ok {
		t.Fatal("empty plan opened Ancients")
	}
	p.plan(now)
	if p.gild.nextCheck.IsZero() {
		t.Fatal("earned-gift checker remained blocked")
	}
	p.nextUpgrades = now.Add(time.Minute)
	p.nextClickerRead = time.Time{}
	capture()
	for _, kind := range []analysisKind{fishAnalysis, skillAnalysis, heroAnalysis, mercenaryAnalysis, autoClickerAnalysis} {
		job := take(kind)
		if job.frame.id != p.frame.id || job.frame.image != p.frame.image || job.startup != noStartup {
			t.Fatal("ordinary worker used a stale/startup frame", kind)
		}
		if kind == skillAnalysis {
			accept(p.analyze(ctx, kind, job))
		}
	}
	mode := take(progressionAnalysis)
	if mode.modeOnly || mode.frame.id != p.frame.id || mode.skills != skillsReady(1) {
		t.Fatal("ordinary progression did not reuse the skill observation")
	}
	accept(p.analyze(ctx, progressionAnalysis, mode))
	p.plan(now)
	if _, ok := p.queue[castSkill]; !ok {
		t.Fatal("ordinary skill input remained blocked")
	}
	// A subsequent observed combat wall can still schedule a reset assessment.
	p.ascension = ascensionPlanner{highestZone: 5, wallZone: 5, fullCombatFailed: true, lastObservation: now.Add(time.Second), lastProgress: now}
	capture()
	if job := take(ascensionAnalysis); !job.economy {
		t.Fatal("combat wall did not schedule an Ascension reward read")
	}
}
