package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func firstAscensionPipeline(now time.Time) *gamePipeline {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{ascension: true, ascensionStall: 3 * time.Minute, ascensionMinGain: .25, ascensionCapital: math.Inf(-1)})
	p.generation, p.layout = 1, 1
	p.frame = testPipelineFrame()
	p.frame.id, p.frame.generation, p.frame.at = 10, p.generation, now
	p.frame.context.heroes = true
	p.ascension.firstRun = true
	p.state[progressionAnalysis] = observation{frame: p.frame, progression: progressionState{Known: true, Enabled: true, Zone: 131}}
	p.ascension.observeProgress(p.state[progressionAnalysis].progression, 0, now, false)
	return p
}

func TestAscensionFirstRunSaveEvidence(t *testing.T) {
	first := ancientcalc.TranscensionPreview{SaveHash: "fresh", Transcendent: true}
	later := first
	later.Ascensions = 1
	before := first
	before.Transcendent = false
	for _, tc := range []struct {
		name string
		out  exportResult
		want bool
	}{
		{"snapshot", exportResult{transcension: &transcension.Snapshot{State: ancientcalc.TranscensionState{SaveHash: "fresh", Transcendent: true}}}, true},
		{"preview", exportResult{prestige: &first}, true},
		{"plan", exportResult{plan: &ancientPlan{Transcension: &first}}, true},
		{"later", exportResult{prestige: &later}, false},
		{"before Transcension", exportResult{prestige: &before}, false},
		{"unknown", exportResult{}, false},
		{"unbound preview", exportResult{prestige: &ancientcalc.TranscensionPreview{Transcendent: true}}, false},
		{"newer snapshot wins", exportResult{transcension: &transcension.Snapshot{State: ancientcalc.TranscensionState{SaveHash: "new", Transcendent: true, AscensionsThisTranscension: 1}}, prestige: &first}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ascensionPlanner{firstRun: true}
			p.observeSave(tc.out)
			if p.firstRun != tc.want {
				t.Fatalf("firstRun=%t, want %t", p.firstRun, tc.want)
			}
		})
	}
}

func TestAscensionFirstRunCandidate(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name   string
		change func(*gamePipeline)
		want   bool
	}{
		{"zone 131 while progressing", func(*gamePipeline) {}, true},
		{"zone 130 boss not yet beaten", func(p *gamePipeline) { p.state[progressionAnalysis].progression.Zone = 130 }, false},
		{"zone 149", func(p *gamePipeline) { p.state[progressionAnalysis].progression.Zone = 149 }, true},
		{"zone 129", func(p *gamePipeline) { p.state[progressionAnalysis].progression.Zone = 129 }, false},
		{"later or unknown run", func(p *gamePipeline) { p.ascension.firstRun = false }, false},
		{"unknown progression", func(p *gamePipeline) { p.state[progressionAnalysis].progression.Known = false }, false},
		{"missing frame", func(p *gamePipeline) { p.state[progressionAnalysis].frame.id = 0 }, false},
		{"future frame", func(p *gamePipeline) { p.state[progressionAnalysis].frame.id = p.frame.id + 1 }, false},
		{"barrier", func(p *gamePipeline) { p.barriers[progressionAnalysis] = p.frame.id + 1 }, false},
		{"stale", func(p *gamePipeline) { p.state[progressionAnalysis].frame.at = now.Add(-11 * time.Second) }, false},
		{"future time", func(p *gamePipeline) { p.state[progressionAnalysis].frame.at = now.Add(time.Second) }, false},
		{"old generation", func(p *gamePipeline) { p.state[progressionAnalysis].frame.generation-- }, false},
		{"old layout", func(p *gamePipeline) { p.state[progressionAnalysis].frame.layout-- }, false},
		{"old context", func(p *gamePipeline) { p.state[progressionAnalysis].frame.context.heroes = false }, false},
		{"unreadable newest progression", func(p *gamePipeline) { p.ascension.invalidate() }, false},
		{"retry backoff", func(p *gamePipeline) { p.ascension.nextCheck = now.Add(time.Minute) }, false},
		{"active reset", func(p *gamePipeline) { p.ascension.active = true }, false},
		{"startup save not acquired", func(p *gamePipeline) { p.startupCheck = true }, false},
		{"incomplete hero setup", func(p *gamePipeline) { p.startup = startupHeroes }, true},
		{"incomplete upgrade footer", func(p *gamePipeline) { p.startup = startupUpgrades }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := firstAscensionPipeline(now)
			tc.change(p)
			if got := p.ascensionCandidate(now); got != tc.want {
				t.Fatalf("candidate=%t, want %t", got, tc.want)
			}
		})
	}
	p := firstAscensionPipeline(now)
	p.ascension.firstRun = false
	p.ascension.wallZone, p.ascension.fullCombatFailed = 130, true
	if !p.ascensionCandidate(now) {
		t.Fatal("ordinary wall policy was lost")
	}
}

