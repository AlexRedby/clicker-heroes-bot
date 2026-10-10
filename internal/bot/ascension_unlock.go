package bot

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type ascensionUnlockObservation struct {
	hero                        heroObservation
	fromTop, ready, unavailable bool
}

// The reset upgrade is not bought by the footer and has no saved ownership.
// Prepare only its hero; the recognized HUD spiral remains the reset control.
func readAscensionUnlockObservation(ctx context.Context, frame gameFrame, read heroReaders, fromTop bool) (ascensionUnlockObservation, error) {
	out := ascensionUnlockObservation{hero: heroObservation{frame: frame, startup: true}, fromTop: fromTop}
	if !bootstrapHeroes(frame.context) || !heroQuantityBarPresent(frame.image) {
		return out, nil
	}
	o := &out.hero
	o.x1 = heroQuantitySelected(frame.image, 122)
	if !o.x1 {
		return out, nil
	}
	b := frame.image.Bounds()
	viewport := heroListViewport(frame.image)
	var height int
	o.thumb, height, o.thumbFound = heroScrollbarThumb(frame.image)
	atTop := o.thumbFound && o.thumb.Y-height/2 <= b.Min.Y+b.Dy()*420/1000+max(3, b.Dy()/100)
	out.fromTop = out.fromTop || atTop
	for _, available := range []bool{true, false} {
		for _, band := range findHeroButtonBands(frame.image, available) {
			button := image.Pt(b.Min.X+b.Dx()*8/100, (band.Min.Y+band.Max.Y-1)/2)
			region := ascensionUnlockNameRegion(frame.image, button)
			if band.Min.Y <= viewport.Min.Y+max(3, b.Dy()/150) || band.Max.Y >= viewport.Max.Y-max(3, b.Dy()/150) || !region.In(viewport) {
				continue
			}
			name, err := readAscensionUnlockName(ctx, frame.image, region)
			if err != nil {
				return out, err
			}
			if name != "Amenhotep" {
				continue
			}
			kind, err := readHeroButtonKind(frame.image, button)
			if err != nil {
				return out, err
			}
			if kind == heroButtonUnknown {
				return out, fmt.Errorf("Ascension unlock: Amenhotep caption is obscured")
			}
			o.button, o.found, o.owned = button, true, kind == heroButtonLevelUp
			if o.owned {
				if read.level == nil {
					return out, fmt.Errorf("Ascension unlock: hero level reader unavailable")
				}
				o.level, err = read.level(ctx, frame.image, button)
				if err != nil {
					return out, err
				}
				out.ready = o.level >= 150
			}
			out.unavailable = !available
			return out, nil
		}
	}
	if !o.thumbFound || heroScrollbarAtEnd(frame.image, o.thumb, height) && out.fromTop {
		out.unavailable, out.fromTop = true, false // Earlier hires may still be needed to expose Amenhotep.
		return out, nil
	}
	if !out.fromTop {
		o.startupScroll = image.Pt(o.thumb.X, b.Min.Y+b.Dy()*420/1000+height/2)
	} else {
		o.sweep.bottom = true // Use the existing overlapping page drag.
		o.startupScroll = image.Pt(o.thumb.X, min(o.thumb.Y+max(3, height/2), b.Min.Y+b.Dy()*965/1000-height/2))
	}
	return out, nil
}

func (o ascensionUnlockObservation) action() (gameAction, bool) {
	a := gameAction{frame: o.hero.frame, hero: o.hero}
	switch {
	case !o.hero.x1 && heroQuantityBarPresent(o.hero.frame.image):
		a.kind = selectQuantity
	case o.hero.startupScroll != (image.Point{}):
		a.kind, a.point, a.target = scrollHeroes, o.hero.thumb, o.hero.startupScroll
	case o.hero.found && !o.ready && !o.unavailable:
		a.kind, a.point = buyHero, o.hero.button
	default:
		return a, false
	}
	return a, true
}

