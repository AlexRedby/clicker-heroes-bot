package main

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"
)

func TestHeroRunner(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	now := time.Now()
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	before := heroObservation{frame: frame, button: image.Pt(204, 894), thumbFound: true, bottom: true, x1: true, found: true, owned: true, level: 100, gold: 100, nextPrice: 102}
	for _, scenario := range []string{"purchase", "saving", "fish obstruction", "three failures", "stale final failure", "stale final success", "cancelled confirmation", "unowned"} {
		t.Run(scenario, func(t *testing.T) {
			controls := pauseControl{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, fishInterval: time.Second})
			p.layout = 1
			p.frame = frame
			p.hero.failures = 2
			initial := before
			if scenario == "saving" {
				initial.nextPrice = 100.5
			}
			if scenario == "unowned" {
				initial.owned = false
				initial.level = 0
			}
			p.hero.observe(initial, observation{}, now)
			action, ok := p.hero.action(now)
			if scenario == "saving" {
				if ok {
					t.Fatal("bought instead of saving")
				}
				return
			}
			if !ok || action.kind != buyHero {
				t.Fatalf("purchase action: %+v %t", action, ok)
			}
			p.hero.sent(action, now)
			for attempt := 1; attempt <= 5; attempt++ {
				after := before
				after.frame.id = uint64(attempt + 1)
				after.frame.at = now.Add(time.Duration(attempt) * time.Second)
				after.stable = true
				if scenario == "purchase" || scenario == "unowned" || (scenario == "stale final success" && attempt == 5) {
					after.level = 101
				}
				if scenario == "fish obstruction" {
					p.state[fishAnalysis] = observation{kind: fishAnalysis, frame: after.frame, found: true}
				}
				if attempt == 5 && (scenario == "stale final failure" || scenario == "stale final success") {
					controls.toggle()
					controls.toggle()
				}
				if attempt == 5 && scenario == "cancelled confirmation" {
					cancel()
				}
				if err := p.accept(ctx, observation{kind: heroAnalysis, frame: after.frame, hero: after}, after.frame.at); err != nil {
					t.Fatal(err)
				}
				if p.hero.pending == nil {
					break
				}
			}
			fish := observation{kind: fishAnalysis, frame: gameFrame{id: 8, layout: 1, at: now.Add(6 * time.Second)}}
			if err := p.accept(ctx, fish, fish.frame.at); err != nil {
				t.Fatal(err)
			}
			wantFailures := 2
			if scenario == "purchase" || scenario == "unowned" {
				wantFailures = 0
			}
			if scenario == "three failures" {
				wantFailures = 3
			}
			if p.hero.failures != wantFailures || p.hero.enabled != (wantFailures < 3) {
				t.Fatalf("failures=%d enabled=%t; want %d", p.hero.failures, p.hero.enabled, wantFailures)
			}
		})
	}
}

func TestHeroRunnerRetry(t *testing.T) {
	now := time.Now()
	p := heroRunner{enabled: true}
	for failure := 1; failure <= 3; failure++ {
		before := heroObservation{frame: gameFrame{id: 1, layout: 1, at: now}, button: image.Pt(204, 894), found: true, thumbFound: true, bottom: true, x1: true, level: 100}
		p.latest = before
		p.nextScan = time.Time{}
		a, ok := p.action(now)
		if !ok {
			t.Fatal("retry missing")
		}
		p.sent(a, now)
		for i := 1; i <= 5; i++ {
			after := before
			after.frame.id = uint64(i + 1)
			after.frame.at = now.Add(time.Duration(i) * time.Second)
			after.stable = true
			p.observe(after, observation{}, after.frame.at)
		}
		fish := observation{frame: gameFrame{id: 7, layout: 1, at: now.Add(6 * time.Second)}}
		p.finishFailure(fish, fish.frame.at)
		if p.failures != failure || p.enabled != (failure < 3) || !p.nextScan.Equal(fish.frame.at.Add(30*time.Second)) {
			t.Fatalf("retry %d: %+v", failure, p)
		}
		now = now.Add(time.Minute)
	}
}

func TestHeroObservationInterruptedOCR(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	for _, cancelRead := range []string{"gold", "price", "level"} {
		t.Run(cancelRead, func(t *testing.T) {
			controls := pauseControl{}
			ctx := context.Background()
			now := time.Now()
			frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
			interrupt := func(name string) {
				if name == cancelRead {
					controls.toggle()
					controls.toggle()
				}
			}
			read := heroReaders{gold: func(context.Context, image.Image) (float64, error) { interrupt("gold"); return 100, nil }, price: func(context.Context, image.Image, image.Point) (float64, error) { interrupt("price"); return 102, nil }, level: func(context.Context, image.Image, image.Point) (int, error) { interrupt("level"); return 100, nil }}
			o, err := readHeroObservation(ctx, frame, read, nil)
			if err != nil {
				t.Fatal(err)
			}
			p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
			p.layout = 1
			p.hero.failures = 2
			if err := p.accept(ctx, observation{kind: heroAnalysis, frame: frame, hero: o}, now); err != nil {
				t.Fatal(err)
			}
			if p.hero.latest.frame.id != 0 || p.hero.failures != 2 {
				t.Fatal("stale OCR was consumed")
			}
		})
	}
}

func TestHeroObservationNamesFailedOCRFields(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	frame := gameFrame{id: 1, at: time.Now(), image: screen, context: c}
	for _, field := range []string{"hero gold", "next hero price", "hero level"} {
		t.Run(field, func(t *testing.T) {
			failure := errUnreadableGameNumber
			reader := heroReaders{
				gold: func(context.Context, image.Image) (float64, error) {
					if field == "hero gold" {
						return 0, failure
					}
					return 996, nil
				},
				price: func(context.Context, image.Image, image.Point) (float64, error) {
					if field == "next hero price" {
						return 0, failure
					}
					return 999, nil
				},
				level: func(context.Context, image.Image, image.Point) (int, error) {
					if field == "hero level" {
						return 0, failure
					}
					return 16014, nil
				},
			}
			out, err := readHeroObservation(context.Background(), frame, reader, nil)
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), field) {
				t.Fatalf("field error=%v", err)
			}
			if field == "hero level" {
				_, err = readHeroObservation(context.Background(), frame, reader, &out)
				if !errors.Is(err, failure) || !strings.Contains(err.Error(), field) {
					t.Fatalf("confirmation error=%v", err)
				}
			}
		})
	}
}
