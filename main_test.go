package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
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
	input.click = func(image.Point) error { return nil }
	if _, selected, err := selectHeroQuantity(ctx, &controls, generation, input, 122); selected || err != nil {
		t.Fatal("failed x1 selection accepted")
	}
	if _, selected, err := selectHeroQuantity(ctx, &controls, generation, input, 435); !selected || err != nil {
		t.Fatal("confirmed MAX selection rejected")
	}
	// Simulate pause/resume while awaiting the selection screenshot, outside the input lock.
	captures := 0
	input.click = func(image.Point) error { return nil }
	input.capture = func() (image.Image, error) {
		captures++
		if captures == 2 {
			controls.toggle()
			controls.toggle()
		}
		return bottom, nil
	}
	if _, selected, err := selectHeroQuantity(ctx, &controls, generation, input, 435); selected || err != nil {
		t.Fatal("selection from an invalidated frame accepted")
	}
}
