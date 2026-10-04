package main

import (
	"image"
	"image/color"
	"image/draw"

	"gocv.io/x/gocv"
)

// Purchased upgrades carry the same green check on every hero. Match the frame
// and check artwork in a bounded strip; names, levels and icon artwork are irrelevant.
func readHeroUpgradeState(screen image.Image, button image.Point) (complete, visible bool, err error) {
	if screen == nil || !button.In(screen.Bounds()) {
		return false, false, nil
	}
	b := screen.Bounds()
	region := image.Rect(b.Min.X+b.Dx()*15/100, button.Y, b.Min.X+b.Dx()*40/100, button.Y+b.Dy()*12/100).Intersect(b)
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false, false, err
	}
	defer scene.Close()
	ref, err := templateImage("heroes/upgrade-frame.png")
	if err != nil {
		return false, false, err
	}
	// Low-level cards show a single blank, locked upgrade slot. Its frame is
	// different from the artwork-bearing unlocked slots.
	locked, err := templateImage("heroes/upgrade-locked-1280.png")
	if err != nil {
		return false, false, err
	}
	lockedSize := image.Pt(max(1, locked.Bounds().Dx()*b.Dx()/1280), max(1, locked.Bounds().Dy()*b.Dy()/720))
	if scene.Rows() < lockedSize.Y || min(scene.Cols(), b.Dx()*45/1000) < lockedSize.X {
		return false, false, nil
	}
	firstColumn := scene.Region(image.Rect(0, 0, min(scene.Cols(), b.Dx()*45/1000), scene.Rows()))
	lockedScore, err := controlTemplateScore(firstColumn, locked, lockedSize)
	firstColumn.Close()
	if err != nil {
		return false, false, err
	}
	if lockedScore >= 0.90 {
		return false, true, nil
	}
	// Strip columns have fixed pitch; find each whole frame without scanning large
	// image regions or treating a nearby hero sprite as another upgrade.
	size := image.Pt(max(1, 75*b.Dx()/2560), max(1, 80*b.Dy()/1440))
	firstY, count, checked := -1, 0, 0
	for i := 0; i < 8; i++ {
		x := b.Min.X + (391+i*75)*b.Dx()/2560
		search := image.Rect(x-max(2, b.Dx()/640), region.Min.Y, x+size.X+max(2, b.Dx()/640), region.Max.Y).Intersect(region)
		if firstY >= 0 {
			search.Min.Y = max(search.Min.Y, firstY-max(2, b.Dy()/360))
			search.Max.Y = min(search.Max.Y, firstY+size.Y+max(2, b.Dy()/360))
		}
		if search.Dx() < size.X || search.Dy() < size.Y {
			break
		}
		roi := scene.Region(search.Sub(region.Min))
		score, point, e := controlTemplateMatch(roi, ref, size)
		roi.Close()
		if e != nil {
			return false, false, e
		}
		if score < 0.82 {
			break
		}
		top := search.Min.Add(point)
		if firstY < 0 {
			firstY = top.Y
		}
		cell := image.Rect(top.X, top.Y, top.X+size.X, top.Y+size.Y)
		if cell.Max.Y >= b.Max.Y {
			return false, false, nil
		}
		yes, e := upgradeCheckMatched(screen, cell)
		if e != nil {
			return false, false, e
		}
		count++
		if yes {
			checked++
		}
	}
	return count > 0 && count == checked, count > 0, nil
}

func upgradeCheckMatched(screen image.Image, cell image.Rectangle) (bool, error) {
	b := screen.Bounds()
	ref, err := templateImage("heroes/upgrade-check.png")
	if err != nil {
		return false, err
	}
	size := image.Pt(max(1, ref.Bounds().Dx()*b.Dx()/2560), max(1, ref.Bounds().Dy()*b.Dy()/1440))
	region := image.Rect(cell.Min.X-max(2, b.Dx()/640), cell.Min.Y-max(2, b.Dy()/360), cell.Min.X+size.X+max(2, b.Dx()/640), cell.Min.Y+size.Y+max(2, b.Dy()/360)).Intersect(b)
	binary := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, blue := rgb(screen.At(x, y))
			v := uint8(0)
			if g > 130 && g*4 > r*5 && g*5 > blue*6 {
				v = 255
			}
			binary.SetRGBA(x-region.Min.X, y-region.Min.Y, color.RGBA{v, v, v, 255})
		}
	}
	scene, err := gocv.ImageToMatRGB(binary)
	if err != nil {
		return false, err
	}
	defer scene.Close()
	template, err := gocv.ImageToMatRGB(ref)
	if err != nil {
		return false, err
	}
	defer template.Close()
	score, err := templateScore(scene, template, size)
	return score >= 0.80, err
}
