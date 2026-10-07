package bot

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"clicker-heroes-bot/internal/vision"

	xdraw "golang.org/x/image/draw"
)

func TestRelicJunkDialogRealAndScaled(t *testing.T) {
	for _, fixture := range []string{"ascension-junk", "ascension-junk-940"} {
		t.Run(fixture, func(t *testing.T) {
			base := loadTestImage(t, "../../testdata/"+fixture+".png")
			for _, scale := range []int{1, 2} {
				t.Run(string(rune('0'+scale)), func(t *testing.T) {
					img := base
					if scale == 1 {
						scaled := image.NewRGBA(image.Rect(0, 0, 1280, 720))
						xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), base, base.Bounds(), draw.Src, nil)
						img = scaled
					}
					if !relicJunkDialog(img) {
						t.Fatal("junk blocker not recognized")
					}
					for _, yes := range []bool{true, false} {
						point, found, err := relicJunkControl(img, yes)
						if err != nil || !found || point == (image.Point{}) {
							t.Fatalf("yes=%v point=%v found=%v err=%v", yes, point, found, err)
						}
					}
				})
			}

		})
	}
}

func TestRelicJunkDialogRejectsOtherModalsAndChangedText(t *testing.T) {
	for _, path := range []string{
		"../../testdata/ascension-confirm.png",
		"../../testdata/relic-salvage-dialog.png",
		"../../testdata/relic-upgrade.png",
		"../../testdata/mercenary-quests.png",
	} {
		t.Run(path, func(t *testing.T) {
			if relicJunkDialog(loadTestImage(t, path)) {
				t.Fatal("unrelated modal recognized as junk blocker")
			}
		})
	}
	base := loadTestImage(t, "../../testdata/ascension-junk.png")
	changed := image.NewRGBA(base.Bounds())
	draw.Draw(changed, changed.Bounds(), base, base.Bounds().Min, draw.Src)
	// Removing the distinctive warning heading must reject the dialog.
	draw.Draw(changed, vision.Rect(changed, image.Rect(390, 255, 890, 310)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if relicJunkDialog(changed) {
		t.Fatal("damaged fixed heading accepted")
	}
	for _, yes := range []bool{true, false} {
		if point, found, err := relicJunkControl(changed, yes); err != nil || found || point != (image.Point{}) {
			t.Fatalf("changed text control accepted: yes=%v point=%v found=%v err=%v", yes, point, found, err)
		}
	}
}

func TestRelicJunkDialogIgnoresCountAndBackgroundButRequiresControls(t *testing.T) {
	base := loadTestImage(t, "../../testdata/ascension-junk.png")
	changed := image.NewRGBA(base.Bounds())
	draw.Draw(changed, changed.Bounds(), base, base.Bounds().Min, draw.Src)
	draw.Draw(changed, vision.Rect(changed, image.Rect(838, 316, 888, 343)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(changed, vision.Rect(changed, image.Rect(0, 0, 1280, 200)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if !relicJunkDialog(changed) {
		t.Fatal("variable count or scenery affected fixed-text matching")
	}
	for _, yes := range []bool{true, false} {
		if _, found, err := relicJunkControl(changed, yes); err != nil || !found {
			t.Fatal("variable count affected native control")
		}
	}
	draw.Draw(changed, vision.Rect(changed, image.Rect(660, 403, 860, 468)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	for _, yes := range []bool{true, false} {
		if _, found, err := relicJunkControl(changed, yes); err != nil || found {
			t.Fatal("incomplete dialog allowed input")
		}
	}
}

func TestRelicSalvageDialogRequiresItsOwnHeadingAndControls(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/relic-salvage-dialog.png")
	if !relicSalvageDialog(screen) || relicJunkDialog(screen) {
		t.Fatal("ordinary salvage prompt not distinguished")
	}
	for _, yes := range []bool{true, false} {
		point, found, err := relicJunkControl(screen, yes)
		if err != nil || !found || point == (image.Point{}) {
			t.Fatal("salvage control missing", yes, point, err)
		}
	}
	changed := image.NewRGBA(screen.Bounds())
	draw.Draw(changed, changed.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(changed, vision.Rect(changed, image.Rect(390, 316, 890, 340)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if !relicSalvageDialog(changed) {
		t.Fatal("variable reward line affected fixed-heading recognition")
	}
	for _, name := range []string{"ascension-confirm", "ascension-junk-940", "relic-upgrade"} {
		if relicSalvageDialog(loadTestImage(t, "../../testdata/"+name+".png")) {
			t.Fatal("unrelated prompt accepted", name)
		}
	}
}
