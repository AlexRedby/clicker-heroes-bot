package main

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	data, err := os.ReadFile("internal/ancientcalc/testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	controls := &pauseControl{}
	window, screen := "101:Clicker Heroes", main
	var steps []exportStep
	p := newGamePipeline(controls, heroInput{}, pipelineReaders{context: recognizedGame, window: func() string { return window }}, pipelineOptions{export: &saveExportOptions{dir: dir, reserve: "1%", skillRate: 1, planOutput: filepath.Join(dir, "plan.json")}, fishInterval: time.Second, heroes: true, skills: true, progression: true, monster: true})
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
		// No background analyzer should run during this transaction.
		for kind, ch := range jobs {
			if analysisKind(kind) != exportAnalysis && len(ch) > 0 {
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
	if p.export.requested || p.export.active || p.ancient.plan == nil || p.ancient.plan.savePath != filepath.Join(dir, "clickerHeroSave-new.txt") {
		t.Fatal("fresh plan not installed")
	}
	if len(steps) != 3 || steps[0] != exportOpenMenu || steps[1] != exportSave || steps[2] != exportCloseMenu {
		t.Fatalf("click sequence %v", steps)
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
	p.accept(context.Background(), observation{kind: exportAnalysis, frame: a.frame, exportPlan: &ancientPlan{}}, time.Now())
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
	plan, err := readExportPlan(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "1%", skillRate: 1}})
	if writeErr := <-done; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || plan == nil || plan.savePath != path {
		t.Fatalf("partial export: %v %v", plan, err)
	}
}

func TestExportStartsAfterConfirmedAscension(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{export: &saveExportOptions{dir: t.TempDir()}})
	p.export.requested = false
	p.ancient = ancientPlanner{plan: &ancientPlan{}, finished: true}
	p.frame = testPipelineFrame()
	p.frame.id = 4
	p.layout = p.frame.layout
	p.frame.context.heroes = true
	p.ascension = ascensionPlanner{active: true, step: waitAscensionReset, lastInputFrame: 3}
	err := p.accept(context.Background(), observation{kind: ascensionAnalysis, frame: p.frame, ascension: ascensionObservation{frame: p.frame, zone: 1}}, time.Now())
	if err != nil || p.controls.isPaused() || !p.export.requested || p.ancient.plan != nil || p.ascension.active {
		t.Fatalf("post-reset export: %v", err)
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
	if saveMenu(screen) {
		t.Fatal("Import accepted as Save")
	}
}
