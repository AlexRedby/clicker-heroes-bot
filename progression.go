package main

import (
	"context"
	"fmt"
	"image"
	"math"
	"time"
)

type progressionState struct {
	Known, Enabled bool
	Zone           int
	Damage         float64
	DamageKnown    bool
	Buffs          uint8
}

type progressionPlanner struct {
	seen, lastEnabled                bool
	lastZone, wallZone, failures     int
	bossZone                         int
	bossDamage, wallDamage           float64
	bossDamageKnown, wallDamageKnown bool
	bossBuffs, wallBuffs             uint8
	nextAttempt, nextScan            time.Time
}

func (p *progressionPlanner) rememberBoss(zone int, s progressionState) {
	if p.bossZone != zone {
		p.bossZone, p.bossBuffs, p.bossDamageKnown = zone, 0, false
	}
	p.bossBuffs |= s.Buffs
	if s.DamageKnown && (!p.bossDamageKnown || s.Damage > p.bossDamage) {
		p.bossDamage, p.bossDamageKnown = s.Damage, true
	}
}

func (p *progressionPlanner) observe(s progressionState, now time.Time) bool {
	if !s.Known || s.Zone <= 0 {
		return false
	}
	if p.seen && s.Zone < p.lastZone-1 {
		// Ascension or a manual zone jump starts a fresh progression assessment.
		*p = progressionPlanner{nextScan: p.nextScan}
	}
	if s.Enabled {
		if p.wallZone > 0 && s.Zone > p.wallZone {
			p.wallZone, p.failures = 0, 0
		}
		if s.Zone%5 == 0 {
			p.rememberBoss(s.Zone, s)
		}
	} else if p.seen && p.lastEnabled && s.Zone%5 == 4 {
		wall := s.Zone + 1
		if p.wallZone != wall {
			p.failures = 0
		}
		p.wallZone = wall
		p.failures++
		p.wallDamage, p.wallDamageKnown = s.Damage, s.DamageKnown
		p.wallBuffs = s.Buffs
		if p.bossZone == wall {
			p.wallBuffs |= p.bossBuffs
			if p.bossDamageKnown && (!p.wallDamageKnown || p.bossDamage > p.wallDamage) {
				p.wallDamage, p.wallDamageKnown = p.bossDamage, true
			}
		}
		// Repeated losses need more farming, even if temporary buffs reappear.
		delay := time.Minute * time.Duration(1<<min(p.failures-1, 4))
		p.nextAttempt = now.Add(min(delay, 15*time.Minute))
		fmt.Printf("boss %d failed; farming until damage improves or a stronger combat buff appears (retry no earlier than %s)\n", wall, p.nextAttempt.Format("15:04:05"))
	}
	p.seen, p.lastZone, p.lastEnabled = true, s.Zone, s.Enabled
	if s.Enabled || now.Before(p.nextAttempt) {
		return false
	}
	if p.wallZone == 0 {
		return true
	}
	if !p.wallDamageKnown && s.DamageKnown {
		p.wallDamage, p.wallDamageKnown = s.Damage, true
	}
	// Displayed damage is only a growth proxy; it does not measure click rate or predict a kill.
	growth := p.wallDamageKnown && s.DamageKnown && s.Damage-p.wallDamage >= math.Log10(2)
	strongerBuff := s.Buffs & ^p.wallBuffs != 0
	return growth || strongerBuff
}

func (p *progressionPlanner) run(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, read func(context.Context, image.Image) (progressionState, error), scanFish func(image.Image) (bool, error)) (bool, error) {
	if !controls.valid(ctx, generation) || time.Now().Before(p.nextScan) {
		return false, nil
	}
	p.nextScan = time.Now().Add(2 * time.Second)
	frame, err := input.capture()
	if err != nil {
		return false, fmt.Errorf("capture progression: %w", err)
	}
	if frame == nil {
		return false, fmt.Errorf("capture progression returned no image")
	}
	s, err := read(ctx, frame)
	if ctx.Err() != nil || !controls.valid(ctx, generation) {
		return false, nil
	}
	if err != nil {
		p.nextScan = time.Now().Add(30 * time.Second)
		fmt.Printf("progression numbers unreadable: %v; retrying in 30s\n", err)
		return false, nil
	}
	if !p.observe(s, time.Now()) {
		return false, nil
	}
	present, err := scanFish(frame)
	if err != nil || present {
		return false, err
	}
	// A is a toggle: re-read after fish detection before changing the mode.
	frame, err = input.capture()
	if err != nil {
		return false, fmt.Errorf("capture progression before toggle: %w", err)
	}
	if frame == nil {
		return false, fmt.Errorf("capture progression before toggle returned no image")
	}
	s, err = read(ctx, frame)
	if !controls.valid(ctx, generation) || err != nil || !p.observe(s, time.Now()) {
		return false, nil
	}
	before := s
	p.nextAttempt = time.Now().Add(30 * time.Second)
	pressed, err := tapGameKey(ctx, controls, generation, input, "a")
	if err != nil {
		return false, fmt.Errorf("enable progression: %w", err)
	}
	if !pressed {
		return false, nil
	}
	for range 3 {
		timer := time.NewTimer(150 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return true, nil
		case <-timer.C:
		}
		if !controls.valid(ctx, generation) {
			return true, nil
		}
		after, err := input.capture()
		if err != nil {
			return true, fmt.Errorf("capture progression confirmation: %w", err)
		}
		if after == nil {
			return true, fmt.Errorf("capture progression confirmation returned no image")
		}
		s, err = read(ctx, after)
		if ctx.Err() != nil || !controls.valid(ctx, generation) {
			return true, nil
		}
		if err != nil {
			continue
		}
		if s.Known && s.Enabled && s.Zone > 0 {
			if p.wallZone > 0 {
				// Remember the confirmed attempt even if no boss frame is captured.
				p.rememberBoss(p.wallZone, before)
			}
			p.observe(s, time.Now())
			fmt.Printf("enabled progression at zone %d\n", s.Zone)
			return true, nil
		}
	}
	fmt.Println("progression activation not confirmed; retrying no earlier than 30s")
	return true, nil
}