func TestAscensionFirstRunKeepsCompletedPreflight(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.ascension.relicsChecked = true
	for zone := 131; zone <= 134; zone++ {
		p.ascension.observeProgress(progressionState{Known: true, Enabled: true, Zone: zone}, 0, now, false)
		if !p.ascension.firstRun || !p.ascension.relicsChecked {
			t.Fatal("forward first-run progression revoked its completed preflight")
		}
	}
	p.ascension.observeProgress(progressionState{Known: true, Zone: 1}, 0, now, false)
	if p.ascension.firstRun || p.ascension.relicsChecked {
		t.Fatal("zone reset retained first-run or relic evidence")
	}
}

func TestAscensionFirstRunPauseAndResetEvidence(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.ascension.relicsChecked = true
	p.reset(p.generation + 1)
	if !p.ascension.firstRun || p.ascension.relicsChecked || p.ascensionCandidate(now) {
		t.Fatal("F8 must retain run identity but revoke input and progression evidence")
	}
	p.frame.id++
	p.frame.generation, p.frame.layout = p.generation, p.layout
	p.state[progressionAnalysis] = observation{frame: p.frame, progression: progressionState{Known: true, Enabled: true, Zone: 131}}
	p.ascension.observeProgress(p.state[progressionAnalysis].progression, 0, now, false)
	if !p.ascensionCandidate(now) {
		t.Fatal("fresh post-F8 progression did not resume first-run policy")
	}
	p.ascension.active, p.ascension.step = true, waitAscensionReset
	if !p.ascension.observe(ascensionObservation{frame: p.frame, zone: 1}, nil, now) || p.ascension.firstRun {
		t.Fatal("confirmed Ascension retained first-run identity")
	}
	p.ascension.firstRun = true
	p.ascension.interrupt()
	if p.ascension.firstRun {
		t.Fatal("interrupted window assessment retained first-run identity")
	}
}

func TestAscensionFirstRunSharedAnalysisAndInputGate(t *testing.T) {
	for _, positive := range []bool{false, true} {
		t.Run(map[bool]string{false: "zero reward", true: "positive reward"}[positive], func(t *testing.T) {
			now := time.Now()
			screen := loadTestImage(t, "../../testdata/hero-panel-max.png")
			c := gameContext{known: true, heroes: true, window: "game", bounds: screen.Bounds()}
			p := firstAscensionPipeline(now)
			p.controls = &pauseControl{generation: p.generation}
			p.input.capture = func() (image.Image, error) { return screen, nil }
			p.readers.context = func(image.Image) (gameContext, error) { return c, nil }
			p.readers.window = func() string { return "game" }
			p.options.progression = true
			p.frame.image, p.frame.context = screen, c
			p.state[progressionAnalysis].frame = p.frame
			jobs := mercenaryRecoveryJobs()
			if err := p.capture(context.Background(), now, jobs); err != nil {
				t.Fatal(err)
			}
			var job analysisJob
			select {
			case job = <-jobs[ascensionAnalysis]:
				if !job.economy {
					t.Fatal("first-run candidate did not schedule soul analysis")
				}
			default:
				t.Fatal("first-run candidate still waits for a boss wall")
			}
			p.queue[buyHero] = gameAction{kind: buyHero, frame: p.frame}
			if !p.planAscension(now) || len(p.queue) != 0 {
				t.Fatal("pending candidate read did not arbitrate gameplay inputs")
			}
			reward := math.Inf(-1)
			if positive {
				reward = math.Log10(390700)
			}
			out := observation{kind: ascensionAnalysis, frame: job.frame, ascension: ascensionObservation{frame: job.frame, economy: true, bank: math.Inf(-1), souls: reward}}
			if err := p.accept(context.Background(), out, now); err != nil {
				t.Fatal(err)
			}
			p.planAscension(now)
			a, ok := p.queue[handleAscension]
			if ok != positive || ok && a.ascension != openAscension {
				t.Fatalf("first-run input with positive=%t: %+v, queued=%t", positive, a, ok)
			}
		})
	}
}

