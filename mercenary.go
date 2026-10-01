package main

import (
	"fmt"
	"image"
	"time"
)

type mercenaryQuest struct {
	reward   string
	duration time.Duration
	point    image.Point
}

type mercenaryObservation struct {
	frame                   gameFrame
	notify                  bool
	collect, start, running []image.Point
	thumb                   image.Point
	thumbFound, top, bottom bool
	quests                  []mercenaryQuest
	selected                int
	okay                    image.Point
	readable                bool
}

type mercenaryStep uint8

const (
	openMercenaries mercenaryStep = iota
	claimMercenaryReward
	openMercenaryQuest
	selectMercenaryQuest
	closeMercenaryQuest
	scrollMercenariesTop
	scrollMercenariesBottom
	returnToHeroes
	claimAndOpenMercenaryQuest
)

type mercenaryCommand struct {
	step  mercenaryStep
	quest int
}

type mercenaryAttempt struct {
	action gameAction
	until  time.Time
}

type mercenaryPlanner struct {
	latest                mercenaryObservation
	roster                *mercenaryObservation
	notificationConfirmed bool
	pending               *mercenaryAttempt
	nextScan              time.Time
	unreadableUntil       time.Time
	contextUntil          time.Time
	active                bool
	returnHeroes          bool
	topVisited            bool
	bottomVisited         bool
	aborting              bool
	collectOnly           bool
	questRow              image.Point
}

func mercenaryQuestRank(q mercenaryQuest) int {
	// Free recruitment restores a vacant roster slot; never click paid Hire.
	if q.reward == "recruitment" && q.duration > 0 && q.duration <= 8*time.Hour {
		return 0
	}
	if q.reward == "rubies" {
		for i, d := range []time.Duration{4 * time.Hour, 2 * time.Hour, time.Hour, 30 * time.Minute, 15 * time.Minute, 8 * time.Hour, 5 * time.Minute} {
			if q.duration == d {
				return 1 + i
			}
		}
		if q.duration == 24*time.Hour {
			return 300
		}
		if q.duration == 48*time.Hour {
			return 2000
		}
		return -1
	}
	switch q.reward {
	case "gold", "hero souls", "relics", "skills":
		minutes := int(q.duration / time.Minute)
		if q.duration < 5*time.Minute || q.duration > 48*time.Hour {
			return -1
		}
		if q.duration <= 4*time.Hour {
			return 10 + minutes
		}
		if q.duration <= 24*time.Hour {
			return 400 + minutes
		}
		return 3000 + minutes
	}
	return -1
}

func chooseMercenaryQuest(quests []mercenaryQuest) int {
	best, rank := -1, int(^uint(0)>>1)
	for i, q := range quests {
		if r := mercenaryQuestRank(q); r >= 0 && r < rank {
			best, rank = i, r
		}
	}
	return best
}

func (p *mercenaryPlanner) interrupt() {
	*p = mercenaryPlanner{}
}

func (p *mercenaryPlanner) expects(c gameContext, now time.Time) bool {
	if c.known {
		p.contextUntil = time.Time{}
	}
	if p.pending != nil && c == p.pending.action.frame.context {
		// A delayed or missed input may leave the original screen visible.
		return true
	}
	if !c.known && c.modal == noGildModal && (p.active || p.pending != nil) {
		// A transient HUD miss during an input must not restart the roster sweep.
		if p.contextUntil.IsZero() {
			p.contextUntil = now.Add(5 * time.Second)
		}
		return now.Before(p.contextUntil)
	}
	if p.pending == nil {
		return p.active && c.known && c.mercenaries && !c.questDialog
	}
	switch p.pending.action.mercenary.step {
	case openMercenaries, claimMercenaryReward, selectMercenaryQuest, closeMercenaryQuest, scrollMercenariesTop, scrollMercenariesBottom:
		return c.known && c.mercenaries && !c.questDialog
	case openMercenaryQuest, claimAndOpenMercenaryQuest:
		return c.known && c.questDialog
	case returnToHeroes:
		return c.known && c.heroes
	}
	return false
}

