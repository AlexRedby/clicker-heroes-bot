package bot

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func TestReadSkillStatesRealFixtures(t *testing.T) {
	for _, path := range []string{"../../testdata/hero-tsuchi-x1.png", "../../testdata/hero-economy-x1.png", "../../testdata/hero-gog-before.png", "../../testdata/hero-panel-max.png"} {
		original := loadTestImage(t, path)
		for _, scale := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/scale, original.Bounds().Dy()/scale))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			states, err := readSkillStates(context.Background(), screen)
			if err != nil {
				t.Fatal(err)
			}
			for i, state := range states {
				wantKnown := path != "../../testdata/hero-gog-before.png" || i < 2 // Locks cover the remaining icons.
				wantActive := path == "../../testdata/hero-tsuchi-x1.png" && (i == 1 || i == 3 || i == 4 || i == 6)
				wantEnergized := path == "../../testdata/hero-tsuchi-x1.png" && i == 4
				if state.Known != wantKnown || state.Ready != (i < 2) || state.Active != wantActive || state.Energized != wantEnergized {
					t.Errorf("%s scale%d skill%d=%+v, want known=%t, ready=%t, active=%t, energized=%t", path, scale, i+1, state, wantKnown, i < 2, wantActive, wantEnergized)
				}
			}
		}
	}
}

func TestReadSkillStatesRejectsUnknownIcons(t *testing.T) {
	original := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
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

func TestReadSkillStatesWithoutToolbar(t *testing.T) {
	for _, path := range []string{"../../testdata/hero-startup-zero.png", "../../testdata/hero-startup-gold.png"} {
		original := loadTestImage(t, path)
		for _, scale := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/scale, original.Bounds().Dy()/scale))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			states, err := readSkillStates(context.Background(), screen)
			if err != nil || states != ([9]skillState{}) {
				t.Fatalf("%s scale%d absent toolbar: states=%+v err=%v", path, scale, states, err)
			}
		}
	}
}

func TestProgressionReaderUsesSharedBuffsAndModeOnly(t *testing.T) {
	states := [9]skillState{
		2: {Known: true, Active: true},
		6: {Known: true, Active: true, Energized: true},
	}
	if got, want := progressionBuffs(states), uint8(1|2|8); got != want {
		t.Fatalf("progressionBuffs=%04b, want %04b", got, want)
	}
	screen := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
	// The progression boot icon is a global HUD control; a non-Heroes panel
	// must not become unknown merely because the Heroes tab is hidden.
	nonHeroes := image.NewRGBA(screen.Bounds())
	draw.Draw(nonHeroes, nonHeroes.Bounds(), screen, screen.Bounds().Min, draw.Src)
	b := nonHeroes.Bounds()
	draw.Draw(nonHeroes, image.Rect(b.Min.X+b.Dx()*45/1000, b.Min.Y+b.Dy()*15/100, b.Min.X+b.Dx()*70/1000, b.Min.Y+b.Dy()*21/100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(nonHeroes, image.Rect(b.Min.X+b.Dx()*90/1000, b.Min.Y+b.Dy()*33/100, b.Min.X+b.Dx()*450/1000, b.Min.Y+b.Dy()*38/100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	known, _, err := progressionMode(nonHeroes)
	if err != nil || !known {
		t.Fatalf("global progression HUD on non-Heroes panel: known=%t err=%v", known, err)
	}
	state, err := readProgressionState(context.Background(), nonHeroes, states, true)
	if err != nil || !state.Known || state.Zone != 0 || state.Buffs != 0 {
		t.Fatalf("mode-only state=%+v err=%v", state, err)
	}
}

func BenchmarkReadSkillStates(b *testing.B) {
	screen := loadTestImage(b, "../../testdata/hero-tsuchi-x1.png")
	b.ResetTimer()
	for b.Loop() {
		if _, err := readSkillStates(context.Background(), screen); err != nil {
			b.Fatal(err)
		}
	}
}

// Clean, unlocked icons rendered from official client build 1.0e12-6144 sprites.
// Unlike the older screen fixtures, every slot is ready, with no cooldown overlay.
func TestReadSkillStatesReadyIcons(t *testing.T) {
	original := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
	icons := loadTestImage(t, "../../testdata/skills-ready-icons.png")
	ready := image.NewRGBA(original.Bounds())
	draw.Draw(ready, ready.Bounds(), original, original.Bounds().Min, draw.Src)
	for i := 0; i < 9; i++ {
		center := skillButton(ready, i).Add(image.Pt(2, -2))
		region := image.Rect(center.X-51, center.Y-51, center.X+51, center.Y+51)
		draw.Draw(ready, region, icons, image.Pt(0, i*102), draw.Src)
	}
	for _, scale := range []int{1, 2} {
		screen := image.NewRGBA(image.Rect(0, 0, ready.Bounds().Dx()/scale, ready.Bounds().Dy()/scale))
		xdraw.CatmullRom.Scale(screen, screen.Bounds(), ready, ready.Bounds(), draw.Src, nil)
		states, err := readSkillStates(context.Background(), screen)
		if err != nil {
			t.Fatal(err)
		}
		for i, state := range states {
			if !state.Known || !state.Ready {
				t.Errorf("scale%d skill%d=%+v, want known and ready", scale, i+1, state)
			}
		}
	}
}

func TestUtilitySkillsReadyOnRealScreen(t *testing.T) {
	for _, path := range []string{"../../testdata/hero-scrollbar-before.png", "../../testdata/fish-over-scrollbar.png"} {
		original := loadTestImage(t, path)
		for _, scale := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/scale, original.Bounds().Dy()/scale))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			states, err := readSkillStates(context.Background(), screen)
			if err != nil {
				t.Fatal(err)
			}
			p := skillPlanner{}
			if !states[8].Known || !states[8].Ready {
				t.Fatalf("%s scale%d Reload: %+v", path, scale, states[8])
			}
			if got := fmt.Sprint(p.plan(states, time.Now())); got != "[3 5 8 9]" {
				t.Fatalf("%s scale%d plan=%s, states=%+v", path, scale, got, states)
			}
			for _, index := range []int{7, 8} {
				for _, scenario := range []string{"covered", "wrong icon"} {
					damaged := image.NewRGBA(screen.Bounds())
					draw.Draw(damaged, damaged.Bounds(), screen, screen.Bounds().Min, draw.Src)
					center := skillButton(screen, index)
					region := image.Rect(center.X-screen.Bounds().Dx()*20/1000, center.Y-screen.Bounds().Dy()*32/1000, center.X+screen.Bounds().Dx()*20/1000, center.Y+screen.Bounds().Dy()*32/1000)
					if scenario == "covered" {
						draw.Draw(damaged, region, image.NewUniform(color.Gray{Y: 180}), image.Point{}, draw.Src)
					} else {
						draw.Draw(damaged, region, screen, region.Min.Add(skillButton(screen, 0).Sub(center)), draw.Src)
					}
					got, err := readSkillStates(context.Background(), damaged)
					if err != nil {
						t.Fatal(err)
					}
					if got[index].Known || got[index].Ready {
						t.Fatalf("%s scale%d accepted %s as skill%d: %+v", path, scale, scenario, index+1, got[index])
					}
				}
			}
		}
	}
}
