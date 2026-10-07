package bot

import (
	"image"

	"clicker-heroes-bot/internal/vision"
)

// Match the distinctive fixed heading; the centered reward text moves when
// the Forge Core count changes. Controls are checked separately before input.
func relicJunkDialog(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	if !relicModalCream(screen) {
		return false
	}
	first, firstErr := vision.MatchControl(screen, image.Rect(390, 255, 890, 310), "ascension-junk/title.png")
	return firstErr == nil && first
}

func relicModalCream(screen image.Image) bool {
	for _, point := range []image.Point{{395, 245}, {885, 245}, {395, 480}, {885, 480}} {
		r := vision.Rect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
		red, green, blue := rgb(screen.At(r.X, r.Y))
		if red < 235 || green < 215 || blue < 160 {
			return false
		}
	}
	return true
}

// The ordinary salvage prompt shares the native Yes/No controls but has its
// own heading and button positions. It is not an Ascension reward dialog.
func relicSalvageDialog(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 ||
		!relicCream(screen, image.Pt(395, 245), image.Pt(885, 245), image.Pt(395, 465), image.Pt(885, 465)) {
		return false
	}
	found, err := vision.MatchControl(screen, image.Rect(401, 291, 870, 314), "relics/salvage-title.png")
	return err == nil && found
}

func relicJunkControl(screen image.Image, yes bool) (image.Point, bool, error) {
	regions := [...]image.Rectangle{image.Rect(420, 403, 620, 468), image.Rect(660, 403, 860, 468)}
	if relicSalvageDialog(screen) {
		regions = [...]image.Rectangle{image.Rect(420, 373, 620, 439), image.Rect(660, 373, 860, 439)}
	} else if !relicJunkDialog(screen) {
		return image.Point{}, false, nil
	}
	for i, name := range []string{"ascension-junk/yes.png", "ascension-junk/no.png"} {
		found, err := vision.MatchControl(screen, regions[i], name)
		if err != nil || !found {
			return image.Point{}, false, err
		}
	}
	region := regions[1]
	if yes {
		region = regions[0]
	}
	r := vision.Rect(screen, region)
	return r.Min.Add(r.Size().Div(2)), true, nil
}
