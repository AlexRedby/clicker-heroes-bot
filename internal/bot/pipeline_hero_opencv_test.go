package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"

	"clicker-heroes-bot/internal/vision"
)

func TestStartupOpenCVFailedFrameUsesSharedQueue(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-startup-name-empty.png")
	c, err := recognizedGame(screen)
	if err != nil || !bootstrapHeroes(c) {
		t.Fatalf("context: %+v %v", c, err)
	}
	now := time.Now()
	captures := 0
	var keys []string
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture: func() (image.Image, error) { captures++; return screen, nil },
		click:   func(image.Point) error { return nil }, move: func(image.Point) error { return nil },
		keyToggle: func(key, state string) error { keys = append(keys, key+":"+state); return nil },
	}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true})
	p.layout, p.startup, p.startupCheck = 1, startupHeroes, false
	p.hero.sweep.top = true
	p.frame = gameFrame{id: 1, layout: 1, at: now, image: screen, context: c}
	out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, sweep: p.hero.sweep})
	if out.err != nil || !out.hero.found {
		t.Fatalf("failure frame: %+v %v", out.hero, out.err)
	}
	if err = p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHero || a.point != out.hero.button {
		t.Fatalf("queue: %+v %t", a, ok)
	}
	acted, err := p.execute(context.Background(), a)
	if err != nil || !acted || captures != 0 || len(keys) != 2 || keys[0] != "q:down" || keys[1] != "q:up" {
		t.Fatalf("startup input: %t %v keys=%v captures=%d", acted, err, keys, captures)
	}
}

func TestHeroHireReferenceIgnoresNativeBackground(t *testing.T) {
	reference, err := vision.Template("heroes/hire-1280.png")
	if err != nil {
		t.Fatal(err)
	}
	// These native JPEG edge pixels contain button-background blend, not letters.
	for _, point := range []image.Point{{0, 0}, {49, 0}, {0, 15}, {49, 15}, {6, 1}, {22, 1}} {
		if _, _, _, alpha := reference.At(point.X, point.Y).RGBA(); alpha != 0 {
			t.Fatalf("button background retained in caption mask at %v", point)
		}
	}
	source := loadTestImage(t, "../../testdata/hero-nongilded-successor.jpg")
	for _, background := range []color.RGBA{{98, 190, 247, 255}, {190, 70, 180, 255}, {170, 180, 190, 255}} {
		s := image.NewRGBA(source.Bounds())
		draw.Draw(s, s.Bounds(), source, source.Bounds().Min, draw.Src)
		for y := 538; y < 554; y++ {
			for x := 94; x < 144; x++ {
				r, g, b := rgb(source.At(x, y))
				if min(r, g, b) >= 45 && max(r, g, b)-min(r, g, b) < 35 {
					s.Set(x, y, background)
				}
			}
		}
		if kind, err := readHeroButtonKind(s, image.Pt(102, 563)); err != nil || kind != heroButtonHire {
			scores, _ := heroButtonScores(s, image.Pt(102, 563))
			t.Fatalf("native dark HIRE changed with neutral background %v: %v %v scores=%v", background, kind, err, scores)
		}
	}
}

func TestStartupClippedCaptionUsesSharedQueue(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-startup-clipped-caption.png")
	c, err := recognizedGame(screen)
	if err != nil || !bootstrapHeroes(c) {
		t.Fatalf("context: %+v %v", c, err)
	}
	now := time.Now()
	captures, drags := 0, 0
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture: func() (image.Image, error) { captures++; return screen, nil },
		drag: func(from, to image.Point) error {
			drags++
			if to.Y <= from.Y {
				t.Fatalf("clipped bottom row must scroll down: %v -> %v", from, to)
			}
			return nil
		},
	}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true})
	p.layout, p.startup, p.startupCheck = 1, startupHeroes, false
	p.hero.sweep = startupSweep{top: true, y: 1417}
	p.frame = gameFrame{id: 1, layout: 1, at: now, image: screen, context: c}
	out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, sweep: p.hero.sweep})
	if out.err != nil {
		t.Fatal(out.err)
	}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("clipped caption did not queue scroll: %+v %t", a, ok)
	}
	if acted, err := p.execute(context.Background(), a); !acted || err != nil || captures != 0 || drags != 1 {
		t.Fatalf("scroll: acted=%t error=%v captures=%d drags=%d", acted, err, captures, drags)
	}
}
