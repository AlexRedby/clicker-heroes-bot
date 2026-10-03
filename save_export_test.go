package main

import (
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"

	xdraw "golang.org/x/image/draw"
)

func exportFixture(t *testing.T, name string, width int) image.Image {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if width == img.Bounds().Dx() {
		return img
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, width*img.Bounds().Dy()/img.Bounds().Dx()))
	xdraw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), xdraw.Src, nil)
	return resized
}
func TestSaveMenuRecognitionAndExclusiveExport(t *testing.T) {
	for _, width := range []int{1280, 2560} {
		menu := exportFixture(t, "save-menu.png", width)
		main := exportFixture(t, "ascension-ancients.png", width)
		c, err := recognizedGame(menu)
		if err != nil || !c.known || !c.saveMenu {
			t.Fatalf("menu at %d: %+v %v", width, c, err)
		}
		if saveMenu(main) || saveMenu(exportFixture(t, "ancient-quantity-filled.png", width)) {
			t.Fatal("unrelated screen recognized as save menu")
		}
		for _, which := range []int{0, 1} {
			if _, ok, err := saveControl(menu, which); err != nil || !ok {
				t.Fatalf("menu control %d at %d: %v %v", which, width, ok, err)
			}
		}
		if _, ok, err := saveControl(main, 2); err != nil || !ok {
			t.Fatalf("wrench at %d: %v %v", width, ok, err)
		}
	}
	main := exportFixture(t, "ascension-ancients.png", 1280)
	menu := exportFixture(t, "save-menu.png", 1280)
	dir := t.TempDir()
	// An old file must not satisfy the export request, even with a future timestamp.
	old := filepath.Join(dir, "clickerHeroSave-old.txt")
	if err := os.WriteFile(old, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, time.Now().Add(time.Hour), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	data := testIntegratedGildSave(t, true)
	controls := &pauseControl{}
	window, screen := "101:Clicker Heroes", main
	var steps []exportStep
	p := newGamePipeline(controls, heroInput{}, pipelineReaders{context: recognizedGame, window: func() string { return window }}, pipelineOptions{export: &saveExportOptions{dir: dir, reserve: "1%", skillRate: 1, planOutput: filepath.Join(dir, "plan.json")}, fishInterval: time.Second, heroes: true, skills: true, progression: true, monster: true})
	p.startupCheck = false // This test begins after initial hero setup.
	p.nextUpgrades = time.Now().Add(time.Hour)
	p.input.capture = func() (image.Image, error) { return screen, nil }
	p.input.focus = func(got string) error {
		if got != "101:Clicker Heroes" || window != "!outside-game" {
			t.Fatalf("restore %q from %q", got, window)
		}
		window, screen = got, menu
		return nil
	}
	p.input.click = func(point image.Point) error {
		if window == "!outside-game" {
			t.Fatal("clicked Explorer")
		}
		steps = append(steps, p.export.step)
		switch p.export.step {
		case exportOpenMenu:
			screen = menu
		case exportSave:
			if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-new.txt"), data, 0600); err != nil {
				return err
			}
			window, screen = "!outside-game", image.NewRGBA(main.Bounds())
		case exportCloseMenu:
			screen = main
		default:
			t.Fatalf("unexpected click at %v, stage %v", point, p.export.step)
		}
		return nil
	}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	for i := 0; i < 10 && p.ancient.plan == nil; i++ {
		now = now.Add(time.Second)
		if err := p.capture(context.Background(), now, jobs); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			p.ascension = ascensionPlanner{highestZone: 110, wallZone: 110, fullCombatFailed: true, lastObservation: now, lastProgress: now}
		}
		// Fish remains active; unrelated analyzers wait for this transaction.
		for kind, ch := range jobs {
			if analysisKind(kind) != exportAnalysis && analysisKind(kind) != fishAnalysis && len(ch) > 0 {
				t.Fatalf("background job %d", kind)
			}
		}
		p.plan(now)
		p.enqueue(gameAction{kind: clickMonster, frame: p.frame, point: image.Pt(100, 100)}, now)
		if a, ok := p.nextAction(now); ok {
			if a.kind != handleExport {
				t.Fatalf("background input %v", a.kind)
			}
			acted, err := p.execute(context.Background(), a)
			if err != nil || !acted {
				t.Fatalf("execute %s: %v %v", a.export.step, acted, err)
			}
			p.actionCompleted(actionResult{action: a, acted: true}, now)
		}
		select {
		case job := <-jobs[exportAnalysis]:
			out := p.analyze(context.Background(), exportAnalysis, job)
			if err := p.accept(context.Background(), out, now); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	if p.export.requested || p.export.active || p.ancient.plan == nil || p.ancient.plan.savePath != filepath.Join(dir, "clickerHeroSave-new.txt") || p.relicMessage == "" {
		t.Fatal("fresh plan not installed")
	}
	if _, err := os.Stat(filepath.Join(dir, "clickerHeroSave-new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("generated export was not removed", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("pre-existing export was removed", err)
	}
	if p.ancient.plan.Gilds == nil || p.ancient.plan.GildError != "" || p.ancient.plan.Gilds.Cost != "160" || p.ancient.plan.Gilds.Reserve != "1.6" || p.ancient.plan.Gilds.SaveHash != p.ancient.plan.SaveHash {
		t.Fatalf("fresh plan omitted gild preview: %+v error=%q", p.ancient.plan.Gilds, p.ancient.plan.GildError)
	}
	if len(steps) != 3 || steps[0] != exportOpenMenu || steps[1] != exportSave || steps[2] != exportCloseMenu {
		t.Fatalf("click sequence %v", steps)
	}
	if p.ascension.highestZone != 110 || p.ascension.wallZone != 110 || p.ascension.latest.frame.id != 0 {
		t.Fatal("owned Explorer handoff lost boss history or retained current reward")
	}
	if _, err := os.Stat(filepath.Join(dir, "plan.json")); err != nil {
		t.Fatal(err)
	}
}
func TestExportF8CancelsInputAndFileRead(t *testing.T) {
	controls := &pauseControl{}
	p := newGamePipeline(controls, heroInput{click: func(image.Point) error { t.Fatal("stale Save click"); return nil }}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	ctx, cancel := context.WithCancel(context.Background())
	p.export.active, p.export.step, p.export.cancel = true, exportReadFile, cancel
	a := gameAction{kind: handleExport, frame: testPipelineFrame(), export: &exportCommand{step: exportSave}}
	controls.toggle()
	p.reset(controls.snapshot())
	if acted, err := p.execute(context.Background(), a); acted || err != nil {
		t.Fatalf("stale input: %v %v", acted, err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) || !p.export.requested || p.export.active {
		t.Fatal("export not canceled for safe retry")
	}
	// Old observations cannot install a plan after a new generation starts.
	controls.toggle()
	p.reset(controls.snapshot())
	p.accept(context.Background(), observation{kind: exportAnalysis, frame: a.frame, export: exportResult{plan: &ancientPlan{}}}, time.Now())
	if p.ancient.plan != nil {
		t.Fatal("stale export installed")
	}
}
func TestExportTimeoutPausesWithReason(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	p.frame = testPipelineFrame()
	p.export.active, p.export.step, p.export.deadline = true, exportCloseMenu, time.Now().Add(-time.Second)
	if !p.planExport(time.Now()) || !p.controls.isPaused() || p.controls.message() == "paused: " {
		t.Fatal("timeout did not explain pause")
	}
}
func TestExportReadWaitsForCompleteSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave.txt")
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("internal/ancientcalc/testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { time.Sleep(450 * time.Millisecond); done <- os.WriteFile(path, data, 0600) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "1%", skillRate: 1}})
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || result.plan == nil || result.plan.savePath != path {
		t.Fatalf("partial export: %v %v", result.plan, err)
	}
}

func TestExportStartsAfterConfirmedAscension(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	p.export.requested = false
	p.export.relicsOnly = true
	p.ancient = ancientPlanner{plan: &ancientPlan{}, finished: true}
	p.frame = testPipelineFrame()
	p.frame.id = 4
	p.layout = p.frame.layout
	p.frame.context.heroes = true
	p.ascension = ascensionPlanner{active: true, step: waitAscensionReset, lastInputFrame: 3}
	now := time.Now()
	err := p.accept(context.Background(), observation{kind: ascensionAnalysis, frame: p.frame, ascension: ascensionObservation{frame: p.frame, zone: 1}}, now)
	if err != nil || p.controls.isPaused() || !p.export.requested || p.export.relicsOnly || p.ancient.plan != nil || p.ascension.active {
		t.Fatalf("post-reset export: %v", err)
	}
	path := filepath.Join(dir, "artifacts", "ascension-start-"+now.Format("20060102-150405.000")+".png")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Bounds() != p.frame.image.Bounds() {
		t.Fatalf("startup screenshot bounds = %v", saved.Bounds())
	}
}

func TestReadOnlyRelicExportWaitsForCompleteSaveAndSkipsAncients(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave-relics.txt")
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	// Sanitized schema projection; not a player save. Ancient allocation cannot
	// succeed with this roster/reserve, and must never run for this purpose.
	text := `{"heroSouls":1,"heroSoulsSacrificed":0,"highestFinishedZonePersist":1,"ancientSoulsTotal":0,"numWorldResets":3.0,"transcendent":false,"ancients":{"ancients":{}},"outsiders":{"outsiders":{}},"items":{"equipmentSlots":4,"items":{},"slots":{}}}`
	data := []byte(base64.StdEncoding.EncodeToString([]byte(text)))
	done := make(chan error, 1)
	go func() { time.Sleep(450 * time.Millisecond); done <- os.WriteFile(path, data, 0600) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, before: make(exportSnapshot), relicsOnly: true, options: saveExportOptions{dir: dir, reserve: "invalid"}})
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || result.plan != nil || result.relics == nil || result.relicErr != nil || result.relics.Readiness != "unknown" || result.relics.Snapshot.Ascensions != 3 {
		t.Fatalf("read-only result: %+v %v", result, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned relic-only export was not removed", err)
	}
}

func TestRelicCheckPrecedesAscensionAndRequiresFreshReward(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	c.window = "game"
	frame := gameFrame{id: 2, at: now, image: screen, context: c}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{ascension: true, ascensionStall: 3 * time.Minute, ascensionMinGain: .25, ascensionCapital: 62, fishInterval: time.Second, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout = frame, frame.layout
	p.export.requested = false
	oldPlan := &ancientPlan{}
	p.ancient = ancientPlanner{plan: oldPlan, finished: true}
	p.ascension = ascensionPlanner{highestZone: 110, wallZone: 110, fullCombatFailed: true, lastObservation: now, lastProgress: now}
	p.ascension.latest = ascensionObservation{frame: frame, economy: true, bank: 60, souls: 65}
	p.state[fishAnalysis] = observation{frame: frame}
	p.state[progressionAnalysis] = observation{frame: frame}
	p.plan(now)
	if !p.export.requested || !p.export.relicsOnly || !p.export.active || len(p.queue) != 1 || p.queue[handleExport].export == nil {
		t.Fatalf("Ascension did not request read-only Save: %+v", p.export)
	}
	if _, found := p.queue[handleAscension]; found {
		t.Fatal("Ascension opened before relic check")
	}
	p.frame.id++
	p.export.step, p.export.jobFrame = exportReadFile, p.frame.id
	preview := &ancientcalc.RelicPreview{Readiness: "unknown", Reason: "snapshot only"}
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: exportResult{relics: preview}}, now); err != nil {
		t.Fatal(err)
	}
	if p.controls.isPaused() || p.export.requested || !p.ascension.relicsChecked || p.ancient.plan != oldPlan || !p.ancient.finished || p.options.ascensionCapital != 62 {
		t.Fatal("read-only export changed Ancient spending or failed to finish")
	}
	if p.ascension.latest.frame.id != 0 || p.state[progressionAnalysis].frame.id != 0 || len(p.queue) != 0 || p.ascensionReady(now) {
		t.Fatal("pre-export combat/reward evidence survived")
	}
	// Fresh combat/fish/reward observations allow the original reset policy,
	// without re-exporting the same inventory on every planning iteration.
	p.ascension.observeProgress(progressionState{Known: true, Zone: 109}, 110, now, true)
	p.ascension.latest = ascensionObservation{frame: p.frame, economy: true, bank: 60, souls: 65}
	p.state[fishAnalysis] = observation{frame: p.frame}
	p.state[progressionAnalysis] = observation{frame: p.frame}
	p.plan(now)
	if p.export.requested || p.queue[handleAscension].kind != handleAscension {
		t.Fatal("fresh reward did not resume Ascension without a second export")
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.ascension.relicsChecked {
		t.Fatal("F8 retained the relic check")
	}
	// A previous generation cannot install even an advisory result.
	p.export = saveExporter{requested: true, active: true, relicsOnly: true, step: exportReadFile, jobFrame: frame.id, window: c.window}
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: frame, export: exportResult{relics: preview}}, now); err != nil || p.ascension.relicsChecked {
		t.Fatal("stale preview accepted after F8")
	}
}

func TestRelicExportRejectsChangedLayout(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{}})
	p.frame = testPipelineFrame()
	p.layout = p.frame.layout + 1
	p.export = saveExporter{requested: true, active: true, relicsOnly: true, step: exportReadFile, jobFrame: p.frame.id, window: p.frame.context.window}
	err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: exportResult{relics: &ancientcalc.RelicPreview{Readiness: "unknown"}}}, time.Now())
	if err != nil || !p.controls.isPaused() || p.ascension.relicsChecked || p.ancient.plan != nil {
		t.Fatal("layout change accepted the preview")
	}
}

