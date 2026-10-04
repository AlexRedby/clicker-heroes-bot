package main

import (
	"image"
	"image/draw"

	"gocv.io/x/gocv"
)

// Level-unavailable upgrades use a dark placeholder with a fixed border.
// Search the whole strip: neither the hero's name nor purchased checks matter.
func heroHasLockedUpgrade(screen image.Image, button image.Point) (bool, error) {
	if screen == nil || !button.In(screen.Bounds()) {
		return false, nil
	}
	b := screen.Bounds()
	region := image.Rect(b.Min.X+b.Dx()*15/100, button.Y, b.Min.X+b.Dx()*40/100, button.Y+b.Dy()*12/100).Intersect(b)
	ref, err := templateImage("heroes/upgrade-locked-1280.png")
	if err != nil {
		return false, err
	}
	size := image.Pt(max(1, ref.Bounds().Dx()*b.Dx()/1280), max(1, ref.Bounds().Dy()*b.Dy()/720))
	if region.Dx() < size.X || region.Dy() < size.Y {
		return false, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false, err
	}
	defer scene.Close()
	score, err := controlTemplateScore(scene, ref, size)
	return score >= 0.90, err
}
