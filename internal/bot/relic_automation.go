package bot

import (
	"errors"
	"fmt"
	"image"
	"reflect"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/vision"
)

type relicStep uint8

const (
	relicAcquire relicStep = iota
	relicOpenTab
	relicInspect
	relicHover
	relicPark
	relicEquip
	relicReturn
	relicVerifyEquip
	relicAcknowledge
	relicOpenSalvage
	relicConfirmSalvage
	relicWaitSalvage
)

type relicObservation struct {
	frame gameFrame
	ui    relicUI
	uid   int
}

type relicCommand struct {
	step   relicStep
	before image.Image
}

type relicPlanner struct {
	active, notice, previousAlert, changed, failed bool
	verifyNeeded                                   bool
	step                                           relicStep
	deadline, nextRead, nextCheck                  time.Time
	lastInput, jobFrame                            uint64
	window                                         string
	original, snapshot                             ancientcalc.RelicSnapshot
	latest                                         relicObservation
	base                                           gameFrame
	cards                                          []image.Point
	scan, moves, returnAttempts                    int
	move                                           *ancientcalc.RelicSuggestion
	point                                          image.Point
}

func (r *relicPlanner) interrupt() { *r = relicPlanner{} }

func relicEquipped(snapshot ancientcalc.RelicSnapshot) map[int]int {
	out := map[int]int{}
	for _, item := range snapshot.Items {
		if item.Slot >= 1 && item.Slot <= 4 {
			out[item.Slot] = item.UID
		}
	}
	return out
}

func relicSameItems(before, after ancientcalc.RelicSnapshot) bool {
	if before.Ascensions != after.Ascensions || before.Transcendent != after.Transcendent || before.EquipmentSlots != after.EquipmentSlots || len(before.Items) != len(after.Items) {
		return false
	}
	items := map[int]ancientcalc.Relic{}
	for _, item := range before.Items {
		item.Slot = 0
		items[item.UID] = item
	}
	for _, item := range after.Items {
		item.Slot = 0
		old, found := items[item.UID]
		if !found || !reflect.DeepEqual(old, item) {
			return false
		}
	}
	return true
}

func (p *gamePipeline) startRelics(now time.Time) {
	r := &p.relic
	*r = relicPlanner{active: true, step: relicAcquire, deadline: now.Add(90 * time.Second), window: p.frame.context.window}
	p.ascension.relicsChecked = false
	p.export.requested, p.export.relicsOnly = true, true
	p.queue = make(map[actionKind]gameAction)
	fmt.Println("relics: checking equipment for Active play")
}

// Unknown mapping defers destruction, but does not repeatedly pause ordinary play.
func (p *gamePipeline) relicFailed(reason string, now time.Time) {
	r := &p.relic
	r.failed, r.step, r.latest, r.jobFrame = true, relicReturn, relicObservation{}, 0
	r.deadline, r.nextCheck = now.Add(10*time.Second), now.Add(time.Minute)
	p.ascension.relicsChecked = false
	p.ascension.nextCheck = r.nextCheck
	p.queue = make(map[actionKind]gameAction)
	fmt.Printf("relics: %s; automatic Ascension deferred, retry in 60s\n", reason)
}

func (p *gamePipeline) acceptRelicSave(preview *ancientcalc.RelicPreview, now time.Time) error {
	r := &p.relic
	if preview == nil || !r.active {
		return errors.New("missing owned relic export")
	}
	after := preview.Snapshot
	switch r.step {
	case relicAcquire:
		move, err := ancientcalc.PlanRelicEquipment(after)
		if err != nil {
			return err
		}
		r.original, r.snapshot, r.move = after, after, move
		r.snapshot.Items = append([]ancientcalc.Relic(nil), after.Items...)
		r.step = relicOpenTab
		if move == nil {
			fmt.Println("relics: no clear Active upgrade; checking junk and notifications")
		} else {
			fmt.Printf("relics: locating candidate UID %d for equipment slot %d\n", move.UID, move.Slot)
		}
	case relicVerifyEquip:
		if !relicSameItems(r.original, after) || !reflect.DeepEqual(relicEquipped(r.snapshot), relicEquipped(after)) {
			return errors.New("equipment outcome differs from the batch; no replay")
		}
		// A landed equipment batch needs fresh inventory evidence before salvage.
		discard := false
		if after.AncientLevels != nil {
			var err error
			discard, err = ancientcalc.RelicJunkIsDominated(after)
			if err != nil {
				return err
			}
		}
		if discard {
			r.step, r.moves = relicAcquire, 0
			r.base, r.cards, r.point = gameFrame{}, nil, image.Point{}
			return p.acceptRelicSave(preview, now)
		}
		p.finishRelics(now, true)
	default:
		return errors.New("unexpected relic export")
	}
	return nil
}

