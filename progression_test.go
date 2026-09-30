package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func TestProgressionScreen(t *testing.T) {
	for _, tc := range []struct {
		path    string
		enabled bool
		zone    int
		damage  float64
		buffs   uint8
	}{
		{"hero-scrollbar-before.png", true, 8127, 0, 0}, {"fish-over-scrollbar.png", true, 8171, 0, 0},
		{"hero-owned-disabled.png", true, 5707, 0, 0}, {"hero-economy-x1.png", true, 6344, 0, 0}, {"hero-panel-max.png", true, 5595, 349 + math.Log10(4.505), 0},
		{"hero-tsuchi-x1.png", false, 12654, 819 + math.Log10(2.502), 2}, {"hero-gog-before.png", false, 2404, 158 + math.Log10(6.893), 0}, {"hero-gog-tooltip.png", false, 2404, 289 + math.Log10(1.168), 0},
	} {
		original := loadHeroScreen(t, "testdata/"+tc.path)
		for _, divisor := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/divisor, original.Bounds().Dy()/divisor))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			known, enabled, err := progressionMode(screen)
			if err != nil || !known || enabled != tc.enabled {
				t.Errorf("%s /%d known=%t enabled=%t err=%v", tc.path, divisor, known, enabled, err)
			}
		}
		if _, err := exec.LookPath("tesseract"); err != nil {
			if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
				t.Fatal(err)
			}
			continue
		}
		for _, divisor := range []int{1, 2} {
			screen := image.NewRGBA(image.Rect(0, 0, original.Bounds().Dx()/divisor, original.Bounds().Dy()/divisor))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			state, err := readProgressionState(context.Background(), screen)
			wantDamageKnown := !tc.enabled || tc.zone%5 == 0
			if err != nil || !state.Known || state.Enabled != tc.enabled || state.Zone != tc.zone || state.DamageKnown != wantDamageKnown || math.Abs(state.Damage-tc.damage) > .001 || state.Buffs != tc.buffs {
				t.Errorf("%s /%d state=%+v err=%v", tc.path, divisor, state, err)
			}
		}
	}
	screen := loadHeroScreen(t, "testdata/hero-tsuchi-x1.png")
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(covered, image.Rect(2430, 360, 2560, 480), image.NewUniform(color.Black), image.Point{}, draw.Src)
	for _, invalid := range []image.Image{nil, image.NewRGBA(image.Rect(0, 0, 2560, 1440)), covered} {
		known, _, err := progressionMode(invalid)
		if err != nil || known {
			t.Fatalf("unknown UI accepted: %t %v", known, err)
		}
	}
}

func TestProgressionPlanner(t *testing.T) {
	now := time.Unix(1000, 0)
	farm := progressionState{Known: true, Zone: 109, Damage: 100, DamageKnown: true}
	p := progressionPlanner{}
	if !p.observe(farm, now) {
		t.Fatal("startup farm should enable once")
	}
	boss := farm
	boss.Zone = 110
	boss.Enabled = true
	boss.Buffs = 1
	boss.Damage = 101
	if p.observe(boss, now) {
		t.Fatal("already enabled")
	}
	if p.observe(farm, now) || p.wallZone != 110 || p.failures != 1 {
		t.Fatalf("failed boss not retained: %+v", p)
	}
	if p.observe(farm, now.Add(2*time.Minute)) {
		t.Fatal("unchanged damage retried")
	}
	sameBuff := farm
	sameBuff.Buffs = 1
	if p.observe(sameBuff, now.Add(2*time.Minute)) {
		t.Fatal("same expired buff retried")
	}
	growth := farm
	growth.Damage = 101 + math.Log10(2) + 0.001
	if p.observe(growth, now.Add(30*time.Second)) {
		t.Fatal("cooldown ignored")
	}
	if !p.observe(growth, now.Add(2*time.Minute)) {
		t.Fatal("damage growth ignored")
	}
	stronger := farm
	stronger.Buffs = 5
	if !p.observe(stronger, now.Add(2*time.Minute)) {
		t.Fatal("new energized buff ignored")
	}
	boss.Buffs = 5
	boss.Damage = growth.Damage
	p.observe(boss, now.Add(2*time.Minute))
	p.observe(farm, now.Add(3*time.Minute))
	if p.failures != 2 || !p.nextAttempt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("repeated failure backoff: %+v", p)
	}
	if p.observe(stronger, now.Add(6*time.Minute)) {
		t.Fatal("already tried energized buff retried")
	}
	advanced := boss
	advanced.Zone = 111
	p.observe(advanced, now.Add(6*time.Minute))
	if p.wallZone != 0 || p.failures != 0 {
		t.Fatal("passed boss retains wall")
	}
	reset := farm
	reset.Zone = 1
	if !p.observe(reset, now.Add(6*time.Minute)) || p.wallZone != 0 {
		t.Fatal("reset does not enable progression")
	}
	if p.observe(progressionState{}, now.Add(7*time.Minute)) {
		t.Fatal("unknown UI acted")
	}
}

