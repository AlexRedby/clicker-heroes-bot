//go:build gocv

package main

import (
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocv.io/x/gocv"
)

const gocvFishThreshold = 0.97

func findFishGoCV(screen, fish image.Image) (image.Point, float32, error) {
	sb, fb := screen.Bounds(), fish.Bounds()
	if fb.Empty() {
		return image.Point{}, 0, nil
	}
	screenMat, err := gocv.ImageToMatRGB(screen)
	if err != nil {
		return image.Point{}, 0, err
	}
	defer screenMat.Close()

	var bestPoint image.Point
	var bestScore float32
	for degrees := 0; degrees < 360; degrees += 5 {
		angle := float64(degrees) * math.Pi / 180
		for height := max(50, sb.Dy()/30); height <= min(400, sb.Dy()/4); height += max(2, height/12) {
			template, _, _ := makeFishTemplate(fish, height, angle)
			w, h := template.Bounds().Dx(), template.Bounds().Dy()
			if w > sb.Dx() || h > sb.Dy() {
				continue
			}
			templateMat, err := gocv.ImageToMatRGB(template)
			if err != nil {
				return image.Point{}, 0, err
			}
			maskPixels := make([]byte, w*h)
			for i := range maskPixels {
				if template.Pix[i*4+3] >= 0xe0 {
					maskPixels[i] = 255
				}
			}
			mask, err := gocv.NewMatFromBytes(h, w, gocv.MatTypeCV8UC1, maskPixels)
			if err != nil {
				templateMat.Close()
				return image.Point{}, 0, err
			}
			result := gocv.NewMat()
			err = gocv.MatchTemplate(screenMat, templateMat, &result, gocv.TmCcorrNormed, mask)
			if err == nil {
				_, score, _, location := gocv.MinMaxLoc(result)
				if score > bestScore {
					bestScore = score
					bestPoint = image.Pt(sb.Min.X+location.X+w/2, sb.Min.Y+location.Y+h/2)
				}
			}
			result.Close()
			mask.Close()
			templateMat.Close()
			if err != nil {
				return image.Point{}, 0, err
			}
			if bestScore >= gocvFishThreshold {
				return bestPoint, bestScore, nil
			}
		}
	}
	return bestPoint, bestScore, nil
}

func TestCompareFishMatchers(t *testing.T) {
	fish, err := loadFish()
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	background, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("artifacts/gocv-comparison", 0755); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name     string
		fileName string
		position image.Point
		height   int
		degrees  int
	}{
		{name: "no fish", fileName: "no-fish.png"},
		{name: "upright", fileName: "upright.png", position: image.Pt(180, 220), height: 75},
		{name: "tilted", fileName: "tilted.png", position: image.Pt(850, 40), height: 65, degrees: 17},
		{name: "upside down", fileName: "upside-down.png", position: image.Pt(700, 200), height: 90, degrees: 180},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := image.NewRGBA(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			var inserted image.Rectangle
			var insertedFish *image.RGBA
			if test.height > 0 {
				angle := float64(test.degrees) * math.Pi / 180
				template, _, _ := makeFishTemplate(fish, test.height, angle)
				insertedFish = template
				inserted = template.Bounds().Add(test.position)
				draw.Draw(screen, inserted, template, image.Point{}, draw.Over)
			}
			fixture, err := os.Create(filepath.Join("artifacts/gocv-comparison", test.fileName))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(fixture, screen); err != nil {
				fixture.Close()
				t.Fatal(err)
			}
			if err := fixture.Close(); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			legacyPoint, legacyFound := findFish(screen, fish)
			legacyTime := time.Since(start)
			start = time.Now()
			cvPoint, cvScore, err := findFishGoCV(screen, fish)
			cvTime := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			cvFound := cvScore >= gocvFishThreshold
			t.Logf("legacy found=%t point=%v time=%s; GoCV found=%t point=%v score=%.5f time=%s", legacyFound, legacyPoint, legacyTime, cvFound, cvPoint, cvScore, cvTime)
			if legacyFound != (test.height > 0) {
				t.Fatalf("legacy detection differs from expected fish presence")
			}
			if test.height > 0 && insertedFish.RGBAAt(legacyPoint.X-test.position.X, legacyPoint.Y-test.position.Y).A == 0 {
				t.Fatalf("legacy point misses inserted fish pixels: %v", legacyPoint)
			}
			if test.height > 0 && cvFound && insertedFish.RGBAAt(cvPoint.X-test.position.X, cvPoint.Y-test.position.Y).A == 0 {
				t.Logf("GoCV point misses inserted fish pixels: %v", cvPoint)
			}
		})
	}
}
