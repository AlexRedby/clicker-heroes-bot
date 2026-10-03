package main

import (
	"fmt"
	"image"
	"time"
)

type startupPhase uint8

const (
	noStartup startupPhase = iota
	startupHeroes
	startupUpgrades
	startupProgression
)

func (p *gamePipeline) beginStartup() {
	p.startupCheck = false
	p.startup, p.startupPassive = startupHeroes, false
	p.startupDeadline = time.Time{}
	p.hero.startStartup()
	p.ancient = ancientPlanner{}
	p.state = [analysisCount]observation{}
	p.queue = make(map[actionKind]gameAction)
	p.export.requested = false
	fmt.Println("startup: zone 1; starting affordable hero sweep")
}

func (p *gamePipeline) planStartup(now time.Time) bool {
	if p.startup == noStartup && !p.startupCheck {
		return false
	}
	if p.startupDeadline.IsZero() {
		p.startupDeadline = now.Add(5 * time.Minute)
	}
	if now.After(p.startupDeadline) || !p.hero.enabled {
		reason := "startup did not complete; check Heroes and press F8 to retry"
		if p.startupCheck {
			reason = "startup zone remained unreadable; focus Heroes and press F8 to retry"
		}
		p.controls.pause(reason)
		return true
	}
	if p.startupCheck || !bootstrapHeroes(p.frame.context) {
		return true
	}
	progress := p.state[progressionAnalysis]
	if p.startupPassive && progress.progression.Known {
		if p.startup == startupProgression && progress.progression.Enabled {
			p.startup = noStartup
			p.startupDeadline = time.Time{}
			p.progression = progressionPlanner{}
			p.hero.interrupt()
			p.state = [analysisCount]observation{}
			p.queue = make(map[actionKind]gameAction)
			p.export.requested = true
			fmt.Println("startup: heroes and upgrades ready, progression enabled; requesting fresh save export")
			return true
		}
		if !progress.progression.Enabled && p.progression.pending == nil && !now.Before(p.progression.nextAttempt) {
			p.enqueue(gameAction{kind: enableProgression, frame: progress.frame, progression: progress.progression}, now)
		}
	}
	switch p.startup {
	case startupHeroes:
		if action, ok := p.hero.action(now); ok {
			p.enqueue(action, now)
		}
		if !p.startupPassive && !now.Before(p.nextMonster) {
			point := p.frame.context.bounds.Min.Add(image.Pt(p.frame.context.bounds.Dx()*3/4, p.frame.context.bounds.Dy()/2))
			p.enqueue(gameAction{kind: clickMonster, frame: p.frame, point: point}, now)
			p.nextMonster = now.Add(time.Second)
		}
	case startupUpgrades:
		out := p.state[heroAnalysis]
		if out.frame.id == 0 || p.hero.pending != nil || !p.hero.due(now) {
			return true
		}
		if out.found {
			p.enqueue(gameAction{kind: buyHeroUpgrades, frame: out.frame, point: out.point}, now)
		} else if out.hero.thumbFound && !out.hero.bottom {
			p.enqueue(gameAction{kind: scrollHeroes, frame: out.frame, point: out.hero.thumb, target: image.Pt(out.hero.thumb.X, out.frame.context.bounds.Max.Y-1), hero: out.hero}, now)
		} else {
			p.hero.latest = heroObservation{}
			p.state[heroAnalysis] = observation{}
			p.hero.nextScan = now.Add(time.Second)
		}
	}
	return true
}
