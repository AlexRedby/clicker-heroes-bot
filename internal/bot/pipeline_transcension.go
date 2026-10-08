package bot

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"time"

	"clicker-heroes-bot/internal/transcension"
)

type prestigePipeline struct {
	controller          *transcension.Controller
	summon              *transcension.SummonPlanner
	journal             *transcension.Journal
	snapshot            transcension.Snapshot
	latest              gameFrame
	outsider            outsiderObservation
	confirmation        transcensionObservation
	jobFrame            uint64
	nextRead, nextCheck time.Time
	message             string
}

func (t *prestigePipeline) report(message string) {
	if message != t.message {
		fmt.Println("Transcension:", message)
		t.message = message
	}
}
func (t *prestigePipeline) stage() transcension.Stage {
	if t.controller == nil {
		return transcension.Ordinary
	}
	return t.controller.Stage()
}
func (t *prestigePipeline) exclusive() bool {
	return t.controller != nil && t.controller.HoldsGameplay()
}
func (t *prestigePipeline) holdAncients() bool {
	return t.controller != nil && t.controller.HoldsAncientAllocation()
}
func (t *prestigePipeline) interrupt() {
	if t.controller != nil {
		t.controller.Interrupt()
	}
	t.jobFrame, t.nextRead = 0, time.Time{}
	t.outsider, t.confirmation = outsiderObservation{}, transcensionObservation{}
}
func (t *prestigePipeline) close() {
	if t.journal != nil {
		_ = t.journal.Close()
	}
}

// The export worker supplies parsed facts only. All journal and controller
// mutations stay with the shared queue owner, under the existing input lock.
func (p *gamePipeline) acceptPrestigeSave(ctx context.Context, out exportResult, now time.Time) error {
	if !p.options.transcension {
		return nil
	}
	t := &p.prestige
	if out.transcension == nil {
		if t.exclusive() {
			return fmt.Errorf("fresh Transcension export unavailable: %v", out.transcensionErr)
		}
		return nil
	}
	s := *out.transcension
	if t.controller == nil {
		if s.State.ProfileID == "" {
			t.report("save profile unavailable; continuing ordinary progression")
			return nil
		}
		policy := transcension.Policy{Enabled: true, SkillRate: p.options.export.skillRate, Beyond8k: p.options.export.beyond8k}
		native := nativeTranscensionEvidence(s.State.Build)
		path := filepath.Join(filepath.Dir(p.options.export.planOutput), "transcension-"+s.State.ProfileID+".json")
		journal, err := transcension.OpenJournal(path, s.State.ProfileID)
		if err != nil {
			return fmt.Errorf("open Transcension journal: %w", err)
		}
		controller := transcension.NewController(policy, native)
		if err := controller.BindJournal(journal); err != nil {
			journal.Close()
			return err
		}
		summon := transcension.NewSummonPlanner(policy, native)
		if err := summon.BindController(controller); err != nil {
			journal.Close()
			return err
		}
		t.controller, t.summon, t.journal = controller, summon, journal
	}
	if t.snapshot.State.ProfileID != "" && s.State.ProfileID != t.snapshot.State.ProfileID {
		return fmt.Errorf("Transcension save profile changed")
	}
	before := t.stage()
	switch before {
	case transcension.ReadyForAllocation:
		// Keep the allocator bound to the export accepted by the controller.
		return nil
	case transcension.Uncertain:
		if err := t.controller.Recover(ctx, now, s); err != nil {
			return err
		}
	case transcension.AwaitOutsiders, transcension.SpendOutsiders, transcension.AwaitResetReceipt, transcension.AwaitFeedReceipt, transcension.AwaitFirstSouls, transcension.AwaitAncients:
		if err := t.controller.AcceptExport(ctx, now, s); err != nil {
			return err
		}
	}
	t.outsider, t.confirmation = outsiderObservation{}, transcensionObservation{}
	if t.stage() != transcension.Ordinary && (t.snapshot.State.ProfileID == "" || s.State.Transcensions > t.snapshot.State.Transcensions) {
		p.ancient = ancientPlanner{}
		p.outsiderBase = &s.Preview
		p.relic = relicPlanner{}
		p.gild = gildCollector{}
		p.skill.reset()
		p.progression = progressionPlanner{}
		p.hero.interrupt()
		p.clickers = autoClickerPlanner{footerAttempted: !p.options.heroes}
		t.report("reset confirmed by fresh save; restoring Outsiders")
	}
	t.snapshot = s
	return nil
}

func (p *gamePipeline) requestPrestigeExport() {
	p.export.requested, p.export.prestigeOnly = true, true
	p.export.relicsOnly, p.export.goalsOnly = false, false
	p.queue = make(map[actionKind]gameAction)
}

