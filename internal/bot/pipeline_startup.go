package bot

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
	startupSave
)

func (p *gamePipeline) beginStartup() {
	p.startupCheck = false
	p.startup, p.startupPassive = startupHeroes, false
	p.startupDeadline = time.Time{}
	p.startupExported, p.startupSkillsReady = false, false
	p.nextUpgrades = time.Time{}
	p.hero.startStartup()
	p.state = [analysisCount]observation{}
	p.queue = make(map[actionKind]gameAction)
	p.export.requested = false
	fmt.Println("startup: starting bounded hero skill setup")
}

func (p *gamePipeline) planStartup(now time.Time) bool {
	if p.startup == noStartup && !p.startupCheck {
		return false
	}
	if p.startup == startupSave {
		return p.planExport(now)
	}
	if p.startupDeadline.IsZero() {
		budget := 5 * time.Minute
		if p.startup == startupUpgrades {
			budget = 10 * time.Second
		}
		p.startupDeadline = now.Add(budget)
	}
	// A final drag may finish at the deadline. Allow its settled capture and
	// recognized footer one fixed grace period, never an unbounded retry.
	pool := p.state[autoClickerAnalysis]
	noFooterClicker := !p.options.autoClickers || pool.frame.id != 0 && pool.clickerPool.known &&
		(pool.clickerPool.available == 0 || pool.clickerPool.total <= 1) && autoClickerPoolStable(pool.frame.image, p.frame.image)
	if p.startup == startupUpgrades && (p.clickers.upgrades || p.startupSkillsReady && noFooterClicker || now.After(p.startupDeadline) && !p.startupFooterGrace(now)) {
		if p.clickers.upgrades {
			fmt.Println("startup: existing footer Auto Clicker handles upgrades")
		} else if p.startupSkillsReady && noFooterClicker {
			fmt.Println("startup: saved upgrades ready; continuing without a footer purchase")
		} else {
			fmt.Println("startup: upgrade footer unavailable; continuing with periodic upgrade checks")
		}
		p.finishStartupUpgradePass(now)
		return true
	}
	if p.startup != startupUpgrades && now.After(p.startupDeadline) || !p.hero.enabled {
		reason := "startup did not complete; check Heroes and press F8 to retry"
		if p.startupCheck {
			reason = "startup could not reach Heroes; focus the game and press F8 to retry"
		}
		p.controls.pause(reason)
		return true
	}
	if p.startupCheck || !bootstrapHeroes(p.frame.context) {
		if p.startupCheck && gameScreenVisible(p.frame.context) && !p.frame.context.heroes {
			p.enqueue(gameAction{kind: visitHeroes, frame: p.frame, point: ancientTabPoint(p.frame.image, true)}, now)
		}
		return true
	}
	progress := p.state[progressionAnalysis]
	if p.startupPassive && (!p.options.progression || progress.progression.Known) {
		if p.startup == startupProgression && (!p.options.progression || progress.progression.Enabled) {
			p.startup = noStartup
			p.startupDeadline = time.Time{}
			p.progression = progressionPlanner{}
			p.hero.interrupt()
			p.state = [analysisCount]observation{}
			p.queue = make(map[actionKind]gameAction)
			p.export.requested = p.options.export != nil && !p.startupExported
			fmt.Println("startup: heroes and upgrades ready; continuing automation")
			return true
		}
		if p.options.progression && !progress.progression.Enabled && p.progression.pending == nil && !now.Before(p.progression.nextAttempt) {
			p.enqueue(gameAction{kind: enableProgression, frame: progress.frame, progression: progress.progression}, now)
		}
	}
	switch p.startup {
	case startupHeroes:
		if action, ok := p.hero.action(now); ok {
			p.enqueue(action, now)
		}
		if p.startupNeedsSeedClicks() && !now.Before(p.nextMonster) {
			point := p.frame.context.bounds.Min.Add(image.Pt(p.frame.context.bounds.Dx()*3/4, p.frame.context.bounds.Dy()/2))
			p.enqueue(gameAction{kind: clickMonster, frame: p.frame, point: point}, now)
			p.nextMonster = now.Add(time.Second)
		}
	case startupUpgrades:
		if p.clickers.pending != nil && p.clickers.pending.target == autoClickerUpgrades {
			delete(p.queue, buyHeroUpgrades)
			return true
		}
		out := p.state[heroAnalysis]
		if out.frame.id == 0 || p.hero.pending != nil || !p.hero.due(now) {
			return true
		}
		if out.found && !p.startupSkillsReady && !now.Before(p.nextUpgrades) {
			p.enqueue(gameAction{kind: buyHeroUpgrades, frame: out.frame, point: out.point}, now)
		} else if out.upgradesKnown {
			p.finishStartupUpgradePass(now)
		} else if action, ok := p.hero.action(now); ok {
			p.enqueue(action, now)
		} else {
			p.hero.latest = heroObservation{}
			p.state[heroAnalysis] = observation{}
			p.hero.nextScan = now.Add(time.Second)
		}
	}
	return true
}

func (p *gamePipeline) startupFooterGrace(now time.Time) bool {
	foot := p.state[heroAnalysis]
	return p.startup == startupUpgrades && now.Before(p.startupDeadline.Add(2*time.Second)) &&
		(p.hero.pending != nil && p.hero.pending.action.kind == scrollHeroes ||
			foot.frame.id != 0 && now.Sub(foot.frame.at) < 2*time.Second && foot.upgradesKnown)
}

// Seed clicks earn starter gold only after a successful read proves it is needed.
func (p *gamePipeline) startupNeedsSeedClicks() bool {
	out := p.state[heroAnalysis]
	return p.startup == startupHeroes && !p.startupPassive && p.hero.pending == nil &&
		bootstrapHeroes(p.frame.context) && out.err == nil && out.frame.id != 0 &&
		out.frame.layout == p.layout && out.frame.generation == p.generation &&
		out.hero.startupNeedsGold && !out.hero.found && out.hero.startupScroll == (image.Point{})
}

// The footer pass buys upgrades unlocked by the bounded level sweep.
func (p *gamePipeline) finishStartupUpgradePass(now time.Time) {
	pool := p.state[autoClickerAnalysis]
	poolCurrent := pool.frame.id != 0 && pool.clickerPool.known && pool.frame.generation == p.frame.generation &&
		pool.frame.layout == p.frame.layout && pool.frame.context == p.frame.context && autoClickerPoolStable(pool.frame.image, p.frame.image)
	// A bulk upgrade purchase does not establish footer clicker placement.
	// Keep the existing bounded pass while its pool result is pending or eligible.
	if p.options.autoClickers && !p.clickers.upgrades && !p.clickers.footerAttempted && p.clickers.footerPasses == 0 &&
		(now.Before(p.startupDeadline) || p.startupFooterGrace(now)) && (!poolCurrent || pool.clickerPool.available > 0 && pool.clickerPool.total > 1) {
		p.hero.latest = heroObservation{}
		return
	}
	p.hero.interrupt()
	p.state[heroAnalysis] = observation{}
	p.barriers[heroAnalysis] = p.frame.id + 1
	p.heroJobFrame = 0
	if !p.clickers.upgrades {
		p.clickers.noteFooterUnavailable()
	}
	p.clickerFooterUntil = time.Time{}
	p.startup = startupProgression
	fmt.Println("startup: upgrades handled; waiting for progression")
	p.startupDeadline = time.Time{}
	p.state[progressionAnalysis] = observation{}
	p.barriers[progressionAnalysis] = p.frame.id + 1
	p.nextProgression = time.Time{}
	p.queue = make(map[actionKind]gameAction)
}
