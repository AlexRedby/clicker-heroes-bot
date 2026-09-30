package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strconv"
	"time"
)

type skillPlanner struct {
	// Reload prepares a second wave; do not immediately overwrite the first one.
	reloaded [9]bool
	retryAt  [9]time.Time
	// An interrupted Energize must be consumed by a fresh, recognized target.
	pendingEnergize bool
	seenGeneration  bool
	generation      uint64
}

func (p *skillPlanner) plan(states [9]skillState, now time.Time) []int {
	ready := func(key int) bool {
		s := states[key-1]
		return s.Known && s.Ready && !now.Before(p.retryAt[key-1])
	}
	for i, s := range states {
		if s.Known && !s.Active {
			p.reloaded[i] = false
		}
	}
	// Finish an interrupted charge with an ordinary buff, never Dark Ritual.
	if p.pendingEnergize {
		for _, key := range []int{5, 3, 1, 7, 4, 2} {
			if ready(key) {
				return []int{key}
			}
		}
		return nil
	}
	// Refresh uninterrupted energized buffs before spending utility skills elsewhere.
	for _, key := range []int{5, 3, 1, 7, 4, 2} {
		if ready(key) && states[key-1].Active && states[key-1].Energized && !p.reloaded[key-1] {
			return []int{key}
		}
	}
	if ready(3) && ready(5) && ready(8) && ready(9) && !states[2].Active && !states[4].Active {
		return []int{3, 5, 8, 9}
	}
	// When the pair is unavailable, strengthen an available important buff directly.
	if ready(8) {
		for _, key := range []int{5, 3} {
			if ready(key) && !states[key-1].Energized && !(p.reloaded[key-1] && states[key-1].Active) {
				return []int{8, key}
			}
		}
	}
	// A normal Reload has a known target only immediately after our confirmed cast.
	if !ready(8) && ready(9) && ready(5) && !states[4].Active {
		return []int{5, 9}
	}
	for _, key := range []int{3, 5, 1, 2, 4, 7, 6} {
		if ready(key) && !(p.reloaded[key-1] && states[key-1].Active) {
			return []int{key}
		}
	}
	return nil
}

func tapSkill(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, key int) (bool, error) {
	return controls.runClick(ctx, generation, func() (err error) {
		name := strconv.Itoa(key)
		defer func() { err = errors.Join(err, input.keyToggle(name, "up")) }()
		if err = input.keyToggle(name, "down"); err != nil {
			return err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	})
}

func unexpectedSkillActivation(before, after [9]skillState, key int) bool {
	for i, state := range before {
		if i != key-1 && state.Known && state.Ready && after[i].Known && !after[i].Ready {
			return true
		}
	}
	return false
}

func (p *skillPlanner) run(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, screen image.Image, read func(context.Context, image.Image) ([9]skillState, error)) (bool, error) {
	states, err := read(ctx, screen)
	if ctx.Err() != nil {
		return false, nil
	}
	if err != nil || !controls.valid(ctx, generation) {
		return false, err
	}
	if !p.seenGeneration || p.generation != generation {
		p.reloaded = [9]bool{}
		// A cooldown cannot reveal whether an old Energize charge was consumed.
		// First cast a useful ordinary buff when reconnecting to that ambiguous state.
		p.pendingEnergize = p.pendingEnergize || (states[7].Known && !states[7].Ready)
		p.seenGeneration, p.generation = true, generation
	}
	keys := p.plan(states, time.Now())
	acted := false
	confirmed := make(map[int]bool, len(keys))
	for _, key := range keys {
		if !controls.valid(ctx, generation) || !states[key-1].Known || !states[key-1].Ready {
			return acted, nil
		}
		if key == 8 || key == 9 {
			// Each intended target must remain recognized through the utility sequence.
			for _, target := range keys {
				if target == 8 || target == 9 {
					continue
				}
				if !states[target-1].Known || (!confirmed[target] && !states[target-1].Ready) {
					return acted, nil
				}
			}
		}
		if key == 8 {
			p.pendingEnergize = true
		}
		pressed, err := tapSkill(ctx, controls, generation, input, key)
		if err != nil {
			return acted, fmt.Errorf("skill hotkey %d: %w", key, err)
		}
		if !pressed {
			return acted, nil
		}
		acted = true
		before := states
		ok, unexpected := false, false
		// Key release and UI animation may land on different game frames.
		for attempt := 0; attempt < 3; attempt++ {
			timer := time.NewTimer(150 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return acted, nil
			case <-timer.C:
			}
			if !controls.valid(ctx, generation) {
				return acted, nil
			}
			frame, err := input.capture()
			if err != nil {
				return acted, fmt.Errorf("capture skill confirmation: %w", err)
			}
			if frame == nil {
				return acted, errors.New("capture skill confirmation returned no image")
			}
			states, err = read(ctx, frame)
			if ctx.Err() != nil {
				return acted, nil
			}
			if err != nil {
				return acted, err
			}
			if !controls.valid(ctx, generation) {
				return acted, nil
			}
			unexpected = unexpected || unexpectedSkillActivation(before, states, key)
			ok = states[key-1].Known && !states[key-1].Ready
			if ok {
				break
			}
		}
		if !ok {
			p.retryAt[key-1] = time.Now().Add(30 * time.Second)
			if key == 8 && states[7].Known && states[7].Ready && !states[7].Active {
				p.pendingEnergize = false
			}
			// Dark Ritual's 20-use limit is game-owned; a no-op does not stop skills.
			fmt.Printf("skill %d activation not confirmed; retrying in 30s\n", key)
			return acted, nil
		}
		confirmed[key] = true
		if key != 8 {
			p.pendingEnergize = false
		}
		if key == 9 {
			for target := range confirmed {
				if target != 8 && target != 9 && states[target-1].Known && states[target-1].Ready {
					p.reloaded[target-1] = true
				}
			}
		}
		fmt.Printf("activated skill %d\n", key)
		if unexpected {
			if before[7].Known && before[7].Ready && states[7].Known && !states[7].Ready {
				p.pendingEnergize = true
			}
			// An external hotkey or skill Auto Clicker invalidates Reload's history.
			return acted, nil
		}
	}
	return acted, nil
}
