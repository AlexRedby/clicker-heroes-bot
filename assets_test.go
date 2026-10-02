package main

import (
	"image"
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedTemplatesDecodeOnce(t *testing.T) {
	err := fs.WalkDir(templateFiles, "assets", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name := strings.TrimPrefix(path, "assets/")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			results := make(chan image.Image, 8)
			for range cap(results) {
				go func() {
					img, err := templateImage(name)
					if err != nil {
						t.Error(err)
					}
					results <- img
				}()
			}
			first := <-results
			if first == nil || first.Bounds().Empty() {
				t.Error("missing or empty template")
			}
			for range cap(results) - 1 {
				if img := <-results; img != first {
					t.Error("concurrent callers received different decoded images")
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if img, err := templateImage("ui/missing.png"); err == nil || img != nil || !strings.Contains(err.Error(), "ui/missing.png") {
		t.Fatalf("missing template: image=%v error=%v", img, err)
	}
}
