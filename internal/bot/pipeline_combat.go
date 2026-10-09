package bot

import (
	"image"
	"time"
)

// Purchases and navigation take priority; otherwise active progression can earn gold.
func (p *gamePipeline) monsterAssistReady() bool {
	if !p.options.heroes && !p.options.progression || p.startupCheck || p.startup == startupSave ||
		!bootstrapHeroes(p.frame.context) || !gameScreenVisible(p.frame.context) ||
		p.export.requested || p.export.active || p.ancient.active || p.ascension.active ||
		p.relic.active || p.gild.active || p.gildMoving || p.mercenary.active ||
		p.prestige.exclusive() || p.hero.pending != nil || p.clickers.pending != nil {
		return false
	}
	for kind := range p.queue {
		if kind != clickMonster && kind != collectFish && kind != parkPointer {
			return false
		}
	}
	return true
}

func (p *gamePipeline) planMonsterAssist(now time.Time) {
	if now.Before(p.nextMonster) || !p.options.monster && !p.monsterAssistReady() {
		return
	}
	point, interval := p.options.monsterPoint, p.options.clickInterval
	if !p.options.monster {
		b := p.frame.context.bounds
		point, interval = b.Min.Add(image.Pt(b.Dx()*3/4, b.Dy()/2)), 100*time.Millisecond
	}
	p.enqueue(gameAction{kind: clickMonster, frame: p.frame, point: point}, now)
	p.nextMonster = now.Add(interval)
}
