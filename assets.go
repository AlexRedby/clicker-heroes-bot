package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"sync"

	"gocv.io/x/gocv"
)

//go:embed assets/*/*.png
var templateFiles embed.FS
var templateImages sync.Map

type decodedPNG struct {
	once  sync.Once
	image image.Image
	err   error
}

// Decoded templates are shared read-only by concurrent analyzers.
func templateImage(name string) (image.Image, error) {
	entry, found := templateImages.Load(name)
	if !found {
		entry, _ = templateImages.LoadOrStore(name, &decodedPNG{})
	}
	reference := entry.(*decodedPNG)
	reference.once.Do(func() {
		data, err := templateFiles.ReadFile("assets/" + name)
		if err == nil {
			reference.image, _, err = image.Decode(bytes.NewReader(data))
		}
		if err != nil {
			reference.err = fmt.Errorf("template %s: %w", name, err)
		}
	})
	return reference.image, reference.err
}

func controlRect(screen image.Image, r image.Rectangle) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+r.Min.X*b.Dx()/1280, b.Min.Y+r.Min.Y*b.Dy()/720,
		b.Min.X+r.Max.X*b.Dx()/1280, b.Min.Y+r.Max.Y*b.Dy()/720)
}

// Match only the fixed control region, allowing a few pixels of rendering offset.
func matchControl(screen image.Image, region image.Rectangle, name string) (bool, error) {
	reference, err := templateImage(name)
	if err != nil {
		return false, err
	}
	r := controlRect(screen, region)
	margin := max(2, screen.Bounds().Dx()/640)
	search := r.Inset(-margin).Intersect(screen.Bounds())
	if r.Empty() || search.Dx() < r.Dx() || search.Dy() < r.Dy() {
		return false, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, search.Dx(), search.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, search.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false, err
	}
	defer scene.Close()
	source, err := gocv.ImageToMatRGB(reference)
	if err != nil {
		return false, err
	}
	defer source.Close()
	score, err := templateScore(scene, source, r.Size())
	return score >= 0.90, err
}