func TestAscensionFirstRunManualResetDuringPauseRevokesEarlyReset(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.reset(p.generation + 1)
	p.ascension.observeProgress(progressionState{Known: true, Enabled: true, Zone: 1}, 0, now, false)
	if p.ascension.firstRun {
		t.Fatal("manual reset during F8 retained the first-run exception")
	}
}

func TestAscensionFirstRunTimeoutRetainsRunIdentity(t *testing.T) {
	for _, navigation := range []bool{false, true} {
		p := firstAscensionPipeline(time.Now())
		now := p.frame.at
		p.ascension.active = true
		p.ascension.deadline = now.Add(-time.Second)
		p.ascension.relicsChecked = true
		if navigation {
			p.frame.context.ascension, p.frame.context.heroes = true, false
			p.planNavigation(now)
		} else {
			p.planAscension(now)
		}
		if !p.ascension.firstRun || p.ascension.highestZone != 131 || p.ascension.active || p.ascension.relicsChecked || !p.ascension.nextCheck.Equal(now.Add(time.Minute)) || p.ascensionCandidate(now) {
			t.Fatalf("timeout lost run identity or kept input permission (navigation=%t): %+v", navigation, p.ascension)
		}
	}
}

func TestAscensionFirstRunOwnedDialogRetriesThenConfirms(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.controls.generation = p.generation
	p.frame.image = loadTestImage(t, "../../testdata/hero-panel-max.png")
	p.frame.context.bounds = p.frame.image.Bounds()
	p.state[progressionAnalysis].frame = p.frame
	p.ascension.relicsChecked = true
	p.options.ascensionCapital = 99
	p.ascension.latest = ascensionObservation{frame: p.frame, economy: true, bank: math.Inf(-1), souls: math.Log10(390700)}
	p.startup = startupHeroes
	p.plan(now)
	if p.startup != noStartup {
		t.Fatal("first reset waited for startup completion")
	}
	open, ok := p.nextAction(now)
	if !ok || open.kind != handleAscension || open.ascension != openAscension {
		t.Fatal("first Ascension did not open for positive reward", open, ok)
	}
	clicks := 0
	p.input.click = func(image.Point) error { clicks++; return nil }
	if acted, err := p.execute(context.Background(), open); err != nil || !acted {
		t.Fatal("opening input failed", acted, err)
	}
	p.actionCompleted(actionResult{action: open, acted: true}, now)
	screen := loadTestImage(t, "../../testdata/ascension-confirm.png")
	p.input.capture = func() (image.Image, error) { return screen, nil }
	p.readers.context = func(im image.Image) (gameContext, error) {
		c, err := recognizedGame(im)
		c.window = p.frame.context.window
		return c, err
	}
	reads := 0
	p.readers.ascension = func(ctx context.Context, f gameFrame) (ascensionObservation, error) {
		reads++
		if reads == 1 {
			return ascensionObservation{frame: f}, errors.New("temporary reward OCR failure")
		}
		return readAscensionObservation(ctx, f)
	}
	jobs := mercenaryRecoveryJobs()
	for attempt := 1; attempt <= 2; attempt++ {
		at := now.Add(time.Duration(attempt) * time.Second)
		if err := p.capture(context.Background(), at, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[ascensionAnalysis]:
			if err := p.accept(context.Background(), p.analyze(context.Background(), ascensionAnalysis, job), at); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("owned dialog did not request a fresh reward frame", attempt)
		}
		p.plan(at)
		a, ready := p.nextAction(at)
		if attempt == 1 {
			if ready || !p.ascension.active || p.ascension.step != openAscension || p.ascension.latest.frame.id != 0 || !p.ascension.deadline.Equal(now.Add(20*time.Second)) {
				t.Fatal("temporary OCR failure cancelled or confirmed the dialog", a, ready)
			}
			continue
		}
		if !ready || a.kind != handleAscension || a.ascension != confirmAscension {
			t.Fatal("fresh recognized reward did not confirm the owned dialog", a, ready)
		}
		if acted, err := p.execute(context.Background(), a); err != nil || !acted {
			t.Fatal("confirmation input failed", acted, err)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, at)
	}
	if clicks != 2 || p.ascension.step != waitAscensionReset || !p.ascension.active {
		t.Fatal("confirmation not submitted exactly once", clicks, p.ascension.step)
	}
	screen = loadTestImage(t, "../../testdata/hero-post-transcension-start.png")
	at := now.Add(3 * time.Second)
	if err := p.capture(context.Background(), at, jobs); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-jobs[ascensionAnalysis]:
		if err := p.accept(context.Background(), p.analyze(context.Background(), ascensionAnalysis, job), at); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("post-reset frame was not analyzed")
	}
	if p.ascension.active || p.ascension.firstRun {
		t.Fatal("confirmed first Ascension retained ownership or first-run identity")
	}
}

