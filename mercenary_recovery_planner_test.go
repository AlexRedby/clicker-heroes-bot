package main

import (
	"image"
	"testing"
	"time"
)

func recoveryRosterObservation(id uint64, at time.Time, c gameContext, card mercenaryDeadCard) mercenaryObservation {
	return mercenaryObservation{
		frame:     mercenaryPlannerFrame(id, at, c),
		readable:  true,
		top:       true,
		bottom:    true,
		deadCards: []mercenaryDeadCard{card},
	}
}

func recoveryPromptObservation(id uint64, at time.Time, c gameContext, method mercenaryRecoveryMethod, budget mercenaryRecoveryBudget) mercenaryObservation {
	return mercenaryObservation{
		frame:    mercenaryPlannerFrame(id, at, c),
		readable: true,
		recovery: &mercenaryRecoveryPrompt{
			method: method,
			budget: budget,
			yes:    image.Pt(406, 548),
			no:     image.Pt(594, 548),
		},
	}
}

func recoveryPlannerContexts() (gameContext, gameContext) {
	bounds := image.Rect(0, 0, 1000, 1000)
	return gameContext{known: true, mercenaries: true, bounds: bounds, window: "game"},
		gameContext{known: true, mercenaries: true, mercenaryDialog: true, bounds: bounds, window: "game"}
}

