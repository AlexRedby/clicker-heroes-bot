package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"
)

func TestStartupHeroNativeSweep(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	frame := func(path string, id uint64) gameFrame {
		s := loadTestImage(t, path)
		return gameFrame{id: id, at: now.Add(time.Duration(id) * time.Second), image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	read := heroReaders{gold: readHeroGold, price: readHeroPrice, level: func(context.Context, image.Image, image.Point) (int, error) {
		t.Fatal("HIRE must not invoke numeric level OCR")
		return 0, nil
	}}
	zero := frame("testdata/hero-startup-zero.png", 1)
	first, err := readStartupHeroObservation(ctx, zero, read, nil, nil, false)
	if err != nil || !first.found || first.owned || first.startupName != "Cid,theHelpfulAdventurer" || first.passiveReady || first.startupComplete {
		t.Fatalf("free Cid: %+v %v", first, err)
	}
	// A cached visit cannot turn an observed HIRE into an owned hero.
	stale, staleErr := readStartupHeroObservation(ctx, zero, read, nil, map[string]bool{"Cid,theHelpfulAdventurer": true}, true)
	if staleErr == nil || stale.found || stale.startupComplete || stale.passiveReady {
		t.Fatalf("stale visit accepted HIRE: %+v %v", stale, staleErr)
	}
	gold := frame("testdata/hero-startup-gold.png", 2)
	for _, want := range []struct {
		y    int
		name string
	}{{655, "Cid,theHelpfulAdventurer"}, {865, "Treebeast"}, {1075, "IvantheDrunkenBrawler"}, {1285, "BrittanyBeachPrincess"}} {
		button := image.Pt(204, want.y)
		name, err := readStartupHeroName(ctx, gold.image, button)
		if err != nil || name != want.name {
			t.Fatalf("native HIRE identity: %q %v", name, err)
		}
		if level, err := readStartupHeroLevel(ctx, gold.image, button, read); err != nil || level != 0 {
			t.Fatalf("native HIRE level: %d %v", level, err)
		}
	}
	p := heroRunner{enabled: true}
	p.startStartup()
	p.observe(first, observation{}, now)
	a, ok := p.action(now)
	if !ok || a.kind != buyHero {
		t.Fatal("native Cid purchase not queued")
	}
	p.sent(a, now)
	unchangedFrame := zero
	unchangedFrame.id++
	unchanged, err := readStartupHeroObservation(ctx, unchangedFrame, read, &first, nil, true)
	if err != nil || !unchanged.stable || unchanged.owned || unchanged.level != 0 {
		t.Fatalf("unchanged HIRE: %+v %v", unchanged, err)
	}
	p.observe(unchanged, observation{}, unchangedFrame.at)
	if p.pending == nil || p.startupDone[first.startupName] {
		t.Fatal("unchanged HIRE confirmed purchase")
	}
	p.interrupt()
	p.startupDone["Treebeast"] = true
	snapshot := p.startupVisits()
	delete(snapshot, "Treebeast")
	if !p.startupDone["Treebeast"] {
		t.Fatal("worker visit map was shared")
	}
	p.interrupt()
	if !p.startupDone["Treebeast"] {
		t.Fatal("F8 lost confirmed visit")
	}
	p.startStartup()
	if len(p.startupDone) != 0 {
		t.Fatal("new reset kept old visits")
	}
	covered := image.NewRGBA(zero.image.Bounds())
	draw.Draw(covered, covered.Bounds(), zero.image, covered.Bounds().Min, draw.Src)
	draw.Draw(covered, startupHeroNameRegion(covered, first.button), image.NewUniform(color.Black), image.Point{}, draw.Src)
	other := zero
	other.image = covered
	if startupHeroStable(first, other) {
		t.Fatal("obscured name accepted")
	}
	quantity := first
	quantity.x1 = false
	p.latest = quantity
	if a, ok := p.action(now); !ok || a.kind != selectQuantity {
		t.Fatal("x1 not restored")
	}
	mature := frame("testdata/hero-tsuchi-x1.png", 99)
	unsupported, err := readStartupHeroObservation(ctx, mature, read, nil, nil, false)
	if err != nil || unsupported.found || unsupported.startupComplete || unsupported.startupScroll == (image.Point{}) {
		t.Fatalf("mature list did not seek top: %+v %v", unsupported, err)
	}
}

// Input scheduling is modeled here; native HIRE -> MAX pixels are tested separately.
func TestStartupHeroSingleLevelHireNeedsOwnedPass(t *testing.T) {
	now := time.Now()
	p := heroRunner{enabled: true}
	p.startStartup()
	before := heroObservation{frame: gameFrame{id: 1, at: now}, startup: true, x1: true, found: true, startupName: "Treebeast", button: image.Pt(204, 865)}
	p.observe(before, observation{}, now)
	a, ok := p.action(now)
	if !ok || a.kind != buyHero {
		t.Fatal("hire action missing")
	}
	p.sent(a, now)
	owned := before
	owned.frame.id++
	owned.frame.at = now.Add(time.Second)
	owned.level, owned.owned, owned.stable = 1, true, true
	p.observe(owned, observation{}, owned.frame.at)
	if p.pending != nil || p.startupDone[before.startupName] {
		t.Fatal("single-level hire finished MAX visit")
	}
	p.observe(owned, observation{}, owned.frame.at)
	a, ok = p.action(owned.frame.at)
	if !ok || a.kind != buyHero {
		t.Fatal("owned MAX action missing")
	}
	p.sent(a, owned.frame.at)
	maxed := owned
	maxed.frame.id++
	maxed.frame.at = maxed.frame.at.Add(time.Second)
	maxed.level = 1000
	p.observe(maxed, observation{}, maxed.frame.at)
	if p.pending != nil || !p.startupDone[before.startupName] {
		t.Fatal("owned MAX visit not recorded")
	}
}

func TestStartupHeroBlockedSuccessorAndBulk(t *testing.T) {
	ctx := context.Background()
	s := loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	b := s.Bounds()
	dark := image.NewRGBA(b)
	draw.Draw(dark, b, s, b.Min, draw.Src)
	// Preserve native labels while making Samurai's button unavailable.
	for y := 1290; y < 1390; y++ {
		for x := 140; x < 350; x++ {
			r, g, blue := rgb(dark.At(x, y))
			if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
				dark.Set(x, y, color.RGBA{R: 45, G: 60, B: 70, A: 255})
			}
		}
	}
	f := gameFrame{id: 10, image: dark, context: gameContext{known: true, heroes: true, bounds: b}}
	visited := map[string]bool{"BrittanyBeachPrincess": true, "TheWanderingFisherman": true, "BettyClicker": true}
	read := heroReaders{
		level: func(context.Context, image.Image, image.Point) (int, error) {
			t.Fatal("visited/disabled owned rows must not invoke numeric level OCR")
			return 0, nil
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

func TestStartupHeroNativeHireState(t *testing.T) {
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	for _, tc := range []struct {
		path string
		y    int
		name string
	}{
		{"testdata/hero-startup-gilded-unhired.png", 736, "TheMaskedSamurai"},
		{"testdata/hero-startup-gilded-unhired.png", 945, "Leon"},
		{"testdata/hero-startup-gilded-unhired.png", 1158, "TheGreatForestSeer"},
		{"testdata/hero-startup-fisherman-before.png", 966, "TheWanderingFisherman"},
		{"testdata/hero-startup-fisherman-before.png", 1152, "BettyClicker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := loadTestImage(t, tc.path)
			button := image.Pt(204, tc.y)
			name, err := readStartupHeroName(ctx, s, button)
			if err != nil || name != tc.name {
				t.Fatalf("identity: %q %v", name, err)
			}
			level, err := readStartupHeroLevel(ctx, s, button, read)
			if err != nil || level != 0 {
				t.Fatalf("visible bare zero: %d %v", level, err)
			}
			noHire := image.NewRGBA(s.Bounds())
			draw.Draw(noHire, noHire.Bounds(), s, s.Bounds().Min, draw.Src)
			draw.Draw(noHire, heroButtonCaptionRegion(s, button), image.NewUniform(color.Black), image.Point{}, draw.Src)
			if _, err := readStartupHeroLevel(ctx, noHire, button, read); err == nil {
				t.Fatal("bare zero without HIRE accepted")
			}
			covered := image.NewRGBA(s.Bounds())
			draw.Draw(covered, covered.Bounds(), s, s.Bounds().Min, draw.Src)
			// The observed HIRE caption establishes zero without reading the numeral.
			for _, fill := range []color.Color{color.Black, color.RGBA{R: 255, G: 224, B: 95, A: 255}} {
				draw.Draw(covered, image.Rect(386, button.Y-80, 412, button.Y+30), image.NewUniform(fill), image.Point{}, draw.Src)
				if level, err := readStartupHeroLevel(ctx, covered, button, read); err != nil || level != 0 {
					t.Fatalf("HIRE depended on numeral OCR: %d %v", level, err)
				}
			}
		})
	}
}

func TestStartupHeroNativeMovedHire(t *testing.T) {
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	frame := func(path string, id uint64) gameFrame {
		s := loadTestImage(t, path)
		return gameFrame{id: id, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	beforeFrame := frame("testdata/hero-startup-fisherman-before.png", 1)
	afterFrame := frame("testdata/hero-startup-fisherman-after.png", 2)
	visited := map[string]bool{"BrittanyBeachPrincess": true}
	before, err := readStartupHeroObservation(ctx, beforeFrame, read, nil, visited, true)
	if err != nil || !before.found || before.owned || before.level != 0 || before.startupName != "TheWanderingFisherman" {
		t.Fatalf("before hire: %+v %v", before, err)
	}
	unchanged, err := readStartupHeroObservation(ctx, beforeFrame, read, &before, visited, true)
	if err != nil || !unchanged.stable || unchanged.owned || unchanged.level != 0 {
		t.Fatalf("unchanged hire: %+v %v", unchanged, err)
	}
	after, err := readStartupHeroObservation(ctx, afterFrame, read, &before, visited, true)
	if err != nil || !after.stable || !after.owned || after.level != 10000 || after.startupName != before.startupName || after.button == before.button {
		t.Fatalf("moved native hire: %+v %v", after, err)
	}
	for _, tc := range []struct {
		name   string
		region image.Rectangle
	}{
		{"name", startupHeroNameRegion(afterFrame.image, after.button)},
		{"level", image.Rect(665, after.button.Y-86, 973, after.button.Y+43)},
	} {
		t.Run("covered-"+tc.name, func(t *testing.T) {
			covered := image.NewRGBA(afterFrame.image.Bounds())
			draw.Draw(covered, covered.Bounds(), afterFrame.image, covered.Bounds().Min, draw.Src)
			draw.Draw(covered, tc.region, image.NewUniform(color.Black), image.Point{}, draw.Src)
			f := afterFrame
			f.image = covered
			o, err := readStartupHeroObservation(ctx, f, read, &before, visited, true)
			if err == nil && o.stable && o.owned {
				t.Fatalf("covered %s confirmed: %+v", tc.name, o)
			}
		})
	}
	wrong := frame("testdata/hero-startup-gilded-unhired.png", 3)
	out, err := readStartupHeroObservation(ctx, wrong, read, &before, visited, true)
	if err == nil && out.stable {
		t.Fatal("different hero confirmed hire")
	}
}

// Translate original card pixels while preserving the real HUD and scrollbar.
// No text is synthesized; positive identity and level assertions use real OCR.
func startupTranslatedCard(t *testing.T, path string, sourceY, targetY int, dark bool) image.Image {
	t.Helper()
	s := loadTestImage(t, path)
	b := s.Bounds()
	base := s
	if b.Dx() == 2560 {
		base = loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	}
	out := image.NewRGBA(b)
	draw.Draw(out, b, base, b.Min, draw.Src)
	viewport := heroListViewport(out)
	viewport.Min.X, viewport.Max.X = b.Min.X+b.Dx()*35/1000, b.Min.X+b.Dx()*44/100
	draw.Draw(out, viewport, image.NewUniform(color.RGBA{R: 255, G: 224, B: 95, A: 255}), image.Point{}, draw.Src)
	source := image.Rect(viewport.Min.X, sourceY-b.Dy()*75/1000, viewport.Max.X, sourceY+b.Dy()*85/1000).Intersect(b)
	destination := source.Add(image.Pt(0, targetY-sourceY)).Intersect(viewport)
	draw.Draw(out, destination, s, destination.Min.Sub(image.Pt(0, targetY-sourceY)), draw.Src)
	if dark {
		for y := viewport.Min.Y; y < viewport.Max.Y; y++ {
			for x := b.Min.X + b.Dx()*55/1000; x < b.Min.X+b.Dx()*140/1000; x++ {
				r, g, blue := rgb(out.At(x, y))
				if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
					out.Set(x, y, color.RGBA{R: 45, G: 60, B: 70, A: 255})
				}
			}
		}
	}
	return out
}

func TestStartupHeroViewportMatrix(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	for _, card := range []struct {
		path, name string
		y, level   int
	}{
		{"testdata/hero-startup-zero.png", "Cid,theHelpfulAdventurer", 655, 0},
		{"testdata/hero-startup-bottom-hire.png", "TheMaskedSamurai", 1337, 0},
		{"testdata/hero-startup-bottom-hire.png", "BrittanyBeachPrincess", 709, 23975},
		{"testdata/hero-nongilded-successor.jpg", "Skogur", 446, 99},
		{"testdata/hero-nongilded-successor.jpg", "Moeru", 563, 0},
	} {
		for _, position := range []struct {
			name string
			y    int
		}{{"top", 455}, {"middle", 638}, {"bottom", 929}} {
			for _, dark := range []bool{false, true} {
				// Moeru is an actual disabled fixture; preserve that native state.
				if card.name == "Moeru" && !dark {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/dark=%t", card.name, position.name, dark), func(t *testing.T) {
					native := loadTestImage(t, card.path)
					target := native.Bounds().Dy() * position.y / 1000
					s := startupTranslatedCard(t, card.path, card.y, target, dark)
					buttons := findHeroButtons(s, !dark)
					if len(buttons) != 1 {
						t.Fatalf("observed buttons: %v", buttons)
					}
					if opposite := findHeroButtons(s, dark); len(opposite) != 0 {
						t.Fatalf("button classified twice: %v", opposite)
					}
					button := buttons[0]
					name, err := readStartupHeroName(ctx, s, button)
					if err != nil || name != card.name {
						t.Fatalf("identity at %v: %q %v", button, name, err)
					}
					level, err := readStartupHeroLevel(ctx, s, button, read)
					if err != nil || level != card.level {
						t.Fatalf("level at %v: %d %v", button, level, err)
					}
				})
			}
		}
	}
}

func TestStartupHeroBoundaryNavigation(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	frame := func(s image.Image) gameFrame {
		return gameFrame{id: 1, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	s := loadTestImage(t, "testdata/hero-startup-bottom-hire.png")
	visited := map[string]bool{"BrittanyBeachPrincess": true, "TheWanderingFisherman": true, "BettyClicker": true}
	out, err := readStartupHeroObservation(ctx, frame(s), read, nil, visited, true)
	if err != nil || !out.found || out.startupName != "TheMaskedSamurai" || out.button.Y <= 1327 || out.owned || out.level != 0 || out.startupComplete {
		t.Fatalf("actual bottom frame: %+v %v", out, err)
	}
	// The old truncated-band point must not yield an invented roster identity.
	// Name localization may recover the real hero, but arbitrary OCR is forbidden.
	name, err := readStartupHeroName(ctx, s, image.Pt(204, 1327))
	if err == nil && name != "TheMaskedSamurai" {
		t.Fatalf("invented hero identity: %q", name)
	}
	bottom := startupTranslatedCard(t, "testdata/hero-startup-bottom-hire.png", 1337, 1435, false)
	clipped, err := readStartupHeroObservation(ctx, frame(bottom), read, nil, nil, true)
	if err != nil || clipped.found || clipped.startupComplete || clipped.startupScroll.Y <= clipped.thumb.Y {
		t.Fatalf("bottom-clipped target: %+v %v", clipped, err)
	}
	// Initial mid-list startup seeks the top; no mid-list purchase can precede Cid.
	initial, err := readStartupHeroObservation(ctx, frame(s), read, nil, nil, false)
	if err != nil || initial.found || initial.startupComplete || initial.startupScroll.Y >= initial.thumb.Y || initial.startupScroll == (image.Point{}) {
		t.Fatalf("initial top seek: %+v %v", initial, err)
	}
	// After a confirmed overlapping scroll, the incomplete top card is behind
	// the sweep. It must not pull the scrollbar up and oscillate.
	top := startupTranslatedCard(t, "testdata/hero-startup-bottom-hire.png", 709, 570, false)
	mid := startupTranslatedCard(t, "testdata/hero-startup-bottom-hire.png", 1337, 920, false)
	combined := image.NewRGBA(s.Bounds())
	draw.Draw(combined, combined.Bounds(), top, top.Bounds().Min, draw.Src)
	draw.Draw(combined, image.Rect(89, 810, 1126, 1050), mid, image.Pt(89, 810), draw.Src)
	next, err := readStartupHeroObservation(ctx, frame(combined), read, nil, visited, true)
	if err != nil || !next.found || next.startupName != "TheMaskedSamurai" || next.startupScroll != (image.Point{}) || next.startupComplete {
		t.Fatalf("monotonic overlap: %+v %v", next, err)
	}
}

func TestStartupHeroRosterIdentity(t *testing.T) {
	if startupHeroNamesErr != nil || len(startupHeroNames) != 54 {
		t.Fatalf("official roster unavailable: %d %v", len(startupHeroNames), startupHeroNamesErr)
	}
	for raw, want := range map[string]string{
		"Cid, the Helpful Adventurer": "Cid,theHelpfulAdventurer",
		"CidtheHelpfulAdventurer":     "Cid,theHelpfulAdventurer",
		"Ivan, the Drunken Brawler":   "IvantheDrunkenBrawler",
		"Brittany, Beach Princess":    "BrittanyBeachPrincess",
		"The Masked Samurai":          "TheMaskedSamurai",
	} {
		if got := startupHeroNames[strings.ToLower(startupHeroLetters(raw))]; got != want {
			t.Fatalf("%q mapped to %q, want %q", raw, got, want)
		}
	}
	for _, raw := range []string{"hBoawlenalBeeen", "IvantheDrunkenBrawiler", "HIRE", "", "TheMaskedSamura"} {
		if _, known := startupHeroNames[strings.ToLower(startupHeroLetters(raw))]; known {
			t.Fatalf("unknown OCR identity accepted: %q", raw)
		}
	}
}

func TestStartupHeroViewportObstruction(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	for _, tc := range []struct {
		name, path string
		sourceY    int
		field      string
	}{
		{"name", "testdata/hero-startup-bottom-hire.png", 1337, "name"},
		{"HIRE", "testdata/hero-startup-bottom-hire.png", 1337, "caption"},
		{"LVL", "testdata/hero-startup-bottom-hire.png", 709, "level"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := startupTranslatedCard(t, tc.path, tc.sourceY, 920, false)
			button := findHeroButtons(s, true)[0]
			region := startupHeroNameRegion(s, button)
			switch tc.field {
			case "caption":
				region = image.Rect(179, button.Y-40, 294, button.Y+10)
			case "level":
				region = image.Rect(665, button.Y-29, 973, button.Y+21)
			}
			covered := image.NewRGBA(s.Bounds())
			draw.Draw(covered, covered.Bounds(), s, s.Bounds().Min, draw.Src)
			fill := color.Color(color.Black)
			if tc.field == "caption" {
				// Cover the letters without changing the blue button band geometry.
				fill = color.RGBA{R: 98, G: 190, B: 247, A: 255}
			}
			if tc.field == "level" {
				fill = color.RGBA{R: 255, G: 224, B: 95, A: 255}
			}
			draw.Draw(covered, region, image.NewUniform(fill), image.Point{}, draw.Src)
			f := gameFrame{id: 1, image: covered, context: gameContext{known: true, heroes: true, bounds: covered.Bounds()}}
			o, err := readStartupHeroObservation(ctx, f, read, nil, nil, true)
			if err == nil || o.found || o.startupComplete || o.startupScroll != (image.Point{}) {
				t.Fatalf("middle obstruction accepted: %+v %v", o, err)
			}
			if tc.field == "caption" && (!strings.Contains(err.Error(), "crop") || !strings.Contains(err.Error(), "caption")) {
				t.Fatalf("HIRE crop/raw evidence missing: %v", err)
			}
			// A blocked modal prevents all hero OCR and input decisions.
			f.context.saveMenu = true
			o, err = readStartupHeroObservation(ctx, f, heroReaders{}, nil, nil, true)
			if err != nil || o.found || o.startupComplete || o.startupScroll != (image.Point{}) {
				t.Fatalf("modal consumed hero fields: %+v %v", o, err)
			}
		})
	}
}

func TestStartupHeroBoundaryHireConfirmation(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	read := heroReaders{level: readHeroLevel, gold: readHeroGold, price: readHeroPrice}
	frame := func(s image.Image, id uint64) gameFrame {
		return gameFrame{id: id, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	beforeFrame := frame(startupTranslatedCard(t, "testdata/hero-startup-fisherman-before.png", 966, 1337, false), 1)
	before, err := readStartupHeroObservation(ctx, beforeFrame, read, nil, nil, true)
	if err != nil || !before.found || before.owned || before.startupName != "TheWanderingFisherman" {
		t.Fatalf("before boundary hire: %+v %v", before, err)
	}
	// Apply the same translation to the actual after frame; the real card/button
	// geometry changes, preserving the native HIRE -> 10000 evidence.
	afterFrame := frame(startupTranslatedCard(t, "testdata/hero-startup-fisherman-after.png", 943, 1337-(966-943), false), 2)
	after, err := readStartupHeroObservation(ctx, afterFrame, read, &before, nil, true)
	if err != nil || !after.stable || !after.owned || after.level != 10000 || after.startupName != before.startupName || after.button == before.button {
		t.Fatalf("boundary hire confirmation: %+v %v", after, err)
	}
	clippedFrame := frame(startupTranslatedCard(t, "testdata/hero-startup-fisherman-after.png", 943, 1455, false), 3)
	unknown, err := readStartupHeroObservation(ctx, clippedFrame, read, &before, nil, true)
	if err == nil || unknown.owned || unknown.startupComplete {
		t.Fatalf("clipped confirmation credited: %+v %v", unknown, err)
	}
}