func (p *mercenaryPlanner) observe(o mercenaryObservation, now time.Time) {
	if o.frame.id <= p.latest.frame.id {
		return
	}
	if p.pending != nil {
		a := p.pending.action
		if o.frame.id <= a.frame.id {
			return
		}
		confirmed := false
		switch a.mercenary.step {
		case openMercenaries:
			confirmed = o.frame.context.mercenaries && !o.frame.context.questDialog
		case claimMercenaryReward:
			confirmed = o.frame.context.mercenaries && !o.frame.context.questDialog
		case openMercenaryQuest, claimAndOpenMercenaryQuest:
			confirmed = o.frame.context.questDialog
		case selectMercenaryQuest:
			confirmed = o.frame.context.mercenaries && !o.frame.context.questDialog
		case closeMercenaryQuest:
			confirmed = o.frame.context.mercenaries && !o.frame.context.questDialog
		case scrollMercenariesTop:
			confirmed = o.readable && o.top
		case scrollMercenariesBottom:
			confirmed = o.readable && o.bottom
		case returnToHeroes:
			confirmed = o.frame.context.heroes
		}
		if !confirmed && now.Before(p.pending.until) {
			return
		}
		p.pending = nil
		if !confirmed {
			fmt.Printf("mercenary action %d not confirmed; leaving quests for the next check\n", a.mercenary.step)
			p.aborting = true
			p.collectOnly = false
			// A failed close/return must not cause a rapid retry loop.
			if a.mercenary.step == closeMercenaryQuest || a.mercenary.step == returnToHeroes {
				p.active = false
				p.nextScan = now.Add(time.Minute)
			}
		} else {
			switch a.mercenary.step {
			case scrollMercenariesTop:
				p.topVisited = true
			case scrollMercenariesBottom:
				p.bottomVisited = true
			case returnToHeroes:
				p.active = false
				p.returnHeroes = false
				p.questRow = image.Point{}
				p.roster = nil
				p.nextScan = now.Add(time.Minute)
			case closeMercenaryQuest:
				// Only a confirmed closed quest lets us resume safe reward collection.
				p.collectOnly = p.aborting && p.questRow != (image.Point{})
				p.questRow = image.Point{}
				p.unreadableUntil = time.Time{}
				if p.collectOnly {
					fmt.Println("mercenary quest skipped; collecting remaining rewards before returning to Heroes")
				}
			case selectMercenaryQuest:
				p.questRow = image.Point{}
			}
		}
	}
	// Two fresh observations in the same layout reject a transient obstruction.
	previous := p.latest
	age := o.frame.at.Sub(previous.frame.at)
	p.notificationConfirmed = o.frame.context.heroes && o.notify && previous.notify &&
		o.frame.context == previous.frame.context && o.frame.layout == previous.frame.layout &&
		o.frame.generation == previous.frame.generation && age >= 0 && age <= 5*time.Second
	if o.readable {
		p.unreadableUntil = time.Time{}
	} else if (o.frame.context.mercenaries || o.frame.context.questDialog) && p.unreadableUntil.IsZero() {
		p.unreadableUntil = now.Add(5 * time.Second)
	}
	p.latest = o
	if p.roster == nil && o.readable && o.frame.context.mercenaries && !o.frame.context.questDialog {
		roster := o
		roster.collect = append([]image.Point(nil), o.collect...)
		roster.start = append([]image.Point(nil), o.start...)
		p.roster = &roster
	}
}

// Keep the known click plan current without reading buttons or timers again.
func (p *mercenaryPlanner) captured(frame gameFrame, now time.Time) {
	c := frame.context
	if c.mercenaries && !c.questDialog && p.roster != nil {
		o := *p.roster
		o.frame = frame
		p.observe(o, now)
	} else if p.pending != nil && (p.pending.action.mercenary.step == selectMercenaryQuest || p.pending.action.mercenary.step == closeMercenaryQuest || p.pending.action.mercenary.step == returnToHeroes) {
		p.observe(mercenaryObservation{frame: frame, selected: -1}, now)
	}
}

func (p *mercenaryPlanner) needsRead(c gameContext) bool {
	if c.questDialog {
		if p.pending != nil && (p.pending.action.mercenary.step == selectMercenaryQuest || p.pending.action.mercenary.step == closeMercenaryQuest) {
			return false
		}
		return p.latest.frame.id == 0 || !p.latest.readable || !p.latest.frame.context.questDialog
	}
	if c.mercenaries {
		return p.roster == nil
	}
	return p.pending == nil
}

