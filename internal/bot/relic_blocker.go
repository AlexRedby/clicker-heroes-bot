package bot

import (
	"image"

	"clicker-heroes-bot/internal/vision"
)

// relicJunkDialog recognizes the exact client blocker text. The Forge Core
// amount is deliberately outside the templates because it changes every save.
func relicJunkDialog(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	if !relicModalCream(screen) {
		return false
	}
	first, firstErr := vision.MatchControl(screen, image.Rect(390, 255, 890, 310), "ascension-junk/title.png")
	prefix, prefixErr := vision.MatchControl(screen, image.Rect(390, 316, 690, 350), "ascension-junk/reward-prefix.png")
	return firstErr == nil && prefixErr == nil && first && prefix
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

func relicJunkControl(screen image.Image, yes bool) (image.Point, bool, error) {
	if !relicJunkDialog(screen) {
		return image.Point{}, false, nil
	}
	regions := [...]image.Rectangle{image.Rect(420, 403, 620, 468), image.Rect(660, 403, 860, 468)}
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
