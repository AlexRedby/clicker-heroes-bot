package main

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestViewportNativeGuardAndRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	base := heroInput{keyTap: func(string) error { calls++; return nil }, keyToggle: func(string, string) error { calls++; return nil }, typeText: func(string) error { calls++; return nil }}
	// This identity can never match the native PID/window-number format.
	input := bindViewportInput(ctx, base, viewportGeometry{Scene: nativeScene{Window: "fixture-only"}, ROI: image.Rect(0, 0, 100, 100)})
	for _, action := range []func() error{
		func() error { return input.move(image.Pt(5, 5)) }, func() error { return input.click(image.Pt(5, 5)) },
		func() error { return input.drag(image.Pt(5, 5), image.Pt(10, 10)) }, func() error { return input.keyTap("a") },
		func() error { return input.keyToggle("q", "down") }, func() error { return input.typeText("1") },
	} {
		if err := action(); !errors.Is(err, errInputContext) {
			t.Fatalf("unknown geometry error=%v", err)
		}
	}
	if calls != 0 {
		t.Fatal("unknown geometry reached native input")
	}
	cancel()
	if err := input.keyToggle("q", "down"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled input=%v", err)
	}
	if err := input.keyToggle("q", "up"); err != nil || calls != 1 {
		t.Fatal("cancellation suppressed held-key release")
	}
}

func TestViewportHUDAndDecorations(t *testing.T) {
	game := exportFixture(t, "hero-panel-max.png", 1280)
	for _, frame := range []image.Rectangle{image.Rect(40, 60, 1320, 780), image.Rect(40, 38, 1320, 780), image.Rect(40, 38, 1320, 802)} {
		desktop := image.NewRGBA(image.Rect(0, 0, 1500, 900))
		draw.Draw(desktop, desktop.Bounds(), image.NewUniform(color.RGBA{30, 30, 30, 255}), image.Point{}, draw.Src)
		roi := image.Rect(40, 60, 1320, 780)
		draw.Draw(desktop, roi, game, game.Bounds().Min, draw.Src)
		got, err := findViewport(desktop, frame)
		if err != nil || got != roi {
			t.Fatalf("frame=%v viewport=%v want=%v err=%v", frame, got, roi, err)
		}
		crop := compactCrop(desktop, got)
		old, _ := findHeroLevelButton(game)
		fresh, _ := findHeroLevelButton(crop)
		if old != fresh {
			t.Fatalf("hero coordinate shifted: %v vs %v", old, fresh)
		}
		known, err := recognizedGame(crop)
		if err != nil || !known.heroes {
			t.Fatalf("crop HUD=%+v err=%v", known, err)
		}
	}
	unknown := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	if _, err := findViewport(unknown, unknown.Bounds()); err == nil {
		t.Fatal("unknown desktop accepted")
	}
	modal := exportFixture(t, "ancient-quantity.png", 1280)
	if got, err := findViewport(modal, modal.Bounds()); err != nil || got != modal.Bounds() {
		t.Fatal("verified modal failed to establish a recovery viewport", got, err)
	}
}

func TestViewportHUDAt640(t *testing.T) {
	for _, path := range []string{"testdata/hero-panel-max.png", "testdata/mercenary-collect.png"} {
		im := mercenaryScaled(t, path, 4)
		roi, err := findViewport(im, im.Bounds())
		if err != nil || roi != im.Bounds() {
			t.Fatalf("%s: viewport=%v err=%v", path, roi, err)
		}
		c, err := recognizedGame(im)
		if err != nil || !c.known || c.heroes != (path == "testdata/hero-panel-max.png") || c.mercenaries != (path == "testdata/mercenary-collect.png") {
			t.Fatalf("%s: context=%+v err=%v", path, c, err)
		}
		for _, size := range []image.Point{{639, 360}, {640, 359}} {
			clipped := compactCrop(im, image.Rectangle{Max: size})
			if _, err := findViewport(clipped, clipped.Bounds()); err == nil {
				t.Fatalf("undersized viewport accepted: %v", size)
			}
		}
		covered := image.NewRGBA(im.Bounds())
		draw.Draw(covered, covered.Bounds(), im, im.Bounds().Min, draw.Src)
		draw.Draw(covered, image.Rect(614, 6, 630, 22), image.NewUniform(color.Black), image.Point{}, draw.Src)
		if viewportHUD(covered) {
			t.Fatal("HUD with missing Settings anchor accepted")
		}
	}
	for _, size := range []image.Point{{639, 360}, {640, 359}, {640, 360}} {
		blank := image.NewRGBA(image.Rectangle{Max: size})
		if _, err := findViewport(blank, blank.Bounds()); err == nil {
			t.Fatalf("unknown viewport accepted: %v", size)
		}
	}
	source := mercenaryScaled(t, "testdata/mercenary-revive.png", 4)
	modal := image.NewRGBA(source.Bounds())
	region := image.Rect(160, 120, 480, 230)
	draw.Draw(modal, region, source, region.Min, draw.Src)
	if got, err := findViewport(modal, modal.Bounds()); err != nil || got != modal.Bounds() {
		t.Fatal("verified recovery modal did not establish a viewport", got, err)
	}
}

