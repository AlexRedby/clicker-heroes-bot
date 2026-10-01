package main

import (
	_ "embed"
	"image"
	"image/draw"
	"time"

	"gocv.io/x/gocv"
)

//go:embed assets/gild-controls.png
var gildControlsPNG []byte
var gildControlsImage decodedPNG

type gildModal uint8

const (
	noGildModal gildModal = iota
	gildChestModal
	gildRewardModal
	gildRosterModal
	unknownGildModal
)

func gildRect(screen image.Image, r image.Rectangle) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+r.Min.X*b.Dx()/1280, b.Min.Y+r.Min.Y*b.Dy()/720,
		b.Min.X+r.Max.X*b.Dx()/1280, b.Min.Y+r.Max.Y*b.Dy()/720)
}

// Match only the fixed control region, allowing a few pixels of rendering offset.
func gildMatch(screen image.Image, region, reference image.Rectangle) (bool, error) {
	r := gildRect(screen, region)
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
	atlas, err := gildControlsImage.get(gildControlsPNG)
	if err != nil {
		return false, err
	}
	patch := image.NewRGBA(image.Rect(0, 0, reference.Dx(), reference.Dy()))
	draw.Draw(patch, patch.Bounds(), atlas, reference.Min, draw.Src)
	source, err := gocv.ImageToMatRGB(patch)
	if err != nil {
		return false, err
	}
	defer source.Close()
	score, err := templateScore(scene, source, r.Size())
	return score >= 0.90, err
}

func readGildModal(screen image.Image) (gildModal, error) {
	if screen.Bounds().Dx() < 500 || screen.Bounds().Dy() < 500 {
		return noGildModal, nil
	}
	// Check each modal's empty corners before running local template matches.
	cream := func(points []image.Point) bool {
		for _, point := range points {
			p := gildRect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
			r, g, b := rgb(screen.At(p.X, p.Y))
			if r < 235 || g < 225 || b < 160 {
				return false
			}
		}
		return true
	}
	giftPanel := cream([]image.Point{{400, 250}, {880, 250}, {400, 500}, {880, 500}})
	rosterPanel := cream([]image.Point{{135, 120}, {1145, 120}, {135, 650}, {1145, 650}})
	if !giftPanel && !rosterPanel {
		return noGildModal, nil
	}
	found, err := gildMatch(screen, image.Rect(523, 204, 757, 225), image.Rect(0, 65, 234, 86))
	if err != nil {
		return unknownGildModal, err
	}
	if found {
		chest, err := gildMatch(screen, image.Rect(599, 304, 681, 394), image.Rect(0, 86, 82, 176))
		if chest {
			return gildChestModal, err
		}
		return gildRewardModal, err
	}
	found, err = gildMatch(screen, image.Rect(293, 54, 408, 71), image.Rect(0, 265, 115, 282))
	if found {
		return gildRosterModal, err
	}
	return unknownGildModal, err
}

func gildActionPoint(frame gameFrame) (image.Point, bool, error) {
	var region, reference image.Rectangle
	switch frame.context.modal {
	case noGildModal:
		region, reference = image.Rect(1195, 555, 1270, 620), image.Rect(0, 0, 75, 65)
	case gildChestModal:
		r := gildRect(frame.image, image.Rect(599, 304, 681, 394))
		return r.Min.Add(r.Size().Div(2)), true, nil
	case gildRewardModal:
		region, reference = image.Rect(891, 534, 984, 579), image.Rect(0, 176, 93, 221)
		found, err := gildMatch(frame.image, region, reference)
		if found || err != nil {
			return gildRect(frame.image, region).Min.Add(gildRect(frame.image, region).Size().Div(2)), found, err
		}
		// One or two pending gifts may have no Open All; finish via the visible close button.
		region, reference = image.Rect(973, 106, 1016, 150), image.Rect(0, 221, 43, 265)
	case gildRosterModal:
		region, reference = image.Rect(1128, 17, 1171, 61), image.Rect(0, 221, 43, 265)
	default:
		return image.Point{}, false, nil
	}
	found, err := gildMatch(frame.image, region, reference)
	r := gildRect(frame.image, region)
	return r.Min.Add(r.Size().Div(2)), found, err
}

type gildCollector struct {
	active, opening                 bool
	nextCheck, nextAction, deadline time.Time
	lastPoint                       image.Point
	attempts                        int
}

func (g *gildCollector) interrupt() {
	g.attempts = 0
	g.nextAction, g.deadline = time.Time{}, time.Time{}
}
