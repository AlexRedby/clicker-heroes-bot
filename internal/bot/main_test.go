package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	hook "github.com/robotn/gohook"
)

func TestSaveForNextHero(t *testing.T) {
	if !saveForNextHero(10, 10.9) || !saveForNextHero(10, 9.9) || saveForNextHero(10, 11.1) {
		t.Fatal("the bot should wait only when the next hero costs at most ten times current gold")
	}
}

func TestWriteScreenshotCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifacts", "screenshot.png")
	if err := writeScreenshot(path, []byte("test")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "test" {
		t.Fatalf("screenshot contents = %q, error = %v", data, err)
	}
}

func TestFishClickTracker(t *testing.T) {
	tracker := fishClickTracker{}
	first := image.Pt(700, 400)
	second := image.Pt(850, 120)
	for i, test := range []struct {
		point image.Point
		found bool
		want  bool
	}{
		{first, true, true},
		{first, true, false},
		{first.Add(image.Pt(3, -2)), true, false},
		{second, true, true},
		{second, true, false},
		{image.Point{}, false, false},
		{second, true, false},
		{second.Add(image.Pt(40, -30)), true, false},
		{image.Point{}, false, false},
		{image.Point{}, false, false},
		{image.Point{}, false, false},
		{second, true, true},
		{first, true, true},
	} {
		if got := tracker.shouldClick(test.point, test.found); got != test.want {
			t.Fatalf("scan %d: shouldClick(%v, %v) = %v, want %v", i, test.point, test.found, got, test.want)
		}
		if test.found && test.want {
			tracker.recordClick(test.point)
		}
	}
}

func TestPauseControl(t *testing.T) {
	control := pauseControl{paused: true}
	calls := 0
	click := func() error { calls++; return nil }

	if clicked, err := control.runClick(context.Background(), control.snapshot(), click); clicked || err != nil || calls != 0 {
		t.Fatalf("click while paused: clicked=%v, err=%v, calls=%d", clicked, err, calls)
	}
	if paused := control.toggle(); paused {
		t.Fatal("F8 should resume the bot")
	}
	if clicked, err := control.runClick(context.Background(), control.snapshot(), click); !clicked || err != nil || calls != 1 {
		t.Fatalf("click while running: clicked=%v, err=%v, calls=%d", clicked, err, calls)
	}
	if paused := control.toggle(); !paused {
		t.Fatal("second F8 should pause the bot")
	}
	if clicked, err := control.runClick(context.Background(), control.snapshot(), click); clicked || err != nil || calls != 1 {
		t.Fatalf("click after pause: clicked=%v, err=%v, calls=%d", clicked, err, calls)
	}
}

func TestFishClickTrackerAfterSkippedClick(t *testing.T) {
	tracker := fishClickTracker{}
	point := image.Pt(700, 400)
	if !tracker.shouldClick(point, true) || !tracker.shouldClick(point, true) {
		t.Fatal("a fish should remain clickable when a paused bot skipped the first attempt")
	}
	tracker.recordClick(point)
	if tracker.shouldClick(point, true) {
		t.Fatal("a clicked fish should not be clicked again")
	}
}

func TestPauseKey(t *testing.T) {
	if !isPauseKey(hook.Event{Kind: hook.KeyUp, Keycode: hook.Keycode["f8"]}) {
		t.Fatal("F8 release should toggle pause")
	}
	if isPauseKey(hook.Event{Kind: hook.KeyDown, Keycode: hook.Keycode["f8"]}) ||
		isPauseKey(hook.Event{Kind: hook.KeyUp, Keycode: hook.Keycode["f7"]}) {
		t.Fatal("other key events should not toggle pause")
	}
}

