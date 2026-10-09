package bot

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestIncompleteSkillSetupDefersAndReturnsWithoutExport(t *testing.T) {
	now := time.Now()
	f := testPipelineFrame()
	f.at = now
	f.context.heroes = true
	plan := &ancientPlan{}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.frame, p.layout = f, f.layout
	p.beginStartup()
	p.startupExported = true
	p.ancient = ancientPlanner{plan: plan, finished: true}
	sweep := startupSweep{retry: 2, needsLevels: true}
	out := observation{kind: heroAnalysis, startup: startupHeroes, frame: f, hero: heroObservation{frame: f, startup: true, startupComplete: true, passiveReady: true, sweep: sweep}}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupUpgrades || p.nextSkillSetup != now.Add(time.Minute) {
		t.Fatal("unfinished preparation not deferred", p.startup, p.nextSkillSetup)
	}
	p.finishStartupUpgradePass(now)
	p.plan(now)
	if p.startup != noStartup || p.export.requested || p.ancient.plan != plan {
		t.Fatal("unfinished skills held ordinary play or exported again")
	}
	if p.retrySkillSetup(now.Add(59 * time.Second)) {
		t.Fatal("repeated immediately without earning")
	}
	p.hero.pending = &heroAttempt{}
	if p.retrySkillSetup(now.Add(time.Minute)) {
		t.Fatal("interrupted purchase")
	}
	p.hero.pending = nil
	p.plan(now.Add(time.Minute))
	if p.startup != startupHeroes || p.hero.sweep.retry != 2 || p.hero.sweep.needsLevels || p.export.requested || !p.startupExported || p.ancient.plan != plan {
		t.Fatal("reentry reset independent automation or retained stale sweep", p.startup, p.hero.sweep)
	}
}

func TestCompleteSkillSetupDoesNotRepeat(t *testing.T) {
	now := time.Now()
	f := testPipelineFrame()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.frame, p.layout = f, f.layout
	p.beginStartup()
	p.nextSkillSetup = now.Add(time.Minute)
	out := observation{kind: heroAnalysis, startup: startupHeroes, frame: f, hero: heroObservation{frame: f, startup: true, startupComplete: true, passiveReady: true, sweep: startupSweep{retry: 2}}}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	if !p.nextSkillSetup.IsZero() {
		t.Fatal("completed setup retained retry")
	}
}

func TestHeroHireAndLevelInput(t *testing.T) {
	for _, startup := range []bool{false, true} {
		for _, owned := range []bool{false, true} {
			f := testPipelineFrame()
			clicks, keys := 0, 0
			p := newGamePipeline(&pauseControl{}, heroInput{
				click: func(image.Point) error { clicks++; return nil },
				move:  func(image.Point) error { return nil },
				keyToggle: func(key, state string) error {
					if key != "q" {
						t.Fatal("wrong modifier", key)
					}
					keys++
					return nil
				},
			}, pipelineReaders{}, pipelineOptions{})
			p.frame = f
			acted, err := p.execute(context.Background(), gameAction{kind: buyHero, frame: f, hero: heroObservation{owned: owned, startup: startup}})
			expected := 0
			if owned {
				expected = 2
			}
			if err != nil || !acted || clicks != 1 || keys != expected {
				t.Fatal("HIRE must use x1; leveling must use Q/MAX", startup, owned, acted, err, clicks, keys)
			}
		}
	}
}
