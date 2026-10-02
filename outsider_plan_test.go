package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func outsiderPlanFixture(t *testing.T) (*ancientcalc.TranscensionPreview, outsiderObservation) {
	t.Helper()
	source, err := readTranscensionPreview(context.Background(), "internal/ancientcalc/testdata/prestige-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	ui := outsiderObservation{known: true, wallet: 0, gain: 20, power: 4.04, quantity: "x1", rows: []outsiderScreenRow{{"Xyliqil", 0, 1}, {"Chor'gorloth", 6, 7}}}
	return &source, ui
}

func TestOutsiderAdviceRealFrameAndProjectedReward(t *testing.T) {
	requireAncientOCR(t)
	source, _ := outsiderPlanFixture(t)
	screen := exportFixture(t, "outsiders-top.png", 2560)
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	ui, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: c})
	if err != nil {
		t.Fatal(err)
	}
	report := reconcileOutsiders(context.Background(), ui, source)
	if report.Status != "partial" || len(report.Checks) != 2 || report.Plans == nil || report.RewardMatches == nil || *report.RewardMatches {
		t.Fatalf("report: %+v", report)
	}
	if report.Plans.Now.Spent != 0 || report.Plans.Now.Budget != 0 || report.Plans.AfterReward.Budget != 20 || report.Plans.AfterReward.Spent != 20 {
		t.Fatal("pending reward entered current budget or old +18 estimate was used", report.Plans)
	}
	if report.Plans.AfterReward.Additions[2].Target != 40 || report.Plans.AfterReward.Additions[2].Cost != 4 || report.Plans.AfterReward.Additions[3].Target != 16 || report.Plans.AfterReward.Additions[3].Cost != 16 {
		t.Fatal("incorrect concrete upgrade plan", report.Plans.AfterReward)
	}
	if source.UIVerified || *source.EstimatedASGain != 18 || source.Outsiders.Additions[2].Target != 38 {
		t.Fatal("reconciliation mutated the save preview or authorized its model")
	}
	for _, text := range []string{"2/9", "conditional", "Phandoryss 36 -> 40 (4 AS)", "Ponyboy 15 -> 16 (16 AS)", "save estimate +18, UI +20", "respec"} {
		if !strings.Contains(report.String(), text) {
			t.Fatalf("missing %q in %q", text, report.String())
		}
	}
}

func TestOutsiderAdviceRejectsMismatches(t *testing.T) {
	for _, change := range []func(*ancientcalc.TranscensionPreview, *outsiderObservation){
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.wallet = 1 },
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.power = 4.2 },
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.rows[1].level = 7 },
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.rows[0].cost = 2 },
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) {
			ui.rows = append(ui.rows, ui.rows[0])
		},
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.quantity = "custom" },
		func(_ *ancientcalc.TranscensionPreview, ui *outsiderObservation) { ui.rows = nil },
		func(source *ancientcalc.TranscensionPreview, _ *outsiderObservation) { source.SaveVersion = 8 },
		func(source *ancientcalc.TranscensionPreview, _ *outsiderObservation) { source.Outsiders = nil },
		func(source *ancientcalc.TranscensionPreview, _ *outsiderObservation) {
			source.Outsiders.Additions = source.Outsiders.Additions[:8]
		},
	} {
		source, ui := outsiderPlanFixture(t)
		change(source, &ui)
		got := reconcileOutsiders(context.Background(), ui, source)
		if got.Plans != nil || got.Status != "unavailable" || got.Reason == "" {
			t.Fatalf("mismatch produced a plan: %+v", got)
		}
	}
	source, ui := outsiderPlanFixture(t)
	if got := reconcileOutsiders(context.Background(), ui, nil); got.Plans != nil || got.Status != "unavailable" {
		t.Fatal("missing roster was invented")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := reconcileOutsiders(ctx, ui, source); got.Plans != nil {
		t.Fatal("cancellation produced a plan")
	}
	source.Outsiders.Status = "unsupported"
	if got := reconcileOutsiders(context.Background(), ui, source); got.Plans != nil || got.Status != "unsupported" {
		t.Fatal("unsupported model produced a plan")
	}
}

func TestOutsiderAdviceQuantityAndZeroReward(t *testing.T) {
	source, ui := outsiderPlanFixture(t)
	ui.quantity = "x10"
	ui.rows[0].cost, ui.rows[1].cost = 55, 115
	got := reconcileOutsiders(context.Background(), ui, source)
	if got.Plans == nil || len(got.Checks) != 2 || *got.Checks[0].ExpectedCost != 55 || *got.Checks[1].ExpectedCost != 115 {
		t.Fatal("bulk price compared with x1", got)
	}
	ui.quantity = "MAX"
	got = reconcileOutsiders(context.Background(), ui, source)
	if got.Status != "partial" || got.Plans == nil || got.Checks[0].ExpectedCost != nil || !strings.Contains(got.Reason, "MAX cost is not verified") {
		t.Fatal("MAX price treated as an ordinary fixed quantity", got)
	}
	ui.quantity, ui.gain = "x1", 0
	ui.rows[0].cost, ui.rows[1].cost = 1, 7
	got = reconcileOutsiders(context.Background(), ui, source)
	if got.Plans == nil || got.Plans.AfterReward.Spent != 0 || got.Plans.AfterReward.Budget != 0 {
		t.Fatal("save estimate overrode visible zero reward", got)
	}
}