func TestViewportRelocationInvalidatesPipeline(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{windowed: true, fishInterval: time.Second})
	screen := image.NewRGBA(image.Rect(0, 0, 100, 100))
	g := viewportGeometry{Scene: nativeScene{Window: "game", Display: 1, Bounds: image.Rect(20, 30, 120, 130), Desktop: image.Rect(0, 0, 500, 500)}, Capture: image.Rect(0, 0, 500, 500), ROI: image.Rect(20, 30, 120, 130)}
	p.input.capture = func() (image.Image, error) { return &viewportImage{Image: screen, geometry: g}, nil }
	p.readers.window = func() string { return "game" }
	p.readers.context = func(image.Image) (gameContext, error) { return gameContext{known: true, bounds: screen.Bounds()}, nil }
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	old := p.frame
	p.enqueue(gameAction{kind: collectFish, frame: old, point: image.Pt(5, 5)}, now)
	g.Scene.Bounds = g.Scene.Bounds.Add(image.Pt(20, 0))
	g.ROI = g.ROI.Add(image.Pt(20, 0))
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if p.layout == old.layout || len(p.queue) != 0 || p.frame.context.geometry != g {
		t.Fatal("same-size movement retained old decisions")
	}
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: old, found: true, point: image.Pt(5, 5)}, now); err != nil {
		t.Fatal(err)
	}
	if p.state[fishAnalysis].found {
		t.Fatal("stale fish survived movement")
	}
	// A second native identity check after capture must reject a focus race.
	p.readers.window = func() string { return "other" }
	if err := p.capture(context.Background(), now.Add(2*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if p.controls.isPaused() || p.frame.context.known || len(p.queue) != 0 {
		t.Fatal("capture focus race retained input or stopped observation")
	}
}

func TestViewportInputBinding(t *testing.T) {
	g := viewportGeometry{Scene: nativeScene{Window: "game", Desktop: image.Rect(-500, -300, 0, 200)}, Capture: image.Rect(0, 0, 500, 500), ROI: image.Rect(100, 50, 200, 150)}
	f := testPipelineFrame()
	f.context.geometry = g
	for _, tc := range []struct {
		name   string
		action gameAction
	}{
		{"fish", gameAction{kind: collectFish}},
		{"gild", gameAction{kind: collectGilds}},
		{"ascension", gameAction{kind: handleAscension}},
		{"monster", gameAction{kind: clickMonster}},
		{"park", gameAction{kind: parkPointer}},
		{"hero", gameAction{kind: buyHero}},
		{"scroll", gameAction{kind: scrollHeroes}},
		{"mercenary", gameAction{kind: handleMercenary, mercenary: mercenaryCommand{step: selectMercenaryQuest}}},
		{"ancient", gameAction{kind: handleAncient, ancient: ancientCommand{step: openAncientQuantity}}},
		{"quantity", gameAction{kind: selectQuantity}},
		{"skill", gameAction{kind: castSkill, key: 1}},
		{"progression", gameAction{kind: enableProgression}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			base := heroInput{click: func(image.Point) error { t.Fatal("unbound click"); return nil }, move: func(image.Point) error { t.Fatal("unbound move"); return nil }, keyTap: func(string) error { t.Fatal("unbound key"); return nil }}
			base.bind = func(bound viewportGeometry) heroInput {
				if bound != g {
					t.Fatal("input bound to a different frame")
				}
				pointer := func(point image.Point) error {
					got, err := bound.point(point)
					if err != nil || !got.In(g.Scene.Desktop) {
						t.Fatalf("translation=%v err=%v", got, err)
					}
					calls++
					return nil
				}
				return heroInput{click: pointer, move: pointer, monsterClick: pointer, drag: func(from, to image.Point) error {
					if err := pointer(from); err != nil {
						return err
					}
					return pointer(to)
				}, keyTap: func(string) error { calls++; return nil }, keyToggle: func(string, string) error { calls++; return nil }}
			}
			p := newGamePipeline(&pauseControl{}, base, pipelineReaders{}, pipelineOptions{windowed: true})
			p.frame = f
			a := tc.action
			a.frame = f
			a.point = image.Pt(5, 10)
			a.target = image.Pt(20, 30)
			acted, err := p.execute(context.Background(), a)
			if !acted || err != nil || calls == 0 {
				t.Fatalf("acted=%t calls=%d err=%v", acted, calls, err)
			}
		})
	}
	// Export focus restoration is the sole action allowed without captured game geometry.
	var focused bool
	p := newGamePipeline(&pauseControl{}, heroInput{bind: func(viewportGeometry) heroInput { t.Fatal("bound outside-game focus restoration"); return heroInput{} }, focus: func(string) error { focused = true; return nil }}, pipelineReaders{}, pipelineOptions{windowed: true})
	if acted, err := p.execute(context.Background(), gameAction{kind: handleExport, frame: f, export: &exportCommand{step: exportRestoreGame}}); !acted || err != nil || !focused {
		t.Fatalf("focus restore acted=%t err=%v", acted, err)
	}
}
