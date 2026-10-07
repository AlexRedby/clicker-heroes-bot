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

func TestHeroButtonNativeCaptions(t *testing.T) {
	for _, tc := range []struct {
		path string
		y    int
		want heroButtonKind
	}{
		{"../../testdata/hero-startup-alexa-unreadable.png", 959, heroButtonHire},
		{"../../testdata/hero-startup-alexa-unreadable.png", 1169, heroButtonHire},
		{"../../testdata/hero-startup-alexa-unreadable.png", 749, heroButtonLevelUp},
		{"../../testdata/hero-startup-zero.png", 655, heroButtonHire},
		{"../../testdata/hero-startup-bottom-hire.png", 1337, heroButtonHire},
		{"../../testdata/hero-startup-bottom-hire.png", 709, heroButtonLevelUp},
		{"../../testdata/hero-startup-fisherman-before.png", 966, heroButtonHire},
		{"../../testdata/hero-startup-fisherman-after.png", 943, heroButtonLevelUp},
		{"../../testdata/hero-nongilded-successor.jpg", 563, heroButtonHire},
		{"../../testdata/hero-nongilded-successor.jpg", 446, heroButtonLevelUp},
		{"../../testdata/hero-economy-x1.png", 1127, heroButtonHire},
	} {
		t.Run(tc.path+"/"+image.Pt(0, tc.y).String(), func(t *testing.T) {
			s := loadTestImage(t, tc.path)
			button := image.Pt(s.Bounds().Dx()*8/100, tc.y)
			scores, err := heroButtonScores(s, button)
			got, kindErr := readHeroButtonKind(s, button)
			t.Logf("scores HIRE=%.4f LVLUP=%.4f", scores[0], scores[1])
			if err != nil || kindErr != nil || got != tc.want {
				t.Fatalf("kind=%v want=%v scores=%v errors=%v/%v", got, tc.want, scores, err, kindErr)
			}
		})
	}
}

func TestHeroButtonBackgroundAndBrightness(t *testing.T) {
	for _, tc := range []struct {
		y    int
		want heroButtonKind
	}{{959, heroButtonHire}, {749, heroButtonLevelUp}} {
		for _, background := range []color.RGBA{{45, 60, 70, 255}, {230, 50, 110, 255}, {170, 180, 190, 255}} {
			for _, dim := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%v/dim=%v", tc.y, background, dim), func(t *testing.T) {
					source := loadTestImage(t, "../../testdata/hero-startup-alexa-unreadable.png")
					s := image.NewRGBA(source.Bounds())
					draw.Draw(s, s.Bounds(), source, source.Bounds().Min, draw.Src)
					button := image.Pt(204, tc.y)
					region := heroButtonCaptionRegion(s, button)
					for y := region.Min.Y; y < region.Max.Y; y++ {
						for x := region.Min.X; x < region.Max.X; x++ {
							r, g, blue := rgb(source.At(x, y))
							if blue > 150 && blue > r+40 && blue >= g-10 && g > 90 {
								s.Set(x, y, background)
							} else if dim {
								s.Set(x, y, color.RGBA{uint8(r / 3), uint8(g / 3), uint8(blue / 3), 255})
							}
						}
					}
					for _, width := range []int{1280, 1920, 2560} {
						resized := heroButtonResized(s, width)
						point := image.Pt(button.X*width/2560, button.Y*width/2560)
						got, err := readHeroButtonKind(resized, point)
						scores, _ := heroButtonScores(resized, point)
						t.Logf("width=%d scores=%v", width, scores)
						if err != nil || got != tc.want {
							t.Errorf("width=%d kind=%v want=%v scores=%v error=%v", width, got, tc.want, scores, err)
						}
					}
				})
			}
		}
	}
}

func heroButtonResized(source image.Image, width int) image.Image {
	s := image.NewRGBA(image.Rect(0, 0, width, source.Bounds().Dy()*width/source.Bounds().Dx()))
	xdraw.ApproxBiLinear.Scale(s, s.Bounds(), source, source.Bounds(), draw.Src, nil)
	return s
}