func TestMercenaryRecoveryBuryConfirmsOnceAndRebuildsRoster(t *testing.T) {
	now := time.Now()
	rosterContext, dialogContext := recoveryPlannerContexts()
	card := mercenaryDeadCard{
		row:    image.Pt(700, 400),
		traits: mercenaryRecoveryTraits{Known: true, Level: 10, RubyBonusPercent: 8},
		revive: image.Pt(740, 440),
		bury:   image.Pt(790, 440),
	}
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	p.observe(recoveryRosterObservation(1, now, rosterContext, card), now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != openMercenaryRecovery || a.mercenary.recovery != mercenaryRecoveryBury || a.point != card.bury || a.target != card.row {
		t.Fatalf("wanted Bury prompt, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	now = now.Add(time.Second)
	p.observe(recoveryPromptObservation(2, now, dialogContext, mercenaryRecoveryBury, mercenaryRecoveryBudget{}), now)
	a, ok = p.action(now)
	if !ok || a.mercenary.step != confirmMercenaryRecovery || a.mercenary.recovery != mercenaryRecoveryBury {
		t.Fatalf("wanted one Bury confirmation, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	if !p.recoverySubmitted {
		t.Fatal("Bury confirmation was not recorded")
	}
	if _, ok := p.action(now.Add(time.Millisecond)); ok {
		t.Fatal("Bury confirmation was repeated while pending")
	}

	// Closure invalidates the old cached plan; captured() first installs an
	// unreadable placeholder, then the readable result with the same frame id
	// must be accepted as the fresh roster read.
	now = now.Add(time.Second)
	frame := mercenaryPlannerFrame(3, now, rosterContext)
	p.captured(frame, now)
	if p.pending != nil || p.recoveryRow != nil || p.recoverySubmitted {
		t.Fatalf("confirmed Bury did not invalidate recovery plan: %+v", p)
	}
	if !p.needsRead(rosterContext) {
		t.Fatal("confirmed Bury did not require a fresh roster read")
	}
	newCard := card
	newCard.row = image.Pt(700, 550)
	fresh := recoveryRosterObservation(3, now, rosterContext, newCard)
	p.observe(fresh, now)
	if p.roster == nil || len(p.roster.deadCards) != 1 || p.roster.deadCards[0].row != newCard.row {
		t.Fatalf("fresh roster was not accepted after Bury: %+v", p.roster)
	}
}

func TestMercenaryRecoveryPaidReviveConfirmsOnce(t *testing.T) {
	now := time.Now()
	rosterContext, dialogContext := recoveryPlannerContexts()
	card := mercenaryDeadCard{row: image.Pt(700, 400), traits: mercenaryRecoveryTraits{Known: true, Level: 2}, revive: image.Pt(740, 440), bury: image.Pt(790, 440)}
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	p.observe(recoveryRosterObservation(1, now, rosterContext, card), now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != openMercenaryRecovery || a.mercenary.recovery != mercenaryRecoveryRubies || a.point != card.revive {
		t.Fatalf("wanted paid Revive prompt, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	now = now.Add(time.Second)
	p.observe(recoveryPromptObservation(2, now, dialogContext, mercenaryRecoveryRubies, mercenaryRecoveryBudget{Known: true, Cost: 100, Rubies: 100}), now)
	a, ok = p.action(now)
	if !ok || a.mercenary.step != confirmMercenaryRecovery || a.mercenary.recovery != mercenaryRecoveryRubies {
		t.Fatalf("wanted paid Revive confirmation, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	if _, ok := p.action(now.Add(time.Millisecond)); ok {
		t.Fatal("paid Revive confirmation was repeated")
	}
}

func TestMercenaryRecoveryBudgetFailureClicksNo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget mercenaryRecoveryBudget
	}{
		{"unreadable budget", mercenaryRecoveryBudget{}},
		{"insufficient funds", mercenaryRecoveryBudget{Known: true, Cost: 100, Rubies: 99}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			rosterContext, dialogContext := recoveryPlannerContexts()
			card := mercenaryDeadCard{row: image.Pt(700, 400), traits: mercenaryRecoveryTraits{Known: true, Level: 2}, revive: image.Pt(740, 440), bury: image.Pt(790, 440)}
			p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
			p.observe(recoveryRosterObservation(1, now, rosterContext, card), now)
			open, ok := p.action(now)
			if !ok || open.mercenary.step != openMercenaryRecovery {
				t.Fatalf("wanted recovery prompt, got %+v ok=%t", open, ok)
			}
			p.sent(open, now)
			now = now.Add(time.Second)
			p.observe(recoveryPromptObservation(2, now, dialogContext, mercenaryRecoveryRubies, tc.budget), now)
			no, ok := p.action(now)
			if !ok || no.mercenary.step != cancelMercenaryRecovery || no.point != image.Pt(594, 548) {
				t.Fatalf("wanted safe No, got %+v ok=%t", no, ok)
			}
			if no.mercenary.step == confirmMercenaryRecovery || p.recoverySubmitted {
				t.Fatal("unsafe recovery confirmation was scheduled")
			}
		})
	}
}

func TestMercenaryRecoveryExtraLifeIsUnsupported(t *testing.T) {
	now := time.Now()
	rosterContext, _ := recoveryPlannerContexts()
	card := mercenaryDeadCard{row: image.Pt(700, 400), traits: mercenaryRecoveryTraits{Known: true, Level: 11, ExtraLives: 1}, revive: image.Pt(740, 440), bury: image.Pt(790, 440)}
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	p.observe(recoveryRosterObservation(1, now, rosterContext, card), now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != returnToHeroes {
		t.Fatalf("unsupported extra-life recovery must not open a control, got %+v ok=%t", a, ok)
	}
}

func TestMercenaryRecoveryF8OrphanDialogAndTimeoutClickNo(t *testing.T) {
	now := time.Now()
	rosterContext, dialogContext := recoveryPlannerContexts()
	card := mercenaryDeadCard{row: image.Pt(700, 400), traits: mercenaryRecoveryTraits{Known: true, Level: 10, RubyBonusPercent: 8}, revive: image.Pt(740, 440), bury: image.Pt(790, 440)}
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	p.observe(recoveryRosterObservation(1, now, rosterContext, card), now)
	open, ok := p.action(now)
	if !ok || open.mercenary.step != openMercenaryRecovery {
		t.Fatalf("wanted recovery prompt, got %+v ok=%t", open, ok)
	}
	p.sent(open, now)
	p.interrupt()
	p.observe(recoveryPromptObservation(2, now.Add(time.Second), dialogContext, mercenaryRecoveryBury, mercenaryRecoveryBudget{}), now.Add(time.Second))
	no, ok := p.action(now.Add(time.Second))
	if !ok || no.mercenary.step != cancelMercenaryRecovery || no.point != image.Pt(594, 548) {
		t.Fatalf("F8 orphan dialog did not schedule safe No: %+v ok=%t", no, ok)
	}

	// A missed confirmation expires into No and never emits a second Yes.
	p = mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	p.observe(recoveryRosterObservation(3, now, rosterContext, card), now)
	open, _ = p.action(now)
	p.sent(open, now)
	p.observe(recoveryPromptObservation(4, now.Add(time.Second), dialogContext, mercenaryRecoveryBury, mercenaryRecoveryBudget{}), now.Add(time.Second))
	yes, ok := p.action(now.Add(time.Second))
	if !ok || yes.mercenary.step != confirmMercenaryRecovery {
		t.Fatalf("wanted initial confirmation, got %+v ok=%t", yes, ok)
	}
	p.sent(yes, now.Add(time.Second))
	late := now.Add(7 * time.Second)
	// captured() supplies the cached prompt after the timeout; repeated captures
	// must not create another confirmation or re-read a new modal decision.
	p.captured(mercenaryPlannerFrame(5, late, dialogContext), late)
	p.captured(mercenaryPlannerFrame(5, late, dialogContext), late)
	no, ok = p.action(late)
	if !ok || no.mercenary.step != cancelMercenaryRecovery || no.point != image.Pt(594, 548) {
		t.Fatalf("missed confirmation did not fall back to No: %+v ok=%t", no, ok)
	}
	if no.mercenary.step == confirmMercenaryRecovery {
		t.Fatal("missed confirmation repeated Yes")
	}
}

func TestMercenaryRecoveryCancelKeepsLivingPlan(t *testing.T) {
	now := time.Now()
	rosterContext, dialogContext := recoveryPlannerContexts()
	living := image.Pt(700, 550)
	card := mercenaryDeadCard{row: image.Pt(700, 400), traits: mercenaryRecoveryTraits{Known: true, Level: 2}, revive: image.Pt(740, 440), bury: image.Pt(790, 440)}
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	o := recoveryRosterObservation(1, now, rosterContext, card)
	o.collect = nil
	p.observe(o, now)
	// The cached roster contains a living row alongside the dead card, while
	// latest deliberately has no living action so recovery is attempted first.
	p.roster.collect = []image.Point{living}
	open, ok := p.action(now)
	if !ok || open.mercenary.step != openMercenaryRecovery {
		t.Fatalf("wanted recovery prompt, got %+v ok=%t", open, ok)
	}
	p.sent(open, now)
	now = now.Add(time.Second)
	p.observe(recoveryPromptObservation(2, now, dialogContext, mercenaryRecoveryRubies, mercenaryRecoveryBudget{}), now)
	no, ok := p.action(now)
	if !ok || no.mercenary.step != cancelMercenaryRecovery {
		t.Fatalf("wanted safe No for unreadable budget, got %+v ok=%t", no, ok)
	}
	p.sent(no, now)
	now = now.Add(time.Second)
	main := mercenaryObservation{frame: mercenaryPlannerFrame(3, now, rosterContext), readable: true, top: true, bottom: true, collect: []image.Point{living}}
	p.observe(main, now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != claimAndOpenMercenaryQuest || a.point != living {
		t.Fatalf("cancelled recovery did not continue living plan: %+v ok=%t", a, ok)
	}
	if a.point == card.row || len(p.roster.deadCards) != 0 {
		t.Fatalf("cancelled dead card was retried: action=%+v roster=%+v", a, p.roster)
	}
}