func relicEquipmentPoint(screen image.Image, slot int) image.Point {
	r := vision.Rect(screen, relicEquipmentRegions[slot-1])
	return r.Min.Add(r.Size().Div(2))
}

func (r *relicPlanner) acknowledge(frame gameFrame, ui relicUI) {
	r.cards = append([]image.Point(nil), ui.junk...)
	for i, present := range ui.equipment {
		if present {
			r.cards = append(r.cards, relicEquipmentPoint(frame.image, i+1))
		}
	}
	r.scan, r.step = 0, relicAcknowledge
}

func (r *relicPlanner) observe(out relicObservation, err error) error {
	if !r.active || out.frame.id <= r.lastInput {
		return nil
	}
	r.latest = out
	if r.step == relicHover {
		// OCR of unrelated junk is unnecessary evidence. Skip unreadable cards,
		// but require a unique match for the exact candidate before any drag.
		if err == nil && out.uid == r.move.UID {
			r.point = r.cards[r.scan]
		}
		r.scan++
		r.step = relicPark
		return nil
	}
	if err != nil || !out.ui.known {
		return errors.New("obscured or unsupported relic inventory")
	}
	equipped := relicEquipped(r.snapshot)
	for i, present := range out.ui.equipment {
		if present != (equipped[i+1] != 0) {
			return errors.New("equipment occupancy differs from fresh export")
		}
	}
	if len(out.ui.junk)+len(equipped) != len(r.snapshot.Items) {
		return errors.New("visible inventory differs from fresh export")
	}
	if r.base.id != 0 && !relicInventoryStable(r.base.image, out.frame.image) {
		return errors.New("inventory changed while locating the candidate")
	}
	if r.cards == nil {
		r.base, r.cards = out.frame, out.ui.junk
	}
	if r.move == nil {
		var err error
		r.move, err = ancientcalc.PlanRelicEquipment(r.snapshot)
		if err != nil {
			return err
		}
	}
	// Four moves per Save batch; retry combat before another batch.
	if r.move == nil && !r.verifyNeeded && r.snapshot.AncientLevels != nil {
		// No recommendation alone never authorizes destruction. Every junk
		// item must be no better than a kept item in every supported Active bonus.
		discard, err := ancientcalc.RelicJunkIsDominated(r.snapshot)
		if err != nil {
			return err
		}
		if discard && len(out.ui.junk) > 0 {
			r.step = relicOpenSalvage
			return nil
		}
	}
	if r.move == nil || r.moves >= 4 {
		r.acknowledge(out.frame, out.ui)
	} else if r.point != (image.Point{}) {
		r.step = relicEquip
	} else if r.scan < len(r.cards) {
		r.step = relicHover
	} else {
		return errors.New("candidate tooltip missing or ambiguous")
	}
	return nil
}