func TestHeroButtonDoesNotStartOCR(t *testing.T) {
	original := tesseractExecutable
	tesseractExecutable = "/missing/hero-caption-must-not-use-ocr"
	t.Cleanup(func() { tesseractExecutable = original })
	s := loadTestImage(t, "../../testdata/hero-startup-alexa-unreadable.png")
	for _, tc := range []struct {
		y    int
		want heroButtonKind
	}{{959, heroButtonHire}, {749, heroButtonLevelUp}, {400, heroButtonUnknown}} {
		if got, err := readHeroButtonKind(s, image.Pt(204, tc.y)); err != nil || got != tc.want {
			t.Fatalf("kind=%v want=%v err=%v", got, tc.want, err)
		}
	}
}

func TestHeroButtonAmbiguousCaption(t *testing.T) {
	source := loadTestImage(t, "../../testdata/hero-startup-alexa-unreadable.png")
	s := image.NewRGBA(source.Bounds())
	draw.Draw(s, s.Bounds(), source, source.Bounds().Min, draw.Src)
	button := image.Pt(204, 959)
	region := heroButtonCaptionRegion(s, button)
	draw.Draw(s, region, image.NewUniform(color.RGBA{98, 190, 247, 255}), image.Point{}, draw.Src)
	for i, name := range []string{"heroes/hire.png", "heroes/level-up.png"} {
		caption, err := vision.Template(name)
		if err != nil {
			t.Fatal(err)
		}
		pos := image.Pt(180, region.Min.Y+5+40*i)
		draw.Draw(s, caption.Bounds().Add(pos), caption, caption.Bounds().Min, draw.Over)
	}
	scores, _ := heroButtonScores(s, button)
	if got, err := readHeroButtonKind(s, button); err != nil || got != heroButtonUnknown {
		t.Fatalf("ambiguous kind=%v scores=%v err=%v", got, scores, err)
	}
}

func TestHeroButtonResolutionAndPosition(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		y          int
		want       heroButtonKind
	}{
		{"blue-hire", "../../testdata/hero-startup-alexa-unreadable.png", 959, heroButtonHire},
		{"blue-level", "../../testdata/hero-startup-alexa-unreadable.png", 749, heroButtonLevelUp},
		{"native-dark-hire", "../../testdata/hero-nongilded-successor.jpg", 563, heroButtonHire},
	} {
		for _, width := range []int{1280, 1920, 2560} {
			for _, position := range []int{455, 638, 929} {
				t.Run(fmt.Sprintf("%s/%d/%d", tc.name, width, position), func(t *testing.T) {
					source := loadTestImage(t, tc.path)
					s := image.NewRGBA(image.Rect(0, 0, width, width*9/16))
					xdraw.ApproxBiLinear.Scale(s, s.Bounds(), source, source.Bounds(), draw.Src, nil)
					old := image.Pt(width*8/100, tc.y*s.Bounds().Dy()/source.Bounds().Dy())
					button := image.Pt(old.X, s.Bounds().Dy()*position/1000)
					region := heroButtonCaptionRegion(s, old)
					caption := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
					draw.Draw(caption, caption.Bounds(), s, region.Min, draw.Src)
					// Translate the real native pixels; keep the actual HUD viewport.
					draw.Draw(s, image.Rect(0, s.Bounds().Dy()*38/100, width*42/100, s.Bounds().Dy()), image.NewUniform(color.RGBA{220, 170, 50, 255}), image.Point{}, draw.Src)
					dest := region.Add(image.Pt(0, button.Y-old.Y))
					draw.Draw(s, dest, caption, image.Point{}, draw.Src)
					got, err := readHeroButtonKind(s, button)
					scores, _ := heroButtonScores(s, button)
					t.Logf("scores=%v", scores)
					if err != nil || got != tc.want {
						t.Fatalf("kind=%v want=%v scores=%v error=%v", got, tc.want, scores, err)
					}
				})
			}
		}
	}
}

