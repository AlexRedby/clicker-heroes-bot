package main

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeroUnreadableDiagnosticsUseSharedFrame(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-startup-gold.png")
	now := time.Now()
	captures := 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { captures++; return screen, nil }}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.startup, p.layout = startupHeroes, 1
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	readError := errors.New("startup HIRE caption unreadable")
	accept := func(err error) {
		t.Helper()
		p.frame = frame
		if e := p.accept(context.Background(), observation{kind: heroAnalysis, startup: startupHeroes, frame: frame, hero: heroObservation{frame: frame, startup: true}, err: err}, frame.at); e != nil {
			t.Fatal(e)
		}
		frame.id++
		frame.at = frame.at.Add(time.Second)
	}
	accept(readError)
	if len(p.diagnostics) != 1 || captures != 0 {
		t.Fatalf("diagnostics=%d captures=%d", len(p.diagnostics), captures)
	}
	diagnostic := <-p.diagnostics
	if diagnostic.after.frame.id != 1 || diagnostic.after.frame.image != screen || !errors.Is(diagnostic.readError, readError) {
		t.Fatal("diagnostic lost the analyzed frame or reason")
	}
	accept(readError)
	p.hero.interrupt()
	accept(readError)
	if len(p.diagnostics) != 0 {
		t.Fatal("unchanged read failure saved repeatedly or after F8")
	}
	accept(nil)
	accept(readError)
	if len(p.diagnostics) != 1 || captures != 0 {
		t.Fatal("recovered reader did not start a new failure episode")
	}
	<-p.diagnostics
	accept(errors.New("startup hero name unreadable"))
	if len(p.diagnostics) != 1 {
		t.Fatal("changed read failure was not recorded")
	}
	blocked := errors.New("startup caption crop unreadable")
	accept(blocked)
	<-p.diagnostics
	accept(blocked)
	if len(p.diagnostics) != 1 {
		t.Fatal("busy worker permanently suppressed new failure evidence")
	}

	t.Chdir(t.TempDir())
	saveHeroReadFailure(diagnostic.after.frame, diagnostic.readError)
	paths, err := filepath.Glob("artifacts/hero-unreadable-*.png")
	if err != nil || len(paths) != 1 {
		t.Fatalf("saved failure frames: %v %v", paths, err)
	}
	f, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved, err := png.Decode(f)
	if err != nil || saved.Bounds() != screen.Bounds() {
		t.Fatalf("saved frame bounds: %v", err)
	}
	r, g, b, a := screen.At(204, 655).RGBA()
	sr, sg, sb, sa := saved.At(204, 655).RGBA()
	if r != sr || g != sg || b != sb || a != sa {
		t.Fatal("diagnostic changed the screenshot")
	}
}

func TestStartupBottomHeroQueuesExactTarget(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	c, err := recognizedGame(screen)
	if err != nil || !bootstrapHeroes(c) {
		t.Fatalf("native game context: %+v %v", c, err)
	}
	now := time.Now()
	frame := gameFrame{id: 4, layout: 1, at: now, image: screen, context: c}
	captures := 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { captures++; return screen, nil }}, pipelineReaders{heroes: heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}}, pipelineOptions{heroes: true})
	p.frame, p.layout, p.startup, p.startupCheck = frame, frame.layout, startupHeroes, false
	p.hero.startupTop = true
	p.hero.startupDone = map[string]bool{"BrittanyBeachPrincess": true, "TheWanderingFisherman": true, "BettyClicker": true}
	read := func() observation {
		return p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, startupTop: true, startupVisits: p.hero.startupVisits()})
	}
	out := read()
	if out.err != nil || !out.hero.found || out.hero.startupName != "TheMaskedSamurai" || out.hero.owned || out.hero.level != 0 || out.hero.startupComplete {
		t.Fatalf("native bottom target: %+v %v", out.hero, out.err)
	}
	if out.hero.button.Y <= 1327 || out.hero.button.Y >= screen.Bounds().Max.Y-20 {
		t.Fatalf("truncated detector center retained: %v", out.hero.button)
	}
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != buyHero || a.hero.startupName != "TheMaskedSamurai" || a.point != out.hero.button || captures != 0 {
		t.Fatalf("queued native target: %+v %t captures=%d", a, ok, captures)
	}
	// Interrupt before dispatch: the same pending hero remains eligible and visits survive.
	p.hero.interrupt()
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	out = read()
	if err := p.accept(context.Background(), out, p.frame.at); err != nil {
		t.Fatal(err)
	}
	p.plan(p.frame.at)
	a, ok = p.nextAction(p.frame.at)
	if !ok || a.kind != buyHero || a.hero.startupName != "TheMaskedSamurai" || p.hero.nextScan.After(p.frame.at) || len(p.diagnostics) != 0 || p.startup != startupHeroes {
		t.Fatal("F8 lost the target or introduced an unreadable/backoff loop")
	}
}