func (p *gamePipeline) finishRelics(now time.Time, verified bool) {
	r := &p.relic
	changed, failed := r.changed, r.failed
	*r = relicPlanner{nextCheck: now.Add(time.Minute)}
	p.ascension.invalidate()
	p.ascension.relicsChecked = verified && !failed
	if failed {
		p.ascension.nextCheck = r.nextCheck
	}
	if changed {
		p.ascension.interrupt()
		p.progression = progressionPlanner{}
		if verified && !failed {
			fmt.Println("relics: equipment improved; retry combat before Ascension")
		} else {
			fmt.Println("relics: equipment outcome uncertain; refreshing combat before Ascension")
		}
	}
	p.queue = make(map[actionKind]gameAction)
	p.state = [analysisCount]observation{}
	p.nextProgression, p.nextFish, p.ascension.nextRead = time.Time{}, time.Time{}, time.Time{}
}

func (p *gamePipeline) planRelics(now time.Time) bool {
	r := &p.relic
	if !r.active {
		if p.options.progression && p.frame.context.relics && relicPanelPresent(p.frame.image) {
			*r = relicPlanner{active: true, failed: true, step: relicReturn, window: p.frame.context.window, deadline: now.Add(10 * time.Second)}
			p.ascension.relicsChecked = false
			return true
		}
		if r.notice && p.options.progression && p.options.export != nil && p.frame.context.heroes && !now.Before(r.nextCheck) && !p.ascension.active && !p.gild.active && !p.mercenary.active && p.mercenary.pending == nil && p.hero.pending == nil && p.skill.pending == nil && p.progression.pending == nil {
			p.startRelics(now)
			return true
		}
		return p.frame.context.relics
	}
	if p.export.requested {
		return true
	}
	if now.After(r.deadline) {
		if r.failed {
			// Only an observed Heroes frame completes a return. Retry at most
			// twice per minute while an unchanged/covered panel remains visible.
			r.returnAttempts, r.deadline = 0, now.Add(time.Minute)
			fmt.Println("relics: Heroes return unconfirmed; retrying when panel is visible")
		} else {
			p.relicFailed("equipment visit timed out", now)
		}
	}
	if p.frame.context.window != r.window {
		p.relicFailed("game window changed", now)
		p.finishRelics(now, false)
		return true
	}
	if p.frame.id <= r.lastInput {
		return true
	}
	if r.step == relicReturn && p.frame.context.heroes {
		if r.verifyNeeded {
			r.verifyNeeded = false
			r.step = relicVerifyEquip
			p.export.requested, p.export.relicsOnly = true, true
		} else {
			p.finishRelics(now, !r.failed)
		}
		return true
	}
	if r.step == relicOpenTab && p.frame.context.relics {
		if r.move == nil && r.snapshot.AncientLevels == nil {
			ui := readRelicUI(p.frame.image)
			if !ui.known {
				return true
			}
			r.acknowledge(p.frame, ui)
		} else {
			r.step = relicInspect
		}
		return true
	}
	if (r.step == relicAcknowledge || r.step >= relicOpenSalvage) && now.Before(r.nextRead) {
		return true
	}
	if r.step == relicOpenSalvage && p.frame.context.relicJunk && p.frame.context.relics {
		r.step = relicConfirmSalvage
	}
	if r.step == relicWaitSalvage {
		if p.frame.context.relicJunk {
			return true
		} // Never replay Yes.
		ui := readRelicUI(p.frame.image)
		if !ui.known || len(ui.junk) != 0 {
			return true
		}
		equipped := relicEquipped(r.snapshot)
		for i, present := range ui.equipment {
			if present != (equipped[i+1] != 0) {
				p.relicFailed("equipment changed during junk salvage", now)
				return true
			}
		}
		fmt.Println("relics: dominated junk salvaged; equipped items preserved")
		r.acknowledge(p.frame, ui)
		return true
	}
	a := gameAction{kind: handleRelic, frame: p.frame, relic: relicCommand{step: r.step, before: r.base.image}}
	switch r.step {
	case relicOpenSalvage:
		if !p.frame.context.relics || p.frame.context.relicJunk || !readRelicUI(p.frame.image).known || !relicInventoryStable(r.base.image, p.frame.image) {
			return true
		}
		rg := vision.Rect(p.frame.image, image.Rect(225, 505, 415, 546))
		a.point = rg.Min.Add(rg.Size().Div(2))
	case relicConfirmSalvage:
		if !p.frame.context.relics || !p.frame.context.relicJunk {
			return true
		}
		point, found, err := relicJunkControl(p.frame.image, true)
		if err != nil || !found {
			return true
		}
		a.point = point
	case relicOpenTab:
		if !p.frame.context.heroes {
			return true
		}
		a.point = relicTabPoint(p.frame.image)
	case relicAcknowledge:
		if r.scan >= len(r.cards) {
			r.step = relicReturn
			return true
		}
		a.point = r.cards[r.scan]
	case relicHover:
		if r.latest.frame.id == 0 {
			return true
		}
		a.point = r.cards[r.scan]
	case relicPark:
		a.point = parkPoint(p.frame.context.bounds)
	case relicEquip:
		a.point, a.target = r.point, relicEquipmentPoint(p.frame.image, r.move.Slot)
	case relicReturn:
		if r.returnAttempts >= 2 {
			return true
		}
		point, found, err := navigationControl(p.frame, navigationHeroes)
		if err != nil || !found {
			return true
		}
		a.point = point
	default:
		return true
	}
	p.enqueue(a, now)
	return true
}