func TestAscensionUnreadableDialogRevokesQueuedConfirmationAndTimesOut(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.frame.image = loadTestImage(t, "../../testdata/ascension-confirm.png")
	c, err := recognizedGame(p.frame.image)
	if err != nil {
		t.Fatal(err)
	}
	p.frame.context = c
	p.ascension.sent(openAscension, p.frame.id-1, now)
	p.ascension.step = confirmAscension
	p.queue[handleAscension] = gameAction{kind: handleAscension, frame: p.frame, ascension: confirmAscension}
	deadline := p.ascension.deadline
	for i := 1; i <= 19; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		p.frame.id++
		p.frame.at = at
		p.ascension.observe(ascensionObservation{frame: p.frame}, errors.New("reward OCR unavailable"), at)
		p.planAscension(at)
		if len(p.queue) != 0 || !p.ascension.active || !p.ascension.deadline.Equal(deadline) {
			t.Fatal("failed read retained old confirmation or renewed deadline", i)
		}
	}
	p.planAscension(deadline.Add(time.Millisecond))
	if a, ok := p.queue[navigateGame]; !ok || a.navigation != navigationAscension || p.ascension.active {
		t.Fatal("persistent OCR failure did not cancel through bounded navigation", a, ok)
	}
}

func TestFirstAscensionPreemptsIncompleteStartup(t *testing.T) {
	for _, phase := range []startupPhase{startupHeroes, startupUpgrades, startupProgression} {
		t.Run(fmt.Sprint(phase), func(t *testing.T) {
			now := time.Now()
			p := firstAscensionPipeline(now)
			p.frame.image = loadTestImage(t, "../../testdata/hero-panel-max.png")
			p.frame.context.bounds = p.frame.image.Bounds()
			p.state[progressionAnalysis].frame = p.frame
			p.startup = phase
			p.ascension.relicsChecked = true
			p.ascension.latest = ascensionObservation{frame: p.frame, economy: true, bank: math.Inf(-1), souls: 5}
			p.progression.wantAction = true
			p.ancient.plan = &ancientPlan{}
			for _, kind := range []actionKind{buyHero, buyHeroUpgrades, scrollHeroes, castSkill, handleAncient} {
				p.queue[kind] = gameAction{kind: kind, frame: p.frame}
			}
			p.plan(now)
			if p.startup != noStartup || p.progression.wantAction {
				t.Fatal("unrelated startup/progression work retained priority")
			}
			a, ok := p.nextAction(now)
			if !ok || a.kind != handleAscension || a.ascension != openAscension || len(p.queue) != 0 {
				t.Fatal("startup/Ancient queue prevented immediate Ascension", a, ok, p.queue)
			}
		})
	}
}