func TestOutsiderPlanJobOwnsSourceAndNeverEnqueuesInput(t *testing.T) {
	source, ui := outsiderPlanFixture(t)
	frame := testPipelineFrame()
	frame.context.outsiders = true
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return frame.context, nil },
		outsiders: func(_ context.Context, f gameFrame) (outsiderObservation, error) {
			out := ui
			out.frame = f
			return out, nil
		},
	}, pipelineOptions{outsiderBase: source, fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	job := <-jobs[outsiderAnalysis]
	if job.outsiderBase != source || job.frame.image != p.frame.image {
		t.Fatal("job does not own the observed frame/source")
	}
	out := p.analyze(context.Background(), outsiderAnalysis, job)
	newSource := *source
	newSource.SaveHash = "new-export"
	p.outsiderBase = &newSource
	if err := p.accept(context.Background(), out, now); err != nil || p.state[outsiderAnalysis].outsiderPlan.Plans != nil || len(p.queue) != 0 {
		t.Fatal("old save plan survived source replacement", err)
	}
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[outsiderAnalysis]
	if err := p.accept(context.Background(), p.analyze(context.Background(), outsiderAnalysis, job), now); err != nil || p.state[outsiderAnalysis].outsiderPlan.Plans == nil || len(p.queue) != 0 {
		t.Fatal("read-only plan was not accepted or created input", err)
	}
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.state[outsiderAnalysis].outsiderPlan.Plans != nil {
		t.Fatal("F8 retained the UI plan")
	}
}

func TestOutsiderOfflinePreviewPreservesBothInputs(t *testing.T) {
	requireAncientOCR(t)
	save := "internal/ancientcalc/testdata/prestige-save.txt"
	shot := "testdata/outsiders-top.png"
	var output bytes.Buffer
	if err := previewOutsiders(context.Background(), save, shot, "", &output); err != nil {
		t.Fatal(err)
	}
	var report outsiderAdvice
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || report.Plans == nil || report.Status != "partial" {
		t.Fatal("invalid offline plan", err)
	}
	for _, input := range []string{save, shot} {
		original, err := os.ReadFile(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, link := range []func(string, string) error{os.Link, os.Symlink} {
			alias := filepath.Join(t.TempDir(), "input-alias")
			absolute, err := filepath.Abs(input)
			if err != nil {
				t.Fatal(err)
			}
			if err := link(absolute, alias); err != nil {
				t.Skip(err)
			}
			if err := writeReadOnlyPreview(context.Background(), report, alias, nil, []string{save, shot}); err == nil {
				t.Fatal("JSON overwrote an input alias")
			}
		}
		if current, err := os.ReadFile(input); err != nil || !bytes.Equal(current, original) {
			t.Fatal("input was modified", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := previewOutsiders(ctx, save, shot, "", &output); !errors.Is(err, context.Canceled) {
		t.Fatal("offline cancellation", err)
	}
}

func TestOutsiderScreenshotBounds(t *testing.T) {
	file := filepath.Join(t.TempDir(), "image.png")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := readOutsiderScreenshot(file); err == nil {
		t.Fatal("tiny image was accepted")
	}
	if err := os.Truncate(file, (32<<20)+1); err != nil {
		t.Fatal(err)
	}
	if _, err := readOutsiderScreenshot(file); err == nil {
		t.Fatal("oversized image was accepted")
	}
	if _, err := readOutsiderScreenshot(filepath.Dir(file)); err == nil {
		t.Fatal("directory was accepted as image")
	}
}

func TestRelicOnlyExportRefreshesIndependentOutsiderSource(t *testing.T) {
	// No owned Ancients: the normal Ancient allocator must not be involved.
	text := `{"version":7,"readPatchNumber":"1.0e12-6144","numberOfTranscensions":1,"numAscensionsThisTranscension":0,"heroSouls":0,"heroSoulsSacrificed":0,"primalSouls":0,"totalHeroLevels":0,"ancientSouls":0,"highestFinishedZonePersist":1,"ancientSoulsTotal":0,"numWorldResets":0,"transcendent":true,"ancients":{"ancients":{}},"outsiders":{"outsiders":{}},"items":{"equipmentSlots":4,"items":{},"slots":{}}}`
	data := []byte(base64.StdEncoding.EncodeToString([]byte(text)))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-new.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, relicsOnly: true, options: saveExportOptions{dir: dir, reserve: "invalid"}})
	if err != nil || result.plan != nil || result.prestige == nil || result.relics == nil {
		t.Fatalf("independent read-only export: %+v %v", result, err)
	}
	source, _ := outsiderPlanFixture(t)
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{outsiderBase: source})
	p.frame = testPipelineFrame()
	p.layout = p.frame.layout
	p.export = saveExporter{requested: true, active: true, relicsOnly: true, step: exportReadFile, jobFrame: p.frame.id, window: p.frame.context.window}
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: result}, time.Now()); err != nil || p.outsiderBase != result.prestige || p.outsiderBase.SaveHash == source.SaveHash || p.ancient.plan != nil {
		t.Fatal("accepted fresh read-only export retained the old roster or enabled Ancients", err)
	}
	result.prestige = nil
	p.frame.id++
	p.export = saveExporter{requested: true, active: true, relicsOnly: true, step: exportReadFile, jobFrame: p.frame.id, window: p.frame.context.window}
	if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: result}, time.Now()); err != nil || p.outsiderBase != nil || p.controls.isPaused() || p.ancient.plan != nil {
		t.Fatal("missing metadata retained an older roster or blocked the independent relic flow", err)
	}
}