func ascensionUnlockNameRegion(screen image.Image, button image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*15/100, button.Y-b.Dy()*65/1000, b.Min.X+b.Dx()*37/100, button.Y-b.Dy()*18/1000)
}

func ascensionUnlockNameMask(screen image.Image, region image.Rectangle) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, region.Dx(), region.Dy()))
	for y := 0; y < region.Dy(); y++ {
		for x := 0; x < region.Dx(); x++ {
			c := screen.At(region.Min.X+x, region.Min.Y+y)
			r, g, blue := rgb(c)
			v := uint8(255)
			if gildPurple(c) || min(r, g, blue) > 180 && max(r, g, blue)-min(r, g, blue) < 55 {
				v = 0
			}
			m.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return m
}

func readAscensionUnlockName(ctx context.Context, screen image.Image, region image.Rectangle) (string, error) {
	mask := ascensionUnlockNameMask(screen, region)
	// Native white names sit above the white level line; gilded names have a
	// different vertical offset. Keep the first complete text line in the crop.
	start, last, line := -1, -1, image.Rectangle{}
	for y := 0; y <= mask.Bounds().Dy()+2; y++ {
		ink := 0
		if y < mask.Bounds().Dy() {
			for x := 0; x < mask.Bounds().Dx(); x++ {
				if mask.GrayAt(x, y).Y == 0 {
					ink++
				}
			}
		}
		if ink >= max(2, screen.Bounds().Dx()/500) {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start >= 0 && y-last > 2 {
			if last-start >= max(4, screen.Bounds().Dy()/100) {
				line = image.Rect(0, max(0, start-2), mask.Bounds().Dx(), min(mask.Bounds().Dy(), last+3))
				break
			}
			start = -1
		}
	}
	if line.Empty() {
		return "", nil
	}
	mask = mask.SubImage(line).(*image.Gray)
	scale := max(1, 3840/screen.Bounds().Dx())
	up := image.NewGray(image.Rect(0, 0, mask.Bounds().Dx()*scale+20, mask.Bounds().Dy()*scale+20))
	draw.Draw(up, up.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.NearestNeighbor.Scale(up, up.Bounds().Inset(10), mask, mask.Bounds(), draw.Src, nil)
	raw, err := readTextImage(ctx, up, 7, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz,' ")
	return strings.TrimSpace(raw), err
}

// The initial exact OCR binds the target. Recheck its static name pixels and
// trusted caption before using a fresh available button at the same position.
func ascensionUnlockHeroStable(before heroObservation, current gameFrame) bool {
	if !before.found || !bootstrapHeroes(current.context) || !heroQuantitySelected(current.image, 122) || before.frame.context != current.context || before.frame.generation != current.generation || before.frame.layout != current.layout || before.frame.image == nil || before.frame.image.Bounds() != current.image.Bounds() {
		return false
	}
	region := ascensionUnlockNameRegion(before.frame.image, before.button)
	if !region.In(heroListViewport(current.image)) {
		return false
	}
	a, z := ascensionUnlockNameMask(before.frame.image, region), ascensionUnlockNameMask(current.image, region)
	ink := 0
	for i, v := range a.Pix {
		if v != z.Pix[i] {
			return false
		}
		if v == 0 {
			ink++
		}
	}
	if ink < 8 {
		return false
	}
	kind, err := readHeroButtonKind(current.image, before.button)
	if err != nil || kind != heroButtonHire && kind != heroButtonLevelUp || (kind == heroButtonLevelUp) != before.owned {
		return false
	}
	for _, band := range findHeroButtonBands(current.image, true) {
		if before.button.In(band.Inset(-current.image.Bounds().Dx()/20)) && absDiff(before.button.Y, (band.Min.Y+band.Max.Y-1)/2) <= max(2, current.image.Bounds().Dy()/200) {
			return true
		}
	}
	return false
}
