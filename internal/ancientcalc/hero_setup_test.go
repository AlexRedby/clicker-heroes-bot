package ancientcalc

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
)

func readySetupFixture(t *testing.T, latest int) gildFixture {
	t.Helper()
	f := newGildFixture()
	var defs []setupHero
	if err := json.Unmarshal(gildHeroData, &defs); err != nil {
		t.Fatal(err)
	}
	f.Upgrades = map[string]bool{}
	for _, hero := range defs {
		if hero.ID > latest {
			break
		}
		level := int64(1)
		for _, u := range hero.Upgrades {
			if u.Kind == "reset" {
				continue
			}
			level = max(level, u.Level)
			f.Upgrades[strconv.Itoa(u.ID)] = true
		}
		setGildHero(f, hero.ID, int(level), 0, false)
	}
	return f
}

func TestPlanHeroSetupRoutes(t *testing.T) {
	for _, raw := range []bool{false, true} {
		f := readySetupFixture(t, 20)
		p, err := PlanHeroSetup(context.Background(), f.save(t, raw))
		if err != nil || !p.PassiveReady || p.NeedsLevels || len(p.Heroes) != 0 || len(p.MissingUpgrades) != 0 {
			t.Fatalf("ready raw=%v: %+v %v", raw, p, err)
		}
		if f.Upgrades["106"] {
			t.Fatal("fixture bought reset")
		}
		f.Upgrades["7"] = false // Cid's first active skill is missing, but its level is sufficient.
		p, err = PlanHeroSetup(nil, f.save(t, raw))
		if err != nil || p.NeedsLevels || len(p.Heroes) != 1 || p.Heroes[0].ID != 1 || len(p.MissingUpgrades) != 1 || p.MissingUpgrades[0] != 7 {
			t.Fatalf("footer only: %+v %v", p, err)
		}
		setGildHero(f, 1, 0, 0, false)
		p, err = PlanHeroSetup(nil, f.save(t, raw))
		if err != nil || !p.NeedsLevels || p.Heroes[0].RequiredLevel != 25 {
			t.Fatalf("missing Cid: %+v %v", p, err)
		}
	}
}

func TestPlanHeroSetupCrossHeroPrerequisites(t *testing.T) {
	f := readySetupFixture(t, 20)
	delete(f.Upgrades, "79")
	delete(f.Upgrades, "75")
	setGildHero(f, 18, 1, 0, false)
	p, err := PlanHeroSetup(nil, f.save(t, false))
	if err != nil || !p.NeedsLevels || len(p.Heroes) != 2 || p.Heroes[0].ID != 18 || p.Heroes[0].RequiredLevel != 50 || p.Heroes[1].ID != 19 {
		t.Fatalf("cross hero: %+v %v", p, err)
	}
	for _, id := range p.MissingUpgrades {
		if id == 106 || id == 132 {
			t.Fatal("reset included")
		}
	}
}

func TestPlanHeroSetupZeroGoldAndFutureHeroes(t *testing.T) {
	f := newGildFixture()
	p, err := PlanHeroSetup(nil, f.save(t, false))
	if err != nil || p.PassiveReady || !p.NeedsLevels || len(p.Heroes) != 2 || p.Heroes[0].ID != 1 || p.Heroes[1].ID != 2 {
		t.Fatalf("zero gold: %+v %v", p, err)
	}
	f = readySetupFixture(t, 2)
	p, err = PlanHeroSetup(nil, f.save(t, false))
	if err != nil || len(p.Heroes) != 0 || p.NeedsLevels {
		t.Fatalf("future heroes must not trigger sweep: %+v %v", p, err)
	}
	f = readySetupFixture(t, 54)
	p, err = PlanHeroSetup(nil, f.save(t, false))
	if err != nil || len(p.Heroes) != 0 || p.NeedsLevels {
		t.Fatalf("full catalog: %+v %v", p, err)
	}
}

func TestPlanHeroSetupUnsupportedVersusIncomplete(t *testing.T) {
	cases := []struct {
		name string
		edit func(*gildFixture)
	}{
		{"missing roster", func(f *gildFixture) { delete(f.Heroes, "1") }},
		{"missing ownership", func(f *gildFixture) { f.Upgrades = nil }},
		{"wrong identity", func(f *gildFixture) { f.Heroes["1"].(map[string]any)["uid"] = 2 }},
		{"fractional level", func(f *gildFixture) { f.Heroes["1"].(map[string]any)["level"] = 1.5 }},
		{"locked hired", func(f *gildFixture) { setGildHero(*f, 1, 1, 0, true) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := readySetupFixture(t, 2)
			tc.edit(&f)
			_, err := PlanHeroSetup(nil, f.save(t, false))
			if !errors.Is(err, ErrHeroSetupUnsupported) {
				t.Fatalf("expected unsupported: %v", err)
			}
		})
	}
	f := readySetupFixture(t, 2)
	exported := f.save(t, false)
	if _, err := PlanHeroSetup(nil, exported[:len(exported)/2]); err == nil || errors.Is(err, ErrHeroSetupUnsupported) {
		t.Fatalf("incomplete export should retry: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanHeroSetup(ctx, exported); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestPlanHeroSetupLeavesLatestThresholdToProgression(t *testing.T) {
	f := readySetupFixture(t, 42)
	setGildHero(f, 42, 5654, 0, false)
	delete(f.Upgrades, "203")
	p, err := PlanHeroSetup(nil, f.save(t, false))
	if err != nil || p.NeedsLevels || len(p.Heroes) != 0 || len(p.MissingUpgrades) != 0 {
		t.Fatalf("ordinary latest threshold triggered startup: %+v %v", p, err)
	}
	f = readySetupFixture(t, 24)
	setGildHero(f, 24, 75, 0, false)
	delete(f.Upgrades, "115") // Reload is active-skill preparation even on the latest hero.
	p, err = PlanHeroSetup(nil, f.save(t, false))
	if err != nil || !p.NeedsLevels || len(p.Heroes) != 1 || p.Heroes[0].RequiredLevel != 100 {
		t.Fatalf("latest active skill was skipped: %+v %v", p, err)
	}
}
