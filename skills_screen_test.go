package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func TestReadSkillStatesRealFixtures(t *testing.T) {
	for _, path := range []string{"testdata/hero-tsuchi-x1.png", "testdata/hero-economy-x1.png", "testdata/hero-gog-before.png", "testdata/hero-panel-max.png"} {
		original := loadHeroScreen(t, path)
		for _, scale := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/scale, original.Bounds().Dy()/scale))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			states, err := readSkillStates(context.Background(), screen)
			if err != nil {
				t.Fatal(err)
			}
			for i, state := range states {
				wantKnown := path != "testdata/hero-gog-before.png" || i < 2 // Locks cover the remaining icons.
				wantActive := path == "testdata/hero-tsuchi-x1.png" && (i == 1 || i == 3 || i == 4 || i == 6)
				wantEnergized := path == "testdata/hero-tsuchi-x1.png" && i == 4
				if state.Known != wantKnown || state.Ready != (i < 2) || state.Active != wantActive || state.Energized != wantEnergized {
					t.Errorf("%s scale%d skill%d=%+v, want known=%t, ready=%t, active=%t, energized=%t", path, scale, i+1, state, wantKnown, i < 2, wantActive, wantEnergized)
				}
			}
		}
	}
}

func TestReadSkillStatesRejectsUnknownIcons(t *testing.T) {
	original := loadHeroScreen(t, "testdata/hero-tsuchi-x1.png")
	for _, scenario := range []string{"blank", "covered", "wrong icon"} {
		screen := image.NewRGBA(original.Bounds())
		draw.Draw(screen, screen.Bounds(), original, original.Bounds().Min, draw.Src)
		center := skillButton(screen, 0)
		region := image.Rect(center.X-45, center.Y-43, center.X+45, center.Y+43)
		switch scenario {
		case "blank":
			draw.Draw(screen, screen.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
		case "covered":
			draw.Draw(screen, region, image.NewUniform(color.Gray{Y: 180}), image.Point{}, draw.Src)
		case "wrong icon":
			other := skillButton(screen, 1)
			draw.Draw(screen, region, original, region.Min.Add(other.Sub(center)), draw.Src)
		}
		states, err := readSkillStates(context.Background(), screen)
		if err != nil {
			t.Fatal(err)
		}
		if states[0].Known || states[0].Ready {
			t.Fatalf("%s icon accepted: %+v", scenario, states[0])
		}
	}
}

func BenchmarkReadSkillStates(b *testing.B) {
	f, err := os.Open("testdata/hero-tsuchi-x1.png")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	screen, err := png.Decode(f)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := readSkillStates(context.Background(), screen); err != nil {
			b.Fatal(err)
		}
	}
}
