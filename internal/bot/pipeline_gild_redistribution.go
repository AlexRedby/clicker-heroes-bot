package bot

import (
	"fmt"
	"image"
	"time"
)

func (p *gamePipeline) reportGildMove(message string) {
	if message != p.gildMoveMessage {
		fmt.Println("Gild redistribution:", message)
		p.gildMoveMessage = message
	}
}

func (p *gamePipeline) acceptGildSave(out exportResult, source gameFrame, now time.Time) {
	if !p.options.progression || out.gilds == nil {
		if p.export.gildsOnly {
			p.gildMove.active, p.gildMoving = false, false
			p.reportGildMove(fmt.Sprintf("fresh distribution unavailable; continuing ordinary play: %v", out.gildErr))
		}
		return
	}
	if !out.at.IsZero() {
		source.at = out.at
	}
	wasPending := p.gildMove.awaitingExport
	if err := p.gildMove.install(*out.gilds, source); err != nil {
		p.gildMove.active, p.gildMoving = false, false
		p.reportGildMove(err.Error() + "; continuing ordinary play")
		return
	}
	p.gildMoving = false
	p.gildRefreshDue = time.Time{}
	p.gildJobFrame, p.nextGildRead = 0, time.Time{}
	if wasPending && out.gilds.MoveGilds == 0 && out.gilds.Target.ID != 0 {
		p.reportGildMove("fresh save confirms all gilds on " + out.gilds.Target.Name)
	} else if p.gildMove.reason != "" {
		p.reportGildMove(p.gildMove.reason)
	}
}

func (p *gamePipeline) planGildRedistribution(now time.Time) bool {
	g := &p.gildMove
	if !p.options.progression || p.options.export == nil {
		p.gildMoving = false
		return false
	}
	if p.gildMoving && !bootstrapHeroes(p.frame.context) && p.frame.context.modal != gildRosterModal {
		g.interrupt()
		p.gildMoving = false
		return false
	}
	if !p.gildMoving {
		if p.startup != noStartup || p.startupCheck || p.export.requested || p.prestige.exclusive() || p.prestige.holdAncients() || p.ancient.active || p.ascension.active || p.relic.active || p.mercenary.active || p.mercenary.pending != nil || p.hero.pending != nil || p.skill.pending != nil || p.progression.pending != nil || p.gild.active || !bootstrapHeroes(p.frame.context) || p.options.export == nil {
			return false
		}
		if !p.gildRefreshDue.IsZero() && !now.Before(p.gildRefreshDue) {
			p.export.requested, p.export.gildsOnly = true, true
			p.export.relicsOnly, p.export.goalsOnly, p.export.prestigeOnly = false, false, false
			p.queue = make(map[actionKind]gameAction)
			return p.planExport(now)
		}
		if !g.active {
			return false
		}
		p.gildMoving = true
		p.queue = make(map[actionKind]gameAction)
		p.hero.interrupt()
		p.reportGildMove(fmt.Sprintf("preparing %d gilds for %s", g.plan.MoveGilds, g.plan.Target.Name))
	}
	cmd, found := g.next(now)
	if !g.active {
		p.gildMoving = false
		p.reportGildMove(g.reason)
		return false
	}
	if !found {
		return true
	}
	if cmd.action == gildRefreshSave {
		g.submitted(cmd, now, nil)
		p.export.requested, p.export.gildsOnly = true, true
		p.export.relicsOnly, p.export.goalsOnly, p.export.prestigeOnly = false, false, false
		p.gildMoving = false
		p.queue = make(map[actionKind]gameAction)
		return p.planExport(now)
	}
	a := gameAction{kind: handleGildRedistribution, frame: cmd.frame, point: cmd.point, gildMove: cmd}
	if cmd.action == gildScrollHeroes {
		if thumb, _, found := heroScrollbarThumb(cmd.frame.image); found {
			a.point, a.target = thumb, image.Pt(thumb.X, cmd.frame.context.bounds.Max.Y-1)
		}
	}
	p.enqueue(a, now)
	return true
}

func gildActionLabel(action gildRedistributionAction) string {
	return [...]string{"", "open roster", "scroll Heroes", "scroll roster", "Q/all transfer", "close roster", "refresh save"}[action]
}
