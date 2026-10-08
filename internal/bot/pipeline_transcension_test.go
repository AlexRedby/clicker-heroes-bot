package bot

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func TestRunTranscensionIsExplicit(t *testing.T) {
	options := pipelineOptions{progression: true, export: &saveExportOptions{dir: t.TempDir()}, fishInterval: time.Second, gildInterval: time.Minute, ascensionStall: time.Minute, ascensionMinGain: 0.25}
	got, err := configureRun(options)
	if err != nil || got.transcension {
		t.Fatal("ordinary progression enabled reset integration", got, err)
	}
	options.progression, options.transcension = false, true
	if _, err := configureRun(options); err == nil {
		t.Fatal("prestige without ordinary recovery accepted")
	}
}

func TestPrestigeQueueModalAndRecoveredReset(t *testing.T) {
	now := time.Now()
	zone, cycle, asc, gain := 300, 0, 6, 79
	s := transcension.Snapshot{Generation: 1, ExportedAt: now, State: ancientcalc.TranscensionState{
		SaveHash: strings.Repeat("a", 64), ProfileID: strings.Repeat("b", 64), Build: "1.0e12-6144", SaveVersion: 7,
		Ascensions: 6, AscensionsThisTranscension: 6, HighestZone: 300, CurrentZone: &zone,
		CurrentTranscensionID: &cycle, CurrentAscensionCycleID: &cycle, CurrentAscensionID: &asc, HeroSouls: "100", Ancients: []ancientcalc.Level{},
	}}
	for i, id := range []int{1, 2, 3, 5, 6, 7, 8, 9, 10} {
		s.State.Outsiders = append(s.State.Outsiders, ancientcalc.Level{ID: id, Name: []string{"Xyliqil", "Chor'gorloth", "Phandoryss", "Ponyboy", "Borb", "Rhageist", "K'Ariqua", "Orphalas", "Sen-Akhan"}[i], Level: "0"})
	}
	s.Preview = ancientcalc.TranscensionPreview{SaveHash: s.State.SaveHash, Build: s.State.Build, SaveVersion: 7, Ascensions: 6, HighestZone: 300, EstimatedASGain: &gain}
	journal, err := transcension.OpenJournal(filepath.Join(t.TempDir(), "journal.json"), s.State.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	// Injected evidence exercises the input boundary; production evidence remains unavailable.
	ctl := transcension.NewController(transcension.Policy{Enabled: true, SkillRate: 1}, transcension.NativeEvidence{Build: s.State.Build, ResetRecovery: true, Feed: true, AncientSummon: true})
	if err := ctl.BindJournal(journal); err != nil {
		t.Fatal(err)
	}
	summon := transcension.NewSummonPlanner(transcension.Policy{Enabled: true, SkillRate: 1}, transcension.NativeEvidence{Build: s.State.Build, AncientSummon: true})
	if err := summon.BindController(ctl); err != nil {
		t.Fatal(err)
	}
	timing := transcension.Timing{At: now, Generation: 1, SaveHash: s.State.SaveHash, CompletedLoops: 2, WallConfirmed: true}
	if err := ctl.Begin(context.Background(), now, s, timing); err != nil {
		t.Fatal(err)
	}
	out := transcension.Observation{Frame: 1, Generation: 1, Layout: 1, At: now, Screen: transcension.OutsidersScreen, Known: true, Reward: gain, OpenKnown: true}
	for _, r := range s.State.Outsiders {
		cost, e := ancientcalc.OutsiderFeedCost(r.ID, 0, 1)
		if e != nil {
			t.Fatal(e)
		}
		out.Rows = append(out.Rows, transcension.Row{ID: r.ID, Name: r.Name, Cost: cost})
	}
	ctl.Observe(out)
	if cmd := ctl.Next(now); cmd.Action != transcension.OpenReset {
		t.Fatal("reset entry", cmd)
	} else if err := ctl.Reserve(now, cmd); err != nil {
		t.Fatal(err)
	}
	screen := exportFixture(t, "transcension-confirm.png", 1280)
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	old := gameFrame{id: 2, generation: 1, layout: 1, at: now, image: screen, context: c}
	clicks := 0
	p := newGamePipeline(&pauseControl{generation: 1}, heroInput{click: func(image.Point) error { clicks++; return nil }}, pipelineReaders{}, pipelineOptions{transcension: true, heroes: true})
	p.generation, p.layout = 1, 1
	p.prestige = prestigePipeline{controller: ctl, summon: summon, snapshot: s}
	p.prestigeFrame(old, outsiderObservation{}, transcensionObservation{frame: old, known: true, confirm: true, no: true, respecKnown: true, reward: gain})
	p.frame = old
	p.frame.id = 3
	cmd := ctl.Next(now)
	a, ok := p.prestigeAction(cmd)
	if !ok || !p.prestigeActionStable(a, now) {
		t.Fatal("unchanged newer capture invalidated owned observation", cmd, a, ok)
	}
	changed := image.NewRGBA(screen.Bounds())
	draw.Draw(changed, changed.Bounds(), screen, screen.Bounds().Min, draw.Src)
	draw.Draw(changed, image.Rect(413, 520, 518, 561), image.NewUniform(color.Black), image.Point{}, draw.Src)
	p.frame.image = changed
	if p.prestigeActionStable(a, now) {
		t.Fatal("missing Yes accepted")
	}
	p.frame.image = screen
	p.enqueue(a, now)
	selected, ok := p.nextAction(now)
	if !ok || selected.kind != handleTranscension {
		t.Fatal("owned modal cancelled or deadlocked", selected, ok)
	}
	if acted, err := p.execute(context.Background(), selected); !acted || err != nil || clicks != 1 || ctl.Stage() != transcension.AwaitResetReceipt {
		t.Fatal("submission not reserved once", acted, err, clicks, ctl.Stage())
	}
	if acted, _ := p.execute(context.Background(), selected); acted || clicks != 1 {
		t.Fatal("pending reset replayed")
	}
	ctl.Interrupt()
	p.ancient = ancientPlanner{blocked: true}
	p.gild.active = true
	p.clickers.blocked = true
	fresh := s
	fresh.Generation = 2
	fresh.ExportedAt = now.Add(time.Second)
	fresh.State.SaveHash = strings.Repeat("c", 64)
	fresh.State.Transcensions, fresh.State.Transcendent, fresh.State.AscensionsThisTranscension = 1, true, 0
	*fresh.State.CurrentZone, *fresh.State.CurrentTranscensionID, *fresh.State.CurrentAscensionCycleID, *fresh.State.CurrentAscensionID = 1, 1, 1, 0
	fresh.State.AncientSouls, fresh.State.AncientSoulsTotal, fresh.State.HeroSouls = gain, gain, "0"
	if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &fresh}, fresh.ExportedAt); err != nil {
		t.Fatal(err)
	}
	if ctl.Stage() != transcension.SpendOutsiders || p.ancient.blocked || p.gild.active || p.clickers.blocked {
		t.Fatal("recovered reset retained old game plans", ctl.Stage(), p.ancient, p.gild, p.clickers)
	}

	// Complete the pure controller spending loop without native input.
	quantity, frame := 1, uint64(10)
	for attempts := 0; attempts < 100 && ctl.Stage() != transcension.AwaitFirstSouls; attempts++ {
		at := fresh.ExportedAt.Add(time.Second)
		ui := transcension.Observation{Frame: frame, Generation: 2, Layout: 1, At: at, Screen: transcension.OutsidersScreen, Known: true, Wallet: fresh.State.AncientSouls, Quantity: quantity, QuantityKnown: true}
		for _, row := range fresh.State.Outsiders {
			level, _ := strconv.Atoi(row.Level)
			cost, err := ancientcalc.OutsiderFeedCost(row.ID, level, quantity)
			if err != nil {
				t.Fatal(err)
			}
			ui.Rows = append(ui.Rows, transcension.Row{ID: row.ID, Name: row.Name, Level: level, Cost: cost, FeedKnown: true})
		}
		ctl.Observe(ui)
		command := ctl.Next(at)
		if command.Action == transcension.NoAction {
			t.Fatal("restoration stalled", ctl.Stage(), ctl.Reason())
		}
		if err := ctl.Reserve(at, command); err != nil {
			t.Fatal(err)
		}
		switch command.Action {
		case transcension.SelectQuantity:
			quantity = command.Quantity
		case transcension.FeedOutsider:
			fresh.State.Outsiders = append([]ancientcalc.Level(nil), fresh.State.Outsiders...)
			for i, row := range fresh.State.Outsiders {
				if row.ID == command.ID {
					level, _ := strconv.Atoi(row.Level)
					fresh.State.Outsiders[i].Level = strconv.Itoa(level + command.Quantity)
				}
			}
			fresh.State.AncientSouls -= command.Cost
			fresh.State.SaveHash = fmt.Sprintf("%064x", frame+100)
			fresh.ExportedAt = at.Add(time.Second)
			if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &fresh}, fresh.ExportedAt); err != nil {
				t.Fatal(err)
			}
		case transcension.BootstrapHeroes:
		default:
			t.Fatal("unexpected restoration action", command)
		}
		frame++
	}
	if ctl.Stage() != transcension.AwaitFirstSouls {
		t.Fatal("spending did not yield ordinary earning", ctl.Stage())
	}
	// F8 inside an ordinary Ascension dialog must cancel before exporting.
	p.controls.generation = 2
	p.generation = 2
	p.controls.toggle()
	p.controls.toggle()
	p.reset(4)
	modal := exportFixture(t, "ascension-confirm.png", 1280)
	modalContext, err := recognizedGame(modal)
	if err != nil {
		t.Fatal(err)
	}
	p.frame = gameFrame{id: frame, generation: 4, layout: p.layout, at: fresh.ExportedAt, image: modal, context: modalContext}
	p.plan(fresh.ExportedAt)
	action, ok := p.nextAction(fresh.ExportedAt)
	if !ok || action.kind != navigateGame || action.navigation != navigationAscension {
		t.Fatal("F8 modal prevented free cancellation", action, ok)
	}
	fresh.Generation = 4
	fresh.ExportedAt = fresh.ExportedAt.Add(2 * time.Second)
	if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &fresh}, fresh.ExportedAt); err != nil {
		t.Fatal(err)
	}
	// Relic-only exports must reach their owner while Ancient allocation is held.
	fresh.ExportedAt = fresh.ExportedAt.Add(time.Second)
	p.frame = testPipelineFrame()
	p.frame.id = frame + 1
	p.frame.generation = 4
	p.frame.layout = p.layout
	p.frame.context.heroes = true
	p.export = saveExporter{requested: true, active: true, relicsOnly: true, step: exportReadFile, jobFrame: p.frame.id, window: p.frame.context.window}
	p.relic = relicPlanner{active: true, step: relicAcquire}
	preview := ancientcalc.RelicPreview{Snapshot: ancientcalc.RelicSnapshot{EquipmentSlots: 4, Items: []ancientcalc.Relic{}, AncientLevels: map[int]string{}, OutsiderLevels: map[int]string{}}}
	observation := observation{kind: exportAnalysis, frame: p.frame, export: exportResult{transcension: &fresh, relics: &preview}}
	if err := p.applyObservation(context.Background(), observation, fresh.ExportedAt); err != nil {
		t.Fatal(err)
	}
	if p.relic.step != relicOpenTab || p.export.requested {
		t.Fatal("relic-only export discarded while earning", p.relic.step, p.export)
	}
}
