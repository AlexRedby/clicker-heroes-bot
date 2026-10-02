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
		{"hero-tsuchi-x1.png", false, 12654, 819 + math.Log10(2.502), 2 | 32}, {"hero-gog-before.png", false, 2404, 158 + math.Log10(6.893), 0}, {"hero-gog-tooltip.png", false, 2404, 289 + math.Log10(1.168), 0},
	} {
		original := loadTestImage(t, "testdata/"+tc.path)
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
			skills, skillErr := readSkillStates(context.Background(), screen)
			if skillErr != nil {
				t.Fatal(skillErr)
			}
			state, err := readProgressionState(context.Background(), screen, skills, false)
			wantDamageKnown := !tc.enabled || tc.zone%5 == 0
			if err != nil || !state.Known || state.Enabled != tc.enabled || state.Zone != tc.zone || state.DamageKnown != wantDamageKnown || math.Abs(state.Damage-tc.damage) > .001 || state.Buffs != tc.buffs {
				t.Errorf("%s /%d state=%+v err=%v", tc.path, divisor, state, err)
			}
		}
	}
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
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

func TestProgressionCombatContinuity(t *testing.T) {
	for _, scenario := range []string{"continuous expiry", "gap after proof", "gap before fallback", "unknown", "cancelled context", "new attempt", "delayed analysis"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1000, 0)
			boss := progressionState{Known: true, Enabled: true, Zone: 110, FullCombat: true}
			farm := progressionState{Known: true, Zone: 109}
			p := progressionPlanner{}
			p.observe(boss, now)
			p.observe(boss, now.Add(2*time.Second))
			failedAt := now.Add(4 * time.Second)
			switch scenario {
			case "continuous expiry":
				boss.FullCombat = false
				for sec := 3; sec <= 30; sec++ {
					p.observe(boss, now.Add(time.Duration(sec)*time.Second))
				}
				failedAt = now.Add(31 * time.Second)
			case "gap after proof":
				p.observe(boss, now.Add(13*time.Second))
				failedAt = now.Add(14 * time.Second)
			case "gap before fallback":
				failedAt = now.Add(13 * time.Second)
			case "unknown":
				p.observe(progressionState{}, now.Add(3*time.Second))
			case "cancelled context":
				p.invalidateCombat()
			case "new attempt":
				p.observe(farm, now.Add(3*time.Second))
				p.observe(boss, now.Add(4*time.Second))
				failedAt = now.Add(5 * time.Second)
			case "delayed analysis":
				p = progressionPlanner{}
				boss.observedAt = now
				p.observe(boss, now.Add(100*time.Second))
				boss.observedAt = now.Add(time.Second)
				p.observe(boss, now.Add(102*time.Second))
				farm.observedAt = now.Add(2 * time.Second)
				failedAt = now.Add(103 * time.Second)
			}
			p.observe(farm, failedAt)
			if p.wallZone != 110 || p.wallFullCombat != (scenario == "continuous expiry") {
				t.Fatalf("wall=%d fullCombat=%t", p.wallZone, p.wallFullCombat)
			}
		})
	}
}

func TestProgressionAllCombatBuffRetries(t *testing.T) {
	now := time.Unix(1000, 0)
	var states [9]skillState
	for _, key := range []int{1, 2, 3, 7} {
		states[key-1] = skillState{Known: true, Active: true}
	}
	for _, key := range []int{1, 2, 3, 7} {
		t.Run(string(rune('0'+key)), func(t *testing.T) {
			p := progressionPlanner{}
			boss := progressionState{Known: true, Enabled: true, Zone: 110, FullCombat: true, Buffs: progressionBuffs(states)}
			p.observe(boss, now)
			p.observe(boss, now.Add(2*time.Second))
			farm := boss
			farm.Zone, farm.Enabled = 109, false
			if p.observe(farm, now.Add(3*time.Second)) {
				t.Fatal("unchanged failed full combat retried")
			}
			stronger := states
			stronger[key-1].Energized = true
			farm.Buffs = progressionBuffs(stronger)
			if !p.observe(farm, now.Add(4*time.Second)) {
				t.Fatal("new energized combat buff ignored")
			}
			boss.Buffs = farm.Buffs
			p.observe(boss, now.Add(5*time.Second))
			p.observe(boss, now.Add(7*time.Second))
			if p.observe(farm, now.Add(8*time.Second)) || p.observe(farm, now.Add(3*time.Minute)) {
				t.Fatal("already tried energized window retried")
			}
		})
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
	for _, scenario := range []string{"enable", "already enabled", "unknown", "missed toggle", "external toggle", "pause during read", "pause during confirmation", "missed boss frame"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now()
			controls := pauseControl{}
			generation := controls.snapshot()
			p := progressionPlanner{}
			state := progressionState{Known: scenario != "unknown", Enabled: scenario == "already enabled", Zone: 109, Damage: 100, DamageKnown: true}
			if scenario == "missed boss frame" {
				p = progressionPlanner{seen: true, lastZone: 109, wallZone: 110, wallDamage: 100, wallDamageKnown: true}
				state.Buffs = 1
			}
			if scenario == "pause during read" {
				controls.toggle()
				controls.toggle()
			}
			controls.runClick(context.Background(), generation, func() error { p.observeFrame(state, 1, now); return nil })
			if scenario == "external toggle" {
				state.Enabled = true
				p.observeFrame(state, 2, now)
			}
			want := scenario == "enable" || scenario == "missed toggle" || scenario == "pause during confirmation" || scenario == "missed boss frame"
			if p.wantAction != want {
				t.Fatalf("wantAction=%t, expected %t", p.wantAction, want)
			}
			if !want {
				return
			}
			p.sent(state, 2, now)
			if scenario == "pause during confirmation" {
				controls.toggle()
				controls.toggle()
			}
			for i := 0; i < 3; i++ {
				after := state
				after.Enabled = scenario != "missed toggle"
				controls.runClick(context.Background(), generation, func() error { p.observeFrame(after, uint64(i+3), now.Add(time.Second)); return nil })
			}
			if scenario == "missed toggle" && (p.pending != nil || p.wantAction || !p.nextAttempt.Equal(now.Add(30*time.Second))) {
				t.Fatal("missed toggle lost its backoff")
			}
			if scenario == "pause during confirmation" && p.pending == nil {
				t.Fatal("stale confirmation consumed")
			}
			if scenario == "missed boss frame" {
				state.Enabled = false
				state.Buffs = 0
				p.observe(state, now.Add(2*time.Second))
				state.Buffs = 1
				if p.observe(state, now.Add(2*time.Minute)) {
					t.Fatal("forgot previously tried buff")
				}
			}
		})
	}
}
