package main

import (
	"fmt"
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
	nextAttempt                      time.Time
	pending                          *progressionAttempt
	wantAction                       bool
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
		*p = progressionPlanner{}
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

type progressionAttempt struct {
	before   progressionState
	frameID  uint64
	attempts int
}

func (p *progressionPlanner) observeFrame(s progressionState, frameID uint64, now time.Time) {
	pending := p.pending
	if pending != nil {
		if frameID <= pending.frameID {
			return
		}
		pending.attempts++
		if s.Known && s.Enabled {
			// The mode-only confirmation reuses the decision's zone and boss baseline.
			s.Zone = pending.before.Zone
			if p.wallZone > 0 {
				p.rememberBoss(p.wallZone, pending.before)
			}
			p.pending = nil
			p.wantAction = false
			p.observe(s, now)
			fmt.Printf("enabled progression at zone %d\n", s.Zone)
		} else if pending.attempts >= 3 {
			p.pending = nil
			p.wantAction = false
			fmt.Println("progression activation not confirmed; retrying no earlier than 30s")
		}
		return
	}
	p.wantAction = p.observe(s, now)
}
func (p *progressionPlanner) sent(before progressionState, frameID uint64, now time.Time) {
	p.wantAction = false
	p.pending = &progressionAttempt{before: before, frameID: frameID}
	p.nextAttempt = now.Add(30 * time.Second)
}
