package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestControlsIgnoreSurroundingScenery(t *testing.T) {
	cases := []struct {
		name, fixture string
		core          image.Rectangle
		find          func(image.Image) (image.Point, bool, error)
	}{
		{"menu close", "save-menu.png", image.Rect(1006, 137, 1034, 165), func(img image.Image) (image.Point, bool, error) { return saveControl(img, 1) }},
		{"settings", "ascension-ancients.png", image.Rect(1232, 16, 1256, 40), func(img image.Image) (image.Point, bool, error) { return saveControl(img, 2) }},
		{"ascension", "ascension-ancients.png", image.Rect(1237, 247, 1251, 263), func(img image.Image) (image.Point, bool, error) { return ascensionControl(img, ascensionSpiral) }},
		{"gild close", "gild-roster.png", image.Rect(1137, 27, 1161, 51), func(img image.Image) (image.Point, bool, error) {
			return gildActionPoint(gameFrame{image: img, context: gameContext{modal: gildRosterModal}})
		}},
		{"single gild close", "gild-reward.png", image.Rect(982, 116, 1006, 140), func(img image.Image) (image.Point, bool, error) {
			return gildActionPoint(gameFrame{image: img, context: gameContext{modal: gildRewardModal}})
		}},
	}
	backgrounds := []color.Color{color.Black, color.White, color.RGBA{0, 170, 230, 255}}
	for _, tc := range cases {
		widths := []int{1280, 2560}
		if tc.name == "settings" {
			widths = append(widths, 640)
		}
		for _, width := range widths {
			original := exportFixture(t, tc.fixture, width)
			core := controlRect(original, tc.core)
			for bg, background := range backgrounds {
				for _, state := range []string{"present", "missing", "wrong"} {
					t.Run(fmt.Sprintf("%s/%d/background%d/%s", tc.name, width, bg, state), func(t *testing.T) {
						img := image.NewRGBA(original.Bounds())
						draw.Draw(img, img.Bounds(), original, original.Bounds().Min, draw.Src)
						// Hide Open All so the single-reward path must recognize its X.
						if tc.name == "single gild close" {
							draw.Draw(img, controlRect(img, image.Rect(885, 528, 989, 583)), image.NewUniform(background), image.Point{}, draw.Src)
						}
						draw.Draw(img, core.Inset(-20*width/1280), image.NewUniform(background), image.Point{}, draw.Src)
						switch state {
						case "present":
							draw.Draw(img, core, original, core.Min, draw.Src)
						case "wrong":
							// An unrelated striped shape must not identify an icon.
							for y := core.Min.Y; y < core.Max.Y; y++ {
								shade := color.Black
								if (y-core.Min.Y)/(3*width/1280)%2 == 0 {
									shade = color.White
								}
								draw.Draw(img, image.Rect(core.Min.X, y, core.Max.X, y+1), image.NewUniform(shade), image.Point{}, draw.Src)
							}
						}
						point, found, err := tc.find(img)
						if err != nil || found != (state == "present") {
							t.Fatalf("point=%v found=%v error=%v", point, found, err)
						}
						if found && !point.In(core) {
							t.Fatalf("click outside icon: %v", point)
						}
					})
				}
			}
		}
	}
}
