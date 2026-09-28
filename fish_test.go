package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"testing"
)

func TestFindFishOnGameScreen(t *testing.T) {
	fish, err := loadFish()
	if err != nil {
		t.Fatal(err)
	}
	// Source: https://images.steamusercontent.com/ugc/402309122797677802/9EF9E586297810235B16974C1B52C2312B13D83E/
	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	background, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if point, found := findFish(background, fish); found {
		t.Fatalf("found fish at %v on a game screen without fish", point)
	}

	fb := fish.Bounds()
	for _, test := range []struct {
		name                string
		x, y, height, angle int
	}{
		{"sky tilted", 850, 40, 65, 17},
		{"sky between angles", 850, 40, 65, 2},
		{"monster tilted", 700, 200, 90, -22},
		{"hero panel upright", 180, 220, 75, 0},
		{"sky sideways", 850, 40, 65, 90},
		{"sky upside down", 850, 40, 65, 180},
		{"hero panel diagonal", 180, 220, 75, 225},
		{"monster sideways", 700, 200, 90, 270},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := image.NewRGBA(background.Bounds())
			fishPixels := image.NewAlpha(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			width := test.height * fb.Dx() / fb.Dy()
			angle := float64(test.angle) * math.Pi / 180
			cos, sin := math.Cos(angle), math.Sin(angle)
			rotWidth := int(math.Ceil(math.Abs(float64(width)*cos) + math.Abs(float64(test.height)*sin)))
			rotHeight := int(math.Ceil(math.Abs(float64(width)*sin) + math.Abs(float64(test.height)*cos)))
			for py := 0; py < rotHeight; py++ {
				for px := 0; px < rotWidth; px++ {
					dx, dy := float64(px)-float64(rotWidth)/2, float64(py)-float64(rotHeight)/2
					sx := cos*dx + sin*dy + float64(width)/2
					sy := -sin*dx + cos*dy + float64(test.height)/2
					if sx < 0 || sy < 0 || sx >= float64(width) || sy >= float64(test.height) {
						continue
					}
					c := fish.At(fb.Min.X+int(sx)*fb.Dx()/width, fb.Min.Y+int(sy)*fb.Dy()/test.height)
					_, _, _, alpha := c.RGBA()
					if alpha >= 0x8000 {
						screen.Set(test.x+px, test.y+py, c)
						fishPixels.SetAlpha(test.x+px, test.y+py, color.Alpha{A: 255})
					}
				}
			}
			point, found := findFish(screen, fish)
			if !found || fishPixels.AlphaAt(point.X, point.Y).A == 0 {
				t.Fatalf("fish location = %v, found = %v", point, found)
			}
		})
	}
}
