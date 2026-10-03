package main

import (
	"image"
	"time"
)

// Pool and footer recognition share ordinary captures; placement uses the common queue.
func (p *gamePipeline) planAutoClickers(now time.Time) {
	if !p.options.autoClickers || !bootstrapHeroes(p.frame.context) || p.export.active || p.ancient.active || p.ascension.active {
		return
	}
	out := p.state[autoClickerAnalysis]
	if out.frame.id == 0 || !out.clickerPool.known || out.clickerPool.available == 0 || !autoClickerPoolStable(out.frame.image, p.frame.image) {
		return
	}
	hero := p.state[heroAnalysis]
	if p.options.heroes && hero.found && (hero.upgrades || hero.startup == startupUpgrades) && heroUpgradeButtonStable(hero.frame.image, p.frame.image, hero.point) {
		if cmd, ok := p.clickers.command(p.frame, out.clickerPool, autoClickerUpgrades, hero.point); ok {
			p.enqueue(gameAction{kind: placeOwnedClicker, frame: p.frame, point: cmd.point, clicker: cmd}, now)
			return
		}
	}
	b := p.frame.context.bounds
	point := b.Min.Add(image.Pt(b.Dx()*3/4, b.Dy()/2))
	if cmd, ok := p.clickers.command(p.frame, out.clickerPool, autoClickerMonster, point); ok {
		p.enqueue(gameAction{kind: placeOwnedClicker, frame: p.frame, point: cmd.point, clicker: cmd}, now)
	}
}
