package bot

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestListScrollModesUseSharedInput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		action    gameAction
		mode      listScrollMode
		primitive string
	}{
		{"hero edge", gameAction{kind: scrollHeroes}, listScrollEdge, "drag"},
		{"hero page", gameAction{kind: scrollHeroes, hero: heroObservation{startup: true, sweep: startupSweep{top: true}, startupScroll: image.Pt(20, 60)}}, listScrollPage, "drag"},
		{"mercenary top", gameAction{kind: handleMercenary, mercenary: mercenaryCommand{step: scrollMercenariesTop}}, listScrollEdge, "drag"},
		{"mercenary bottom", gameAction{kind: handleMercenary, mercenary: mercenaryCommand{step: scrollMercenariesBottom}}, listScrollEdge, "drag"},
		{"ancient record", gameAction{kind: handleAncient, ancient: ancientCommand{step: scrollAncients, direction: 1}}, listScrollRecord, "wheel"},
		{"ancient fine", gameAction{kind: handleAncient, ancient: ancientCommand{step: scrollAncients, direction: -1, fine: true}}, listScrollFine, "click"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.action
			a.frame = testPipelineFrame()
			a.point = image.Pt(20, 40)
			a.target = image.Pt(20, 60)
			s, ok := listScrollAction(a)
			if !ok || s.mode != tc.mode {
				t.Fatal("navigation mode", s, ok)
			}
			var calls []string
			input := heroInput{drag: func(from, to image.Point) error {
				if from != a.point || to != a.target {
					t.Fatal("drag endpoints", from, to)
				}
				calls = append(calls, "drag")
				return nil
			}, scroll: func(point image.Point, direction int) error {
				if point != a.point || direction != a.ancient.direction {
					t.Fatal("wheel arguments")
				}
				calls = append(calls, "wheel")
				return nil
			}, click: func(point image.Point) error {
				if point != a.point {
					t.Fatal("arrow point")
				}
				calls = append(calls, "click")
				return nil
			}, move: func(image.Point) error { calls = append(calls, "park"); return nil }}
			p := newGamePipeline(&pauseControl{}, input, pipelineReaders{}, pipelineOptions{})
			acted, err := p.execute(context.Background(), a)
			count := 1
			if tc.primitive != "drag" {
				count = 2
			}
			if !acted || err != nil || len(calls) != count || calls[0] != tc.primitive {
				t.Fatal("shared dispatch", acted, err, calls)
			}
			p.controls.toggle()
			if acted, err := p.execute(context.Background(), a); acted || err != nil || len(calls) != count {
				t.Fatal("F8 replayed scroll", acted, err, calls)
			}
		})
	}
}

func TestHeroScrollUsesLatestThumbCenter(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	f.layout = 1
	thumb, _, ok := heroScrollbarThumb(f.image)
	if !ok {
		t.Fatal("fixture thumb")
	}
	latest := f
	latest.id++
	latest.image = startupShiftThumbBy(t, f.image, 6)
	center, _, ok := heroScrollbarThumb(latest.image)
	if !ok || center == thumb {
		t.Fatal("shifted fixture")
	}
	for _, page := range []bool{false, true} {
		a := gameAction{kind: scrollHeroes, frame: f, point: thumb, target: image.Pt(thumb.X, f.image.Bounds().Max.Y-1)}
		if page {
			a.hero = heroObservation{startup: true, sweep: startupSweep{top: true}, startupScroll: thumb.Add(image.Pt(0, 12))}
			a.target = a.hero.startupScroll
		}
		p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
		p.startupCheck = false
		p.frame, p.layout = latest, 1
		p.enqueue(a, latest.at)
		fresh, ok := p.nextAction(latest.at)
		if !ok || fresh.point != center {
			t.Fatal("drag retained stale center", fresh.point, center, ok)
		}
		if page && fresh.target.Y-fresh.point.Y != a.target.Y-a.point.Y {
			t.Fatal("page overlap changed during recentering")
		}
		if !page && fresh.target != a.target {
			t.Fatal("edge target changed")
		}
	}
}

func TestHeroScrollLagAndRetry(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-clipped-price.png")
	thumb, _, ok := heroScrollbarThumb(f.image)
	if !ok {
		t.Fatal("fixture thumb")
	}
	p := heroRunner{enabled: true}
	now := f.at
	for failure := 1; failure <= 3; failure++ {
		f.id++
		f.at = now
		before := heroObservation{frame: f, thumb: thumb, thumbFound: true, x1: true}
		p.latest = before
		p.nextScan = now
		a, ok := p.action(now)
		if !ok || a.kind != scrollHeroes {
			t.Fatal("scroll missing")
		}
		p.sent(a, now)
		for _, dt := range []time.Duration{400 * time.Millisecond, 600 * time.Millisecond, 800 * time.Millisecond} {
			after := before
			after.frame.id++
			after.frame.at = now.Add(dt)
			p.observe(after, observation{}, after.frame.at)
		}
		if p.pending == nil || p.scrollFailures != failure-1 {
			t.Fatal("early scans failed a lagging drag")
		}
		after := before
		after.frame.id++
		after.frame.at = now.Add(1400 * time.Millisecond)
		p.observe(after, observation{}, after.frame.at)
		wait := 250 * time.Millisecond
		if failure >= 3 {
			wait = 2 * time.Second
		}
		if p.pending != nil || p.latest.frame.id != 0 || p.scrollFailures != failure || !p.nextScan.Equal(after.frame.at.Add(wait)) || !p.enabled || p.failures != 0 {
			t.Fatal("missed drag did not request bounded fresh retry", p)
		}
		now = p.nextScan
	}
	// A partial ordinary edge drag is movement: continue from its new position.
	f.id++
	f.at = now
	before := heroObservation{frame: f, thumb: thumb, thumbFound: true, x1: true}
	p.latest = before
	a, _ := p.action(now)
	p.sent(a, now)
	after := before
	after.frame.id++
	after.frame.at = now.Add(900 * time.Millisecond)
	after.thumb.Y += 8
	p.observe(after, observation{}, after.frame.at)
	next, ok := p.action(after.frame.at)
	if p.pending != nil || p.scrollFailures != 0 || !ok || next.kind != scrollHeroes || next.point != after.thumb {
		t.Fatal("partial drag did not continue from new thumb", next, ok)
	}
}
