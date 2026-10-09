package bot

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"
)

func TestMonsterAssistUsesOrdinaryProgressionQueue(t *testing.T) {
	for _, state := range []string{"unknown", "unreadable", "unavailable", "purchase", "scroll", "menu", "input"} {
		t.Run(state, func(t *testing.T) {
			f := testPipelineFrame()
			f.context.heroes = true
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true})
			p.frame, p.layout, p.startupCheck = f, f.layout, false
			out := observation{frame: f}
			switch state {
			case "unknown":
				out.frame.id = 0
			case "unreadable":
				out.err = errors.New("unreadable game number")
			case "purchase":
				p.queue[buyHero] = gameAction{kind: buyHero}
			case "scroll":
				p.queue[scrollHeroes] = gameAction{kind: scrollHeroes}
			case "menu":
				p.frame.context.saveMenu = true
			case "input":
				p.hero.pending = &heroAttempt{}
			}
			p.state[heroAnalysis] = out
			p.planMonsterAssist(f.at)
			a, ok := p.nextAction(f.at)
			want := state == "unknown" || state == "unreadable" || state == "unavailable"
			if ok != want || ok && (a.kind != clickMonster || a.point != image.Pt(75, 50)) {
				t.Fatalf("assist %s: %+v %t", state, a, ok)
			}
		})
	}
}

func TestStartupSparseProgressionSharesHeroCapture(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-post-transcension-start.png")
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return s, nil }}, pipelineReaders{context: recognizedGame, progression: readProgressionState}, pipelineOptions{heroes: true, progression: true, fishInterval: time.Second})
	p.startupCheck, p.startup = false, startupHeroes
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[progressionAnalysis]) != 1 || len(jobs[heroAnalysis]) != 1 {
		t.Fatal("progression waited for the first hero read")
	}
	progressJob, heroJob := <-jobs[progressionAnalysis], <-jobs[heroAnalysis]
	if progressJob.frame.image != heroJob.frame.image || !progressJob.modeOnly {
		t.Fatal("progression did not reuse the hero capture")
	}
	out := p.analyze(context.Background(), progressionAnalysis, progressJob)
	if out.err != nil || !out.progression.Known || out.progression.Enabled {
		t.Fatalf("sparse disabled boot: %+v %v", out.progression, out.err)
	}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	p.planStartup(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != enableProgression || p.startupPassive {
		t.Fatalf("progression not enabled before DPS: %+v %t", a, ok)
	}
}
