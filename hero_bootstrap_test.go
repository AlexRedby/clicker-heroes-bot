package main

import (
	"context"
	"errors"
	"image"
	"reflect"
	"testing"
	"time"
)

func TestHeroBootstrap(t *testing.T) {
	start := gameFrame{id: 1, generation: 4, layout: 2, at: time.Unix(1000, 0), context: gameContext{known: true, heroes: true}}
	base := bootstrapObservation{frame: start, hero: 24, level: 100, listKnown: true, x1: true,
		heroBuyerEnabled: true, latestHeroKnown: true, ordinaryUpgradesVerified: true, progressionEnabled: true, combatVerified: true,
		clickerOwnershipKnown: true, clickerTargetsVerified: true, ascensionUsable: true}
	base.frame.id++
	base.frame.at = start.at.Add(time.Second)
	for i := range base.skills {
		base.skills[i].Known = true
	} // Cooldown confirms an unlock.
	for _, tc := range []struct {
		name   string
		change func(*bootstrapObservation)
		want   bootstrapStatus
	}{
		{"ready on cooldown", func(*bootstrapObservation) {}, bootstrapReady},
		{"fish", func(o *bootstrapObservation) { o.fish = true }, bootstrapWaiting},
		{"unknown list", func(o *bootstrapObservation) { o.listKnown = false }, bootstrapWaiting},
		{"quantity", func(o *bootstrapObservation) { o.x1 = false }, bootstrapWaiting},
		{"input pending", func(o *bootstrapObservation) { o.inputPending = true }, bootstrapWaiting},
		{"no combat", func(o *bootstrapObservation) { o.combatVerified = false }, bootstrapWaiting},
		{"unknown ownership", func(o *bootstrapObservation) { o.clickerOwnershipKnown = false }, bootstrapWaiting},
		{"unknown targets", func(o *bootstrapObservation) { o.clickerTargetsVerified = false }, bootstrapWaiting},
		{"unknown latest hero", func(o *bootstrapObservation) { o.latestHeroKnown = false }, bootstrapWaiting},
		{"unknown ordinary upgrades", func(o *bootstrapObservation) { o.ordinaryUpgradesVerified = false }, bootstrapWaiting},
		{"progression", func(o *bootstrapObservation) { o.progressionEnabled = false }, bootstrapWaiting},
		{"disabled buyer", func(o *bootstrapObservation) { o.heroBuyerEnabled = false }, bootstrapBlocked},
		{"relics", func(o *bootstrapObservation) { o.relicBlocked = true }, bootstrapBlocked},
		{"save menu", func(o *bootstrapObservation) { o.frame.context.saveMenu = true }, bootstrapWaiting},
		{"reset modal", func(o *bootstrapObservation) { o.frame.context.ascension = true }, bootstrapWaiting},
		{"gild modal", func(o *bootstrapObservation) { o.frame.context.modal = gildChestModal }, bootstrapWaiting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p bootstrapPlanner
			p.start(start, true, true)
			o := base
			tc.change(&o)
			p.observe(o)
			if got := p.next(o.frame.at); got.status != tc.want || got.buyUpgrade {
				t.Fatalf("result = %+v, want status %v", got, tc.want)
			}
		})
	}

	t.Run("handoff freshness and F8", func(t *testing.T) {
		var p bootstrapPlanner
		p.observe(base)
		if p.next(base.frame.at).status == bootstrapReady {
			t.Fatal("ready without handoff")
		}
		p.start(start, true, true)
		p.observe(base)
		if p.next(base.frame.at.Add(4*time.Second)).status == bootstrapReady {
			t.Fatal("sticky stale readiness")
		}
		stale := base
		stale.frame.id, stale.skills = 1, [9]skillState{}
		p.observe(stale)
		if p.next(base.frame.at).status != bootstrapReady {
			t.Fatal("stale observation overwrote fresh state")
		}
		stale = base
		stale.frame.id++
		stale.frame.generation--
		p.observe(stale)
		if !p.active || p.next(base.frame.at).status != bootstrapReady {
			t.Fatal("old generation canceled the current handoff")
		}
		p.interrupt()
		if p.next(base.frame.at).status == bootstrapReady || len(p.confirmed) != 0 {
			t.Fatal("F8 retained proof")
		}
		p.start(start, true, true)
		p.observe(base)
		newGeneration := base
		newGeneration.frame.generation++
		newGeneration.frame.id = start.id // Generation revokes intent even if capture IDs restart.
		p.observe(newGeneration)
		if p.active || p.next(base.frame.at).status == bootstrapReady {
			t.Fatal("generation mismatch retained readiness")
		}
	})

	t.Run("finite Ascension prerequisites", func(t *testing.T) {
		var p bootstrapPlanner
		p.start(start, false, true)
		o := base
		o.skills, o.ascensionUsable = [9]skillState{}, false
		for _, id := range []int{73, 74, 75, 76, 82, 83} {
			var m bootstrapMilestone
			for _, candidate := range bootstrapMilestones {
				if candidate.upgrade == id {
					m = candidate
				}
			}
			o.hero, o.level = m.hero, m.level
			o.bought, o.available = nil, map[int]bool{id: true}
			p.observe(o)
			r := p.next(o.frame.at)
			if r.milestone.upgrade != id || !r.buyUpgrade || !p.sentUpgrade(r, o.frame.at) {
				t.Fatalf("upgrade %d: %+v", id, r)
			}
			if p.sentUpgrade(r, o.frame.at) {
				t.Fatal("replayed pending upgrade")
			}
			o.frame.id++
			o.frame.at = o.frame.at.Add(250 * time.Millisecond)
			o.bought = map[int]bool{id: true}
			p.observe(o)
			if p.pending != nil || !p.confirmed[id] {
				t.Fatalf("upgrade %d not confirmed", id)
			}
			o.frame.id++
			o.frame.at = o.frame.at.Add(250 * time.Millisecond)
		}
		o.hero, o.level, o.bought = 20, 50, nil
		p.observe(o)
		if r := p.next(o.frame.at); r.levels != 100 || r.milestone.upgrade != 0 || r.buyUpgrade {
			t.Fatalf("Amenhotep preparation = %+v", r)
		}
		o.level, o.frame.id = 150, o.frame.id+1
		o.frame.at = o.frame.at.Add(time.Second)
		p.observe(o)
		if r := p.next(o.frame.at); r.status != bootstrapWaiting || r.buyUpgrade || p.sentUpgrade(bootstrapResult{frame: o.frame, milestone: bootstrapMilestone{upgrade: 106}, buyUpgrade: true}, o.frame.at) {
			t.Fatalf("level alone authorized reset/readiness: %+v", r)
		}
		o.ascensionUsable, o.frame.id = true, o.frame.id+1
		o.frame.at = o.frame.at.Add(time.Second)
		p.observe(o)
		if p.next(o.frame.at).status != bootstrapReady {
			t.Fatal("verified entry not ready")
		}
	})

	for _, scenario := range []string{"Auto Clicker", "no gold", "missed input", "timeout", "layout", "fish", "stale", "implicit prerequisites", "late queue result"} {
		t.Run(scenario, func(t *testing.T) {
			var p bootstrapPlanner
			p.start(start, false, true)
			o := base
			o.hero, o.level, o.skills, o.ascensionUsable = 18, 10, [9]skillState{}, false
			o.available = map[int]bool{73: true}
			if scenario == "Auto Clicker" {
				o.upgradesClicker = true
			}
			if scenario == "no gold" {
				o.available = nil
			}
			if scenario == "implicit prerequisites" {
				o.hero, o.level, o.bought = 20, 50, map[int]bool{83: true}
			}
			p.observe(o)
			if scenario == "implicit prerequisites" {
				for _, id := range []int{73, 74, 75, 76, 82, 83} {
					if !p.confirmed[id] {
						t.Fatalf("missing ancestor %d", id)
					}
				}
				return
			}
			if scenario == "Auto Clicker" || scenario == "no gold" {
				if r := p.next(o.frame.at); r.buyUpgrade || r.status != bootstrapWaiting || p.sentUpgrade(r, o.frame.at) {
					t.Fatalf("duplicate/unaffordable input: %+v", r)
				}
				return
			}
			r := p.next(o.frame.at)
			if scenario == "late queue result" {
				o.frame.id++
				o.frame.at = o.frame.at.Add(250 * time.Millisecond)
				p.observe(o)
				if !p.sentUpgrade(r, o.frame.at) || p.pending == nil || p.pending.frame.id != r.frame.id {
					t.Fatal("new capture lost the sent queue action")
				}
				return
			}
			if !p.sentUpgrade(r, o.frame.at) {
				t.Fatal("no upgrade intent")
			}
			if scenario == "timeout" {
				if p.next(o.frame.at.Add(11*time.Second)).status != bootstrapBlocked {
					t.Fatal("unbounded confirmation wait")
				}
				return
			}
			for i := 0; i < 3; i++ {
				if scenario != "stale" {
					o.frame.id++
					o.frame.at = o.frame.at.Add(250 * time.Millisecond)
				}
				if scenario == "layout" {
					o.frame.layout++
				}
				if scenario == "fish" {
					o.fish = true
				}
				p.observe(o)
			}
			if scenario == "fish" {
				if p.pending != nil || p.failure != "" {
					t.Fatal("fish obstruction retained the pending input or charged a failure")
				}
			} else if scenario == "stale" {
				if p.pending == nil || p.pending.checks != 0 || p.failure != "" {
					t.Fatal("obstructed/stale frame counted as a failed click")
				}
			} else if p.next(o.frame.at).status != bootstrapBlocked {
				t.Fatal("failed purchase retried automatically")
			}
		})
	}
}

func TestHeroSupportQuantity(t *testing.T) {
	for _, tc := range []struct {
		missing int
		key     string
	}{{1, ""}, {9, ""}, {10, "shift"}, {24, "shift"}, {25, "z"}, {99, "z"}, {100, "ctrl"}, {150, "ctrl"}} {
		for _, cancelOnDown := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			var calls []string
			input := heroInput{
				keyToggle: func(key, state string) error {
					calls = append(calls, key+" "+state)
					if cancelOnDown && state == "down" {
						cancel()
					}
					return nil
				},
				click: func(image.Point) error { calls = append(calls, "click"); return nil },
			}
			err := clickHeroLevels(ctx, input, image.Pt(10, 20), tc.missing)
			cancel()
			want := []string{"click"}
			if tc.key != "" {
				want = []string{tc.key + " down", "click", tc.key + " up"}
				if cancelOnDown {
					want = []string{tc.key + " down", tc.key + " up"}
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel error: %v", err)
					}
				}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("deficit %d: calls %v, want %v", tc.missing, calls, want)
			}
		}
	}
	if clickHeroLevels(context.Background(), heroInput{}, image.Point{}, 0) == nil {
		t.Fatal("zero deficit sent input")
	}
}
