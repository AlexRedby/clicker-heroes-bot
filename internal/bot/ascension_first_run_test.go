package bot

import (
	"context"
	"image"
	"math"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func firstAscensionPipeline(now time.Time) *gamePipeline {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{ascension: true, ascensionStall: 3 * time.Minute})
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
		{"startup", func(p *gamePipeline) { p.startupCheck = true }, false},
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
