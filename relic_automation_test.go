package main

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func relicEquipmentFixture() ancientcalc.RelicSnapshot {
	s := ancientcalc.RelicSnapshot{EquipmentSlots: 4, Ascensions: 3}
	for i := 1; i <= 4; i++ {
		kind, level := 2, "9999"
		if i == 1 {
			kind, level = 15, "100"
		}
		s.Items = append(s.Items, ancientcalc.Relic{UID: i, Slot: i, Level: "100", Rarity: 1, Bonuses: []ancientcalc.RelicBonus{{Type: kind, Level: level}}})
	}
	s.Items = append(s.Items, ancientcalc.Relic{UID: 99, Slot: 5, Level: "525", Rarity: 1, Bonuses: []ancientcalc.RelicBonus{{Type: 15, Level: "470"}, {Type: 17, Level: "4.23"}, {Type: 9, Level: "20"}, {Type: 21, Level: "1"}}},
		ancientcalc.Relic{UID: 100, Slot: 6, Level: "20", Rarity: 1, Bonuses: []ancientcalc.RelicBonus{{Type: 2, Level: "1"}}},
		ancientcalc.Relic{UID: 101, Slot: 7, Level: "20", Rarity: 2, Bonuses: []ancientcalc.RelicBonus{{Type: 3, Level: "1"}}})
	return s
}

func TestRelicEquipmentThroughSharedQueue(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	screen := loadTestImage(t, "testdata/relic-inventory.png")
	clicks, drags, moves := 0, 0, 0
	input := heroInput{capture: func() (image.Image, error) { return screen, nil }, click: func(image.Point) error { clicks++; return nil }, move: func(image.Point) error { moves++; return nil }, drag: func(from, to image.Point) error {
		drags++
		if from != image.Pt(279, 852) || to != image.Pt(220, 597) {
			t.Fatalf("wrong drag %v -> %v", from, to)
		}
		return nil
	}}
	p := newGamePipeline(&pauseControl{}, input, pipelineReaders{context: recognizedGame}, pipelineOptions{progression: true, export: &saveExportOptions{}, fishInterval: time.Second})
	p.frame = gameFrame{id: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds(), window: "game"}}
	p.startRelics(now)
	p.export.requested = false
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: relicEquipmentFixture()}, now); err != nil {
		t.Fatal(err)
	}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	p.readers.window = func() string { return "game" }
	act := func(step relicStep) {
		t.Helper()
		now = now.Add(time.Second)
		p.settleUntil = time.Time{}
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != handleRelic || a.relic.step != step {
			t.Fatalf("action=%+v ok=%v want step=%v", a, ok, step)
		}
		acted, err := p.execute(ctx, a)
		if err != nil || !acted {
			t.Fatal("input rejected", acted, err)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, now)
	}
	read := func(path string) {
		t.Helper()
		screen = loadTestImage(t, path)
		now = now.Add(time.Second)
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
		select {
		case job := <-jobs[relicAnalysis]:
			if err := p.accept(ctx, p.analyze(ctx, relicAnalysis, job), now); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("missing shared relic analysis")
		}
	}
	act(relicOpenTab)
	read("testdata/relic-inventory.png")
	act(relicHover)
	read("testdata/relic-tooltip.png")
	act(relicPark)
	read("testdata/relic-inventory.png")
	act(relicEquip)
	if drags != 1 || clicks != 1 || moves != 3 {
		t.Fatal("card was clicked or extra input", drags, clicks, moves)
	}
	if p.ascension.relicsChecked {
		t.Fatal("unverified drag authorized Ascension")
	}
	// The native input is modeled here; the final export proves whether it landed.
	after := p.relic.snapshot
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: after}, now); err == nil {
		t.Fatal("unexpected early export accepted")
	}
	p.relic.step = relicVerifyEquip
	p.progression.wallZone = 100
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: after}, now); err != nil {
		t.Fatal(err)
	}
	if p.relic.active || p.ascension.relicsChecked || p.progression.wallZone != 0 {
		t.Fatal("verified upgrade did not restart combat")
	}
}