func (p *gamePipeline) prestigeFrame(frame gameFrame, ui outsiderObservation, confirmation transcensionObservation) {
	t := &p.prestige
	if t.controller == nil {
		return
	}
	o, err := nativeTranscensionObservation(frame, ui, confirmation, t.snapshot.State)
	if err != nil {
		t.report(err.Error())
		return
	}
	t.latest = frame
	t.controller.Observe(o)
	t.outsider, t.confirmation = ui, confirmation
	if t.summon.Active() {
		t.summon.Observe(nativeAncientSummonObservation(frame))
	}
}

// A wall assessment reuses a recent ordinary/relic export. Request one export
// only when needed; no timer exports are added during normal zone climbing.
func (p *gamePipeline) considerTranscension(now time.Time) bool {
	t := &p.prestige
	if !p.options.transcension || t.controller == nil || t.stage() != transcension.Ordinary || p.startup != noStartup || p.startupCheck || p.export.requested || p.relic.active || p.ancient.active || p.ascension.active || p.mercenary.active || p.hero.pending != nil || p.skill.pending != nil || p.progression.pending != nil || p.progression.wantAction || !p.frame.context.heroes || !p.ascension.due(now, p.options.ascensionStall) || now.Before(t.nextCheck) {
		return false
	}
	if t.snapshot.Preview.EstimatedASGain == nil || *t.snapshot.Preview.EstimatedASGain <= 0 {
		t.nextCheck = now.Add(time.Minute)
		return false
	}
	if now.Sub(t.snapshot.ExportedAt) > 25*time.Second || t.snapshot.Generation != p.generation {
		p.requestPrestigeExport()
		return true
	}
	timing := transcension.Timing{At: now, Generation: p.generation, SaveHash: t.snapshot.State.SaveHash, WallConfirmed: p.progression.wallFullCombat}
	if err := t.controller.Begin(context.Background(), now, t.snapshot, timing); err != nil {
		t.report(err.Error())
		t.nextCheck = now.Add(time.Minute)
		return false
	}
	p.ancient = ancientPlanner{}
	p.queue = make(map[actionKind]gameAction)
	t.report("combat wall and useful reward established; opening Outsiders")
	return true
}

func (p *gamePipeline) prestigeAction(cmd transcension.Command) (gameAction, bool) {
	f := p.frame
	if p.prestige.latest.id == cmd.Frame {
		f = p.prestige.latest
	}
	a := gameAction{kind: handleTranscension, frame: f, prestige: cmd}
	if cmd.Frame != f.id || cmd.Generation != f.generation || cmd.Layout != f.layout {
		return a, false
	}
	var found bool
	var err error
	switch cmd.Action {
	case transcension.OpenOutsiders:
		if !bootstrapHeroes(f.context) {
			return a, false
		}
		a.point, found = outsiderEntryPoint(f.image)
	case transcension.OpenReset:
		a.point, found, err = transcensionControl(f.image, transcensionOpen)
	case transcension.ConfirmReset:
		a.point, found, err = transcensionControl(f.image, transcensionYes)
	case transcension.SelectQuantity:
		if p.prestige.outsider.frame.id == f.id {
			a.point, found = outsiderQuantityPoint(p.prestige.outsider, cmd.Quantity)
		}
	case transcension.FeedOutsider:
		if p.prestige.outsider.frame.id == f.id {
			a.point, found = outsiderFeedPoint(p.prestige.outsider, cmd.Name)
		}
	case transcension.ScrollOutsidersTop, transcension.ScrollOutsidersDown:
		if !f.context.outsiders {
			return a, false
		}
		a.point, _, found = listScrollbarThumb(f.image, 417)
		a.target = image.Pt(a.point.X, f.context.bounds.Min.Y+f.context.bounds.Dy()*455/1000)
		if cmd.Action == transcension.ScrollOutsidersDown {
			a.target.Y = min(f.context.bounds.Max.Y-1, a.point.Y+f.context.bounds.Dy()*24/100)
		}
	default:
		return a, false
	}
	return a, found && err == nil
}

