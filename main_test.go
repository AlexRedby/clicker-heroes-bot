package main

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
	ctx := context.Background()
	controls := pauseControl{}
	generation := controls.snapshot()
	bottom := loadHeroScreen(t, "testdata/hero-panel-max.png")
	top := image.NewRGBA(bottom.Bounds())
	draw.Draw(top, top.Bounds(), bottom, bottom.Bounds().Min, draw.Src)
	thumb, height, found := heroScrollbarThumb(bottom)
	if !found {
		t.Fatal("fixture thumb missing")
	}
	rect := image.Rect(thumb.X-12, thumb.Y-height/2-4, thumb.X+12, thumb.Y+height/2+4)
	draw.Draw(top, rect, image.NewUniform(color.RGBA{R: 95, G: 62, B: 12, A: 255}), image.Point{}, draw.Src)
	// Move the same recognizable thumb upward, leaving an incomplete drag result.
	draw.Draw(top, rect.Add(image.Pt(0, -200)), bottom, rect.Min, draw.Src)
	if heroScrollbarAtBottom(top) {
		t.Fatal("partial drag considered bottom")
	}
	drags := 0
	input := heroInput{capture: func() (image.Image, error) { return top, nil }, drag: func(from, to image.Point) error {
		drags++
		if from.X != to.X || to.Y != bottom.Bounds().Max.Y-1 {
			t.Fatalf("drag target %v is not the bottom screen edge below %v", to, from)
		}
		return nil
	}, move: func(image.Point) error { return nil }}
	fish := func(image.Image) (bool, error) { return false, nil }
	if _, _, found, err := findHeroButtonWithScroll(ctx, &controls, generation, input, top, fish); found || err != nil || drags != 1 {
		t.Fatalf("partial drag: found=%t err=%v drags=%d", found, err, drags)
	}
	input.capture = func() (image.Image, error) { return bottom, nil }
	if _, _, found, err := findHeroButtonWithScroll(ctx, &controls, generation, input, top, fish); !found || err != nil {
		t.Fatalf("completed drag: found=%t err=%v", found, err)
	}
	beforeDrags := drags
	if _, _, found, err := findHeroButtonWithScroll(ctx, &controls, generation, input, bottom, fish); !found || err != nil || drags != beforeDrags {
		t.Fatalf("already at bottom: found=%t err=%v drags=%d, want %d", found, err, drags, beforeDrags)
	}
	taps := 0
	input.keyTap = func(key string) error {
		if key != "t" {
			t.Fatalf("unexpected key %q", key)
		}
		taps++
		return nil
	}
	input.click = func(image.Point) error { t.Fatal("quantity selection used a mouse click"); return nil }
	if _, selected, err := selectHeroX1(ctx, &controls, generation, input, bottom); selected || err != nil || taps != 5 {
		t.Fatal("unresponsive x1 hotkey was accepted or retried beyond one cycle")
	}
	// Simulate pause/resume while awaiting a screenshot after pressing T.
	captures := 0
	input.capture = func() (image.Image, error) {
		captures++
		if captures == 1 {
			controls.toggle()
			controls.toggle()
		}
		return bottom, nil
	}
	if _, selected, err := selectHeroX1(ctx, &controls, generation, input, bottom); selected || err != nil {
		t.Fatal("selection from an invalidated frame accepted")
	}
}

func TestHeroCaptureClearsHover(t *testing.T) {
	controls := pauseControl{}
	generation := controls.snapshot()
	screen := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
	moved, captured := false, false
	input := heroInput{
		move: func(p image.Point) error {
			if p.X <= screen.Bounds().Dx()*75/100 || !p.In(screen.Bounds()) {
				t.Fatalf("cursor did not leave the hero tooltip: %v", p)
			}
			moved = true
			return nil
		},
		capture: func() (image.Image, error) {
			if !moved {
				t.Fatal("capture happened before clearing hover")
			}
			captured = true
			return screen, nil
		},
	}
	if got, err := captureHeroScreen(context.Background(), &controls, generation, input, screen.Bounds()); err != nil || got != screen || !captured {
		t.Fatalf("hero capture: image=%v error=%v captured=%t", got, err, captured)
	}
	// Pause after moving the mouse invalidates the frame before it is captured.
	captured = false
	input.move = func(image.Point) error { go controls.toggle(); return nil }
	if got, err := captureHeroScreen(context.Background(), &controls, generation, input, screen.Bounds()); got != nil || err != nil || captured {
		t.Fatalf("paused hero capture: image=%v error=%v captured=%t", got, err, captured)
	}
}