func TestRelicEquipmentRejectsMissedDragAndAmbiguousTooltip(t *testing.T) {
	requireAncientOCR(t)
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true})
	s := relicEquipmentFixture()
	p.relic = relicPlanner{active: true, step: relicAcquire}
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: s}, now); err != nil {
		t.Fatal(err)
	}
	p.relic.move = &ancientcalc.RelicSuggestion{UID: 99, Slot: 1, ReplaceUID: 1}
	p.relic.sent(gameAction{frame: gameFrame{id: 1}, relic: relicCommand{step: relicEquip}}, now)
	p.relic.step = relicVerifyEquip
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: s}, now); err == nil {
		t.Fatal("missed drag accepted")
	}
	p.relicFailed("missed drag", now)
	if p.controls.paused || p.ascension.relicsChecked || !now.Before(p.relic.nextCheck) {
		t.Fatal("failure paused play or released gate")
	}
	tooltip := loadTestImage(t, "testdata/relic-tooltip.png")
	f := gameFrame{image: tooltip, context: gameContext{relics: true}}
	out, err := readRelicObservation(context.Background(), f, &s, true)
	if err != nil || out.uid != 99 {
		t.Fatal("native tooltip", out.uid, err)
	}
	duplicate := s.Items[4]
	duplicate.UID = 102
	s.Items = append(s.Items, duplicate)
	if _, err := readRelicObservation(context.Background(), f, &s, true); err == nil {
		t.Fatal("duplicate identity accepted")
	}
	f.image = loadTestImage(t, "testdata/relic-upgrade.png")
	if _, err := readRelicObservation(context.Background(), f, &s, true); err == nil {
		t.Fatal("upgrade modal accepted as equipment")
	}
}

func TestRelicEquipmentUnknownMappingReturnsAndDefers(t *testing.T) {
	now := time.Now()
	s := relicEquipmentFixture()
	r := relicPlanner{active: true, step: relicInspect, snapshot: s}
	f := gameFrame{id: 2, image: loadTestImage(t, "testdata/relic-inventory.png")}
	ui := readRelicUI(f.image)
	if err := r.observe(relicObservation{frame: f, ui: ui}, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(ui.junk); i++ {
		r.step = relicHover
		if err := r.observe(relicObservation{frame: f}, errors.New("unknown")); err != nil {
			t.Fatal(err)
		}
		r.step = relicInspect
		err := r.observe(relicObservation{frame: f, ui: ui}, nil)
		if (i == len(ui.junk)-1) != (err != nil) {
			t.Fatal("scan was not bounded", i, err)
		}
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true})
	p.relic = r
	p.relicFailed("ambiguous mapping", now)
	p.frame = gameFrame{id: 3, context: gameContext{known: true, heroes: true}}
	p.planRelics(now)
	if p.relic.active || p.controls.paused || p.ascension.relicsChecked || !now.Before(p.ascension.nextCheck) {
		t.Fatal("unknown inventory didn't continue ordinary play with a gate")
	}
}

func TestRelicEquipmentNoCandidateDoesNotVisitAndPauseRevokes(t *testing.T) {
	now := time.Now()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	s := relicEquipmentFixture()
	s.Items = s.Items[:4]
	p.relic = relicPlanner{active: true, step: relicAcquire}
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: s}, now); err != nil {
		t.Fatal(err)
	}
	if p.relic.active || !p.ascension.relicsChecked || len(p.queue) != 0 {
		t.Fatal("no candidate opened inventory")
	}
	p.relic = relicPlanner{active: true, snapshot: s, step: relicEquip}
	p.queue[handleRelic] = gameAction{kind: handleRelic}
	p.reset(p.generation + 1)
	if p.relic.active || p.ascension.relicsChecked || len(p.queue) != 0 {
		t.Fatal("pause retained equipment ownership")
	}
	if !reflect.DeepEqual(s.Items, relicEquipmentFixture().Items[:4]) {
		t.Fatal("save snapshot mutated")
	}
}

