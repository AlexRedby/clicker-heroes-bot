package bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func initialSetupSave(t *testing.T, ownedAncients bool) []byte {
	t.Helper()
	var catalog []struct {
		ID       int
		Upgrades []struct{ ID int }
	}
	data, err := os.ReadFile("../ancientcalc/assets/gild-heroes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	heroes, upgrades := map[string]any{}, map[string]bool{}
	for _, hero := range catalog {
		heroes[fmt.Sprint(hero.ID)] = map[string]any{"id": hero.ID, "uid": hero.ID, "level": 1000, "epicLevel": 0, "locked": false}
		for _, upgrade := range hero.Upgrades {
			upgrades[fmt.Sprint(upgrade.ID)] = true
		}
	}
	payload := map[string]any{
		"heroSouls": "160", "heroSoulsSacrificed": 0, "highestFinishedZonePersist": "42",
		"ancientSoulsTotal": 1, "numWorldResets": 3, "transcendent": true,
		"ancients":       map[string]any{"ancients": map[string]any{"19": map[string]any{"level": "1", "spentHeroSouls": "1"}}},
		"outsiders":      map[string]any{"outsiders": map[string]any{"1": map[string]any{"level": "0"}}},
		"heroCollection": map[string]any{"heroes": heroes}, "upgrades": upgrades,
	}
	if !ownedAncients {
		payload["ancients"] = map[string]any{"ancients": map[string]any{}}
	}
	data, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(data))
}

func TestInitialFreshExportPrecedesReadySetupAndIsReused(t *testing.T) {
	ctx := context.Background()
	screen := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	heroes, menu := screen, exportFixture(t, "save-menu.png", screen.Bounds().Dx())
	dir := t.TempDir()
	old := filepath.Join(dir, "clickerHeroSave-old.txt")
	if err := os.WriteFile(old, []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	data := initialSetupSave(t, true)
	window := "game"
	var steps []exportStep
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{
		context: recognizedGame, window: func() string { return window }, heroes: noStartupOCR(t),
	}, pipelineOptions{heroes: true, export: &saveExportOptions{dir: dir, reserve: "1%", skillRate: 1, planOutput: filepath.Join(dir, "plan.json")}, fishInterval: time.Second})
	p.input.capture = func() (image.Image, error) { return screen, nil }
	p.input.focus = func(got string) error { window, screen = got, menu; return nil }
	p.input.click = func(image.Point) error {
		steps = append(steps, p.export.step)
		switch p.export.step {
		case exportOpenMenu:
			screen = menu
		case exportSave:
			if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-new.txt"), data, 0600); err != nil {
				return err
			}
			window, screen = "!outside-game", image.NewRGBA(heroes.Bounds())
		case exportCloseMenu:
			screen = heroes
		default:
			t.Fatal("unexpected initial export click", p.export.step)
		}
		return nil
	}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	for i := 0; i < 12 && !p.startupExported; i++ {
		now = now.Add(time.Second)
		p.nextCapture = time.Time{}
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
		for kind, ch := range jobs {
			if analysisKind(kind) != fishAnalysis && analysisKind(kind) != exportAnalysis && len(ch) != 0 {
				t.Fatal("setup ran before fresh initial export", kind)
			}
		}
		p.plan(now)
		if a, ok := p.nextAction(now); ok {
			if a.kind != handleExport {
				t.Fatal("input escaped initial export", a.kind)
			}
			acted, err := p.execute(ctx, a)
			if err != nil || !acted {
				t.Fatal("initial export input", err, acted)
			}
			p.actionCompleted(actionResult{action: a, acted: true}, now)
		}
		select {
		case job := <-jobs[exportAnalysis]:
			if !job.export.initialSetup {
				t.Fatal("initial purpose lost")
			}
			out := p.analyze(ctx, exportAnalysis, job)
			if out.err != nil || out.export.heroErr != nil || out.export.heroSetup == nil || out.export.heroSetup.NeedsLevels {
				t.Fatal("ready save did not establish setup", out.err, out.export.heroErr)
			}
			if err := p.accept(ctx, out, now); err != nil {
				t.Fatal(err)
			}
		default:
		}
	}
	plan := p.ancient.plan
	if !p.startupExported || !p.startupSkillsReady || p.startup != startupUpgrades || plan == nil || p.controls.isPaused() {
		t.Fatal("fresh ready snapshot was not installed", p.startup)
	}
	if len(steps) != 3 || steps[0] != exportOpenMenu || steps[1] != exportSave || steps[2] != exportCloseMenu {
		t.Fatal("export repeated", steps)
	}
	p.controls.toggle()
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	p.plan(now.Add(time.Second))
	p.plan(now.Add(2 * time.Second))
	if p.startup != noStartup || p.export.requested || p.export.active || p.ancient.plan != plan || !p.startupExported {
		t.Fatal("ready setup or F8 requested a second export")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("initial export removed old save", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "clickerHeroSave-new.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("generated save not cleaned", err)
	}
}

