package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func TestSaveSnapshotConsumersAndNotification(t *testing.T) {
	reward := &ancientcalc.AscensionState{PendingHeroSouls: "1000", SoulCapital: "10", CapitalKnown: true}
	relic := &ancientcalc.RelicPreview{Snapshot: ancientcalc.RelicSnapshot{Items: []ancientcalc.Relic{{UID: 1, Slot: 1}}}}
	var s saveSnapshot
	s.accept(exportResult{ascension: reward, relics: relic}, 110, 110)
	if !s.ascensionCurrent(110) || s.ascensionCurrent(115) || !s.relicsCurrent() {
		t.Fatal("accepted consumers unavailable")
	}
	s.rewardDirty = true
	if s.ascensionCurrent(110) || !s.relicsCurrent() {
		t.Fatal("reward mutation invalidated unrelated inventory")
	}
	s.relicAlertHandled = true
	s.accept(exportResult{ascension: reward, relics: relic}, 110, 110)
	if !s.relicAlertHandled {
		t.Fatal("unchanged export repeated the relic notification")
	}
	changed := &ancientcalc.RelicPreview{Snapshot: ancientcalc.RelicSnapshot{Items: []ancientcalc.Relic{{UID: 1, Slot: 1}, {UID: 2, Slot: 0}}}}
	s.accept(exportResult{ascension: reward, relics: changed}, 110, 110)
	if s.relicAlertHandled {
		t.Fatal("new inventory lost its notification")
	}
	s.accept(exportResult{ascensionErr: errors.New("unsupported")}, 110, 110)
	if s.ascensionCurrent(110) || s.relicsCurrent() {
		t.Fatal("new failed facts kept old consumer data")
	}
}

func TestSaveSnapshotPausePreservesFactsAndActionsInvalidate(t *testing.T) {
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	p.save.accept(exportResult{ascension: &ancientcalc.AscensionState{PendingHeroSouls: "100"}, relics: &ancientcalc.RelicPreview{}}, 110, 110)
	p.reset(p.generation + 1)
	if !p.save.ascensionCurrent(110) || !p.save.relicsCurrent() {
		t.Fatal("F8 erased unchanged facts")
	}
	p.actionCompleted(actionResult{action: gameAction{kind: buyHero}}, now)
	if p.save.ascensionCurrent(110) || !p.save.relicsCurrent() {
		t.Fatal("hero purchase did not invalidate reward only")
	}
	p.actionCompleted(actionResult{action: gameAction{kind: handleAncient, ancient: ancientCommand{step: confirmAncientQuantity}}}, now)
	if p.save.relicsCurrent() || p.gildRefreshDue.IsZero() {
		t.Fatal("Ancient spending retained stale consumers")
	}
}

func TestSaveRewardFailureBackoffDoesNotBlockGameplay(t *testing.T) {
	now := time.Now()
	for _, unknownCapital := range []bool{false, true} {
		p := firstAscensionPipeline(now)
		p.ascension.firstRun = false
		p.ascension.wallZone = 110
		p.ascension.fullCombatFailed = true
		p.frame.context.saveMenu = false
		p.export.active = true
		p.export.step = exportReadFile
		p.export.jobFrame = p.frame.id
		p.export.window = p.frame.context.window
		p.export.relicsOnly = true
		p.options.export = &saveExportOptions{}
		out := exportResult{relics: &ancientcalc.RelicPreview{}, ascensionErr: errors.New("missing reward")}
		if unknownCapital {
			out.ascension = &ancientcalc.AscensionState{PendingHeroSouls: "100", CapitalKnown: false}
			out.ascensionErr = nil
		}
		if err := p.applyObservation(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: out}, now); err != nil {
			t.Fatal(err)
		}
		if !p.ascension.nextRead.After(now) || !p.save.rewardDirty || p.export.requested {
			t.Fatal("failed save assessment can loop exports", p.ascension.nextRead, p.export)
		}
		p.planAscension(now.Add(time.Second))
		if p.export.requested || p.ascension.active {
			t.Fatal("backoff restarted unavailable reward assessment")
		}
	}
}

func TestSharedFreshRelicSnapshotServesPendingNotification(t *testing.T) {
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{}})
	p.export.requested = false
	p.startupCheck = false
	p.relic.notice = true
	p.save.relicsDirty = true
	p.save.accept(exportResult{relics: &ancientcalc.RelicPreview{Snapshot: ancientcalc.RelicSnapshot{EquipmentSlots: 4, Transcendent: true}}}, 0, 0)
	p.startRelics(now)
	if p.export.requested || p.relic.active {
		t.Fatal("fresh shared inventory forced another export")
	}
}

func TestFailedGildRefreshDoesNotLoopExports(t *testing.T) {
	now := time.Now()
	for _, invalidPlan := range []bool{false, true} {
		p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{}})
		p.gildRefreshDue = now
		out := exportResult{gildErr: errors.New("unsupported")}
		if invalidPlan {
			plan := gildMovePlan()
			out.gilds = &plan
		}
		p.acceptGildSave(out, gameFrame{}, now)
		if !p.gildRefreshDue.IsZero() || p.gildMove.active {
			t.Fatal("failed fresh distribution can repeat exports")
		}
	}
}
