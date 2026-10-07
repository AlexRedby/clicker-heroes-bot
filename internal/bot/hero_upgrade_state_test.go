package bot

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"clicker-heroes-bot/internal/vision"

	xdraw "golang.org/x/image/draw"
)

func TestHeroLockedUpgradeNativeFrames(t *testing.T) {
	for _, tc := range []struct {
		path   string
		button image.Point
		locked bool
	}{
		{"../../testdata/hero-startup-cid-locked.png", image.Pt(204, 655), true},
		{"../../testdata/hero-nongilded-successor.jpg", image.Pt(102, 446), true},
		{"../../testdata/hero-nongilded-successor.jpg", image.Pt(102, 342), false},
		{"../../testdata/hero-startup-bottom-hire.png", image.Pt(204, 709), false},
		// Fisherman's artwork is gray despite already meeting its level requirements.
		{"../../testdata/hero-startup-bottom-hire.png", image.Pt(204, 919), false},
		{"../../testdata/hero-startup-bottom-hire.png", image.Pt(204, 1129), false},
		{"../../testdata/hero-startup-fisherman-after.png", image.Pt(204, 943), false},
		{"../../testdata/hero-startup-name-empty.png", image.Pt(204, 856), false},
		{"../../testdata/hero-startup-name-empty.png", image.Pt(204, 1066), false},
		{"../../testdata/hero-owned-disabled.png", image.Pt(204, 940), false},
	} {
		t.Run(fmt.Sprintf("%s/%v", tc.path, tc.button), func(t *testing.T) {
			source := loadTestImage(t, tc.path)
			for _, width := range []int{1280, 1920, 2560} {
				s := heroButtonResized(source, width)
				point := image.Pt(tc.button.X*width/source.Bounds().Dx(), tc.button.Y*width/source.Bounds().Dx())
				locked, err := heroHasLockedUpgrade(s, point)
				if err != nil || locked != tc.locked {
					t.Fatalf("width=%d locked=%t want=%t err=%v", width, locked, tc.locked, err)
				}
			}
		})
	}
}

func TestHeroLockedUpgradeAnywhereInStrip(t *testing.T) {
	// Compose the real unavailable-slot artwork into a real unlocked row.
	// Detection must not depend on preceding icons, purchased checks or slot count.
	source := loadTestImage(t, "../../testdata/hero-startup-bottom-hire.png")
	ref, err := vision.Template("heroes/upgrade-locked-1280.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []int{0, 2, 4, 7} {
		t.Run(fmt.Sprint(column), func(t *testing.T) {
			s := image.NewRGBA(source.Bounds())
			draw.Draw(s, s.Bounds(), source, s.Bounds().Min, draw.Src)
			if column > 0 {
				draw.Draw(s, image.Rect(380, 960, 462, 1042), image.NewUniform(color.RGBA{255, 224, 95, 255}), image.Point{}, draw.Src)
			}
			r := image.Rect(387+column*75, 960, 387+column*75+ref.Bounds().Dx()*2, 960+ref.Bounds().Dy()*2)
			xdraw.NearestNeighbor.Scale(s, r, ref, ref.Bounds(), draw.Src, nil)
			for _, width := range []int{1280, 1920, 2560} {
				locked, err := heroHasLockedUpgrade(heroButtonResized(s, width), image.Pt(204*width/2560, 919*width/2560))
				if err != nil || !locked {
					t.Fatalf("column=%d width=%d locked=%t err=%v", column, width, locked, err)
				}
			}
		})
	}
}

func TestHeroLockedUpgradeClippingAndCover(t *testing.T) {
	source := loadTestImage(t, "../../testdata/hero-nongilded-successor.jpg")
	s := image.NewRGBA(source.Bounds())
	draw.Draw(s, s.Bounds(), source, s.Bounds().Min, draw.Src)
	draw.Draw(s, image.Rect(192, 446, 512, 532), image.NewUniform(color.RGBA{30, 30, 30, 255}), image.Point{}, draw.Src)
	for _, tc := range []struct {
		screen image.Image
		button image.Point
	}{{nil, image.Point{}}, {source, image.Pt(102, 719)}, {source, image.Pt(-1, -1)}, {s, image.Pt(102, 446)}} {
		if locked, err := heroHasLockedUpgrade(tc.screen, tc.button); err != nil || locked {
			t.Fatalf("covered/clipped row reported a lock: %t %v", locked, err)
		}
	}
}
