package main

import "time"

type bootstrapMilestone struct {
	hero, level, upgrade, prerequisite, skillKey int
}

// Official 1.0e12-6144 upgrades. finalUpgrade (106) performs a reset and
// deliberately has no purchase step here; ascensionPlanner owns that action.
var bootstrapMilestones = [...]bootstrapMilestone{
	{1, 25, 7, 0, 1},
	{3, 75, 108, 0, 2},
	{10, 100, 109, 0, 3},
	{14, 100, 110, 0, 4},
	{16, 100, 112, 0, 5},
	{18, 10, 73, 0, 0},
	{18, 25, 74, 73, 0},
	{18, 50, 75, 74, 0},
	{18, 75, 76, 75, 6},
	{20, 25, 82, 76, 0},
	{20, 50, 83, 82, 0},
	{21, 100, 113, 0, 7},
	{23, 100, 114, 0, 8},
	{24, 100, 115, 0, 9},
}

type bootstrapObservation struct {
	frame gameFrame
	// These row facts must come from a positively identified, unobstructed row.
	// listKnown also requires supported geometry and a stable row/list position.
	hero, level       int
	bought, available map[int]bool
	skills            [9]skillState
	// inputPending includes queued actions, native confirmation and held modifiers.
	listKnown, x1, fish, inputPending bool
	// latestHeroKnown requires ownership plus a successor or verified complete roster end.
	heroBuyerEnabled, latestHeroKnown                                     bool
	ordinaryUpgradesVerified                                              bool
	progressionEnabled, combatVerified, clickerTargetsVerified            bool
	clickerOwnershipKnown, upgradesClicker, ascensionUsable, relicBlocked bool
}

type bootstrapStatus uint8

const (
	bootstrapPending bootstrapStatus = iota
	bootstrapWaiting
	bootstrapReady
	bootstrapBlocked
)

type bootstrapResult struct {
	frame      gameFrame
	status     bootstrapStatus
	reason     string
	milestone  bootstrapMilestone
	levels     int
	buyUpgrade bool
}

type bootstrapUpgradeAttempt struct {
	milestone bootstrapMilestone
	frame     gameFrame
	after     time.Time
	checks    int
}

type bootstrapPlanner struct {
	active, requireSkills, requireAscension bool
	startFrame                              gameFrame
	latest                                  bootstrapObservation
	confirmed                               map[int]bool
	pending                                 *bootstrapUpgradeAttempt
	failure                                 string
}

// The caller starts only after this cycle's spending/Heroes handoff (or an
// explicitly validated no-spending route). F8 must interrupt before restarting.
func (p *bootstrapPlanner) start(frame gameFrame, skills, ascension bool) {
	*p = bootstrapPlanner{active: frame.id != 0 && bootstrapHeroes(frame.context),
		requireSkills: skills, requireAscension: ascension, startFrame: frame, confirmed: make(map[int]bool)}
}

func bootstrapHeroes(c gameContext) bool {
	return c.known && c.heroes && c.modal == noGildModal && !c.ascension && !c.ancients && !c.ancientDialog && !c.saveMenu && !c.questDialog && !c.mercenaries
}

func (p *bootstrapPlanner) interrupt() { *p = bootstrapPlanner{} }

func (p *bootstrapPlanner) confirm(upgrade int) {
	for _, m := range bootstrapMilestones {
		if m.upgrade == upgrade && !p.confirmed[upgrade] {
			p.confirmed[upgrade] = true
			p.confirm(m.prerequisite)
			return
		}
	}
}

func (p *bootstrapPlanner) observe(o bootstrapObservation) {
	if !p.active {
		return
	}
	if o.frame.generation != p.startFrame.generation {
		if o.frame.generation > p.startFrame.generation {
			p.interrupt()
		}
		return
	}
	if o.frame.id <= p.startFrame.id || o.frame.id <= p.latest.frame.id || o.frame.at.Before(p.startFrame.at) || o.frame.at.Before(p.latest.frame.at) {
		return
	}
	p.latest = o
	if o.fish {
		p.pending = nil // Reobserve the purchase after collection; do not charge a failure.
		return
	}
	if !bootstrapHeroes(o.frame.context) || !o.listKnown || o.relicBlocked {
		return
	}
	for _, m := range bootstrapMilestones {
		if (o.hero == m.hero && o.level >= m.level && o.bought[m.upgrade]) || (m.skillKey != 0 && o.skills[m.skillKey-1].Known) {
			p.confirm(m.upgrade)
		}
	}
	if a := p.pending; a != nil && o.frame.id > a.frame.id && !o.frame.at.Before(a.after) {
		if o.frame.layout != a.frame.layout || o.hero != a.milestone.hero {
			p.failure = "upgrade confirmation lost its row or layout"
			p.pending = nil
			return
		}
		if p.confirmed[a.milestone.upgrade] {
			p.pending = nil
			return
		}
		a.checks++
		if a.checks >= 3 {
			p.failure = "ordinary upgrade purchase not confirmed"
			p.pending = nil
		}
	}
}

