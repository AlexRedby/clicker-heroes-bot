package vision

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

//go:embed assets/*/*.png assets/orange-fish.png
var templateFiles embed.FS
var templateImages sync.Map

type decodedPNG struct {
	once  sync.Once
	image image.Image
	err   error
}

// Decoded templates are shared read-only by concurrent analyzers.
func Template(name string) (image.Image, error) {
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

// Rect maps a 1280x720 reference region into the captured image.
func Rect(screen image.Image, r image.Rectangle) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+r.Min.X*b.Dx()/1280, b.Min.Y+r.Min.Y*b.Dy()/720,
		b.Min.X+r.Max.X*b.Dx()/1280, b.Min.Y+r.Max.Y*b.Dy()/720)
}

// Match only the fixed control region, allowing a few pixels of rendering offset.
func MatchControl(screen image.Image, region image.Rectangle, name string) (bool, error) {
	r := Rect(screen, region)
	margin := max(2, screen.Bounds().Dx()/640)
	_, found, err := findControl(screen, r.Inset(-margin).Intersect(screen.Bounds()), r.Size(), name)
	return found, err
}

// FindControl locates fixed-size artwork within a reference-coordinate region.
func FindControl(screen image.Image, search image.Rectangle, size image.Point, name string) (image.Rectangle, bool, error) {
	return findControl(screen, Rect(screen, search).Intersect(screen.Bounds()), Rect(screen, image.Rectangle{Max: size}).Size(), name)
}

func findControl(screen image.Image, search image.Rectangle, size image.Point, name string) (image.Rectangle, bool, error) {
	reference, err := Template(name)
	if err != nil {
		return image.Rectangle{}, false, err
	}
	if size.X <= 0 || size.Y <= 0 || search.Dx() < size.X || search.Dy() < size.Y {
		return image.Rectangle{}, false, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, search.Dx(), search.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, search.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return image.Rectangle{}, false, err
	}
	defer scene.Close()
	score, point, err := controlMatch(scene, reference, size)
	return image.Rectangle{Min: point.Add(search.Min), Max: point.Add(search.Min).Add(size)}, score >= 0.90, err
}

// Transparent artwork excludes the changing game location from UI matching.
func ControlScore(scene gocv.Mat, reference image.Image, size image.Point) (float32, error) {
	score, _, err := controlMatch(scene, reference, size)
	return score, err
}

func controlMatch(scene gocv.Mat, reference image.Image, size image.Point) (float32, image.Point, error) {
	source, err := gocv.ImageToMatRGB(reference)
	if err != nil {
		return 0, image.Point{}, err
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
	if visible == 0 {
		return 0, image.Point{}, nil
	}
	scaled, mask := gocv.NewMat(), gocv.NewMat()
	defer scaled.Close()
	defer mask.Close()
	if err := gocv.Resize(source, &scaled, size, 0, 0, gocv.InterpolationArea); err != nil {
		return 0, image.Point{}, err
	}
	if transparent {
		sourceMask, err := gocv.NewMatFromBytes(b.Dy(), b.Dx(), gocv.MatTypeCV8U, pixels)
		if err != nil {
			return 0, image.Point{}, err
		}
		defer sourceMask.Close()
		if err = gocv.Resize(sourceMask, &mask, size, 0, 0, gocv.InterpolationNearestNeighbor); err != nil {
			return 0, image.Point{}, err
		}
	}
	return locateTemplate(scene, scaled, mask)
}

// TemplateScore matches an already converted opaque template.
func TemplateScore(scene, source gocv.Mat, size image.Point) (float32, error) {
	reference, mask := gocv.NewMat(), gocv.NewMat()
	defer reference.Close()
	defer mask.Close()
	if err := gocv.Resize(source, &reference, size, 0, 0, gocv.InterpolationArea); err != nil {
		return 0, err
	}
	return matchTemplate(scene, reference, mask)
}

func matchTemplate(scene, reference, mask gocv.Mat) (float32, error) {
	score, _, err := locateTemplate(scene, reference, mask)
	return score, err
}

func locateTemplate(scene, reference, mask gocv.Mat) (float32, image.Point, error) {
	result := gocv.NewMat()
	defer result.Close()
	if err := gocv.MatchTemplate(scene, reference, &result, gocv.TmCcoeffNormed, mask); err != nil {
		return 0, image.Point{}, err
	}
	_, score, _, point := gocv.MinMaxLoc(result)
	if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
		return 0, image.Point{}, nil
	}
	return score, point, nil
}