func (p *mercenaryPlanner) action(now time.Time) (gameAction, bool) {
	o := p.latest
	a := gameAction{kind: handleMercenary, frame: o.frame}
	if p.pending != nil || o.frame.id == 0 || now.Before(p.nextScan) {
		return a, false
	}
	c := o.frame.context
	if !p.active {
		if c.heroes && o.notify && p.notificationConfirmed {
			a.mercenary.step = openMercenaries
			a.point = mercenaryPoint(c.bounds, 383, 200)
			return a, true
		}
		if !c.mercenaries {
			return a, false
		}
		// Also service a roster left open by the user or an interrupted run.
		p.active, p.returnHeroes, p.topVisited, p.bottomVisited, p.aborting, p.collectOnly = true, true, false, false, false, false
	}
	// Buttons can be briefly unreadable while a reward or tab animates. Wait
	// for a fresh readable frame before abandoning the visit; never click guesses.
	if !o.readable && (!p.aborting || p.collectOnly) && now.Before(p.unreadableUntil) {
		return a, false
	}
	if c.questDialog {
		if !o.readable || p.aborting || p.questRow == (image.Point{}) {
			a.mercenary.step = closeMercenaryQuest
			a.point = mercenaryPoint(c.bounds, 811, 53)
			p.aborting = true
		} else if o.selected >= 0 {
			p.aborting = true
			return p.action(now)
		} else if index := chooseMercenaryQuest(o.quests); index >= 0 {
			a.mercenary = mercenaryCommand{step: selectMercenaryQuest, quest: index}
			a.point = o.quests[index].point
		} else {
			p.aborting = true
			return p.action(now)
		}
		return a, true
	}
	if !c.mercenaries {
		p.interrupt()
		return a, false
	}
	if !o.readable {
		p.aborting = true
		p.collectOnly = false
	}
	if !p.aborting || p.collectOnly {
		if !p.topVisited && o.thumbFound && !o.top {
			a.mercenary.step, a.point = scrollMercenariesTop, o.thumb
			a.target = mercenaryPoint(c.bounds, 458, 387)
			return a, true
		}
		p.topVisited = true
		if len(o.collect) > 0 {
			a.mercenary.step, a.point = claimMercenaryReward, o.collect[0]
			if !p.aborting {
				a.mercenary.step = claimAndOpenMercenaryQuest
			}
			return a, true
		}
		if !p.aborting && len(o.start) > 0 {
			a.mercenary.step, a.point = openMercenaryQuest, o.start[0]
			return a, true
		}
		if o.thumbFound && !o.bottom && !p.bottomVisited {
			a.mercenary.step, a.point = scrollMercenariesBottom, o.thumb
			a.target = mercenaryPoint(c.bounds, 458, 964)
			return a, true
		}
	}
	if p.returnHeroes {
		a.mercenary.step = returnToHeroes
		a.point = mercenaryPoint(c.bounds, 63, 200)
		return a, true
	}
	p.active = false
	p.nextScan = now.Add(time.Minute)
	return a, false
}

func (p *mercenaryPlanner) sent(a gameAction, now time.Time) {
	p.latest = mercenaryObservation{}
	p.pending = &mercenaryAttempt{a, now.Add(5 * time.Second)}
	p.nextScan = now.Add(200 * time.Millisecond)
	switch a.mercenary.step {
	case openMercenaries:
		p.active, p.returnHeroes, p.topVisited, p.aborting = true, true, false, false
		p.bottomVisited = false
		p.collectOnly = false
		p.roster = nil
	case openMercenaryQuest, claimAndOpenMercenaryQuest:
		p.questRow = a.point
		// Accepted clicks consume the plan; the game applies them without another row read.
		p.consumeRow(a.point)
	case selectMercenaryQuest:
		fmt.Println("mercenary quest sent")
	case claimMercenaryReward:
		p.consumeRow(a.point)
	case scrollMercenariesTop, scrollMercenariesBottom:
		p.roster = nil
	}
}

func (p *mercenaryPlanner) consumeRow(point image.Point) {
	if p.roster == nil {
		return
	}
	for _, points := range []*[]image.Point{&p.roster.collect, &p.roster.start} {
		for i, row := range *points {
			if absDiff(row.X, point.X) <= 4 && absDiff(row.Y, point.Y) <= 4 {
				*points = append((*points)[:i], (*points)[i+1:]...)
				break
			}
		}
	}
}

func mercenaryPoint(b image.Rectangle, x, y int) image.Point {
	return b.Min.Add(image.Pt(b.Dx()*x/1000, b.Dy()*y/1000))
}

// Recheck the clickable region against the latest frame. OCR may finish after
// several captures; a user click or popup change must invalidate its decision.
func mercenaryActionStable(a gameAction, current gameFrame) bool {
	if a.frame.context != current.context {
		return false
	}
	if a.mercenary.step == openMercenaries {
		// The tab stays fixed while its notification moves between captures.
		return mercenaryNotification(current.image)
	}
	if a.mercenary.step != selectMercenaryQuest {
		return true
	}
	if a.frame.image == nil || current.image == nil {
		return false
	}
	b := current.context.bounds
	// Offer text must still match the decision before the first selection click.
	region := image.Rectangle{Min: mercenaryPoint(b, 255, 209), Max: mercenaryPoint(b, 595, 799)}
	changed, total := 0, 0
	for y := region.Min.Y; y < region.Max.Y; y += max(1, b.Dy()/500) {
		for x := region.Min.X; x < region.Max.X; x += max(1, b.Dx()/800) {
			r, g, blue := rgb(a.frame.image.At(x, y))
			cr, cg, cb := rgb(current.image.At(x, y))
			white := min(r, g, blue) > 170 && max(r, g, blue)-min(r, g, blue) < 55
			currentWhite := min(cr, cg, cb) > 170 && max(cr, cg, cb)-min(cr, cg, cb) < 55
			if white || currentWhite {
				total++
				if white != currentWhite {
					changed++
				}
			}
		}
	}
	return total > 0 && changed*100 < total
}