func (r *relicPlanner) sent(a gameAction, now time.Time) {
	r.lastInput, r.latest, r.nextRead = a.frame.id, relicObservation{}, now.Add(400*time.Millisecond)
	switch a.relic.step {
	case relicConfirmSalvage:
		r.step = relicWaitSalvage
	case relicOpenTab:
		if r.move != nil || r.snapshot.AncientLevels != nil {
			r.step = relicInspect
		}
	case relicPark:
		r.step = relicInspect
	case relicAcknowledge:
		r.scan++
	case relicReturn:
		r.returnAttempts++
	case relicEquip:
		for i := range r.snapshot.Items {
			item := &r.snapshot.Items[i]
			if item.UID == r.move.UID {
				item.Slot = r.move.Slot
			} else if item.UID == r.move.ReplaceUID {
				item.Slot = 0
			}
		}
		r.changed, r.verifyNeeded, r.step = true, true, relicInspect
		r.moves++
		r.base, r.cards, r.scan, r.move, r.point = gameFrame{}, nil, 0, nil, image.Point{}
	}
}

func relicActionStable(a gameAction, current gameFrame) bool {
	if a.frame.context != current.context || a.frame.generation != current.generation || !current.context.known {
		return false
	}
	switch a.relic.step {
	case relicOpenTab:
		return current.context.heroes
	case relicConfirmSalvage:
		point, found, err := relicJunkControl(current.image, true)
		return current.context.relics && current.context.relicJunk && err == nil && found && point == a.point
	case relicOpenSalvage:
		return current.context.relics && !current.context.relicJunk && readRelicUI(current.image).known && relicInventoryStable(a.relic.before, current.image)
	case relicHover:
		return current.context.relics && readRelicUI(current.image).known && relicInventoryStable(a.relic.before, current.image)
	case relicPark, relicAcknowledge:
		// Only pointer movement: an owned tooltip may hide the panel anchors.
		return gameScreenVisible(current.context)
	case relicReturn:
		point, found, err := navigationControl(current, navigationHeroes)
		return gameScreenVisible(current.context) && err == nil && found && point == a.point
	case relicEquip:
		return current.context.relics && readRelicUI(current.image).known && relicInventoryStable(a.relic.before, current.image)
	}
	return false
}

func relicPixelsStable(before, after image.Image, region image.Rectangle) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	r := vision.Rect(before, region)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a, b, c := rgb(before.At(x, y))
			d, e, f := rgb(after.At(x, y))
			if absDiff(a, d) > 12 || absDiff(b, e) > 12 || absDiff(c, f) > 12 {
				return false
			}
		}
	}
	return true
}

func relicInventoryStable(before, after image.Image) bool {
	return relicPixelsStable(before, after, image.Rect(73, 265, 565, 334)) && relicPixelsStable(before, after, image.Rect(100, 390, 540, 690))
}
