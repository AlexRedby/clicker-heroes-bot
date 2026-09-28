package main

import (
	"image"
	"image/draw"
	"image/jpeg"
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
		name         string
		x, y, height int
	}{
		{"sky", 850, 40, 65},
		{"monster", 700, 200, 90},
		{"hero panel", 180, 220, 75},
	} {
		t.Run(test.name, func(t *testing.T) {
			screen := image.NewRGBA(background.Bounds())
			draw.Draw(screen, screen.Bounds(), background, background.Bounds().Min, draw.Src)
			width := test.height * fb.Dx() / fb.Dy()
			for py := 0; py < test.height; py++ {
				for px := 0; px < width; px++ {
					c := fish.At(fb.Min.X+px*fb.Dx()/width, fb.Min.Y+py*fb.Dy()/test.height)
					_, _, _, alpha := c.RGBA()
					if alpha >= 0x8000 {
						screen.Set(test.x+px, test.y+py, c)
					}
				}
			}
			point, found := findFish(screen, fish)
			if !found || point.X < test.x+width/4 || point.X > test.x+width || point.Y < test.y+test.height/4 || point.Y > test.y+test.height*3/4 {
				t.Fatalf("fish location = %v, found = %v", point, found)
			}
		})
	}
}