func (p *bootstrapPlanner) next(now time.Time) bootstrapResult {
	result := bootstrapResult{status: bootstrapWaiting, frame: p.latest.frame}
	if !p.active || p.latest.frame.id == 0 {
		result.reason = "fresh bootstrap handoff/observation required"
		return result
	}
	o := p.latest
	if p.pending != nil && !now.Before(p.pending.after.Add(10*time.Second)) {
		p.failure, p.pending = "ordinary upgrade confirmation timed out", nil
	}
	if p.failure != "" || o.relicBlocked || !o.heroBuyerEnabled {
		result.status, result.reason = bootstrapBlocked, p.failure
		if o.relicBlocked {
			result.reason = "relic inventory blocks bootstrap"
		}
		if !o.heroBuyerEnabled {
			result.reason = "required hero buyer disabled"
		}
		return result
	}
	if now.Before(o.frame.at) || now.Sub(o.frame.at) > 3*time.Second {
		result.reason = "fresh bootstrap observation required"
		return result
	}
	if !bootstrapHeroes(o.frame.context) || !o.listKnown || !o.x1 || o.fish {
		result.reason = "ordinary unobstructed Heroes list with x1 required"
		return result
	}
	if o.inputPending || p.pending != nil {
		result.reason = "input confirmation pending"
		return result
	}
	if !o.combatVerified {
		result.reason = "verified combat source required to earn bootstrap gold"
		return result
	}
	if !o.clickerOwnershipKnown {
		result.reason = "existing clicker ownership must be recognized before manual purchases"
		return result
	}
	for _, m := range bootstrapMilestones {
		ascensionStep := m.hero == 18 || m.hero == 20
		skillStep := m.hero != 20
		if p.confirmed[m.upgrade] || !(p.requireSkills && skillStep || p.requireAscension && !o.ascensionUsable && ascensionStep) {
			continue
		}
		result.status, result.milestone = bootstrapPending, m
		if o.hero != m.hero {
			result.reason = "visit required support hero"
		} else if o.level < m.level {
			result.levels = m.level - o.level
			result.reason = "hire/level required support hero"
		} else if o.upgradesClicker {
			result.status, result.reason = bootstrapWaiting, "observe upgrades owned by the existing Auto Clicker"
		} else if o.available[m.upgrade] {
			result.buyUpgrade = true
		} else {
			result.status, result.reason = bootstrapWaiting, "required ordinary upgrade unavailable; wait for gold or recognition"
		}
		return result
	}
	if p.requireSkills {
		for _, s := range o.skills {
			if !s.Known {
				result.reason = "all configured skill unlocks must be visible; cooldown is allowed"
				return result
			}
		}
	}
	if p.requireAscension && !o.ascensionUsable {
		// Preparing the prerequisites/level never authorizes buying finalUpgrade.
		result.status = bootstrapPending
		result.milestone = bootstrapMilestone{hero: 20, level: 150}
		if o.hero == 20 && o.level < 150 {
			result.levels = 150 - o.level
		} else if o.hero == 20 {
			result.status = bootstrapWaiting
		}
		result.reason = "prepare Amenhotep 150 and verify the Ascension entry"
		return result
	}
	if !o.latestHeroKnown || !o.ordinaryUpgradesVerified || !o.progressionEnabled || !o.clickerTargetsVerified {
		result.reason = "latest hero, ordinary upgrades, progression and existing clicker targets must be verified"
		return result
	}
	result.status = bootstrapReady
	return result
}

// Call only after the shared queue actually sent this ordinary upgrade click.
// Level purchases use heroRunner's existing bounded confirmation instead.
func (p *bootstrapPlanner) sentUpgrade(r bootstrapResult, now time.Time) bool {
	if !p.active || p.pending != nil || !r.buyUpgrade || r.frame.generation != p.startFrame.generation || r.frame.id <= p.startFrame.id {
		return false
	}
	for _, m := range bootstrapMilestones {
		if m == r.milestone {
			if !p.confirmed[m.upgrade] {
				p.pending = &bootstrapUpgradeAttempt{milestone: m, frame: r.frame, after: now.Add(200 * time.Millisecond)}
			}
			return true
		}
	}
	return false
}
