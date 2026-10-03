package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type skillPlanner struct {
	// Reload prepares a second wave; do not immediately overwrite the first one.
	reloaded [9]bool
	retryAt  [9]time.Time
	// An interrupted Energize must be consumed by a fresh, recognized target.
	pendingEnergize bool
	keys            []int
	confirmed       map[int]bool
	pending         *skillAttempt
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

func holdGameKey(ctx context.Context, input heroInput, name string) (err error) {
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
}
func tapGameKey(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, name string) (bool, error) {
	return controls.runClick(ctx, generation, func() error { return holdGameKey(ctx, input, name) })
}

func unexpectedSkillActivation(before, after [9]skillState, key int) bool {
	for i, state := range before {
		if i != key-1 && state.Known && state.Ready && after[i].Known && !after[i].Ready {
			return true
		}
	}
	return false
}

type skillAttempt struct {
	key      int
	before   [9]skillState
	frameID  uint64
	attempts int
}

func (p *skillPlanner) interrupt() {
	p.keys = nil
	p.confirmed = nil
	p.pending = nil
}
func (p *skillPlanner) reset() { *p = skillPlanner{} }
func (p *skillPlanner) observeFrame(states [9]skillState, frameID, _ uint64, now time.Time) {
	pending := p.pending
	if pending == nil || frameID <= pending.frameID {
		return
	}
	pending.attempts++
	key := pending.key
	if states[key-1].Known && !states[key-1].Ready {
		unexpected := unexpectedSkillActivation(pending.before, states, key)
		p.pending = nil
		p.confirmed[key] = true
		if key != 8 {
			p.pendingEnergize = false
		}
		if key != 8 && key != 9 {
			p.reloaded[key-1] = false
		}
		if key == 9 {
			for target := range p.confirmed {
				if target != 8 && target != 9 && states[target-1].Known && states[target-1].Ready {
					p.reloaded[target-1] = true
				}
			}
		}
		fmt.Printf("activated skill %d\n", key)
		if unexpected {
			p.keys = nil
		}
		return
	}
	if pending.attempts >= 3 {
		p.retryAt[key-1] = now.Add(30 * time.Second)
		if key == 8 && states[7].Known && states[7].Ready && !states[7].Active {
			p.pendingEnergize = false
		}
		p.pending = nil
		p.keys = nil
		fmt.Printf("skill %d activation not confirmed; retrying in 30s (before=%+v, after=%+v, frames=%d/%d)\n", key, pending.before[key-1], states[key-1], pending.frameID, frameID)
	}
}
func (p *skillPlanner) nextKey(states [9]skillState, frameID uint64, now time.Time) int {
	if p.pending != nil || frameID == 0 {
		return 0
	}
	if len(p.keys) == 0 {
		p.keys = p.plan(states, now)
		p.confirmed = make(map[int]bool, len(p.keys))
	}
	if len(p.keys) == 0 {
		return 0
	}
	key := p.keys[0]
	if !states[key-1].Known || !states[key-1].Ready {
		p.keys = nil
		return 0
	}
	if key == 8 || key == 9 {
		targets := append([]int(nil), p.keys...)
		for target := range p.confirmed {
			targets = append(targets, target)
		}
		for _, target := range targets {
			if target == 8 || target == 9 {
				continue
			}
			if !states[target-1].Known || (!p.confirmed[target] && !states[target-1].Ready) {
				p.keys = nil
				return 0
			}
		}
	}
	return key
}
func (p *skillPlanner) sent(key int, before [9]skillState, frameID uint64, now time.Time) {
	if key == 8 {
		p.pendingEnergize = true
	}
	p.pending = &skillAttempt{key: key, before: before, frameID: frameID}
	if len(p.keys) > 0 && p.keys[0] == key {
		p.keys = p.keys[1:]
	}
}
