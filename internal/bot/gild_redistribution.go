package bot

import (
	"context"
	"fmt"
	"image"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

type gildRedistributionAction uint8

const (
	gildOpenRoster gildRedistributionAction = iota + 1
	gildScrollHeroes
	gildScrollRoster
	gildTransferAll
	gildCloseRoster
	gildRefreshSave
)

type gildRedistributionCommand struct {
	action   gildRedistributionAction
	frame    gameFrame
	point    image.Point
	region   image.Rectangle
	saveHash string
}

// Only the queue owner installs fresh exports and submits commands. This
// planner never captures frames, writes a journal, or dispatches input.
type gildRedistribution struct {
	plan                            ancientcalc.GildPlan
	source                          gameFrame
	stopAfterClose, blocked         bool
	ui                              gildRedistributionObservation
	active, awaitingExport          bool
	nextAction, attemptedAt         time.Time
	scrolls, attempts, preparations int
	lastRoster                      [32]byte
	waitFrame                       uint64
	reason                          string
}

func (g *gildRedistribution) install(plan ancientcalc.GildPlan, source gameFrame) error {
	if source.id == 0 || source.layout == 0 || source.at.IsZero() || source.context.window == "" || len(plan.SaveHash) != 64 {
		return fmt.Errorf("gild export source identity is missing")
	}
	if g.awaitingExport && !source.at.After(g.attemptedAt) {
		return fmt.Errorf("gild outcome requires a newer export")
	}
	attempts, preparations := g.attempts, g.preparations
	same := sameGildDistribution(plan, g.plan)
	blocked := same && g.blocked
	reason := ""
	if blocked {
		reason = g.reason
	}
	if !same {
		attempts, preparations = 0, 0
	}
	*g = gildRedistribution{plan: plan, source: source, attempts: attempts, preparations: preparations, blocked: blocked, reason: reason}
	g.active = plan.Eligible && plan.MoveGilds > 0 && plan.Target.ID >= 28 && plan.Target.ID <= 46 && attempts < 2 && !blocked
	if attempts >= 2 {
		g.reason = "two transfer attempts left the same distribution; continuing ordinary play"
	}
	return nil
}

func sameGildDistribution(a, b ancientcalc.GildPlan) bool {
	if a.Target.ID != b.Target.ID || a.Ascensions != b.Ascensions || a.Transcensions != b.Transcensions || a.TranscensionTimestamp != b.TranscensionTimestamp || len(a.Heroes) != len(b.Heroes) {
		return false
	}
	for i := range a.Heroes {
		if a.Heroes[i].ID != b.Heroes[i].ID || a.Heroes[i].Gilds != b.Heroes[i].Gilds {
			return false
		}
	}
	return true
}

func (g *gildRedistribution) observe(ui gildRedistributionObservation) {
	if ui.frame.id > g.ui.frame.id {
		g.ui = ui
	}
}

func (g *gildRedistribution) interrupt() {
	g.blocked = false
	if !g.active {
		return
	}
	g.awaitingExport = true
	if g.ui.frame.at.After(g.attemptedAt) {
		g.attemptedAt = g.ui.frame.at
	}
	g.waitFrame = 0
	g.nextAction = time.Time{}
	g.reason = "interrupted; refresh current distribution before another transfer"
}

func (g *gildRedistribution) next(now time.Time) (gildRedistributionCommand, bool) {
	u := g.ui
	c := gildRedistributionCommand{frame: u.frame, saveHash: g.plan.SaveHash}
	if !g.active || now.Before(g.nextAction) || u.frame.id <= g.waitFrame || u.frame.at.After(now) || now.Sub(u.frame.at) > 2*time.Second {
		return c, false
	}
	if !u.frame.context.known {
		if now.Sub(g.source.at) > 30*time.Second {
			g.active, g.blocked = false, true
			g.reason = "gild controls remained unknown; continuing ordinary play"
		}
		return c, false
	}
	if u.frame.generation != g.source.generation || u.frame.context.window != g.source.context.window || u.frame.context.bounds != g.source.context.bounds {
		g.interrupt()
	}
	if !g.awaitingExport && (now.Sub(g.source.at) > 30*time.Second || now.Before(g.source.at)) {
		g.preparations++
		if g.preparations >= 2 {
			g.stopAfterClose, g.blocked = true, true
		}
		g.awaitingExport = true
		g.reason = "gild preparation export expired"
	}
	if g.awaitingExport {
		if u.roster && u.close == (image.Point{}) {
			g.active, g.blocked = false, true
			g.reason = "roster close control unavailable; fresh distribution still required"
			return c, false
		}
		if u.roster && u.close != (image.Point{}) {
			c.action, c.point = gildCloseRoster, u.close
		} else if bootstrapHeroes(u.frame.context) {
			if g.stopAfterClose {
				g.active = false
				return c, false
			}
			c.action = gildRefreshSave
		}
		return c, c.action != 0
	}
	if u.roster {
		if u.close == (image.Point{}) {
			g.active, g.blocked = false, true
			g.reason = "roster close control unavailable"
			return c, false
		}
		if u.targetFound && u.targetID == g.plan.Target.ID {
			c.action, c.point, c.region = gildTransferAll, u.target, u.targetRegion
		} else if g.scrolls >= 16 || u.down == (image.Point{}) || (g.scrolls > 0 && u.rosterHash == g.lastRoster) {
			g.awaitingExport, g.stopAfterClose, g.blocked = true, true, true
			g.reason = "named target not found in the bounded roster search"
			c.action, c.point = gildCloseRoster, u.close
		} else if u.down != (image.Point{}) {
			c.action, c.point = gildScrollRoster, u.down
		}
	} else if bootstrapHeroes(u.frame.context) {
		if u.entry != (image.Point{}) {
			c.action, c.point, c.region = gildOpenRoster, u.entry, u.entryRegion
		} else if g.scrolls < 16 {
			c.action, c.point = gildScrollHeroes, u.frame.image.Bounds().Min.Add(image.Pt(u.frame.image.Bounds().Dx()*3/10, u.frame.image.Bounds().Dy()*7/10))
		} else {
			g.active, g.blocked = false, true
			g.reason = "Gilded entry not found in the bounded Heroes search"
		}
	}
	return c, c.action != 0 && (c.action == gildRefreshSave || c.point != (image.Point{}))
}

func (g *gildRedistribution) submitted(cmd gildRedistributionCommand, now time.Time, err error) {
	if !g.active || cmd.saveHash != g.plan.SaveHash {
		return
	}
	g.waitFrame, g.nextAction = cmd.frame.id, now.Add(250*time.Millisecond)
	switch cmd.action {
	case gildTransferAll:
		// Even an input error can follow mouse-down. Only a fresh distribution
		// can establish whether Q+click moved the gilds; never retry blindly.
		g.attempts++
		g.awaitingExport, g.attemptedAt = true, now
	case gildScrollHeroes, gildScrollRoster:
		g.scrolls++
		g.lastRoster = g.ui.rosterHash
	case gildOpenRoster:
		g.scrolls = 0
	case gildCloseRoster:
		if g.stopAfterClose {
			g.active = false
		}
	case gildRefreshSave:
		g.active = false
	}
	if err != nil {
		g.interrupt()
		g.waitFrame, g.nextAction = cmd.frame.id, now.Add(250*time.Millisecond)
	}
}

// The shared dispatcher calls this against its last guarded frame. Target
// typography must remain unchanged between OCR and mouse-down.
func gildRedistributionCommandValid(cmd gildRedistributionCommand, frame gameFrame) bool {
	if !frame.context.known || frame.image == nil || cmd.frame.generation != frame.generation || cmd.frame.layout != frame.layout || cmd.frame.context.window != frame.context.window {
		return false
	}
	switch cmd.action {
	case gildOpenRoster:
		return bootstrapHeroes(frame.context) && heroTextStable(cmd.frame.image, frame.image, cmd.region)
	case gildScrollHeroes, gildRefreshSave:
		return bootstrapHeroes(frame.context)
	case gildTransferAll:
		return frame.context.modal == gildRosterModal && gildNameStable(cmd.frame.image, frame.image, cmd.region)
	case gildScrollRoster, gildCloseRoster:
		return frame.context.modal == gildRosterModal
	}
	return false
}

func executeGildRedistribution(ctx context.Context, input heroInput, cmd gildRedistributionCommand) error {
	switch cmd.action {
	case gildTransferAll:
		return withHeldKey(ctx, input, "q", 100*time.Millisecond, func() error { return input.click(cmd.point) })
	case gildScrollHeroes:
		return input.scroll(cmd.point, 5)
	case gildOpenRoster, gildScrollRoster, gildCloseRoster:
		return input.click(cmd.point)
	default:
		return fmt.Errorf("gild command requires the shared export owner")
	}
}