func TestStartupOverlapScrollPreservesHeroVisits(t *testing.T) {
	requireAncientOCR(t)
	beforeScreen := loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	afterScreen := loadTestImage(t, "testdata/hero-startup-gilded-unhired.png")
	now := time.Now()
	frame := gameFrame{id: 4, layout: 1, at: now, image: beforeScreen, context: gameContext{known: true, heroes: true, bounds: beforeScreen.Bounds()}}
	drags := 0
	p := newGamePipeline(&pauseControl{}, heroInput{drag: func(from, to image.Point) error { drags++; return nil }}, pipelineReaders{heroes: heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}}, pipelineOptions{heroes: true})
	p.frame, p.layout, p.startup, p.startupCheck = frame, frame.layout, startupHeroes, false
	p.hero.startupTop = true
	p.hero.startupDone = map[string]bool{"BrittanyBeachPrincess": true, "TheWanderingFisherman": true, "BettyClicker": true}
	thumb, height, found := heroScrollbarThumb(beforeScreen)
	if !found {
		t.Fatal("native scrollbar missing")
	}
	// Scheduling uses a modeled cropped-row result; the next target uses real OCR.
	partial := heroObservation{frame: frame, startup: true, x1: true, thumbFound: true, thumb: thumb, startupScroll: image.Pt(thumb.X, thumb.Y+height/2)}
	if err := p.accept(context.Background(), observation{kind: heroAnalysis, startup: startupHeroes, frame: frame, hero: partial}, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != scrollHeroes || a.target.Y <= a.point.Y || p.hero.nextScan.After(now) {
		t.Fatalf("partial row did not queue forward navigation: %+v %t", a, ok)
	}
	acted, err := p.execute(context.Background(), a)
	if err != nil || !acted || drags != 1 {
		t.Fatalf("navigation input: %t %v drags=%d", acted, err, drags)
	}
	p.actionCompleted(actionResult{action: a, acted: acted}, now)
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	p.frame.image = afterScreen
	out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, startupTop: true, startupVisits: p.hero.startupVisits()})
	if out.err != nil || !out.hero.found || out.hero.startupName != "TheMaskedSamurai" {
		t.Fatalf("post-scroll native target: %+v %v", out.hero, out.err)
	}
	if err := p.accept(context.Background(), out, p.frame.at); err != nil {
		t.Fatal(err)
	}
	p.hero.interrupt()
	if len(p.hero.startupVisits()) != 3 || p.hero.pending != nil {
		t.Fatal("F8 lost confirmed visits or left a scroll pending")
	}
	p.frame.id++
	p.frame.at = p.frame.at.Add(time.Second)
	out = p.analyze(context.Background(), heroAnalysis, analysisJob{frame: p.frame, startup: startupHeroes, startupTop: true, startupVisits: p.hero.startupVisits()})
	if err := p.accept(context.Background(), out, p.frame.at); err != nil {
		t.Fatal(err)
	}
	p.plan(p.frame.at)
	a, ok = p.nextAction(p.frame.at)
	if !ok || a.kind != buyHero || a.hero.startupName != "TheMaskedSamurai" || drags != 1 || len(p.diagnostics) != 0 {
		t.Fatalf("navigation/F8 entered a repeat-scroll loop: %+v %t drags=%d", a, ok, drags)
	}
}
