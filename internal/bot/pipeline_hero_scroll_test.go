package bot

import (
	"context"
	"testing"
	"time"
)

func TestStartupPendingScrollCannotCompleteSweep(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-missed-hire.png")
	f.layout = 1
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true})
	p.frame, p.layout = f, f.layout
	p.beginStartup()
	p.hero.sent(gameAction{kind: scrollHeroes, frame: f, hero: heroObservation{frame: f, startup: true}}, f.at)
	for i := 0; i < 3; i++ {
		f.id++
		f.at = f.at.Add(time.Second)
		p.frame = f
		// Even an observed end condition must wait for successful navigation;
		// reaching the no-motion retry limit cannot turn it into completion.
		out := heroObservation{frame: f, startup: true, startupComplete: true, passiveReady: true, thumbFound: true}
		if err := p.accept(context.Background(), observation{kind: heroAnalysis, startup: startupHeroes, frame: f, hero: out}, f.at); err != nil {
			t.Fatal(err)
		}
		if p.startup != startupHeroes || i < 2 && p.hero.pending == nil {
			t.Fatal("no-motion scroll was treated as a completed sweep")
		}
		p.plan(f.at)
		if _, ok := p.queue[buyHero]; ok {
			t.Fatal("unconfirmed navigation permitted a purchase")
		}
	}
}
