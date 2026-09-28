package main

import (
	"image"
	"os"
	"path/filepath"
	"testing"
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
	}
}