func TestStaleAndCancelledInput(t *testing.T) {
	control := pauseControl{}
	generation := control.snapshot()
	control.toggle()
	control.toggle()
	calls := 0
	action := func() error { calls++; return nil }
	if acted, err := control.runClick(context.Background(), generation, action); acted || err != nil || calls != 0 {
		t.Fatal("pre-pause decision was allowed after resume")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if acted, err := control.runClick(ctx, control.snapshot(), action); acted || err != nil || calls != 0 {
		t.Fatal("cancelled input was allowed")
	}
}

func TestDesktopPoint(t *testing.T) {
	pixels := image.Rect(0, 0, 2560, 1440)
	for _, desktop := range []image.Rectangle{image.Rect(0, 0, 1280, 720), image.Rect(-1280, 0, 0, 720)} {
		got, err := desktopPoint(image.Pt(2559, 1439), pixels, desktop)
		if err != nil || !got.In(desktop) || got != desktop.Max.Sub(image.Pt(1, 1)) {
			t.Fatalf("scaled point=%v error=%v", got, err)
		}
	}
	for _, point := range []image.Point{image.Pt(-1, 1), image.Pt(2560, 1), image.Pt(1, 1440)} {
		if _, err := desktopPoint(point, pixels, pixels); err == nil {
			t.Fatalf("outside point accepted: %v", point)
		}
	}
}

func TestHeroScrollAndQuantity(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-panel-max.png")
	now := time.Now()
	frame := gameFrame{id: 1, generation: 0, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	o, err := readHeroObservation(context.Background(), frame, heroReaders{}, nil)
	if err != nil || !o.bottom || o.x1 {
		t.Fatalf("unexpected geometry: %+v %v", o, err)
	}
	p := heroRunner{enabled: true}
	p.observe(o, observation{}, now)
	for taps := 0; taps < 5; taps++ {
		a, ok := p.action(now)
		if !ok || a.kind != selectQuantity {
			t.Fatalf("quantity action %d: %+v %t", taps, a, ok)
		}
		p.sent(a, now)
		now = now.Add(time.Second)
		o.frame.id++
		o.frame.at = now
		p.observe(o, observation{}, now)
	}
	if _, ok := p.action(now); ok || !p.nextScan.After(now.Add(25*time.Second)) {
		t.Fatal("unresponsive quantity selector did not back off")
	}
	p = heroRunner{enabled: true}
	o.x1 = true
	o.bottom = false
	p.observe(o, observation{}, now)
	a, ok := p.action(now)
	if !ok || a.kind != scrollHeroes || a.point != o.thumb || a.target.Y != screen.Bounds().Max.Y-1 {
		t.Fatalf("scroll action: %+v", a)
	}
	p.sent(a, now)
	o.frame.id++
	o.frame.at = now.Add(time.Second)
	p.observe(o, observation{}, o.frame.at)
	if p.pending == nil {
		t.Fatal("incomplete drag accepted as bottom")
	}
	o.bottom = true
	o.frame.id++
	o.frame.at = o.frame.at.Add(time.Second)
	p.observe(o, observation{}, o.frame.at)
	if p.pending != nil {
		t.Fatal("completed drag not confirmed")
	}
}

func TestHeroCaptureClearsHover(t *testing.T) {
	bounds := image.Rect(0, 0, 2560, 1440)
	controls := pauseControl{}
	moved := false
	input := heroInput{move: func(point image.Point) error {
		moved = true
		if point != image.Pt(2176, 720) {
			t.Fatalf("parking point: %v", point)
		}
		return nil
	}, capture: func() (image.Image, error) { t.Fatal("input executor took a screenshot"); return nil, nil }}
	p := newGamePipeline(&controls, input, pipelineReaders{}, pipelineOptions{})
	a := gameAction{kind: parkPointer, point: parkPoint(bounds), frame: gameFrame{context: gameContext{bounds: bounds}}}
	if acted, err := p.execute(context.Background(), a); !acted || err != nil || !moved {
		t.Fatalf("parking acted=%t err=%v", acted, err)
	}
	moved = false
	controls.toggle()
	if acted, err := p.execute(context.Background(), a); acted || err != nil || moved {
		t.Fatal("paused parking moved the pointer")
	}
}

func TestHeroPurchaseTooltipRegression(t *testing.T) {
	before := loadTestImage(t, "../../testdata/hero-gog-before.png")
	after := loadTestImage(t, "../../testdata/hero-gog-tooltip.png")
	button := image.Pt(204, 894)
	if !heroRowUnowned(before, button.Y) || !heroRowHasLevel(after, button.Y) {
		t.Fatal("Gog purchase fixture did not change from unowned to leveled")
	}
	if heroListStable(before, after) {
		t.Fatal("tooltip-covered scrollbar must not be treated as verified")
	}
	if _, err := exec.LookPath("tesseract"); err == nil {
		if level, err := readHeroLevel(context.Background(), after, button); err != nil || level != 227 {
			t.Fatalf("actual Gog purchase level=%d error=%v, want 227", level, err)
		}
	} else if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
		t.Fatal(err)
	}
	cleared := image.NewRGBA(after.Bounds())
	draw.Draw(cleared, cleared.Bounds(), after, after.Bounds().Min, draw.Src)
	b := before.Bounds()
	track := image.Rect(b.Dx()*445/1000, b.Dy()*32/100, b.Dx()*495/1000, b.Dy()*965/1000)
	draw.Draw(cleared, track, before, track.Min, draw.Src)
	if !heroListStable(before, cleared) || !heroRowNameMatches(before, cleared, button, button) || !heroRowHasLevel(cleared, button.Y) {
		t.Fatal("cleared purchase tooltip frame rejected")
	}

}

func TestHeroQuantityHotkeyCycle(t *testing.T) {
	original := loadTestImage(t, "../../testdata/hero-panel-max.png")
	quantities := []int{122, 200, 278, 356, 435}
	state, taps := 3, 0
	now := time.Now()
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{keyTap: func(key string) error {
		if key != "t" {
			t.Fatalf("unexpected key %q", key)
		}
		state = (state + 1) % 5
		taps++
		return nil
	}}, pipelineReaders{}, pipelineOptions{})
	hero := heroRunner{enabled: true}
	for id := uint64(1); id <= 3; id++ {
		screen := image.NewRGBA(original.Bounds())
		draw.Draw(screen, screen.Bounds(), original, original.Bounds().Min, draw.Src)
		b := screen.Bounds()
		for _, quantity := range quantities {
			c := color.RGBA{R: 255, G: 210, B: 30, A: 255}
			if quantity == quantities[state] {
				c = color.RGBA{R: 240, G: 140, B: 20, A: 255}
			}
			x, y := b.Dx()*(quantity-25)/1000, b.Dy()*345/1000
			draw.Draw(screen, image.Rect(x-2, y-2, x+3, y+3), image.NewUniform(c), image.Point{}, draw.Src)
		}
		frame := gameFrame{id: id, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: b}}
		o, err := readHeroObservation(context.Background(), frame, heroReaders{gold: func(context.Context, image.Image) (float64, error) { return 100, nil }, price: func(context.Context, image.Image, image.Point) (float64, error) { return 102, nil }, level: func(context.Context, image.Image, image.Point) (int, error) { return 100, nil }}, nil)
		if err != nil {
			t.Fatal(err)
		}
		hero.observe(o, observation{}, now)
		a, ok := hero.action(now)
		if id == 3 {
			if taps != 2 || !o.x1 || !ok || a.kind != buyHero {
				t.Fatalf("x100 to x1 failed: taps=%d geometry=%+v action=%+v", taps, o, a)
			}
			break
		}
		if !ok || a.kind != selectQuantity {
			t.Fatal("missing T action")
		}
		if acted, err := p.execute(context.Background(), a); !acted || err != nil {
			t.Fatal(err)
		}
		hero.sent(a, now)
		now = now.Add(time.Second)
	}
}