func TestFirstAscensionObservesZoneDuringStartup(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.controls.generation = p.generation
	screen := loadTestImage(t, "../../testdata/hero-panel-max.png")
	p.frame.image = screen
	p.frame.context.bounds = screen.Bounds()
	p.ascension.highestZone = 0
	p.state[progressionAnalysis] = observation{}
	p.startup = startupHeroes
	p.options.progression = true
	p.input.capture = func() (image.Image, error) { return screen, nil }
	p.readers.context = func(image.Image) (gameContext, error) { return p.frame.context, nil }
	p.readers.progression = func(_ context.Context, _ image.Image, _ [9]skillState, modeOnly bool) (progressionState, error) {
		if !modeOnly {
			t.Fatal("first run requested unrelated damage OCR")
		}
		return progressionState{Known: true, Enabled: true}, nil
	}
	jobs := mercenaryRecoveryJobs()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-jobs[progressionAnalysis]:
		if !job.zoneOnly || job.frame.id != p.frame.id {
			t.Fatal("startup lost the shared-frame zone read")
		}
		out := p.analyze(context.Background(), progressionAnalysis, job)
		if out.err != nil || out.progression.Zone <= 130 {
			t.Fatal("native zone was not readable during startup", out.progression, out.err)
		}
		if err := p.accept(context.Background(), out, now); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("startup skipped first Ascension zone observation")
	}
	if !p.firstAscensionRequired() || !p.ascensionCandidate(now) {
		t.Fatal("incomplete hero sweep still excludes first Ascension")
	}
}

func TestFirstAscensionWaitsForSubmittedInputAndKeepsEarning(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.frame.image = loadTestImage(t, "../../testdata/hero-panel-max.png")
	p.frame.context.bounds = p.frame.image.Bounds()
	p.state[progressionAnalysis].frame = p.frame
	p.options.progression = true
	p.startup = startupHeroes
	p.hero.pending = &heroAttempt{}
	p.queue[buyHero] = gameAction{kind: buyHero, frame: p.frame}
	p.plan(now)
	if p.startup != startupHeroes || len(p.queue) != 0 {
		t.Fatal("first Ascension replayed unrelated input while a submitted input is pending")
	}
	p.hero.pending = nil
	p.ascension.latest = ascensionObservation{frame: p.frame, economy: true, bank: math.Inf(-1), souls: math.Inf(-1)}
	p.plan(now)
	if _, ok := p.queue[clickMonster]; !ok || !p.ascension.firstRun || p.ascension.active || p.startup != noStartup {
		t.Fatal("unavailable reward froze gold earning or lost the first Ascension goal")
	}
}

func TestFirstAscensionUnlockKeepsNeededFooter(t *testing.T) {
	now := time.Now()
	p := firstAscensionPipeline(now)
	p.frame.image = loadTestImage(t, "../../testdata/hero-post-transcension-start.png")
	p.frame.context.bounds = p.frame.image.Bounds()
	p.state[progressionAnalysis].frame = p.frame
	p.ascension.unlock.ready = true
	p.queue[buyHeroUpgrades] = gameAction{kind: buyHeroUpgrades, frame: p.frame}
	p.queue[castSkill] = gameAction{kind: castSkill, frame: p.frame}
	p.plan(now)
	if _, ok := p.queue[buyHeroUpgrades]; !ok {
		t.Fatal("first Ascension discarded a needed prerequisite upgrade purchase")
	}
	if _, ok := p.queue[castSkill]; ok {
		t.Fatal("unrelated skill input kept priority over the unlock")
	}
}