func TestInitialExportWaitsForCompleteHeroSaveWithoutAncients(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave-new.txt")
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	data := initialSetupSave(t, false)
	written := make(chan error, 1)
	go func() {
		time.Sleep(450 * time.Millisecond)
		written <- os.WriteFile(path, data, 0600)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, initialSetup: true, before: make(exportSnapshot), options: saveExportOptions{dir: dir, reserve: "1%", skillRate: 1}})
	if writeErr := <-written; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || result.heroSetup == nil || result.heroErr != nil || result.heroSetup.NeedsLevels || result.plan != nil || result.ancientErr == nil {
		t.Fatal("fresh hero setup depended on Ancient ownership", err, result.heroErr, result.ancientErr)
	}
}

func TestSavedReadySetupUsesOnlyOptionalFreeFooterClicker(t *testing.T) {
	requireAncientOCR(t)
	for _, available := range []int{0, 1} {
		t.Run(fmt.Sprint(available), func(t *testing.T) {
			f := startupFrame(t, "../../testdata/hero-startup-zero.png")
			f.context.window = "game"
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true, autoClickers: true, export: &saveExportOptions{}})
			p.frame, p.layout, p.startup, p.startupCheck = f, f.layout, startupSave, false
			p.export = saveExporter{active: true, requested: true, initialSetup: true, step: exportReadFile, jobFrame: f.id, window: f.context.window}
			if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: f, export: exportResult{heroSetup: &ancientcalc.HeroSetupPlan{PassiveReady: true}}}, f.at); err != nil {
				t.Fatal(err)
			}
			f.id++
			f.at = f.at.Add(time.Second)
			p.frame = f
			if available > 0 {
				out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, startup: startupUpgrades})
				if err := p.accept(context.Background(), out, f.at); err != nil {
					t.Fatal(err)
				}
			}
			p.state[autoClickerAnalysis] = observation{frame: f, clickerPool: autoClickerPool{known: true, available: available, total: 3}}
			p.plan(f.at)
			if _, ok := p.queue[buyHeroUpgrades]; ok {
				t.Fatal("saved purchased upgrades caused a bulk purchase")
			}
			if available == 0 {
				if p.startup != startupProgression || len(p.queue) != 0 {
					t.Fatal("occupied clickers caused another setup pass")
				}
			} else {
				a, ok := p.nextAction(f.at)
				if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades {
					t.Fatal("last free clicker lost its footer target", a.kind, ok)
				}
			}
		})
	}
}

