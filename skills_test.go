package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func skillsReady(keys ...int) [9]skillState {
	var states [9]skillState
	for i := range states {
		states[i].Known = true
	}
	for _, key := range keys {
		states[key-1].Ready = true
	}
	return states
}

func TestSkillPlanner(t *testing.T) {
	for _, scenario := range []string{"full pair", "short cooldowns", "partial pair", "normal reload", "energized ongoing", "unknown", "reload first wave", "reload second wave", "pending energize", "ritual", "ritual backoff"} {
		t.Run(scenario, func(t *testing.T) {
			p := skillPlanner{}
			states := skillsReady()
			var want []int
			switch scenario {
			case "full pair":
				states, want = skillsReady(1, 2, 3, 4, 5, 6, 7, 8, 9), []int{3, 5, 8, 9}
			case "short cooldowns":
				states, want = skillsReady(1, 2), []int{1}
			case "partial pair":
				states, want = skillsReady(3, 8, 9), []int{8, 3}
			case "normal reload":
				states, want = skillsReady(5, 9), []int{5, 9}
			case "energized ongoing":
				states, want = skillsReady(5, 8), []int{5}
				states[4].Active, states[4].Energized = true, true
			case "unknown":
				states = [9]skillState{}
				states[4].Ready = true
			case "reload first wave":
				states, want = skillsReady(1, 3, 5, 8, 9), []int{1}
				states[2].Active, states[4].Active = true, true
				p.reloaded[2], p.reloaded[4] = true, true
			case "reload second wave":
				states, want = skillsReady(3, 5), []int{3}
				p.reloaded[2], p.reloaded[4] = true, true
			case "pending energize":
				states, want = skillsReady(1, 5, 6, 8, 9), []int{5}
				p.pendingEnergize = true
			case "ritual":
				states, want = skillsReady(6, 8, 9), []int{6}
			case "ritual backoff":
				states = skillsReady(6, 8, 9)
				p.retryAt[5] = time.Now().Add(time.Minute)
			}
			if got := p.plan(states, time.Now()); !reflect.DeepEqual(got, want) {
				t.Fatalf("plan=%v, want %v", got, want)
			}
		})
	}
}

