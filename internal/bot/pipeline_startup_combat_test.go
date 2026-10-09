package bot

import (
	"errors"
	"image"
	"testing"
	"time"
)

func TestStartupProgressionAndCombatDoNotWaitForHeroOCR(t *testing.T) {
	for _, phase := range []startupPhase{startupHeroes, startupUpgrades, startupProgression} {
		t.Run([]string{"", "heroes", "upgrades", "progression"}[phase], func(t *testing.T) {
			now := time.Now()
			f := testPipelineFrame()
			f.context.heroes = true
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true})
			p.frame, p.layout = f, f.layout
			p.beginStartup()
			p.startup = phase
			p.state[heroAnalysis] = observation{err: errors.New("hero OCR pending")}
			p.state[progressionAnalysis] = observation{frame: f, progression: progressionState{Known: true, Enabled: false}}
			p.planStartup(now)
			if _, ok := p.queue[enableProgression]; !ok {
				t.Fatal("startup waited for passive DPS before enabling progression")
			}
			// Once progression has been handled, otherwise idle startup helps earn gold.
			delete(p.queue, enableProgression)
			p.state[progressionAnalysis].progression.Enabled = true
			p.planStartup(now)
			seed, ok := p.queue[clickMonster]
			if !ok || seed.point != image.Pt(75, 50) || !p.nextMonster.Equal(now.Add(100*time.Millisecond)) {
				t.Fatalf("bounded seed click absent: %+v %t", seed, ok)
			}
			if a, ok := p.nextAction(now); !ok || a.kind != clickMonster {
				t.Fatalf("seed did not reach shared queue: %+v %t", a, ok)
			}
			p.planStartup(now.Add(50 * time.Millisecond))
			if _, ok := p.queue[clickMonster]; ok {
				t.Fatal("seed clicks exceeded their interval")
			}
		})
	}
}

func TestStartupCombatRechecksSafeContext(t *testing.T) {
	for _, boundary := range []string{"save", "transcension", "mercenary", "junk", "pending", "F8"} {
		t.Run(boundary, func(t *testing.T) {
			f := testPipelineFrame()
			f.context.heroes = true
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
			p.frame, p.layout = f, f.layout
			p.beginStartup()
			p.enqueue(gameAction{kind: clickMonster, frame: f, point: image.Pt(75, 50)}, f.at)
			switch boundary {
			case "save":
				p.frame.context.saveMenu = true
			case "transcension":
				p.frame.context.transcension = true
			case "mercenary":
				p.frame.context.mercenaryDialog = true
			case "junk":
				p.frame.context.relicJunk = true
			case "pending":
				p.hero.pending = &heroAttempt{action: gameAction{kind: buyHero, frame: f}}
			case "F8":
				p.controls.toggle()
				p.reset(p.controls.snapshot())
			}
			if a, ok := p.nextAction(f.at); ok {
				t.Fatalf("seed reached %s boundary: %+v", boundary, a)
			}
		})
	}
}

func TestStartupProgressionReadinessDoesNotFinishHeroSetup(t *testing.T) {
	f := testPipelineFrame()
	f.context.heroes = true
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true})
	p.frame, p.layout = f, f.layout
	p.beginStartup()
	p.startup = startupProgression
	p.state[progressionAnalysis] = observation{frame: f, progression: progressionState{Known: true, Enabled: true}}
	p.planStartup(f.at)
	if p.startup != startupProgression {
		t.Fatal("enabled progression completed startup without a passive hero")
	}
	p.startupPassive = true
	p.planStartup(f.at)
	if p.startup != noStartup {
		t.Fatal("ready startup did not hand off to ordinary automation")
	}
}
