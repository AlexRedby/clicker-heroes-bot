package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestStartupOpenCVSkipsUnrelatedLevelOCR(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/hero-startup-alexa-unreadable.png")
	ctx := context.Background()
	calls := 0
	read := heroReaders{gold: readHeroGold, price: readHeroPrice, level: func(ctx context.Context, s image.Image, point image.Point) (int, error) {
		calls++
		return readHeroLevel(ctx, s, point)
	}}
	frame := gameFrame{id: 1, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	owned, err := readStartupHeroObservation(ctx, frame, read, nil, nil, true)
	if err != nil || !owned.found || !owned.owned || owned.startupName != "TheGreatForestSeer" || owned.level != 10000 || calls != 1 {
		t.Fatalf("selected owned row: %+v %v numerical OCR=%d", owned, err, calls)
	}
	// A previously handled level can be covered without blocking the next purchase.
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(covered, image.Rect(665, owned.button.Y-29, 973, owned.button.Y+21), image.NewUniform(color.RGBA{R: 255, G: 224, B: 95, A: 255}), image.Point{}, draw.Src)
	frame.image = covered
	calls = 0
	out, err := readStartupHeroObservation(ctx, frame, read, nil, map[string]bool{"TheGreatForestSeer": true}, true)
	if err != nil || !out.found || out.owned || out.level != 0 || out.startupName != "AlexaAssassin" || !out.passiveReady || out.startupComplete || calls != 0 {
		t.Fatalf("next HIRE after covered visited level: %+v %v numerical OCR=%d", out, err, calls)
	}
	// An unavailable owned row is also skipped without reading its numerical level.
	disabled := image.NewRGBA(screen.Bounds())
	draw.Draw(disabled, disabled.Bounds(), screen, screen.Bounds().Min, draw.Src)
	for y := owned.button.Y - screen.Bounds().Dy()/15; y < owned.button.Y+screen.Bounds().Dy()/15; y++ {
		for x := 140; x < 350; x++ {
			r, g, blue := rgb(disabled.At(x, y))
			if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
				disabled.Set(x, y, color.RGBA{R: 45, G: 60, B: 70, A: 255})
			}
		}
	}
	frame.image = disabled
	calls = 0
	out, err = readStartupHeroObservation(ctx, frame, read, nil, nil, true)
	if err != nil || !out.found || out.owned || out.startupName != "AlexaAssassin" || !out.passiveReady || calls != 0 {
		t.Fatalf("HIRE after disabled owned row: %+v %v numerical OCR=%d", out, err, calls)
	}
	beforeScreen := loadTestImage(t, "testdata/hero-startup-fisherman-before.png")
	afterScreen := loadTestImage(t, "testdata/hero-startup-fisherman-after.png")
	frame.image, frame.context.bounds = beforeScreen, beforeScreen.Bounds()
	calls = 0
	before, err := readStartupHeroObservation(ctx, frame, read, nil, map[string]bool{"BrittanyBeachPrincess": true}, true)
	if err != nil || !before.found || before.owned || calls != 0 {
		t.Fatalf("native unowned hire: %+v %v numerical OCR=%d", before, err, calls)
	}
	frame.id++
	frame.image = afterScreen
	after, err := readStartupHeroObservation(ctx, frame, read, &before, nil, true)
	if err != nil || !after.stable || !after.owned || after.level != 10000 || after.startupName != before.startupName || calls != 1 {
		t.Fatalf("native hire confirmation: %+v %v numerical OCR=%d", after, err, calls)
	}
}

func TestStartupOpenCVAlexaQueuesPurchaseAndRejectsOcclusion(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/hero-startup-alexa-unreadable.png")
	c, err := recognizedGame(screen)
	if err != nil || !bootstrapHeroes(c) {
		t.Fatalf("native context: %+v %v", c, err)
	}
	now := time.Now()
	captures, levels := 0, 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { captures++; return screen, nil }}, pipelineReaders{heroes: heroReaders{gold: readHeroGold, price: readHeroPrice, level: func(context.Context, image.Image, image.Point) (int, error) {
		levels++
		return 0, nil
	}}}, pipelineOptions{heroes: true})
	p.layout, p.startup, p.startupCheck = 1, startupHeroes, false
	p.hero.startupTop = true
	p.hero.startupDone = map[string]bool{"TheGreatForestSeer": true}
	p.frame = gameFrame{id: 1, layout: 1, at: now, image: screen, context: c}
	analyze := func() observation {
		return p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, startupTop: true, startupVisits: p.hero.startupVisits()})
	}
	out := analyze()
	if out.err != nil || !out.hero.found || out.hero.startupName != "AlexaAssassin" || out.hero.owned || out.hero.level != 0 {
		t.Fatalf("actual failure frame: %+v %v", out.hero, out.err)
	}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHero || a.hero.startupName != "AlexaAssassin" || a.point != out.hero.button || levels != 0 || captures != 0 {
		t.Fatalf("queued target: %+v %t numerical OCR=%d captures=%d", a, ok, levels, captures)
	}
	p.hero.interrupt()
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	// Hide H exactly as in the negative that padded Tesseract falsely read as HIRE.
	draw.Draw(covered, image.Rect(179, 925, 207, 974), image.NewUniform(color.RGBA{R: 98, G: 190, B: 247, A: 255}), image.Point{}, draw.Src)
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	p.frame.image = covered
	out = analyze()
	if out.err == nil || out.hero.found || out.hero.startupComplete || levels != 0 {
		t.Fatalf("partial H accepted: %+v %v numerical OCR=%d", out.hero, out.err, levels)
	}
	if err := p.accept(context.Background(), out, p.frame.at); err != nil {
		t.Fatal(err)
	}
	p.plan(p.frame.at)
	if a, ok := p.nextAction(p.frame.at); ok && a.kind == buyHero {
		t.Fatal("unreadable caption queued a purchase")
	}
	if len(p.diagnostics) != 1 || captures != 0 || p.hero.startupDone["AlexaAssassin"] {
		t.Fatal("occlusion lost exact-frame evidence, captured again or credited a purchase")
	}
}

func TestHeroHireReferenceIgnoresNativeBackground(t *testing.T) {
	reference, err := templateImage("heroes/hire-1280.png")
	if err != nil {
		t.Fatal(err)
	}
	// These native JPEG edge pixels contain button-background blend, not letters.
	for _, point := range []image.Point{{0, 0}, {49, 0}, {0, 15}, {49, 15}, {6, 1}, {22, 1}} {
		if _, _, _, alpha := reference.At(point.X, point.Y).RGBA(); alpha != 0 {
			t.Fatalf("button background retained in caption mask at %v", point)
		}
	}
	source := loadTestImage(t, "testdata/hero-nongilded-successor.jpg")
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