func TestHeroPurchaseTooltipRegression(t *testing.T) {
	before := loadHeroScreen(t, "testdata/hero-gog-before.png")
	after := loadHeroScreen(t, "testdata/hero-gog-tooltip.png")
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
	ctx := context.Background()
	controls := pauseControl{}
	cleared := image.NewRGBA(after.Bounds())
	draw.Draw(cleared, cleared.Bounds(), after, after.Bounds().Min, draw.Src)
	moved := false
	input := heroInput{
		move: func(image.Point) error {
			moved = true
			// Simulate dismissing the tooltip: restore only the unchanged scrollbar.
			b := before.Bounds()
			track := image.Rect(b.Dx()*445/1000, b.Dy()*32/100, b.Dx()*495/1000, b.Dy()*965/1000)
			draw.Draw(cleared, track, before, track.Min, draw.Src)
			return nil
		},
		capture: func() (image.Image, error) {
			if !moved {
				return after, nil
			}
			return cleared, nil
		},
	}
	got, err := captureHeroScreen(ctx, &controls, controls.snapshot(), input, before.Bounds())
	if err != nil || !sameHeroRow(before, got, button, button) || !heroRowHasLevel(got, button.Y) {
		t.Fatalf("purchase frame rejected after hover was cleared: %v", err)
	}
}

func TestHeroQuantityHotkeyCycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controls := pauseControl{}
	original := loadHeroScreen(t, "testdata/hero-panel-max.png")
	quantities := []int{122, 200, 278, 356, 435}
	state, taps := 3, 0
	frame := func() image.Image {
		screen := image.NewRGBA(original.Bounds())
		draw.Draw(screen, screen.Bounds(), original, original.Bounds().Min, draw.Src)
		b := screen.Bounds()
		for _, quantity := range quantities {
			colour := color.RGBA{R: 255, G: 210, B: 30, A: 255}
			if quantity == quantities[state] {
				colour = color.RGBA{R: 240, G: 140, B: 20, A: 255}
			}
			x, y := b.Dx()*(quantity-25)/1000, b.Dy()*345/1000
			draw.Draw(screen, image.Rect(x-2, y-2, x+3, y+3), image.NewUniform(colour), image.Point{}, draw.Src)
		}
		return screen
	}
	input := heroInput{
		capture: func() (image.Image, error) { return frame(), nil },
		click:   func(image.Point) error { t.Fatal("quantity selection clicked the screen"); return nil },
		keyTap: func(key string) error {
			if key != "t" {
				t.Fatalf("unexpected key %q", key)
			}
			taps++
			state = (state + 1) % len(quantities)
			return nil
		},
	}
	if _, selected, err := selectHeroX1(ctx, &controls, controls.snapshot(), input, frame()); !selected || err != nil || taps != 2 {
		t.Fatalf("x100 to x1: selected=%t error=%v taps=%d", selected, err, taps)
	}
	if _, selected, err := selectHeroX1(ctx, &controls, controls.snapshot(), input, frame()); !selected || err != nil || taps != 2 {
		t.Fatal("already-selected x1 should not require input")
	}
	state = 3
	failure := errors.New("keyboard unavailable")
	input.keyTap = func(string) error { return failure }
	if _, selected, err := selectHeroX1(ctx, &controls, controls.snapshot(), input, frame()); selected || !errors.Is(err, failure) {
		t.Fatalf("failed key event accepted: selected=%t error=%v", selected, err)
	}
	input.keyTap = func(string) error { cancel(); return nil }
	if _, selected, err := selectHeroX1(ctx, &controls, controls.snapshot(), input, frame()); selected || err != nil {
		t.Fatalf("cancelled key selection accepted: selected=%t error=%v", selected, err)
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
