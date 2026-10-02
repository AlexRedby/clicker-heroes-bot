package main

import (
	"fmt"
	"image"
	"image/draw"
	"strings"
)

// Desktop coordinates are physical pixels on Windows and global points on macOS.
type nativeScene struct {
	Window          string
	Display         int
	Bounds, Desktop image.Rectangle
	Pixels          image.Point
	DPI             uint32
}

type viewportGeometry struct {
	Scene        nativeScene
	Capture, ROI image.Rectangle
}

type viewportImage struct {
	image.Image
	geometry viewportGeometry
	reason   string
}

func isGameTitle(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "clicker heroes") || strings.Contains(title, "clickerheroes")
}

func desktopPoint(point image.Point, pixels, desktop image.Rectangle) (image.Point, error) {
	if !point.In(pixels) || pixels.Empty() || desktop.Empty() {
		return image.Point{}, fmt.Errorf("click coordinate %v is outside captured display %v", point, pixels)
	}
	return desktop.Min.Add(image.Pt((point.X-pixels.Min.X)*desktop.Dx()/pixels.Dx(), (point.Y-pixels.Min.Y)*desktop.Dy()/pixels.Dy())), nil
}

func (g viewportGeometry) point(p image.Point) (image.Point, error) {
	if g.Scene.Window == "" || g.ROI.Empty() || !g.ROI.In(g.Capture) || !p.In(image.Rectangle{Max: g.ROI.Size()}) {
		return image.Point{}, fmt.Errorf("invalid viewport coordinate %v", p)
	}
	return desktopPoint(p.Add(g.ROI.Min), g.Capture, g.Scene.Desktop)
}

// Convert both edges with outward rounding; a partially offscreen window is unsupported.
func windowPixels(scene nativeScene, pixels image.Rectangle) (image.Rectangle, error) {
	b, d := scene.Bounds, scene.Desktop
	if b.Empty() || d.Empty() || pixels.Empty() || !b.In(d) {
		return image.Rectangle{}, fmt.Errorf("game window is clipped or spans displays")
	}
	x0, y0 := b.Min.X-d.Min.X, b.Min.Y-d.Min.Y
	x1, y1 := b.Max.X-d.Min.X, b.Max.Y-d.Min.Y
	return image.Rect(pixels.Min.X+x0*pixels.Dx()/d.Dx(), pixels.Min.Y+y0*pixels.Dy()/d.Dy(),
		pixels.Min.X+(x1*pixels.Dx()+d.Dx()-1)/d.Dx(), pixels.Min.Y+(y1*pixels.Dy()+d.Dy()-1)/d.Dy()), nil
}

func compactCrop(screen image.Image, roi image.Rectangle) *image.RGBA {
	crop := image.NewRGBA(image.Rectangle{Max: roi.Size()})
	draw.Draw(crop, crop.Bounds(), screen, roi.Min, draw.Src)
	return crop
}

// ponytail: client bounds and 16:9 fits only; add candidates from real windowed HUD evidence.
// Each candidate still needs independent HUD anchors; no guessed border grants input.
func viewportCandidates(window image.Rectangle) []image.Rectangle {
	w, h := window.Dx(), window.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	fit := image.Pt(min(w, h*16/9), min(h, w*9/16))
	out := []image.Rectangle{window}
	for _, offset := range []image.Point{window.Size().Sub(fit).Div(2), image.Pt((w-fit.X)/2, h-fit.Y)} {
		r := image.Rectangle{Min: window.Min.Add(offset), Max: window.Min.Add(offset).Add(fit)}
		duplicate := false
		for _, old := range out {
			duplicate = duplicate || old == r
		}
		if !duplicate {
			out = append(out, r)
		}
	}
	return out
}
