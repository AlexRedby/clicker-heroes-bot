package bot

import (
	"image"
	"image/color"
	"image/draw"

	"clicker-heroes-bot/internal/vision"

	"gocv.io/x/gocv"
)

// Level-unavailable upgrades use a dark tile with a fixed border.
// Search the whole strip: neither the hero's name nor purchased checks matter.
func heroHasLockedUpgrade(screen image.Image, button image.Point) (bool, error) {
	if screen == nil || !button.In(screen.Bounds()) {
		return false, nil
	}
	b := screen.Bounds()
	region := image.Rect(b.Min.X+b.Dx()*15/100, button.Y, b.Min.X+b.Dx()*40/100, button.Y+b.Dy()*12/100).Intersect(b)
	ref, err := vision.Template("heroes/upgrade-locked-1280.png")
	if err != nil {
		return false, err
	}
	size := image.Pt(max(1, ref.Bounds().Dx()*b.Dx()/1280), max(1, ref.Bounds().Dy()*b.Dy()/720))
	if region.Dx() < size.X || region.Dy() < size.Y {
		return false, nil
	}
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	// Some unavailable tiles retain dim artwork. Collapse those dark pixels
	// in both images so the shared tile state does not depend on its icon.
	reference := image.NewRGBA(ref.Bounds())
	draw.Draw(reference, reference.Bounds(), ref, ref.Bounds().Min, draw.Src)
	for _, s := range []*image.RGBA{crop, reference} {
		for y := s.Bounds().Min.Y; y < s.Bounds().Max.Y; y++ {
			for x := s.Bounds().Min.X; x < s.Bounds().Max.X; x++ {
				r, g, b := rgb(s.At(x, y))
				if max(r, g, b) < 80 {
					s.Set(x, y, color.Black)
				}
			}
		}
	}
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false, err
	}
	defer scene.Close()
	score, err := vision.ControlScore(scene, reference, size)
	return score >= 0.90, err
}
