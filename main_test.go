package main

import (
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
