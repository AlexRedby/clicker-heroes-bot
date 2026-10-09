package bot

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"
)

func TestHeroRunner(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
	now := time.Now()
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	before := heroObservation{frame: frame, button: image.Pt(204, 894), thumbFound: true, bottom: true, x1: true, found: true, owned: true, level: 100, gold: 100, nextPrice: 102}
	for _, scenario := range []string{"purchase", "saving", "fish obstruction", "three failures", "stale final failure", "stale final success", "cancelled confirmation", "unowned"} {
		t.Run(scenario, func(t *testing.T) {
			controls := pauseControl{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, fishInterval: time.Second})
			p.startupCheck = false // This test begins after initial hero setup.
			p.nextUpgrades = time.Now().Add(time.Hour)
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
		if p.failures != failure || p.enabled != (failure < 3) || !p.nextScan.Equal(now.Add(35*time.Second)) {
			t.Fatalf("retry %d: %+v", failure, p)
		}
		now = now.Add(time.Minute)
	}
}

func TestHeroObservationInterruptedOCR(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
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
			p.startupCheck = false // This test begins after initial hero setup.
			p.nextUpgrades = time.Now().Add(time.Hour)
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
	screen := loadTestImage(t, "../../testdata/hero-skogur-hire.png")
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

func TestHeroTopScrollbarAfterAncientBatch(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-startup-gold.png")
	now := time.Now()
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	out, err := readHeroObservation(context.Background(), frame, heroReaders{}, nil)
	if err != nil || !out.thumbFound || out.bottom || out.startup {
		t.Fatalf("ordinary top-list observation: thumb=%v found=%t bottom=%t startup=%t err=%v", out.thumb, out.thumbFound, out.bottom, out.startup, err)
	}
	if !heroListStable(screen, screen) {
		t.Fatal("stationary top list rejected after startup finished")
	}
	runner := heroRunner{enabled: true}
	runner.observe(out, observation{}, now)
	action, ok := runner.action(now)
	if !ok || action.kind != scrollHeroes || action.point != out.thumb || action.target.Y != screen.Bounds().Max.Y-1 {
		t.Fatalf("ordinary leveling should drag to the bottom: %+v, %t", action, ok)
	}
	pipeline := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	pipeline.startupCheck = false // This test begins after initial hero setup.
	pipeline.nextUpgrades = time.Now().Add(time.Hour)
	pipeline.frame, pipeline.layout = frame, 1
	pipeline.hero.observe(out, observation{}, now)
	pipeline.plan(now)
	if queued, ok := pipeline.nextAction(now); !ok || queued.kind != scrollHeroes {
		t.Fatalf("queue rejected ordinary top-list drag: %+v %t", queued, ok)
	}
}

func TestOrdinarySparseHeroes(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-post-transcension-start.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	f := gameFrame{id: 1, at: now, layout: 1, image: screen, context: c}
	before, err := readHeroObservation(context.Background(), f, noStartupOCR(t), nil)
	if err != nil || !before.bottom || before.thumbFound || !before.found || before.owned || absDiff(before.button.Y, 865) > 3 {
		t.Fatalf("complete short list: %+v error=%v", before, err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.startupCheck, p.frame, p.layout = false, f, 1
	p.hero.observe(before, observation{}, now)
	a, ok := p.hero.action(now)
	if !ok || a.kind != buyHero {
		t.Fatalf("short-list action %+v %t", a, ok)
	}
	p.enqueue(a, now)
	if a, ok = p.nextAction(now); !ok || a.kind != buyHero {
		t.Fatalf("short-list dispatch %+v %t", a, ok)
	}
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(covered, heroUpgradeButtonRegion(screen, before.footer), image.NewUniform(color.Black), image.Point{}, draw.Src)
	f.image = covered
	if heroViewportStable(before, f) {
		t.Fatal("covered footer authorized stale purchase")
	}
	out, err := readHeroObservation(context.Background(), f, noStartupOCR(t), nil)
	if err != nil || out.bottom || out.found {
		t.Fatalf("unproven end accepted: %+v %v", out, err)
	}

	// Hiring can create a scrollbar while the purchased row stays in place.
	after := image.NewRGBA(screen.Bounds())
	draw.Draw(after, after.Bounds(), screen, screen.Bounds().Min, draw.Src)
	long := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
	track := image.Rect(1140, 560, 1200, 1440)
	draw.Draw(after, track, long, track.Min, draw.Src)
	f.image, f.id, f.at = after, 2, now.Add(time.Second)
	if _, _, found := heroScrollbarThumb(after); !found {
		t.Fatal("test did not introduce a scrollbar")
	}
	read := heroReaders{level: func(context.Context, image.Image, image.Point) (int, error) { return 1, nil }}
	out, err = readHeroObservation(context.Background(), f, read, &before)
	if err != nil || !out.stable || out.level != 1 {
		t.Fatalf("hire expansion: %+v %v", out, err)
	}
	p.hero.sent(a, now)
	unchanged := out
	unchanged.level = 0
	p.hero.observe(unchanged, observation{}, f.at)
	if p.hero.pending == nil {
		t.Fatal("unchanged level confirmed a hire")
	}
	p.hero.observe(out, observation{}, f.at)
	if p.hero.pending != nil || p.hero.failures != 0 {
		t.Fatal("hired row not confirmed")
	}
	// The exception belongs to HIRE, never a level-up on a moving list.
	owned := before
	owned.owned = true
	out, err = readHeroObservation(context.Background(), f, noStartupOCR(t), &owned)
	if err != nil || out.stable {
		t.Fatal("owned moving row accepted")
	}
	draw.Draw(after, image.Rect(715, 790, 935, 820), image.NewUniform(color.White), image.Point{}, draw.Src)
	out, err = readHeroObservation(context.Background(), f, noStartupOCR(t), &before)
	if err != nil || out.stable {
		t.Fatal("different row name accepted")
	}
}