func TestSkillRunConfirmation(t *testing.T) {
	for _, scenario := range []string{"pair", "missed first", "missed second", "missed energize", "unknown target", "external skill", "external energize", "pause after energize", "ritual no-op", "cancelled read"} {
		t.Run(scenario, func(t *testing.T) {
			p := skillPlanner{}
			controls := pauseControl{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			generation := controls.snapshot()
			now := time.Now()
			states := skillsReady(1, 2, 3, 4, 5, 6, 7, 8, 9)
			if scenario == "ritual no-op" {
				states = skillsReady(6, 8, 9)
			}
			events := []string{}
			frameID := uint64(1)
			if scenario == "cancelled read" {
				cancel()
			}
			controls.runClick(ctx, generation, func() error { p.observeFrame(states, frameID, generation, now); return nil })
			for controls.valid(ctx, generation) {
				key := p.nextKey(states, frameID, now)
				if key == 0 {
					break
				}
				before := states
				events = append(events, fmt.Sprintf("%d:down", key), fmt.Sprintf("%d:up", key))
				p.sent(key, before, frameID, now)
				if !(scenario == "missed first" && key == 3) && !(scenario == "missed second" && key == 5) && !(scenario == "missed energize" && key == 8) && scenario != "ritual no-op" {
					states[key-1].Ready = false
					states[key-1].Active = key < 8 && key != 6
				}
				if scenario == "unknown target" && key == 3 {
					states[4].Known = false
				}
				if key == 9 {
					states[2].Ready, states[4].Ready = true, true
				}
				if scenario == "external skill" && key == 5 {
					states[0].Ready = false
				}
				if scenario == "external energize" && key == 5 {
					states[7].Ready = false
				}
				if scenario == "pause after energize" && key == 8 {
					controls.toggle()
					controls.toggle()
				}
				for attempt := 0; attempt < 3 && p.pending != nil; attempt++ {
					frameID++
					now = now.Add(150 * time.Millisecond)
					controls.runClick(ctx, generation, func() error { p.observeFrame(states, frameID, generation, now); return nil })
				}
				if p.pending != nil || len(p.keys) == 0 {
					break
				}
			}
			want := "[3:down 3:up 5:down 5:up 8:down 8:up 9:down 9:up]"
			switch scenario {
			case "missed first", "unknown target":
				want = "[3:down 3:up]"
			case "missed second", "external skill", "external energize":
				want = "[3:down 3:up 5:down 5:up]"
			case "pause after energize", "missed energize":
				want = "[3:down 3:up 5:down 5:up 8:down 8:up]"
			case "ritual no-op":
				want = "[6:down 6:up]"
			case "cancelled read":
				want = "[]"
			}
			if fmt.Sprint(events) != want {
				t.Fatalf("events=%v, want %s", events, want)
			}
			if scenario == "pair" && (!p.reloaded[2] || !p.reloaded[4] || p.pendingEnergize) {
				t.Fatalf("second wave lost: %+v", p)
			}
			if scenario == "ritual no-op" && len(p.plan(states, now)) != 0 {
				t.Fatal("capped ritual did not back off")
			}
			if scenario == "pause after energize" {
				if !p.pendingEnergize {
					t.Fatal("interrupted Energize lost")
				}
				states[4].Ready = true
				if got := p.plan(states, now); !reflect.DeepEqual(got, []int{5}) {
					t.Fatalf("resume plan: %v", got)
				}
			}
			if scenario == "external energize" && p.pendingEnergize {
				t.Fatal("external Energize was attributed to this planner")
			}
		})
	}
}

func TestSkillPlannerStateTransitions(t *testing.T) {
	t.Run("startup spent Energize does not create own charge", func(t *testing.T) {
		p := skillPlanner{}
		states := skillsReady(5, 9)
		states[7].Ready = false
		p.observeFrame(states, 1, 7, time.Now())
		if p.pendingEnergize {
			t.Fatal("startup cooldown fabricated an Energize charge")
		}
		if got := p.plan(states, time.Now()); !reflect.DeepEqual(got, []int{5, 9}) {
			t.Fatalf("plan=%v, want [5 9]", got)
		}
	})

	t.Run("interrupt preserves first wave reserve", func(t *testing.T) {
		p := skillPlanner{reloaded: [9]bool{false, false, true, false, true}}
		states := skillsReady(3, 5, 8, 9)
		states[2].Active, states[4].Active = true, true
		p.interrupt()
		if !p.reloaded[2] || !p.reloaded[4] {
			t.Fatal("first wave reserve was cleared by interrupt")
		}
		if got := p.plan(states, time.Now()); len(got) != 0 {
			t.Fatalf("planned while reserved first wave is active: %v", got)
		}
		states[2].Active = false
		if got := p.plan(states, time.Now()); !reflect.DeepEqual(got, []int{8, 3}) || p.reloaded[2] {
			t.Fatalf("expired target did not release reserve: plan=%v reserve=%v", got, p.reloaded[2])
		}
		if !p.reloaded[4] {
			t.Fatal("one expiry discarded the other reserved target")
		}
		states[2].Ready, states[7].Ready, states[8].Ready = false, false, false
		states[4].Active = false
		if got := p.plan(states, time.Now()); !reflect.DeepEqual(got, []int{5}) || p.reloaded[4] {
			t.Fatalf("second target did not release independently: plan=%v", got)
		}
	})

	t.Run("own Energize charge survives interrupt and is consumed", func(t *testing.T) {
		p := skillPlanner{keys: []int{8, 5}, confirmed: map[int]bool{}}
		states := skillsReady(5, 8)
		p.sent(8, states, 1, time.Now())
		p.interrupt()
		if !p.pendingEnergize {
			t.Fatal("own Energize charge was cleared by interrupt")
		}
		p.keys = []int{5}
		p.confirmed = map[int]bool{}
		p.sent(5, states, 2, time.Now())
		states[4].Ready = false
		p.observeFrame(states, 3, 7, time.Now())
		if p.pendingEnergize {
			t.Fatal("confirmed consumer did not clear own Energize charge")
		}
	})

	t.Run("reset clears retry reserve and charge state", func(t *testing.T) {
		p := skillPlanner{
			reloaded:        [9]bool{true},
			retryAt:         [9]time.Time{time.Now()},
			pendingEnergize: true,
			keys:            []int{8},
			confirmed:       map[int]bool{5: true},
			pending:         &skillAttempt{key: 8},
		}
		p.reset()
		if !reflect.DeepEqual(p, skillPlanner{}) {
			t.Fatalf("planner state survived reset: %+v", p)
		}
	})
}

func TestCappedRitualRetriesStayBounded(t *testing.T) {
	p := skillPlanner{}
	states := skillsReady(6, 8, 9)
	now := time.Unix(1000, 0)
	var frameID uint64 = 1
	for cycle := 0; cycle < 3; cycle++ {
		if key := p.nextKey(states, frameID, now); key != 6 {
			t.Fatalf("ritual attempt spent utility or was skipped: %d", key)
		}
		p.sent(6, states, frameID, now)
		for attempt := 0; attempt < 3; attempt++ {
			frameID++
			p.observeFrame(states, frameID, 1, now)
		}
		if p.pending != nil || p.pendingEnergize || p.nextKey(states, frameID, now.Add(29*time.Second)) != 0 {
			t.Fatal("capped ritual fabricated success or bypassed backoff")
		}
		now = now.Add(30 * time.Second)
	}
}

func TestTapGameKeyReleasesKey(t *testing.T) {
	failure := errors.New("keyboard unavailable")
	for _, scenario := range []string{"success", "paused", "stale", "cancelled down", "down error", "up error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controls := pauseControl{paused: scenario == "paused"}
			generation := controls.snapshot()
			if scenario == "stale" {
				controls.toggle()
				controls.toggle()
			}
			var events []string
			var downAt time.Time
			acted, err := tapGameKey(ctx, &controls, generation, heroInput{keyToggle: func(key, state string) error {
				events = append(events, key+":"+state)
				if state == "down" {
					downAt = time.Now()
				} else if scenario == "success" && time.Since(downAt) < 100*time.Millisecond {
					t.Fatal("skill press was too short")
				}
				if scenario == "cancelled "+state {
					cancel()
				}
				if scenario == state+" error" {
					return failure
				}
				return nil
			}}, "5")
			want := "[5:down 5:up]"
			if scenario == "paused" || scenario == "stale" {
				want = "[]"
			}
			if fmt.Sprint(events) != want || acted != (scenario == "success") {
				t.Fatalf("acted=%t events=%v, want %s", acted, events, want)
			}
			if scenario == "down error" || scenario == "up error" {
				if !errors.Is(err, failure) {
					t.Fatalf("lost key error: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