func TestRelicEquipmentInitialSavePrecedesStartupAndCachedAncients(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	screen := loadTestImage(t, "testdata/hero-economy-x1.png")
	c := gameContext{known: true, heroes: true, window: "game", bounds: screen.Bounds()}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }}, pipelineOptions{heroes: true, progression: true, export: &saveExportOptions{planOutput: filepath.Join(t.TempDir(), "plan.json")}, fishInterval: time.Second})
	p.startupCheck, p.startup = false, startupSave
	p.frame = gameFrame{id: 2, image: screen, context: c, at: now}
	p.export = saveExporter{requested: true, active: true, initialSetup: true, step: exportReadFile, jobFrame: 2, window: "game"}
	plan := &ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Atman"}}}}
	if err := p.accept(ctx, observation{kind: exportAnalysis, frame: p.frame, export: exportResult{plan: plan, relics: &ancientcalc.RelicPreview{Snapshot: relicEquipmentFixture()}}}, now); err != nil {
		t.Fatal(err)
	}
	if !p.relic.active || p.export.requested || p.ancient.plan != plan || p.ascension.relicsChecked {
		t.Fatal("initial snapshot was lost or requested twice")
	}
	p.plan(now.Add(time.Second))
	a, ok := p.nextAction(now.Add(time.Second))
	if !ok || a.kind != handleRelic || a.relic.step != relicOpenTab {
		t.Fatal("startup/Ancient plan blocked initial relic input", a, ok)
	}
	screen = loadTestImage(t, "testdata/relic-inventory.png")
	c.heroes, c.relics, c.bounds = false, true, screen.Bounds()
	p.relic.step, p.relic.lastInput = relicInspect, p.frame.id
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(ctx, now.Add(2*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[relicAnalysis]) != 1 || len(jobs[heroAnalysis]) != 0 {
		t.Fatal("startup took capture ownership from relics")
	}
	// Outcome export also owns input/read scheduling before unfinished startup.
	c.heroes, c.relics = true, false
	p.relic.step = relicVerifyEquip
	p.export = saveExporter{requested: true, relicsOnly: true}
	p.frame.context = c
	p.plan(now.Add(3 * time.Second))
	a, ok = p.nextAction(now.Add(3 * time.Second))
	if !ok || a.kind != handleExport {
		t.Fatal("cached Ancient plan blocked batch outcome export", a, ok)
	}
	p.export.step, p.export.jobFrame, p.export.deadline = exportReadFile, 0, now.Add(time.Minute)
	if err := p.capture(ctx, now.Add(4*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[exportAnalysis]) != 1 {
		t.Fatal("startup blocked fresh outcome read")
	}
}

func TestRelicNotificationRequiresTwoCapturesAndReadonlyDoesNotEquip(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/relic-notification.png")
	c := gameContext{known: true, heroes: true, window: "game", bounds: screen.Bounds()}
	for _, progression := range []bool{false, true} {
		p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }}, pipelineOptions{progression: progression, export: &saveExportOptions{}, fishInterval: time.Second})
		p.export.requested = false
		jobs := make([]chan analysisJob, analysisCount)
		for i := range jobs {
			jobs[i] = make(chan analysisJob, 1)
		}
		if err := p.capture(context.Background(), now, jobs); err != nil {
			t.Fatal(err)
		}
		if p.relic.notice {
			t.Fatal("single notification capture triggered a visit")
		}
		if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
			t.Fatal(err)
		}
		if p.relic.notice != progression {
			t.Fatal("notification changed readonly behavior")
		}
		p.plan(now.Add(2 * time.Second))
		if p.relic.active != progression {
			t.Fatal("notification wasn't integrated into progression")
		}
	}
}

func TestRelicReturnDeadlineAndResumeRecoverHeroes(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/relic-inventory.png")
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true})
	p.frame = gameFrame{id: 4, image: screen, at: now, context: gameContext{known: true, relics: true, window: "game", bounds: screen.Bounds()}}
	p.relic = relicPlanner{active: true, failed: true, step: relicReturn, window: "game", deadline: now.Add(-time.Second), returnAttempts: 2}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleRelic || a.relic.step != relicReturn || !p.relic.active {
		t.Fatal("missed return became inactive/stranded", a, ok)
	}
	p.reset(p.generation + 1)
	p.startup = startupHeroes
	p.ancient.plan = &ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Atman"}}}}
	p.frame.generation, p.frame.layout = p.generation, p.layout
	p.plan(now.Add(time.Second))
	p.plan(now.Add(2 * time.Second))
	a, ok = p.nextAction(now.Add(2 * time.Second))
	if !ok || a.kind != handleRelic || a.relic.step != relicReturn {
		t.Fatal("resume stranded Relics", a, ok)
	}
	covered := loadTestImage(t, "testdata/relic-upgrade.png")
	p.frame.image, p.frame.context.known = covered, false
	p.plan(now.Add(3 * time.Second))
	if _, ok := p.nextAction(now.Add(3 * time.Second)); ok {
		t.Fatal("covering upgrade received input")
	}
	p.frame.image, p.frame.context.known = screen, true
	p.frame.at = now.Add(4 * time.Second)
	p.plan(now.Add(4 * time.Second))
	if _, ok := p.nextAction(now.Add(4 * time.Second)); !ok {
		t.Fatal("unobscured resumed panel did not recover")
	}
	p.frame.context.heroes, p.frame.context.relics = true, false
	p.plan(now.Add(5 * time.Second))
	if p.relic.active || p.ascension.relicsChecked {
		t.Fatal("recovery authorized Ascension or retained ownership")
	}
}

