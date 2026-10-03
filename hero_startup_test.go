package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestStartupHeroNativeSweep(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	frame := func(path string, id uint64) gameFrame {
		s := loadTestImage(t, path)
		return gameFrame{id: id, at: now.Add(time.Duration(id) * time.Second), image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	real := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	zero := frame("testdata/hero-startup-zero.png", 1)
	first, err := readStartupHeroObservation(ctx, zero, real, nil, nil, false)
	if err != nil || !first.found || first.owned || first.startupName != "Cid,theHelpfulAdventurer" || first.passiveReady || first.startupComplete {
		t.Fatalf("free Cid: %+v %v", first, err)
	}
	shortRead := real
	shortRead.level = func(context.Context, image.Image, image.Point) (int, error) { return 1000, nil }
	short, shortErr := readStartupHeroObservation(ctx, zero, shortRead, nil, map[string]bool{"Cid,theHelpfulAdventurer": true}, true)
	if shortErr != nil || short.found || short.startupComplete || short.passiveReady {
		t.Fatalf("known completed short list: %+v %v", short, shortErr)
	}
	gold := frame("testdata/hero-startup-gold.png", 2)
	// Native before-hire frames verify names and bare zero. The level reader stub
	// models confirmed LVL results; a native post-hire pair is still a live gate.
	levels := map[int]int{}
	read := real
	read.level = func(ctx context.Context, s image.Image, p image.Point) (int, error) {
		if n := levels[p.Y]; n > 0 {
			return n, nil
		}
		return readHeroLevel(ctx, s, p)
	}
	p := heroRunner{enabled: true}
	p.startStartup()
	for i, want := range []struct {
		y    int
		name string
	}{{655, "Cid,theHelpfulAdventurer"}, {865, "Treebeast"}, {1075, "IvantheDrunkenBrawler"}, {1285, "BrittanyBeachPrincess"}} {
		gold.id = uint64(3 + i*4)
		o, e := readStartupHeroObservation(ctx, gold, read, nil, p.startupVisits(), p.startupTop)
		if e != nil || !o.found || absDiff(o.button.Y, want.y) > 3 || o.startupName != want.name || o.startupComplete {
			t.Fatalf("row %d: %+v %v", i, o, e)
		}
		p.nextScan = time.Time{}
		p.observe(o, observation{}, now)
		a, ok := p.action(now)
		if !ok || a.kind != buyHero {
			t.Fatalf("row %d action: %+v %t", i, a, ok)
		}
		p.sent(a, now)
		// A unchanged zero must never confirm input.
		after := gold
		after.id++
		unchanged, e := readStartupHeroObservation(ctx, after, read, &o, p.startupVisits(), p.startupTop)
		if e != nil || !unchanged.stable || unchanged.level != 0 {
			t.Fatalf("unchanged: %+v %v", unchanged, e)
		}
		p.observe(unchanged, observation{}, after.at)
		if p.pending == nil {
			t.Fatal("zero level confirmed hire")
		}
		levels[o.button.Y] = 1
		after.id++
		confirmed, e := readStartupHeroObservation(ctx, after, read, &o, p.startupVisits(), p.startupTop)
		if e != nil || !confirmed.stable || !confirmed.owned {
			t.Fatalf("hire confirmation: %+v %v", confirmed, e)
		}
		p.observe(confirmed, observation{}, after.at)
		if p.pending != nil || p.startupDone[o.startupName] {
			t.Fatal("hire alone finished MAX visit")
		}
		// Hire and owned MAX are distinct: confirm owned MAX before marking visited.
		owned, e := readStartupHeroObservation(ctx, after, read, nil, p.startupVisits(), p.startupTop)
		if e != nil || !owned.found || !owned.owned || owned.startupName != o.startupName {
			t.Fatalf("owned row: %+v %v", owned, e)
		}
		p.nextScan = time.Time{}
		p.observe(owned, observation{}, now)
		a, ok = p.action(now)
		if !ok || a.kind != buyHero {
			t.Fatal("owned MAX missing")
		}
		p.sent(a, now)
		levels[o.button.Y] = 1000
		after.id++
		done, e := readStartupHeroObservation(ctx, after, read, &owned, p.startupVisits(), p.startupTop)
		if e != nil {
			t.Fatal(e)
		}
		p.observe(done, observation{}, after.at)
		if !p.startupDone[o.startupName] {
			t.Fatal("owned MAX visit not recorded")
		}
		if i == 1 && done.startupComplete {
			t.Fatal("Treebeast completed the sweep")
		}
	}
	next, e := readStartupHeroObservation(ctx, gold, read, nil, p.startupVisits(), p.startupTop)
	if e != nil || next.startupComplete || next.found || next.startupScroll == (image.Point{}) {
		t.Fatalf("overlap scroll: %+v %v", next, e)
	}
	p.nextScan = time.Time{}
	p.observe(next, observation{}, now)
	a, ok := p.action(now)
	if !ok || a.kind != scrollHeroes || a.target.Y-a.point.Y > 200 {
		t.Fatalf("skipped-page scroll: %+v %t", a, ok)
	}
	snapshot := p.startupVisits()
	p.interrupt()
	if len(p.startupDone) != 4 {
		t.Fatal("F8 lost confirmed visits")
	}
	delete(snapshot, "Treebeast")
	if !p.startupDone["Treebeast"] {
		t.Fatal("worker map was shared")
	}
	p.startStartup()
	if len(p.startupDone) != 0 {
		t.Fatal("new reset kept old visits")
	}
	b := gold.image.Bounds()
	covered := image.NewRGBA(b)
	draw.Draw(covered, b, gold.image, b.Min, draw.Src)
	draw.Draw(covered, startupHeroNameRegion(covered, first.button), image.NewUniform(color.Black), image.Point{}, draw.Src)
	other := gold
	other.image = covered
	if startupHeroStable(first, other) {
		t.Fatal("obscured name accepted")
	}
	quantity := first
	quantity.x1 = false
	p.latest = quantity
	p.nextScan = time.Time{}
	if a, ok := p.action(now); !ok || a.kind != selectQuantity {
		t.Fatal("x1 not restored")
	}
	mature := frame("testdata/hero-tsuchi-x1.png", 99)
	unsupported, e := readStartupHeroObservation(ctx, mature, real, nil, nil, false)
	if e != nil || unsupported.found || unsupported.startupComplete || unsupported.startupScroll == (image.Point{}) {
		t.Fatalf("mature list did not seek top: %+v %v", unsupported, e)
	}
}

func TestStartupHeroBlockedSuccessorAndBulk(t *testing.T) {
	ctx := context.Background()
	s := loadTestImage(t, "testdata/hero-startup-gold.png")
	b := s.Bounds()
	dark := image.NewRGBA(b)
	draw.Draw(dark, b, s, b.Min, draw.Src)
	// Preserve native labels while making Ivan's button unavailable.
	for y := 1010; y < 1140; y++ {
		for x := 140; x < 350; x++ {
			r, g, blue := rgb(dark.At(x, y))
			if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
				dark.Set(x, y, color.RGBA{R: 45, G: 60, B: 70, A: 255})
			}
		}
	}
	f := gameFrame{id: 10, image: dark, context: gameContext{known: true, heroes: true, bounds: b}}
	visited := map[string]bool{"Cid,theHelpfulAdventurer": true, "Treebeast": true}
	read := heroReaders{
		level: func(ctx context.Context, s image.Image, p image.Point) (int, error) {
			if p.Y < 900 {
				return 1000, nil
			}
			return readHeroLevel(ctx, s, p)
		},
		price: func(context.Context, image.Image, image.Point) (float64, error) { return 2, nil },
		gold:  func(context.Context, image.Image) (float64, error) { return 1, nil },
	}
	out, err := readStartupHeroObservation(ctx, f, read, nil, visited, true)
	if err != nil || !out.startupComplete || !out.passiveReady || out.found {
		t.Fatalf("blocked successor: %+v %v", out, err)
	}
	read.gold = func(context.Context, image.Image) (float64, error) { return 3, nil }
	out, err = readStartupHeroObservation(ctx, f, read, nil, visited, true)
	if err != nil || out.startupComplete {
		t.Fatalf("dark but affordable: %+v %v", out, err)
	}
	read.price = func(context.Context, image.Image, image.Point) (float64, error) { return 0, errUnreadableGameNumber }
	out, err = readStartupHeroObservation(ctx, f, read, nil, visited, true)
	if err == nil || out.startupComplete {
		t.Fatal("unknown price completed startup")
	}
	wrongQuantity := image.NewRGBA(b)
	draw.Draw(wrongQuantity, b, s, b.Min, draw.Src)
	wrongQuantity.Set(b.Min.X+b.Dx()*97/1000, b.Min.Y+b.Dy()*345/1000, color.RGBA{R: 255, G: 230, A: 255})
	qframe := f
	qframe.image = wrongQuantity
	quantityOut, quantityErr := readStartupHeroObservation(ctx, qframe, heroReaders{}, nil, visited, true)
	if quantityErr != nil || quantityOut.x1 || quantityOut.startupComplete {
		t.Fatal("non-x1 reader consumed numeric fields")
	}
	qp := heroRunner{enabled: true, latest: quantityOut}
	if a, ok := qp.action(time.Now()); !ok || a.kind != selectQuantity {
		t.Fatal("startup quantity gate missing")
	}
	zero := loadTestImage(t, "testdata/hero-startup-zero.png")
	point, found, err := readHeroUpgradeButton(ctx, zero)
	if err != nil || !found || absDiff(point.Y, 868) > 12 {
		t.Fatalf("bulk footer: %v %t %v", point, found, err)
	}
	if !heroUpgradeButtonStable(zero, zero, point) {
		t.Fatal("same footer rejected")
	}
	covered := image.NewRGBA(zero.Bounds())
	draw.Draw(covered, covered.Bounds(), zero, zero.Bounds().Min, draw.Src)
	draw.Draw(covered, heroUpgradeButtonRegion(covered, point), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if heroUpgradeButtonStable(zero, covered, point) || heroUpgradeButtonStable(nil, zero, point) {
		t.Fatal("unknown footer accepted")
	}
	if _, found, err = readHeroUpgradeButton(ctx, s); err != nil || found {
		t.Fatalf("offscreen footer: %t %v", found, err)
	}
	p := heroRunner{enabled: true}
	first := heroObservation{frame: f, startup: true, thumbFound: true, thumb: image.Pt(1172, 670), startupScroll: image.Pt(1172, 730), x1: true}
	p.latest = first
	a, ok := p.action(time.Now())
	if !ok || a.kind != scrollHeroes {
		t.Fatal("scroll action missing")
	}
	now := time.Now()
	p.sent(a, now)
	unchanged := first
	unchanged.frame.id++
	unchanged.frame.at = now.Add(time.Second)
	p.observe(unchanged, observation{}, unchanged.frame.at)
	if p.pending == nil {
		t.Fatal("unchanged thumb confirmed startup scroll")
	}
}

func TestDisabledHeroUpgradeFooter(t *testing.T) {
	requireAncientOCR(t)
	s := loadTestImage(t, "testdata/hero-startup-zero.png")
	disabled := image.NewRGBA(s.Bounds())
	draw.Draw(disabled, disabled.Bounds(), s, s.Bounds().Min, draw.Src)
	for y := 800; y < 940; y++ {
		for x := 680; x < 1100; x++ {
			r, g, b := rgb(disabled.At(x, y))
			if g > 100 && g > r+50 && g > b+50 {
				disabled.Set(x, y, color.RGBA{R: 60, G: 60, B: 60, A: 255})
			}
		}
	}
	_, known, available, err := readHeroUpgradeFooter(context.Background(), disabled)
	if err != nil || !known || available {
		t.Fatal("disabled footer not recognized", known, available, err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	now := time.Now()
	p.frame = gameFrame{id: 1, at: now, image: disabled, context: gameContext{known: true, heroes: true, bounds: disabled.Bounds()}}
	p.startup, p.startupCheck, p.startupPassive = startupUpgrades, false, true
	p.state[heroAnalysis] = observation{frame: p.frame, upgradesKnown: true}
	p.plan(now)
	if p.startup != startupProgression {
		t.Fatal("disabled footer stalled startup")
	}
	if _, ok := p.nextAction(now); ok {
		t.Fatal("disabled upgrade button clicked")
	}
	p.plan(now.Add(time.Second))
	if p.startup != noStartup || p.controls.isPaused() {
		t.Fatal("upgrade-free setup did not finish")
	}
}

func TestHeroUpgradeFooterAtBottomBoundary(t *testing.T) {
	requireAncientOCR(t)
	s := loadTestImage(t, "testdata/hero-startup-zero.png")
	shifted := image.NewRGBA(s.Bounds())
	draw.Draw(shifted, shifted.Bounds(), s, s.Bounds().Min, draw.Src)
	original := image.Rect(650, 830, 1100, 912)
	draw.Draw(shifted, original, image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(shifted, original.Add(image.Pt(0, 490)), s, original.Min, draw.Src)
	point, known, available, err := readHeroUpgradeFooter(context.Background(), shifted)
	if err != nil || !known || !available || point.Y < 1300 {
		t.Fatal("visible bottom footer skipped", point, known, available, err)
	}
}
