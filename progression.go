package main

import (
	"fmt"
	"math"
	"time"
)

type progressionState struct {
	Known, Enabled bool
	FullCombat     bool
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
	bossFullSince, bossLastFull      time.Time
	bossFullCombat, wallFullCombat   bool
	nextAttempt                      time.Time
	pending                          *progressionAttempt
	wantAction                       bool
}

func (p *progressionPlanner) rememberBoss(zone int, s progressionState, now time.Time) {
	if p.bossZone != zone {
		p.bossZone, p.bossBuffs, p.bossDamageKnown = zone, 0, false
		p.bossFullSince, p.bossFullCombat = time.Time{}, false
	}
	// A farm-frame retry baseline is not evidence of a buffed boss fight.
	if s.Enabled && s.Zone == zone {
		if s.FullCombat {
			if p.bossFullSince.IsZero() || now.Sub(p.bossLastFull) > 10*time.Second {
				p.bossFullSince = now
			}
			p.bossLastFull = now
			p.bossFullCombat = p.bossFullCombat || now.Sub(p.bossFullSince) >= 2*time.Second
		} else {
			p.bossFullSince = time.Time{}
		}
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
		if !p.lastEnabled {
			p.bossFullSince, p.bossFullCombat = time.Time{}, false
		}
		if p.wallZone > 0 && s.Zone > p.wallZone {
			p.wallZone, p.failures, p.wallFullCombat = 0, 0, false
		}
		if s.Zone%5 == 0 {
			p.rememberBoss(s.Zone, s, now)
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
		p.wallFullCombat = p.bossZone == wall && p.bossFullCombat
		if p.bossZone == wall {
			p.wallBuffs |= p.bossBuffs
			if p.bossDamageKnown && (!p.wallDamageKnown || p.bossDamage > p.wallDamage) {
				p.wallDamage, p.wallDamageKnown = p.bossDamage, true
			}
		}
		// Repeated losses need more farming, even if temporary buffs reappear.
		delay := time.Minute * time.Duration(1<<min(p.failures-1, 4))
		p.nextAttempt = now.Add(min(delay, 15*time.Minute))
		fmt.Printf("boss %d failed; farming until damage improves or an untried combat window appears (ordinary retry after %s)\n", wall, p.nextAttempt.Format("15:04:05"))
	}
	p.seen, p.lastZone, p.lastEnabled = true, s.Zone, s.Enabled
	if s.Enabled {
		return false
	}
	if p.wallZone == 0 {
		return !now.Before(p.nextAttempt)
	}
	if !p.wallDamageKnown && s.DamageKnown {
		p.wallDamage, p.wallDamageKnown = s.Damage, true
	}
	// Displayed damage is only a growth proxy; it does not measure click rate or predict a kill.
	growth := p.wallDamageKnown && s.DamageKnown && s.Damage-p.wallDamage >= math.Log10(2)
	strongerBuff := s.Buffs & ^p.wallBuffs != 0
	// Do not waste a short-lived full combat window on the farming backoff.
	if (s.FullCombat && !p.wallFullCombat) || (p.wallFullCombat && (growth || strongerBuff)) {
		p.nextAttempt = now
		return true
	}
	return !now.Before(p.nextAttempt) && (growth || strongerBuff)
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
				p.rememberBoss(p.wallZone, pending.before, now)
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
