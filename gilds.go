package main

import (
	"image"
	"time"
)

type gildModal uint8

const (
	noGildModal gildModal = iota
	gildChestModal
	gildRewardModal
	gildRosterModal
	unknownGildModal
)

func readGildModal(screen image.Image) (gildModal, error) {
	if screen.Bounds().Dx() < 500 || screen.Bounds().Dy() < 500 {
		return noGildModal, nil
	}
	// Check each modal's empty corners before running local template matches.
	cream := func(points []image.Point) bool {
		for _, point := range points {
			p := controlRect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
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
	found, err := matchControl(screen, image.Rect(523, 204, 757, 225), "gilds/reward-title.png")
	if err != nil {
		return unknownGildModal, err
	}
	if found {
		chest, err := matchControl(screen, image.Rect(599, 304, 681, 394), "gilds/chest.png")
		if chest {
			return gildChestModal, err
		}
		return gildRewardModal, err
	}
	found, err = matchControl(screen, image.Rect(293, 54, 408, 71), "gilds/roster-title.png")
	if found {
		return gildRosterModal, err
	}
	return unknownGildModal, err
}

func gildActionPoint(frame gameFrame) (image.Point, bool, error) {
	var region image.Rectangle
	var name string
	switch frame.context.modal {
	case noGildModal:
		// Stay inside the left bow: scenery changes and the notification bounces on the right.
		region, name = image.Rect(1207, 573, 1227, 590), "gilds/gift.png"
	case gildChestModal:
		r := controlRect(frame.image, image.Rect(599, 304, 681, 394))
		return r.Min.Add(r.Size().Div(2)), true, nil
	case gildRewardModal:
		region, name = image.Rect(891, 534, 984, 579), "gilds/open-all.png"
		found, err := matchControl(frame.image, region, name)
		if found || err != nil {
			return controlRect(frame.image, region).Min.Add(controlRect(frame.image, region).Size().Div(2)), found, err
		}
		// One or two pending gifts may have no Open All; finish via the visible close button.
		region, name = image.Rect(982, 116, 1006, 140), "gilds/close.png"
	case gildRosterModal:
		region, name = image.Rect(1137, 27, 1161, 51), "gilds/close.png"
	default:
		return image.Point{}, false, nil
	}
	found, err := matchControl(frame.image, region, name)
	r := controlRect(frame.image, region)
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