func TestInitialSaveF8RetainsPurposeAndRejectsOldPlan(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-startup-zero.png")
	f.context.window = "game"
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame, p.layout, p.startup, p.startupCheck = f, f.layout, startupSave, false
	ctx, cancel := context.WithCancel(context.Background())
	p.export = saveExporter{active: true, requested: true, initialSetup: true, step: exportReadFile, jobFrame: f.id, window: f.context.window, cancel: cancel}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if ctx.Err() == nil || !p.export.initialSetup || !p.export.requested || p.startup != startupSave {
		t.Fatal("F8 lost initial export ownership or purpose")
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: f, export: exportResult{heroSetup: &ancientcalc.HeroSetupPlan{PassiveReady: true}}}, f.at); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupSave || p.startupExported {
		t.Fatal("stale initial export skipped setup after F8")
	}
	f.layout, f.generation = p.layout, p.generation
	p.frame = f
	p.plan(f.at)
	a, ok := p.nextAction(f.at)
	if !ok || a.kind != handleExport || a.export.step != exportOpenMenu {
		t.Fatal("F8 did not restart the owned initial export", a.kind, ok)
	}
	p.frame.context.modal = unknownGildModal
	p.enqueue(a, f.at)
	if _, ok := p.nextAction(f.at); ok {
		t.Fatal("initial export input reached a covering modal")
	}
}

func TestInitialExportRoutesSupportedPartialAndUnknownSetup(t *testing.T) {
	requireAncientOCR(t)
	for _, tc := range []struct {
		name    string
		setup   *ancientcalc.HeroSetupPlan
		heroErr error
		phase   startupPhase
	}{
		{"levels needed", &ancientcalc.HeroSetupPlan{PassiveReady: true, NeedsLevels: true}, nil, startupHeroes},
		{"unbought skill", &ancientcalc.HeroSetupPlan{PassiveReady: true, MissingUpgrades: []int{1}}, nil, startupUpgrades},
		{"no passive hero", &ancientcalc.HeroSetupPlan{}, nil, startupHeroes},
		{"unknown roster", nil, ancientcalc.ErrHeroSetupUnsupported, startupHeroes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := startupFrame(t, "../../testdata/hero-startup-zero.png")
			f.context.window = "game"
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{heroes: noStartupOCR(t)}, pipelineOptions{heroes: true, export: &saveExportOptions{planOutput: filepath.Join(t.TempDir(), "plan.json")}})
			p.frame, p.layout, p.startup, p.startupCheck = f, f.layout, startupSave, false
			p.export = saveExporter{active: true, requested: true, initialSetup: true, step: exportReadFile, jobFrame: f.id, window: f.context.window}
			if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: f, export: exportResult{heroSetup: tc.setup, heroErr: tc.heroErr, ancientErr: errors.New("no owned Ancients")}}, f.at); err != nil {
				t.Fatal(err)
			}
			if p.startup != tc.phase || !p.startupExported || p.ancient.plan != nil || p.controls.isPaused() {
				t.Fatal("wrong initial route", p.startup)
			}
			f.id++
			f.at = f.at.Add(time.Second)
			p.frame = f
			out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, startup: p.startup, sweep: startupSweep{top: true}})
			if err := p.accept(context.Background(), out, f.at); err != nil {
				t.Fatal(err)
			}
			p.plan(f.at)
			a, ok := p.nextAction(f.at)
			want := buyHero
			if tc.phase == startupUpgrades {
				want = buyHeroUpgrades
			}
			if !ok || a.kind != want {
				t.Fatal("partial setup input", a.kind, ok)
			}
		})
	}
}

func TestAscensionInvalidatesInitialSetupSnapshot(t *testing.T) {
	t.Chdir(t.TempDir())
	f := testPipelineFrame()
	f.context.heroes = true
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true, export: &saveExportOptions{}})
	p.frame, p.layout, p.startupCheck = f, f.layout, false
	p.export.requested = false
	p.startupExported, p.startupSkillsReady = true, true
	p.ancient = ancientPlanner{plan: &ancientPlan{}, finished: true}
	p.ascension = ascensionPlanner{active: true, step: waitAscensionReset, lastInputFrame: f.id - 1}
	if err := p.accept(context.Background(), observation{kind: ascensionAnalysis, frame: f, ascension: ascensionObservation{frame: f, zone: 1}}, f.at); err != nil {
		t.Fatal(err)
	}
	if p.startup != startupHeroes || p.startupExported || p.startupSkillsReady || p.export.initialSetup || p.export.requested || p.ancient.plan != nil {
		t.Fatal("Ascension reused the initial hero/Ancient snapshot")
	}
}
