package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"math"
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
	score, err := controlTemplateScore(scene, reference, r.Size())
	return score >= 0.90, err
}

// Transparent artwork excludes the changing game location from UI matching.
func controlTemplateScore(scene gocv.Mat, reference image.Image, size image.Point) (float32, error) {
	source, err := gocv.ImageToMatRGB(reference)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	b := reference.Bounds()
	pixels := make([]byte, b.Dx()*b.Dy())
	transparent, visible := false, 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := reference.At(b.Min.X+x, b.Min.Y+y).RGBA()
			transparent = transparent || a < 65535
			if a > 0 {
				pixels[y*b.Dx()+x] = 255
				visible++
			}
		}
	}
	if !transparent {
		return templateScore(scene, source, size)
	}
	if visible == 0 {
		return 0, nil
	}
	sourceMask, err := gocv.NewMatFromBytes(b.Dy(), b.Dx(), gocv.MatTypeCV8U, pixels)
	if err != nil {
		return 0, err
	}
	defer sourceMask.Close()
	scaled, mask, result := gocv.NewMat(), gocv.NewMat(), gocv.NewMat()
	defer scaled.Close()
	defer mask.Close()
	defer result.Close()
	if err := gocv.Resize(source, &scaled, size, 0, 0, gocv.InterpolationArea); err != nil {
		return 0, err
	}
	if err := gocv.Resize(sourceMask, &mask, size, 0, 0, gocv.InterpolationNearestNeighbor); err != nil {
		return 0, err
	}
	if err := gocv.MatchTemplate(scene, scaled, &result, gocv.TmCcoeffNormed, mask); err != nil {
		return 0, err
	}
	_, score, _, _ := gocv.MinMaxLoc(result)
	if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
		return 0, nil
	}
	return score, nil
}
