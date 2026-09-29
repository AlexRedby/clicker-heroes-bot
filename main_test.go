package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"

	hook "github.com/robotn/gohook"
)

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
		{second, true, true},
		{second, true, false},
		{image.Point{}, false, false},
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

	if clicked, err := control.runClick(click); clicked || err != nil || calls != 0 {
		t.Fatalf("click while paused: clicked=%v, err=%v, calls=%d", clicked, err, calls)
	}
	if paused := control.toggle(); paused {
		t.Fatal("F8 should resume the bot")
	}
	if clicked, err := control.runClick(click); !clicked || err != nil || calls != 1 {
		t.Fatalf("click while running: clicked=%v, err=%v, calls=%d", clicked, err, calls)
	}
	if paused := control.toggle(); !paused {
		t.Fatal("second F8 should pause the bot")
	}
	if clicked, err := control.runClick(click); clicked || err != nil || calls != 1 {
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