func TestHeroButtonClippedAndUnrelated(t *testing.T) {
	for _, tc := range []struct {
		path string
		y    int
	}{
		{"../../testdata/hero-owned-disabled.png", 709},
		{"../../testdata/hero-owned-disabled.png", 920},
		{"../../testdata/hero-startup-alexa-unreadable.png", 400},
	} {
		s := loadTestImage(t, tc.path)
		if got, err := readHeroButtonKind(s, image.Pt(204, tc.y)); err != nil || got != heroButtonUnknown {
			t.Fatalf("%s y=%d kind=%v err=%v", tc.path, tc.y, got, err)
		}
	}
	source := loadTestImage(t, "../../testdata/hero-startup-alexa-unreadable.png")
	old := image.Pt(204, 959)
	region := heroButtonCaptionRegion(source, old)
	for _, y := range []int{560, 1439} {
		s := image.NewRGBA(source.Bounds())
		draw.Draw(s, s.Bounds(), source, source.Bounds().Min, draw.Src)
		draw.Draw(s, image.Rect(0, 548, 1100, 1440), image.NewUniform(color.RGBA{220, 170, 50, 255}), image.Point{}, draw.Src)
		// At the bottom, move the label below its usual center to clip it.
		shift := y - old.Y
		if y == 1439 {
			shift += 20
		}
		draw.Draw(s, region.Add(image.Pt(0, shift)), source, region.Min, draw.Src)
		button := image.Pt(204, y)
		scores, _ := heroButtonScores(s, button)
		if got, err := readHeroButtonKind(s, button); err != nil || got != heroButtonUnknown {
			t.Fatalf("clipped y=%d kind=%v scores=%v err=%v", y, got, scores, err)
		}
	}
	if got, err := readHeroButtonKind(nil, image.Point{}); err != nil || got != heroButtonUnknown {
		t.Fatalf("nil kind=%v err=%v", got, err)
	}
}

func BenchmarkHeroButtonKind(b *testing.B) {
	s := loadTestImage(b, "../../testdata/hero-startup-alexa-unreadable.png")
	button := image.Pt(204, 959)
	b.ResetTimer()
	for b.Loop() {
		if got, err := readHeroButtonKind(s, button); err != nil || got != heroButtonHire {
			b.Fatalf("kind=%v err=%v", got, err)
		}
	}
}

func TestHeroButtonObscuredCaption(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		y          int
		rect       image.Rectangle
		background color.RGBA
	}{
		{"covered", "../../testdata/hero-startup-alexa-unreadable.png", 959, image.Rect(179, 925, 294, 974), color.RGBA{98, 190, 247, 255}},
		// This exact partial-H obstruction was falsely accepted by padded OCR.
		{"missing-H", "../../testdata/hero-startup-alexa-unreadable.png", 959, image.Rect(179, 925, 208, 974), color.RGBA{98, 190, 247, 255}},
		{"missing-I", "../../testdata/hero-startup-alexa-unreadable.png", 959, image.Rect(220, 925, 233, 974), color.RGBA{98, 190, 247, 255}},
		{"missing-R", "../../testdata/hero-startup-alexa-unreadable.png", 959, image.Rect(239, 925, 264, 974), color.RGBA{98, 190, 247, 255}},
		{"level-missing-UP", "../../testdata/hero-startup-alexa-unreadable.png", 749, image.Rect(270, 715, 320, 764), color.RGBA{98, 190, 247, 255}},
		{"native-dark-missing-H", "../../testdata/hero-nongilded-successor.jpg", 563, image.Rect(89, 538, 107, 557), color.RGBA{70, 80, 83, 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := loadTestImage(t, tc.path)
			s := image.NewRGBA(source.Bounds())
			draw.Draw(s, s.Bounds(), source, source.Bounds().Min, draw.Src)
			draw.Draw(s, tc.rect, image.NewUniform(tc.background), image.Point{}, draw.Src)
			for _, width := range []int{1280, 1920, 2560} {
				resized := heroButtonResized(s, width)
				button := image.Pt(width*8/100, tc.y*width/source.Bounds().Dx())
				scores, _ := heroButtonScores(resized, button)
				got, err := readHeroButtonKind(resized, button)
				t.Logf("width=%d scores HIRE=%.4f LVLUP=%.4f", width, scores[0], scores[1])
				if err != nil || got != heroButtonUnknown {
					t.Fatalf("kind=%v scores=%v error=%v", got, scores, err)
				}
			}
		})
	}
}
