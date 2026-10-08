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
	"strconv"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func TestPendingTranscensionFeedExportPrecedesStartup(t *testing.T) {
	for _, phase := range []startupPhase{startupHeroes, startupUpgrades, startupProgression} {
		t.Run(fmt.Sprint(phase), func(t *testing.T) {
			ctx := context.Background()
			start := time.Now().Add(-5 * time.Second)
			before := optInPrestigeSave(t, 1, 4, 0, 40, "3e22", 2)
			before.ExportedAt = start
			before.Preview.History.Available, before.Preview.History.ASGrowingAscensions = true, 3
			after := optInPrestigeSave(t, 2, 0, 75, 115, "0", 2)
			after.ExportedAt = start.Add(2 * time.Second)
			dir := t.TempDir()
			journal, err := transcension.OpenJournal(filepath.Join(dir, "transcension-"+before.State.ProfileID+".json"), before.State.ProfileID)
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
			rows := func(s transcension.Snapshot) []transcension.Row {
				t.Helper()
				var out []transcension.Row
				for _, row := range s.State.Outsiders {
					level, err := strconv.Atoi(row.Level)
					if err != nil {
						t.Fatal(err)
					}
					cost, err := ancientcalc.OutsiderFeedCost(row.ID, level, 1)
					if err != nil {
						t.Fatal(err)
					}
					out = append(out, transcension.Row{ID: row.ID, Name: row.Name, Level: level, Cost: cost, FeedKnown: true})
				}
				return out
			}
			reserve := func(at time.Time, want transcension.Action) transcension.Command {
				t.Helper()
				cmd := ctl.Next(at)
				if cmd.Action != want {
					t.Fatalf("command %+v, want %v", cmd, want)
				}
				if err := ctl.Reserve(at, cmd); err != nil {
					t.Fatal(err)
				}
				return cmd
			}
			// Synthetic controller inputs establish pending ownership without game clicks.
			ctl.Observe(transcension.Observation{Frame: 1, Generation: 1, Layout: 1, At: start, Screen: transcension.OutsidersScreen, Known: true, Reward: 75, Quantity: 1, OpenKnown: true, Rows: rows(before)})
			reserve(start, transcension.OpenReset)
			confirmAt := start.Add(time.Second)
			ctl.Observe(transcension.Observation{Frame: 2, Generation: 1, Layout: 1, At: confirmAt, Screen: transcension.ConfirmationScreen, Known: true, Reward: 75, ConfirmKnown: true, CancelKnown: true, RespecKnown: true})
			reserve(confirmAt, transcension.ConfirmReset)
			if err := ctl.AcceptExport(ctx, after.ExportedAt, after); err != nil {
				t.Fatal(err)
			}
			feedAt := start.Add(3 * time.Second)
			ctl.Observe(transcension.Observation{Frame: 3, Generation: 1, Layout: 1, At: feedAt, Screen: transcension.OutsidersScreen, Known: true, Wallet: 75, Quantity: 1, QuantityKnown: true, Rows: rows(after)})
			feed := reserve(feedAt, transcension.FeedOutsider)
			if ctl.Stage() != transcension.AwaitFeedReceipt || ctl.Next(time.Now()).Action != transcension.FreshExport {
				t.Fatal("FEED did not wait for its receipt", ctl.Stage())
			}

			if err := journal.Close(); err != nil {
				t.Fatal(err)
			}

			outsiders := map[string]any{}
			for _, row := range after.State.Outsiders {
				level, _ := strconv.Atoi(row.Level)
				if row.ID == feed.ID {
					level += feed.Quantity
				}
				spent := 0
				if level > 0 {
					spent, err = ancientcalc.OutsiderFeedCost(row.ID, 0, level)
					if err != nil {
						t.Fatal(err)
					}
				}
				outsiders[strconv.Itoa(row.ID)] = map[string]any{"id": row.ID, "level": level, "spentAncientSouls": spent}
			}
			data, err := json.Marshal(map[string]any{
				"uniqueId": "synthetic-opt-in-profile", "version": 7, "readPatchNumber": after.State.Build,
				"transcendent": true, "numberOfTranscensions": 2, "numWorldResets": 4,
				"numAscensionsThisTranscension": 0, "currentZoneHeight": 1, "highestFinishedZonePersist": 300,
				"ancientSouls": 75 - feed.Cost, "ancientSoulsTotal": 115,
				"heroSouls": "0", "heroSoulsSacrificed": "1e6", "primalSouls": "0", "totalHeroLevels": 0,
				"ancients": map[string]any{"ancients": map[string]any{}}, "outsiders": map[string]any{"outsiders": outsiders},
				"items": map[string]any{"equipmentSlots": 4, "items": map[string]any{}, "slots": map[string]any{}},
				"stats": map[string]any{"currentAscension": map[string]any{"id": 0, "transcensionId": 2}, "currentTranscension": map[string]any{"id": 2}},
			})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "clickerHeroSave-feed.txt")
			if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(data)), 0600); err != nil {
				t.Fatal(err)
			}
			screen := exportFixture(t, "hero-economy-x1.png", 1280)
			c, err := recognizedGame(screen)
			if err != nil || !c.heroes {
				t.Fatal("Heroes classification", c, err)
			}
			c.window = "synthetic-game"
			clicks := 0
			p := newGamePipeline(&pauseControl{generation: 1}, heroInput{
				capture: func() (image.Image, error) { return screen, nil },
				click:   func(image.Point) error { clicks++; return nil },
			}, pipelineReaders{context: recognizedGame, window: func() string { return c.window }}, pipelineOptions{heroes: true, transcension: true, fishInterval: time.Second, export: &saveExportOptions{dir: dir, planOutput: filepath.Join(dir, "plan.json"), skillRate: 1}})
			p.generation, p.layout, p.startup, p.startupCheck = 1, 1, phase, false
			defer p.prestige.close()
			now := time.Now()
			p.frame = gameFrame{id: 10, generation: 1, layout: 1, at: now, image: screen, context: c}
			p.export = saveExporter{active: true, requested: true, prestigeOnly: true, step: exportReadFile, before: make(exportSnapshot), window: c.window, deadline: now.Add(10 * time.Second)}
			p.plan(now)
			if a, ok := p.nextAction(now); ok || clicks != 0 || ctl.Stage() != transcension.AwaitFeedReceipt {
				t.Fatal("pending FEED allowed another action", a, ok, clicks, ctl.Stage())
			}
			jobs := make([]chan analysisJob, analysisCount)
			for i := range jobs {
				jobs[i] = make(chan analysisJob, 1)
			}
			if err := p.capture(ctx, time.Now(), jobs); err != nil {
				t.Fatal(err)
			}
			if err := p.capture(ctx, time.Now(), jobs); err != nil {
				t.Fatal(err)
			}
			if len(jobs[exportAnalysis]) != 1 || len(jobs[heroAnalysis]) != 0 || len(jobs[transcensionAnalysis]) != 0 {
				t.Fatal("startup starved the receipt worker", len(jobs[exportAnalysis]), len(jobs[heroAnalysis]), len(jobs[transcensionAnalysis]))
			}
			result := p.analyze(ctx, exportAnalysis, <-jobs[exportAnalysis])
			if result.err != nil || result.export.transcension == nil {
				t.Fatal("receipt read", result.err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("generated receipt was not removed", err)
			}
			if err := p.accept(ctx, result, time.Now()); err != nil {
				t.Fatal(err)
			}
			ctl = p.prestige.controller
			if ctl == nil {
				t.Fatal("receipt did not restore the journal-backed controller")
			}
			if ctl.Stage() != transcension.SpendOutsiders || p.export.requested || p.export.active || p.controls.isPaused() {
				t.Fatal("receipt did not resume Outsiders", ctl.Stage(), p.export.requested, p.export.active)
			}
			p.prestigeFrame(p.frame, outsiderObservation{}, transcensionObservation{})
			p.plan(time.Now())
			if a, ok := p.nextAction(time.Now()); !ok || a.kind != handleTranscension || a.prestige.Action != transcension.OpenOutsiders || clicks != 0 {
				t.Fatal("receipt did not permit the next Outsiders action", a, ok, clicks)
			}
		})
	}
}
