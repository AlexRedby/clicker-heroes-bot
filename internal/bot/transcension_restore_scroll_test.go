package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func TestRestoredTranscensionScrollsNativeOutsiders(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ctx := context.Background()
			start := time.Now().Add(-5 * time.Second)
			before := optInPrestigeSave(t, 1, 4, 0, 40, "3e22", 2)
			before.ExportedAt = start
			before.Preview.History.Available, before.Preview.History.ASGrowingAscensions = true, 3
			after := optInPrestigeSave(t, 2, 0, 75, 115, "0", 2)
			after.ExportedAt = start.Add(3 * time.Second)
			dir := t.TempDir()
			journalPath := filepath.Join(dir, "transcension-"+before.State.ProfileID+".json")
			journal, err := transcension.OpenJournal(journalPath, before.State.ProfileID)
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			ctl := transcension.NewController(transcension.Policy{Enabled: true, SkillRate: 1}, nativeTranscensionEvidence(before.State.Build))
			if err := ctl.BindJournal(journal); err != nil {
				t.Fatal(err)
			}
			if err := ctl.Begin(ctx, start, before, transcension.Timing{At: start, Generation: 1, SaveHash: before.State.SaveHash, WallConfirmed: true}); err != nil {
				t.Fatal(err)
			}
			// Synthetic receipts establish durable ownership, not native reset acceptance.
			out := transcension.Observation{Frame: 1, Generation: 1, Layout: 1, At: start.Add(time.Second), Screen: transcension.OutsidersScreen, Known: true, Reward: 75, Quantity: 1, OpenKnown: true}
			for _, row := range before.State.Outsiders {
				level, err := strconv.Atoi(row.Level)
				if err != nil {
					t.Fatal(err)
				}
				cost, err := ancientcalc.OutsiderFeedCost(row.ID, level, 1)
				if err != nil {
					t.Fatal(err)
				}
				out.Rows = append(out.Rows, transcension.Row{ID: row.ID, Name: row.Name, Level: level, Cost: cost})
			}
			ctl.Observe(out)
			reserve := func(at time.Time, want transcension.Action) {
				t.Helper()
				cmd := ctl.Next(at)
				if cmd.Action != want {
					t.Fatalf("command %+v, want %v", cmd, want)
				}
				if err := ctl.Reserve(at, cmd); err != nil {
					t.Fatal(err)
				}
			}
			reserve(out.At, transcension.OpenReset)
			confirmAt := start.Add(2 * time.Second)
			ctl.Observe(transcension.Observation{Frame: 2, Generation: 1, Layout: 1, At: confirmAt, Screen: transcension.ConfirmationScreen, Known: true, Reward: 75, ConfirmKnown: true, CancelKnown: true, RespecKnown: true})
			reserve(confirmAt, transcension.ConfirmReset)
			if err := ctl.AcceptExport(ctx, after.ExportedAt, after); err != nil || ctl.Stage() != transcension.SpendOutsiders {
				t.Fatal("reset receipt", err, ctl.Stage())
			}
			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}

			screen := exportFixture(t, "outsiders-post-transcension.png", width)
			c, err := recognizedGame(screen)
			if err != nil || !c.known || !c.outsiders {
				t.Fatal("native Outsiders classification", c, err)
			}
			c.window = "synthetic-game"
			var from, to image.Point
			drags, clicks := 0, 0
			p := newGamePipeline(&pauseControl{generation: 1}, heroInput{
				drag:  func(a, b image.Point) error { drags++; from, to = a, b; return nil },
				click: func(image.Point) error { clicks++; return nil },
			}, pipelineReaders{}, pipelineOptions{transcension: true, heroes: true, export: &saveExportOptions{dir: dir, planOutput: filepath.Join(dir, "plan.json"), skillRate: 1}})
			defer p.prestige.close()
			now := time.Now()
			p.generation, p.layout, p.startup, p.startupCheck = 1, 1, startupSave, false
			p.frame = gameFrame{id: 10, generation: 1, layout: 1, at: now, image: screen, context: c}
			p.export = saveExporter{active: true, requested: true, initialSetup: true, step: exportReadFile, jobFrame: p.frame.id, window: c.window}
			fresh := after
			fresh.ExportedAt = now
			if err := p.applyObservation(ctx, observation{kind: exportAnalysis, frame: p.frame, export: exportResult{transcension: &fresh, ancientErr: errors.New("ancient calculator: Fragsworth must be owned")}}, now); err != nil {
				t.Fatal(err)
			}
			if p.prestige.stage() != transcension.SpendOutsiders || p.startup != startupHeroes || p.controls.isPaused() {
				t.Fatal("initial setup lost recovered restoration", p.prestige.stage(), p.startup)
			}
			ui, err := readOutsiderObservation(ctx, p.frame)
			if err != nil {
				t.Fatal(err)
			}
			p.prestigeFrame(p.frame, ui, transcensionObservation{})
			p.plan(time.Now())
			a, ok := p.nextAction(time.Now())
			if !ok || a.kind != handleTranscension || a.prestige.Action != transcension.ScrollOutsidersDown {
				t.Fatalf("restored Outsiders did not queue downward scroll: available=%t kind=%d command=%+v", ok, a.kind, a.prestige)
			}
			thumb, height, found := listScrollbarThumb(screen, 445)
			if !found || a.point != thumb || a.target.X != thumb.X || a.target.Y != min(c.bounds.Max.Y-1, thumb.Y+max(3, height/2)) {
				t.Fatal("scroll did not use the native thumb", a.point, a.target, thumb, found)
			}
			acted, err := p.execute(ctx, a)
			if err != nil || !acted || drags != 1 || clicks != 0 || from != a.point || to != a.target || p.prestige.stage() != transcension.SpendOutsiders {
				t.Fatal("restored native drag", acted, err, drags, clicks, from, to, p.prestige.stage())
			}
			if replay := p.prestige.controller.Next(time.Now()); replay.Action == transcension.OpenReset || replay.Action == transcension.ConfirmReset {
				t.Fatal("restoration replayed reset", replay)
			}
		})
	}
}