func (p *gamePipeline) prestigeActionStable(a gameAction, now time.Time) bool {
	if p.prestige.controller == nil || a.frame.context != p.frame.context || a.frame.generation != p.frame.generation || a.frame.layout != p.frame.layout || !p.prestige.controller.Allowed(now, a.prestige) {
		return false
	}
	current, found := p.prestigeAction(a.prestige)
	if !found || current.point != a.point || current.target != a.target {
		return false
	}
	switch a.prestige.Action {
	case transcension.OpenOutsiders:
		_, found := outsiderEntryPoint(p.frame.image)
		return found && bootstrapHeroes(p.frame.context)
	case transcension.SelectQuantity:
		return outsiderQuantityControlsKnown(p.frame.image) && outsiderTabSelected(p.frame.image) && relicPixelsStable(a.frame.image, p.frame.image, image.Rect(110, 253, 600, 285))
	case transcension.ConfirmReset:
		for _, which := range []int{transcensionYes, transcensionNo, transcensionUnchecked} {
			if _, found, err := transcensionControl(p.frame.image, which); err != nil || !found {
				return false
			}
		}
		return relicPixelsStable(a.frame.image, p.frame.image, image.Rect(485, 99, 930, 220)) && relicPixelsStable(a.frame.image, p.frame.image, image.Rect(529, 585, 568, 620))
	case transcension.FeedOutsider:
		return slices.Equal(outsiderCardTops(a.frame.image), outsiderCardTops(p.frame.image)) && relicPixelsStable(a.frame.image, p.frame.image, image.Rect(80, 300, 540, 718)) && relicPixelsStable(a.frame.image, p.frame.image, image.Rect(420, 180, 605, 198)) && outsiderQuantity(a.frame.image) == outsiderQuantity(p.frame.image)
	case transcension.OpenReset:
		_, ok, err := transcensionControl(p.frame.image, transcensionOpen)
		return ok && err == nil
	}
	return true
}

func (p *gamePipeline) unsupportedSummon(now time.Time) {
	path := fmt.Sprintf("artifacts/transcension-summon-%s.png", now.Format("20060102-150405.000"))
	if err := saveImage(path, p.frame.image); err != nil {
		p.prestige.report("summon diagnostic failed: " + err.Error())
	} else {
		fmt.Println("saved unsupported Ancient summon screenshot:", path)
	}
	p.controls.pause("Transcension reached an unsupported Ancient summon screen; send the saved screenshot to add its controls")
}

func (p *gamePipeline) planTranscension(now time.Time) bool {
	t := &p.prestige
	if t.controller == nil {
		return p.considerTranscension(now)
	}
	if t.stage() == transcension.Ordinary {
		return p.considerTranscension(now)
	}
	if p.export.requested {
		if t.stage() == transcension.AwaitResetReceipt || t.stage() == transcension.AwaitFeedReceipt {
			t.controller.Next(now) // Retain the input deadline while waiting for a covered export entry.
			if t.stage() == transcension.Uncertain {
				return p.planNavigation(now)
			}
		}
		return p.planExport(now)
	}
	if t.stage() == transcension.Uncertain {
		p.requestPrestigeExport()
		return p.planExport(now)
	}
	if t.stage() == transcension.AwaitFirstSouls {
		return false
	}
	if t.stage() == transcension.ReadyForAllocation {
		if p.ancient.finished {
			if err := t.controller.CompleteAllocation(t.snapshot.State.SaveHash); err != nil {
				t.report(err.Error())
			} else {
				t.report("Ancients restored; ordinary progression resumed")
			}
		}
		return false
	}
	if t.stage() == transcension.AwaitAncients {
		if !t.summon.Active() {
			if err := t.summon.Begin(context.Background(), now, t.snapshot); err != nil {
				t.report(err.Error())
				p.requestPrestigeExport()
				return true
			}
		}
		t.summon.Observe(nativeAncientSummonObservation(p.frame))
		if t.summon.NeedsMoreSouls(now) {
			if err := t.summon.ContinueEarning(now); err != nil {
				t.report(err.Error())
				return true
			}
			if err := t.controller.ContinueEarning(); err != nil {
				t.report(err.Error())
				return true
			}
			t.report("required Ancients are unaffordable; continuing hero progression and Ascension")
			return false
		}
		cmd := t.summon.Next(now)
		if cmd.Action == transcension.ExportSummonReceipt {
			if err := t.summon.Reserve(now, cmd); err != nil {
				t.report(err.Error())
				return true
			}
			p.requestPrestigeExport()
			return p.planExport(now)
		}
		if a, found := nativeAncientSummonAction(cmd, p.frame); found {
			a.kind, a.summon = handleSummon, cmd
			p.enqueue(a, now)
		} else if p.frame.context.ancients && (cmd.Action == transcension.OpenSummonOffers || cmd.Action == transcension.NoSummonAction) {
			p.unsupportedSummon(now)
		}
		return true
	}
	cmd := t.controller.Next(now)
	switch cmd.Action {
	case transcension.FreshExport:
		if err := t.controller.Reserve(now, cmd); err != nil {
			t.report(err.Error())
			return true
		}
		p.requestPrestigeExport()
		return p.planExport(now)
	case transcension.BootstrapHeroes:
		if err := t.controller.Reserve(now, cmd); err != nil {
			t.report(err.Error())
			return true
		}
		p.ancient = ancientPlanner{}
		p.beginStartup()
		t.report("Outsiders restored; starting heroes and first Hero Souls")
	default:
		if a, found := p.prestigeAction(cmd); found {
			p.enqueue(a, now)
		}
	}
	return true
}
