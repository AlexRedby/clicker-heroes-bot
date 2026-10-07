package bot

import (
	"image"
	"image/color"
	"image/draw"
	"testing"

	"clicker-heroes-bot/internal/vision"

	xdraw "golang.org/x/image/draw"
)

func TestReadRelicUIRealStates(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wantKnown bool
		wantJunk  int
	}{
		{"inventory", true, 3},
		{"inventory-one-junk", true, 1},
		{"empty", true, 0},
		{"salvage-dialog", false, 0},
		{"upgrade", false, 0},
		{"tooltip", false, 0},
		{"ascension", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui := readRelicUI(loadTestImage(t, "../../testdata/relic-"+tc.name+".png"))
			if ui.known != tc.wantKnown || len(ui.junk) != tc.wantJunk {
				t.Fatalf("ui=%+v, want known=%v junk=%d", ui, tc.wantKnown, tc.wantJunk)
			}
			if tc.wantKnown {
				for i, equipped := range ui.equipment {
					if !equipped {
						t.Errorf("equipment slot %d not recognized", i+1)
					}
				}
			}
		})
	}
}

func TestRelicPanelPresentAllowsTooltipOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"inventory", true},
		{"inventory-one-junk", true},
		{"tooltip", true},
		{"empty", true},
		{"salvage-dialog", false},
		{"upgrade", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := relicPanelPresent(loadTestImage(t, "../../testdata/relic-"+tc.name+".png"))
			if got != tc.want {
				t.Fatalf("panel present=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadRelicUIRejectsAmbiguousInventory(t *testing.T) {
	base := loadTestImage(t, "../../testdata/relic-inventory.png")
	for _, tc := range []struct {
		name string
		edit func(image.Image) image.Image
	}{
		{"extra-card", func(src image.Image) image.Image {
			out := image.NewRGBA(src.Bounds())
			draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
			draw.Draw(out, vision.Rect(out, image.Rect(320, 382, 385, 470)), image.NewUniform(color.Black), image.Point{}, draw.Src)
			return out
		}},
		{"clipped", func(src image.Image) image.Image {
			out := image.NewRGBA(src.Bounds())
			draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
			draw.Draw(out, image.Rect(out.Bounds().Min.X, out.Bounds().Min.Y, out.Bounds().Min.X+out.Bounds().Dx()/2, out.Bounds().Max.Y), image.NewUniform(color.Black), image.Point{}, draw.Src)
			return out
		}},
		{"obscured", func(src image.Image) image.Image {
			out := image.NewRGBA(src.Bounds())
			draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
			draw.Draw(out, vision.Rect(out, image.Rect(280, 370, 500, 490)), image.NewUniform(color.Black), image.Point{}, draw.Src)
			return out
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readRelicUI(tc.edit(base)); got.known {
				t.Fatalf("ambiguous %s accepted: %+v", tc.name, got)
			}
		})
	}
}

func TestReadRelicUIAcceptsVerifiedFirstRowExtension(t *testing.T) {
	base := loadTestImage(t, "../../testdata/relic-inventory.png")
	out := image.NewRGBA(base.Bounds())
	draw.Draw(out, out.Bounds(), base, base.Bounds().Min, draw.Src)
	src := vision.Rect(out, image.Rect(249, 391, 319, 461))
	for _, region := range relicJunkRegions[3:] {
		dst := vision.Rect(out, region)
		draw.Draw(out, dst, out, src.Min, draw.Src)
	}
	ui := readRelicUI(out)
	if !ui.known || len(ui.junk) != 6 {
		t.Fatalf("extended first row=%+v", ui)
	}
}

func TestReadRelicUIRejectsUnverifiedSecondRow(t *testing.T) {
	base := loadTestImage(t, "../../testdata/relic-inventory.png")
	out := image.NewRGBA(base.Bounds())
	draw.Draw(out, out.Bounds(), base, base.Bounds().Min, draw.Src)
	src := vision.Rect(out, image.Rect(249, 391, 319, 461))
	dst := vision.Rect(out, image.Rect(104, 475, 175, 545))
	draw.Draw(out, dst, out, src.Min, draw.Src)
	if readRelicUI(out).known {
		t.Fatal("unverified second-row card was accepted")
	}
}

func TestReadRelicUIScalesAndPreservesPoint(t *testing.T) {
	base := loadTestImage(t, "../../testdata/relic-inventory.png")
	for _, scale := range []int{1, 2} {
		t.Run(string(rune('0'+scale)), func(t *testing.T) {
			img := base
			if scale == 1 {
				scaled := image.NewRGBA(image.Rect(0, 0, 1280, 720))
				xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), base, base.Bounds(), draw.Src, nil)
				img = scaled
			}
			ui := readRelicUI(img)
			if !ui.known || len(ui.junk) != 3 {
				t.Fatalf("scaled UI=%+v", ui)
			}
			want := relicTabPoint(img)
			if want == (image.Point{}) {
				t.Fatal("relic tab point missing")
			}
		})
	}
}

func TestRelicNotification(t *testing.T) {
	if !relicNotification(loadTestImage(t, "../../testdata/relic-notification.png")) {
		t.Fatal("real relic notification not detected")
	}
	for _, name := range []string{"relic-inventory.png", "relic-empty.png", "relic-tooltip.png", "relic-upgrade.png", "relic-ascension.png"} {
		if relicNotification(loadTestImage(t, "../../testdata/"+name)) {
			t.Fatalf("false notification for %s", name)
		}
	}
}