func TestHeroMaxKeyRelease(t *testing.T) {
	failure := errors.New("input failed")
	button := image.Pt(204, 894)
	for _, scenario := range []string{"success", "down error", "click error", "up error", "cancelled", "paused"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controls := pauseControl{paused: scenario == "paused"}
			var events []string
			input := heroInput{
				capture: func() (image.Image, error) { t.Fatal("Q purchase must not capture a MAX check"); return nil, nil },
				keyToggle: func(key, state string) error {
					if key != "q" {
						t.Fatalf("unexpected key %q", key)
					}
					events = append(events, state)
					if scenario == "cancelled" && state == "down" {
						cancel()
					}
					if scenario == state+" error" {
						return failure
					}
					return nil
				},
				click: func(point image.Point) error {
					if point != button {
						t.Fatalf("wrong hero target: %v", point)
					}
					events = append(events, "click")
					if scenario == "click error" {
						return failure
					}
					return nil
				},
			}
			acted, err := controls.runClick(ctx, controls.snapshot(), func() error { return clickHeroMax(ctx, input, button) })
			want := "[down click up]"
			if scenario == "down error" || scenario == "cancelled" {
				want = "[down up]"
			}
			if scenario == "paused" {
				want = "[]"
			}
			if fmt.Sprint(events) != want {
				t.Fatalf("events=%v, want %s", events, want)
			}
			if acted != (scenario == "success") {
				t.Fatalf("acted=%t for %s", acted, scenario)
			}
			if scenario == "down error" || scenario == "click error" || scenario == "up error" {
				if !errors.Is(err, failure) {
					t.Fatalf("lost input error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClickLeftHoldAndRelease(t *testing.T) {
	failure := errors.New("mouse input failed")
	for _, scenario := range []string{"success", "down error", "up error", "cancelled before", "cancelled down", "cancelled up"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled before" {
				cancel()
			}
			var events []string
			var downAt, upAt time.Time
			err := clickLeft(ctx, func(args ...interface{}) error {
				if args[0] != "left" {
					t.Fatalf("unexpected button %v", args[0])
				}
				state := args[1].(string)
				events = append(events, state)
				if state == "down" {
					downAt = time.Now()
				} else {
					upAt = time.Now()
				}
				if scenario == "cancelled "+state {
					cancel()
				}
				if scenario == state+" error" {
					return failure
				}
				return nil
			})
			want := "[down up]"
			if scenario == "cancelled before" {
				want = "[]"
			}
			if fmt.Sprint(events) != want {
				t.Fatalf("events=%v, want %s", events, want)
			}
			if scenario == "success" {
				if err != nil || upAt.Sub(downAt) < 100*time.Millisecond || time.Since(upAt) < 100*time.Millisecond {
					t.Fatalf("click was too short or did not settle: hold=%s settle=%s error=%v", upAt.Sub(downAt), time.Since(upAt), err)
				}
			} else if scenario == "down error" || scenario == "up error" {
				if !errors.Is(err, failure) {
					t.Fatalf("lost error: %v", err)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}

func TestFishRetryRequiresVisibleFish(t *testing.T) {
	tracker := fishClickTracker{}
	point := image.Pt(1211, 575)
	tracker.recordClick(point)
	if tracker.shouldClick(point, true) {
		t.Fatal("immediate duplicate fish click")
	}
	tracker.lastClickAt = time.Now().Add(-6 * time.Second)
	if tracker.shouldClick(image.Point{}, false) {
		t.Fatal("retried a vanished fish")
	}
	if !tracker.shouldClick(point, true) {
		t.Fatal("persistent visible fish never retried")
	}
}

func TestFishBeforeHeroDrag(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/fish-over-scrollbar.png")
	now := time.Now()
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{fishInterval: time.Second})
	p.layout = 1
	p.frame = gameFrame{id: 1, at: now, layout: 1, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	point := image.Pt(1211, 575)
	p.fishTarget = &point
	p.enqueue(gameAction{kind: scrollHeroes, frame: p.frame, point: image.Pt(1172, 1331)}, now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("fish did not take priority over scrolling")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	p.frame.id++
	p.frame.at = now.Add(time.Second)
	p.enqueue(gameAction{kind: scrollHeroes, frame: p.frame, point: image.Pt(1172, 1331)}, p.frame.at)
	if a, ok := p.nextAction(p.frame.at); !ok || a.kind != scrollHeroes {
		t.Fatal("scroll waited for a post-click fish scan")
	}
}
