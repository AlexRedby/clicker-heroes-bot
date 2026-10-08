package bot

import (
	"context"
	"image"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGildQueueClosesAndReadsOutcomeWithoutReplayingAncients(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	plan := gildMovePlan()
	var keys []string
	p := newGamePipeline(&pauseControl{}, heroInput{
		keyToggle: func(key, state string) error { keys = append(keys, key+" "+state); return nil },
		click:     func(image.Point) error { return nil },
	}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{dir: t.TempDir(), planOutput: filepath.Join(t.TempDir(), "plan.json")}})
	p.frame, p.layout = source, source.layout
	p.export.requested = false
	p.ancient = ancientPlanner{plan: &ancientPlan{}, finished: true}
	oldAncientPlan := p.ancient.plan
	p.acceptGildSave(exportResult{gilds: &plan, at: now}, source, now)
	if !p.planGildRedistribution(now) || !p.gildMoving {
		t.Fatal("eligible current distribution was not scheduled")
	}
	// Model an accepted analyzer result. Native reader tests separately bind a
	// complete name to its text; this regression covers queue/export ownership.
	now = now.Add(time.Second)
	roster := gildMoveFrame(t, now, 2, 2, true)
	p.frame, p.layout = roster, roster.layout
	region := image.Rect(206, 131, 269, 149)
	p.gildMove.observe(gildRedistributionObservation{frame: roster, roster: true, targetFound: true, targetID: plan.Target.ID, target: region.Min.Add(region.Size().Div(2)), targetRegion: region, close: image.Pt(1149, 39)})
	if !p.planGildRedistribution(now) {
		t.Fatal("transfer lost ownership")
	}
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleGildRedistribution || a.gildMove.action != gildTransferAll {
		t.Fatal("transfer dropped by modal queue guard", a, ok)
	}
	if acted, err := p.execute(ctx, a); !acted || err != nil {
		t.Fatal("transfer input", acted, err)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if strings.Join(keys, ",") != "q down,q up" || !p.gildMove.awaitingExport {
		t.Fatal("missing Q or fresh outcome", keys)
	}
	now = now.Add(time.Second)
	p.frame.id++
	p.frame.at = now
	p.gildMove.observe(gildRedistributionObservation{frame: p.frame, roster: true, close: image.Pt(1149, 39)})
	p.planGildRedistribution(now)
	a, ok = p.nextAction(now)
	if !ok || a.gildMove.action != gildCloseRoster {
		t.Fatal("did not close before export", a, ok)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	now = now.Add(time.Second)
	p.frame = gildMoveFrame(t, now, 4, 3, false)
	p.layout = 3
	p.gildMove.observe(gildRedistributionObservation{frame: p.frame})
	p.planGildRedistribution(now)
	if !p.export.requested || !p.export.gildsOnly || p.gildMoving {
		t.Fatal("outcome did not use the shared export owner")
	}
	plan.Eligible, plan.MoveGilds, plan.Target.Gilds = false, 0, 2
	plan.Heroes[0].Gilds, plan.Heroes[1].Gilds = 0, 2
	p.export.active, p.export.step, p.export.jobFrame, p.export.window = true, exportReadFile, p.frame.id, p.frame.context.window
	if err := p.accept(ctx, observation{kind: exportAnalysis, frame: p.frame, export: exportResult{at: now.Add(time.Millisecond), gilds: &plan}}, now); err != nil {
		t.Fatal(err)
	}
	if p.export.requested || p.gildMove.active || p.gildMoving || p.ancient.plan != oldAncientPlan || !p.ancient.finished {
		t.Fatal("gild outcome replayed Ancient allocation or stayed exclusive")
	}
	if p.planGildRedistribution(now.Add(time.Second)) {
		t.Fatal("all-on-target retried")
	}
}

func TestGildQueueRejectsOldGenerationAndLeavesUnexpectedDialogsToNavigation(t *testing.T) {
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout, p.export.requested = source, 1, false
	plan := gildMovePlan()
	p.acceptGildSave(exportResult{gilds: &plan, at: now}, source, now)
	p.gildMoving = true
	cmd := gildRedistributionCommand{action: gildScrollHeroes, frame: source, point: image.Pt(700, 1000), saveHash: plan.SaveHash}
	p.enqueue(gameAction{kind: handleGildRedistribution, frame: source, gildMove: cmd}, now)
	p.generation++
	if _, ok := p.nextAction(now); ok {
		t.Fatal("old generation survived the gild queue guard")
	}
	p.frame.context.heroes, p.frame.context.saveMenu = false, true
	if p.planGildRedistribution(now) || p.gildMoving || !p.gildMove.awaitingExport {
		t.Fatal("unexpected menu retained gild ownership")
	}
}

func TestAncientSpendingRequiresFreshGildBudget(t *testing.T) {
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout, p.export.requested = source, 1, false
	plan := gildMovePlan()
	p.acceptGildSave(exportResult{gilds: &plan, at: now}, source, now)
	p.actionCompleted(actionResult{action: gameAction{kind: handleAncient, frame: source, ancient: ancientCommand{step: confirmAncientQuantity}}}, now)
	p.ancient.active = false
	if !p.planGildRedistribution(now) || !p.export.requested || !p.export.gildsOnly || p.gildMoving {
		t.Fatal("transfer used the wallet from before Ancient spending")
	}
}

func TestEarnedGildsInvalidateThePriorDistribution(t *testing.T) {
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, gilds: true, gildInterval: time.Minute, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout, p.export.requested = source, 1, false
	plan := gildMovePlan()
	p.acceptGildSave(exportResult{gilds: &plan, at: now}, source, now)
	p.gild = gildCollector{active: true, deadline: now.Add(time.Minute)}
	p.planGilds(now)
	if p.gildMove.active || !p.gildRefreshDue.Equal(now.Add(30*time.Second)) {
		t.Fatal("newly earned gilds kept the old transfer count")
	}
}
