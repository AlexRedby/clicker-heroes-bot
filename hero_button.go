package main

import (
	"image"
	"image/draw"

	"gocv.io/x/gocv"
)

type heroButtonKind uint8

const (
	heroButtonUnknown heroButtonKind = iota
	heroButtonHire
	heroButtonLevelUp
)

// Captions are fixed artwork; the transparent references omit the changing
// blue/dark button background and retain every letter's fill and outline.
func readHeroButtonKind(screen image.Image, button image.Point) (heroButtonKind, error) {
	scores, err := heroButtonScores(screen, button)
	if err != nil {
		return heroButtonUnknown, err
	}
	// Native/resampled/dim positives remain above .83; missing-letter and
	// unrelated controls stay below .70 in the regression matrix.
	const minimum = 0.80
	const separation = 0.08
	if scores[0] >= minimum && scores[0]-scores[1] >= separation {
		return heroButtonHire, nil
	}
	if scores[1] >= minimum && scores[1]-scores[0] >= separation {
		return heroButtonLevelUp, nil
	}
	return heroButtonUnknown, nil
}

func heroButtonScores(screen image.Image, button image.Point) ([2]float32, error) {
	var scores [2]float32
	if screen == nil || !button.In(screen.Bounds()) {
		return scores, nil
	}
	b := screen.Bounds()
	region := heroButtonCaptionRegion(screen, button)
	if region.Empty() {
		return scores, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return scores, err
	}
	defer scene.Close()
	// Keep the two native rasterizations: scaling alone cannot recreate the
	// different antialiasing of captions rendered at 1280 and 2560 pixels.
	for _, artwork := range []struct {
		name         string
		index, width int
	}{
		{"heroes/hire.png", 0, 2560}, {"heroes/level-up.png", 1, 2560},
		{"heroes/hire-1280.png", 0, 1280}, {"heroes/level-up-1280.png", 1, 1280},
	} {
		reference, err := templateImage(artwork.name)
		if err != nil {
			return scores, err
		}
		size := image.Pt(max(1, reference.Bounds().Dx()*b.Dx()/artwork.width), max(1, reference.Bounds().Dy()*b.Dy()/(artwork.width*9/16)))
		if size.X > region.Dx() || size.Y > region.Dy() {
			continue
		}
		score, err := controlTemplateScore(scene, reference, size)
		if err != nil {
			return scores, err
		}
		scores[artwork.index] = max(scores[artwork.index], score)
	}
	return scores, nil
}

func heroButtonCaptionRegion(screen image.Image, button image.Point) image.Rectangle {
	if screen == nil {
		return image.Rectangle{}
	}
	b := screen.Bounds()
	// Both HIRE layouts and LVL UP fit above the detected button center.
	return image.Rect(b.Min.X+b.Dx()*55/1000, button.Y-b.Dy()*80/1440,
		b.Min.X+b.Dx()*140/1000, button.Y+b.Dy()*15/1440).Intersect(heroListViewport(screen))
}