func TestProgressionInput(t *testing.T) {
	for _, scenario := range []string{"enable", "already enabled", "unknown", "missed toggle", "fish", "external toggle", "pause during read", "pause during fish", "pause during confirmation", "cancel during key", "unreadable", "missed boss frame"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controls := pauseControl{}
			generation := controls.snapshot()
			p := progressionPlanner{}
			state := progressionState{Known: scenario != "unknown", Enabled: scenario == "already enabled", Zone: 109, Damage: 100, DamageKnown: true}
			if scenario == "missed boss frame" {
				p = progressionPlanner{seen: true, lastZone: 109, wallZone: 110, wallDamage: 100, wallDamageKnown: true}
				state.Buffs = 1
			}
			events := []string{}
			input := heroInput{
				capture: func() (image.Image, error) { return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil },
				keyToggle: func(key, direction string) error {
					events = append(events, key+":"+direction)
					if direction == "down" && scenario == "cancel during key" {
						cancel()
					}
					if direction == "up" && scenario != "missed toggle" {
						state.Enabled = true
					}
					return nil
				},
			}
			reads := 0
			read := func(context.Context, image.Image) (progressionState, error) {
				reads++
				if scenario == "pause during read" || (scenario == "pause during confirmation" && reads == 3) {
					controls.toggle()
					controls.toggle()
				}
				if scenario == "unreadable" {
					return state, context.DeadlineExceeded
				}
				return state, nil
			}
			scan := func(image.Image) (bool, error) {
				if scenario == "external toggle" {
					state.Enabled = true
				}
				if scenario == "pause during fish" {
					controls.toggle()
					controls.toggle()
				}
				return scenario == "fish", nil
			}
			_, err := p.run(ctx, &controls, generation, input, read, scan)
			if err != nil {
				t.Fatal(err)
			}
			wantInput := scenario == "enable" || scenario == "missed toggle" || scenario == "pause during confirmation" || scenario == "cancel during key" || scenario == "missed boss frame"
			if (len(events) > 0) != wantInput || (wantInput && (len(events) != 2 || events[0] != "a:down" || events[1] != "a:up")) {
				t.Fatalf("events=%v", events)
			}
			if scenario == "missed boss frame" {
				state.Enabled, state.Buffs = false, 0
				p.observe(state, time.Now())
				state.Buffs = 1
				if p.observe(state, time.Now().Add(2*time.Minute)) {
					t.Fatal("forgot the combat buff tried without a boss frame")
				}
			}
			if scenario == "missed toggle" {
				p.nextScan = time.Time{}
				if _, err := p.run(ctx, &controls, generation, input, read, scan); err != nil {
					t.Fatal(err)
				}
				if len(events) != 2 || time.Until(p.nextAttempt) < 25*time.Second {
					t.Fatal("missed toggle spammed or lost backoff")
				}
			}
		})
	}
}
