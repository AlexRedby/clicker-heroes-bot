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
	confirmMercenaryQuest
	closeMercenaryQuest
	scrollMercenariesTop
	scrollMercenariesBottom
	returnToHeroes
)

type mercenaryCommand struct {
	step  mercenaryStep
	quest int
	offer mercenaryQuest
}

type mercenaryAttempt struct {
	action gameAction
	until  time.Time
}

type mercenaryPlanner struct {
	latest                mercenaryObservation
	notificationConfirmed bool
	pending               *mercenaryAttempt
	nextScan              time.Time
	unreadableUntil       time.Time
	active                bool
	returnHeroes          bool
	topVisited            bool
	aborting              bool
	questRow              image.Point
	questThumb            image.Point
	chosenQuest           int
	chosenOffer           mercenaryQuest
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

func (p *mercenaryPlanner) expects(c gameContext) bool {
	if p.pending == nil {
		return false
	}
	switch p.pending.action.mercenary.step {
	case openMercenaries, confirmMercenaryQuest, closeMercenaryQuest:
		return c.known && c.mercenaries && !c.questDialog
	case openMercenaryQuest:
		return c.known && c.questDialog
	case returnToHeroes:
		return c.known && c.heroes
	}
	return false
}

func hasMercenaryPoint(points []image.Point, point image.Point) bool {
	for _, candidate := range points {
		if absDiff(candidate.X, point.X) <= 4 && absDiff(candidate.Y, point.Y) <= 4 {
			return true
		}
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
			confirmed = o.readable && hasMercenaryPoint(o.start, a.point) && !hasMercenaryPoint(o.collect, a.point)
		case openMercenaryQuest:
			confirmed = o.frame.context.questDialog
		case selectMercenaryQuest:
			confirmed = o.readable && o.frame.context.questDialog && o.selected == a.mercenary.quest && o.okay != (image.Point{}) && mercenaryOfferMatches(o, a.mercenary.quest, a.mercenary.offer)
		case confirmMercenaryQuest:
			confirmed = o.readable && o.frame.context.mercenaries && !o.frame.context.questDialog && hasMercenaryPoint(o.running, p.questRow) && (p.questThumb == (image.Point{}) || (o.thumbFound && absDiff(o.thumb.Y, p.questThumb.Y) <= 4))
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
			// A failed close/return must not cause a rapid retry loop.
			if a.mercenary.step == closeMercenaryQuest || a.mercenary.step == returnToHeroes {
				p.active = false
				p.nextScan = now.Add(time.Minute)
			}
		} else {
			switch a.mercenary.step {
			case scrollMercenariesTop:
				p.topVisited = true
			case returnToHeroes:
				p.active = false
				p.returnHeroes = false
				p.nextScan = now.Add(time.Minute)
			case confirmMercenaryQuest:
				fmt.Println("mercenary quest confirmed")
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
}

func (p *mercenaryPlanner) action(now time.Time) (gameAction, bool) {
	o := p.latest
	a := gameAction{kind: handleMercenary, frame: o.frame, mercenaryThumb: o.thumb}
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
		p.active, p.topVisited, p.aborting = true, false, false
	}
	// Buttons can be briefly unreadable while a reward or tab animates. Wait
	// for a fresh readable frame before abandoning the visit; never click guesses.
	if !o.readable && !p.aborting && now.Before(p.unreadableUntil) {
		return a, false
	}
	if c.questDialog {
		if !o.readable || p.aborting || p.questRow == (image.Point{}) {
			a.mercenary.step = closeMercenaryQuest
			a.point = mercenaryPoint(c.bounds, 811, 53)
			p.aborting = true
		} else if o.selected >= 0 {
			// Confirm only the selection made by this planner.
			if o.selected != p.chosenQuest || o.okay == (image.Point{}) || !mercenaryOfferMatches(o, o.selected, p.chosenOffer) {
				p.aborting = true
				return p.action(now)
			}
			a.mercenary.step, a.point = confirmMercenaryQuest, o.okay
		} else if index := chooseMercenaryQuest(o.quests); index >= 0 {
			a.mercenary = mercenaryCommand{step: selectMercenaryQuest, quest: index, offer: o.quests[index]}
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
	}
	if !p.aborting {
		if !p.topVisited && o.thumbFound && !o.top {
			a.mercenary.step, a.point = scrollMercenariesTop, o.thumb
			a.target = mercenaryPoint(c.bounds, 458, 387)
			return a, true
		}
		p.topVisited = true
		// Dispatch newly idle mercenaries before collecting the next reward.
		if len(o.start) > 0 {
			a.mercenary.step, a.point = openMercenaryQuest, o.start[0]
			return a, true
		}
		if len(o.collect) > 0 {
			a.mercenary.step, a.point = claimMercenaryReward, o.collect[0]
			return a, true
		}
		if o.thumbFound && !o.bottom {
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
	case openMercenaryQuest:
		p.questRow = a.point
		p.questThumb = a.mercenaryThumb
		p.chosenQuest = -1
	case selectMercenaryQuest:
		p.chosenQuest = a.mercenary.quest
		p.chosenOffer = a.mercenary.offer
	}
}

func mercenaryOfferMatches(o mercenaryObservation, index int, offer mercenaryQuest) bool {
	return index >= 0 && index < len(o.quests) && o.quests[index].reward == offer.reward && o.quests[index].duration == offer.duration
}

func mercenaryPoint(b image.Rectangle, x, y int) image.Point {
	return b.Min.Add(image.Pt(b.Dx()*x/1000, b.Dy()*y/1000))
}

// Recheck the clickable region against the latest frame. OCR may finish after
// several captures; a user click or popup change must invalidate its decision.
func mercenaryActionStable(a gameAction, current gameFrame) bool {
	if a.frame.image == nil || current.image == nil || a.frame.context != current.context {
		return false
	}
	if a.mercenary.step == openMercenaries {
		// The tab stays fixed while its notification moves between captures.
		return mercenaryNotification(current.image)
	}
	b := current.context.bounds
	region := image.Rect(a.point.X-b.Dx()/35, a.point.Y-b.Dy()/55, a.point.X+b.Dx()/35, a.point.Y+b.Dy()/55).Intersect(b)
	textOnly := a.mercenary.step == selectMercenaryQuest || a.mercenary.step == confirmMercenaryQuest
	if textOnly {
		// Compare all offer text, not the empty center of a brown card. A reroll
		// can change the offers while leaving every card's geometry identical.
		region = image.Rectangle{Min: mercenaryPoint(b, 255, 209), Max: mercenaryPoint(b, 595, 799)}
	}
	changed, total := 0, 0
	for y := region.Min.Y; y < region.Max.Y; y += max(1, b.Dy()/500) {
		for x := region.Min.X; x < region.Max.X; x += max(1, b.Dx()/800) {
			r, g, blue := rgb(a.frame.image.At(x, y))
			cr, cg, cb := rgb(current.image.At(x, y))
			if textOnly {
				white := min(r, g, blue) > 170 && max(r, g, blue)-min(r, g, blue) < 55
				currentWhite := min(cr, cg, cb) > 170 && max(cr, cg, cb)-min(cr, cg, cb) < 55
				if white || currentWhite {
					total++
					if white != currentWhite {
						changed++
					}
				}
				continue
			}
			if absDiff(r, cr)+absDiff(g, cg)+absDiff(blue, cb) > 100 {
				changed++
			}
			total++
		}
	}
	if textOnly {
		return total > 0 && changed*100 < total
	}
	return total > 0 && changed*20 < total
}