func TestRelicExportPauseRestartsEquipmentOwnership(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-economy-x1.png")
	c := gameContext{known: true, heroes: true, window: "game", bounds: screen.Bounds()}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, export: &saveExportOptions{}})
	p.frame = gameFrame{id: 1, image: screen, context: c, at: now}
	p.startRelics(now)
	p.relic.step = relicVerifyEquip
	p.export.active, p.export.step, p.export.window = true, exportReadFile, "game"
	p.reset(p.generation + 1)
	if !p.relic.active || p.relic.step != relicAcquire || !p.export.requested || p.ascension.relicsChecked {
		t.Fatal("pause left an orphaned advisory export")
	}
	p.frame.generation, p.frame.layout = p.generation, p.layout
	p.controls.generation = p.generation
	p.export.active, p.export.step, p.export.jobFrame = true, exportReadFile, p.frame.id
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: exportResult{relics: &ancientcalc.RelicPreview{Snapshot: relicEquipmentFixture()}}}, now); err != nil {
		t.Fatal(err)
	}
	if p.ascension.relicsChecked || !p.relic.active || p.relic.step != relicOpenTab {
		t.Fatal("retry bypassed equipment preflight")
	}
}

func TestRelicFishPriorityRejectsOldTooltip(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/relic-inventory.png")
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true, fishInterval: time.Second})
	p.frame = gameFrame{id: 5, image: screen, at: now, context: gameContext{known: true, relics: true, bounds: screen.Bounds()}}
	s := relicEquipmentFixture()
	p.relic = relicPlanner{active: true, step: relicHover, snapshot: s, move: &ancientcalc.RelicSuggestion{UID: 99}, cards: readRelicUI(screen).junk, lastInput: 2}
	point := image.Pt(2000, 600)
	p.fishTarget = &point
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("Relics suppressed cached fish")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	old := gameFrame{id: 4, image: screen, at: now, context: p.frame.context}
	if err := p.accept(context.Background(), observation{kind: relicAnalysis, frame: old, relic: relicObservation{frame: old, uid: 99}}, now); err != nil {
		t.Fatal(err)
	}
	if p.relic.point != (image.Point{}) || p.relic.failed || p.relic.step != relicInspect {
		t.Fatal("old tooltip survived fish input")
	}
}

func TestRelicBatchCapPreservesFurtherUpgradeBeforeAscension(t *testing.T) {
	now := time.Now()
	s := relicEquipmentFixture()
	screen := loadTestImage(t, "testdata/relic-inventory.png")
	f := gameFrame{id: 2, image: screen}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{progression: true})
	p.relic = relicPlanner{active: true, step: relicInspect, snapshot: s, original: s, moves: 4, changed: true}
	if err := p.relic.observe(relicObservation{frame: f, ui: readRelicUI(screen)}, nil); err != nil {
		t.Fatal(err)
	}
	if p.relic.step != relicReturn || p.relic.move == nil {
		t.Fatal("fixture did not reach cap with a remaining upgrade")
	}
	p.relic.step = relicVerifyEquip
	if err := p.acceptRelicSave(&ancientcalc.RelicPreview{Snapshot: s}, now); err != nil {
		t.Fatal(err)
	}
	if p.ascension.relicsChecked {
		t.Fatal("batch cap authorized destruction of useful leftover junk")
	}
}