func TestRuntimeRelicReportOnlyChanges(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	stdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = stdout; writer.Close() }()
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	preview := &ancientcalc.RelicPreview{Readiness: "unknown", Reason: "snapshot only"}
	p.reportRelics(preview, nil)
	p.reportRelics(preview, nil)
	p.reset(1)
	p.reportRelics(preview, nil)
	p.reportRelics(nil, errors.New("missing inventory"))
	writer.Close()
	data, err := io.ReadAll(reader)
	if err != nil || strings.Count(string(data), "relics:") != 2 || strings.Contains(string(data), "ready") {
		t.Fatalf("repeated advisory output: %q %v", data, err)
	}
}
func TestInitialHeroesHUDWithoutProgressionControl(t *testing.T) {
	main := exportFixture(t, "hero-economy-x1.png", 1280)
	screen := image.NewRGBA(main.Bounds())
	draw.Draw(screen, screen.Bounds(), main, main.Bounds().Min, draw.Src)
	draw.Draw(screen, controlRect(screen, image.Rect(1210, 186, 1280, 235)), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	// Confirm the game context remains readable before progression is unlocked.
	if known, _, err := progressionMode(screen); err != nil || known {
		t.Fatalf("progression control still present: %v %v", known, err)
	}
	c, err := recognizedGame(screen)
	if err != nil || !c.known || !c.heroes {
		t.Fatalf("initial Heroes HUD not recognized: %+v %v", c, err)
	}
}

func TestExportWaitsForDelayedExplorer(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	p.frame = testPipelineFrame()
	p.layout = p.frame.layout
	p.export = saveExporter{active: true, requested: true, step: exportRestoreGame, window: p.frame.context.window, deadline: time.Now().Add(time.Second)}
	now := time.Now()
	p.planExport(now)
	if len(p.queue) != 0 {
		t.Fatal("restored focus before Explorer opened")
	}
	p.frame.context.known, p.frame.context.window = false, "!outside-game"
	p.planExport(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleExport || a.export.step != exportRestoreGame {
		t.Fatal("Explorer focus did not schedule restoration")
	}
}
func TestSaveRecognitionRejectsImportButton(t *testing.T) {
	menu := exportFixture(t, "save-menu.png", 1280)
	screen := image.NewRGBA(menu.Bounds())
	draw.Draw(screen, screen.Bounds(), menu, menu.Bounds().Min, draw.Src)
	save := controlRect(screen, image.Rect(290, 192, 449, 216))
	// Identical green controls must be distinguished by their labels.
	draw.Draw(screen, save, menu, image.Pt(save.Min.X, save.Min.Y+50), draw.Src)
	if _, found, err := saveControl(screen, 0); err != nil || found {
		t.Fatal("Import accepted as Save", err)
	}
	if !saveMenu(screen) {
		t.Fatal("menu X did not independently identify the menu")
	}
}

func TestExportClosesMenuWithChangedSaveButton(t *testing.T) {
	for _, width := range []int{1280, 2560} {
		menu := exportFixture(t, "save-menu.png", width)
		screen := image.NewRGBA(menu.Bounds())
		draw.Draw(screen, screen.Bounds(), menu, menu.Bounds().Min, draw.Src)
		draw.Draw(screen, controlRect(screen, image.Rect(290, 192, 449, 216)), &image.Uniform{C: color.RGBA{100, 100, 100, 255}}, image.Point{}, draw.Src)
		// A previous supposed background anchor was inside Recover Save Data's label.
		draw.Draw(screen, controlRect(screen, image.Rect(290, 292, 449, 316)), &image.Uniform{C: color.RGBA{100, 100, 100, 255}}, image.Point{}, draw.Src)
		if _, found, err := saveControl(screen, 0); err != nil || found {
			t.Fatal("changed Save unexpectedly matched", err)
		}
		c, err := recognizedGame(screen)
		if err != nil {
			t.Fatal(err)
		}
		c.window = "101:Clicker Heroes"
		var clicked image.Point
		controls := &pauseControl{}
		p := newGamePipeline(controls, heroInput{click: func(point image.Point) error { clicked = point; return nil }}, pipelineReaders{window: func() string { return c.window }}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
		now := time.Now()
		p.layout = 1
		p.frame = gameFrame{id: 2, layout: 1, at: now, image: screen, context: c}
		p.export = saveExporter{requested: true, active: true, step: exportCloseMenu, window: c.window, lastFrame: 1, deadline: now.Add(time.Second)}
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != handleExport || a.export.step != exportCloseMenu {
			t.Fatalf("no close action at width %d: context=%+v", width, c)
		}
		if acted, err := p.execute(context.Background(), a); err != nil || !acted {
			t.Fatalf("close input: %v %v", acted, err)
		}
		point, found, err := saveControl(screen, 1)
		if err != nil || !found || clicked != point {
			t.Fatalf("wrong close target %v", clicked)
		}
		controls.toggle()
		if acted, err := p.execute(context.Background(), a); err != nil || acted {
			t.Fatal("close input escaped F8")
		}
	}
}
