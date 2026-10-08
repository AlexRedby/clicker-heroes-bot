package bot

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
	"clicker-heroes-bot/internal/vision"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Synthetic schema/receipts exercise production parsing and admission. They
// are not installed-client reset/FEED acceptance evidence.
func optInPrestigeSave(t *testing.T, cycle, ascensions, wallet, total int, souls string, feedID int) transcension.Snapshot {
	t.Helper()
	history := map[string]any{}
	for id := 0; id <= ascensions; id++ {
		history[fmt.Sprint(id)] = map[string]any{"id": id, "transcensionId": cycle, "highestZoneEver": 300, "heroSoulsEnd": fmt.Sprintf("1e%d", id+6)}
	}
	outsiders := map[string]any{}
	for _, id := range []int{1, 2, 3, 5, 6, 7, 8, 9, 10} {
		level := map[int]int{2: 6, 9: 3, 10: 3}[id]
		if id == feedID {
			level++
		}
		spent := level * (level + 1) / 2
		if id == 3 {
			spent = level
		}
		outsiders[fmt.Sprint(id)] = map[string]any{"id": id, "level": level, "spentAncientSouls": spent}
	}
	zone := 300
	if cycle > 1 {
		zone = 1
	}
	raw, err := json.Marshal(map[string]any{
		"version": 7, "readPatchNumber": "1.0e12-6144", "uniqueId": "synthetic-opt-in-profile",
		"transcendent": true, "numberOfTranscensions": cycle, "numWorldResets": 4,
		"numAscensionsThisTranscension": ascensions, "currentZoneHeight": zone, "highestFinishedZonePersist": 300,
		"ancientSouls": wallet, "ancientSoulsTotal": total, "heroSouls": souls, "heroSoulsSacrificed": "1e6",
		"primalSouls": "0", "totalHeroLevels": 0,
		"ancients": map[string]any{"ancients": map[string]any{}}, "outsiders": map[string]any{"outsiders": outsiders},
		"stats": map[string]any{"currentAscension": map[string]any{"id": ascensions, "transcensionId": cycle}, "currentTranscension": map[string]any{"id": cycle, "ascensions": history}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := transcension.ReadSnapshot(context.Background(), []byte(base64.StdEncoding.EncodeToString(raw)), time.Now(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOptInTranscensionProductionAdmission(t *testing.T) {
	requireAncientOCR(t)
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			s := optInPrestigeSave(t, 1, 4, 0, 33, "3e22", 0)
			dir := t.TempDir()
			options, err := configureRun(pipelineOptions{transcension: enabled, progression: true, export: &saveExportOptions{dir: dir, planOutput: filepath.Join(dir, "plan.json"), skillRate: 1}, fishInterval: time.Second, gildInterval: time.Minute, ascensionStall: time.Minute, ascensionMinGain: 0.25})
			if err != nil {
				t.Fatal(err)
			}
			clicks := 0
			p := newGamePipeline(&pauseControl{generation: 1}, heroInput{click: func(image.Point) error { clicks++; return nil }}, pipelineReaders{}, options)
			defer p.prestige.close()
			p.generation, p.layout, p.startup, p.startupCheck = 1, 1, noStartup, false
			setFrame := func(screen image.Image) gameFrame {
				c, err := recognizedGame(screen)
				if err != nil {
					t.Fatal(err)
				}
				p.frame = gameFrame{id: p.frame.id + 1, generation: 1, layout: 1, at: time.Now(), image: screen, context: c}
				return p.frame
			}
			setFrame(exportFixture(t, "hero-economy-x1.png", 1280))
			p.ascension = ascensionPlanner{highestZone: 300, wallZone: 300, fullCombatFailed: true, lastObservation: time.Now()}
			p.progression.wallFullCombat = true
			if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &s}, time.Now()); err != nil {
				t.Fatal(err)
			}
			p.export.requested = false // The ordinary export owner finished acquisition.
			if got := p.considerTranscension(time.Now()); got != enabled {
				t.Fatal("explicit option did not govern admission", got, enabled, p.prestige.message)
			}
			if !enabled {
				if p.prestige.controller != nil || clicks != 0 {
					t.Fatal("default mode acquired reset ownership")
				}
				return
			}
			if native := nativeTranscensionEvidence(s.State.Build); native.ResetRecovery || native.Feed || native.AncientSummon {
				t.Fatal("production factory fabricated live acceptance", native)
			}
			submit := func(want transcension.Action) transcension.Command {
				t.Helper()
				cmd := p.prestige.controller.Next(time.Now())
				if cmd.Action != want {
					t.Fatal("unexpected production command", cmd, want)
				}
				a, found := p.prestigeAction(cmd)
				if !found {
					t.Fatal("recognized control did not provide an action", cmd)
				}
				p.enqueue(a, time.Now())
				a, found = p.nextAction(time.Now())
				if !found || a.kind != handleTranscension {
					t.Fatal("production queue refused input", a, found)
				}
				if acted, err := p.execute(context.Background(), a); !acted || err != nil {
					t.Fatal("production input admission", acted, err)
				}
				return cmd
			}
			p.prestigeFrame(p.frame, outsiderObservation{}, transcensionObservation{})
			submit(transcension.OpenOutsiders)
			// The two supplied roster captures have different rewards. Compose
			// the native +79 header with the native top controls for this test.
			top := image.NewRGBA(image.Rect(0, 0, 1280, 720))
			draw.Draw(top, top.Bounds(), exportFixture(t, "outsiders-top.png", 1280), image.Point{}, draw.Src)
			r := image.Rect(420, 180, 605, 245)
			draw.Draw(top, r, exportFixture(t, "outsiders-bottom.png", 1280), r.Min, draw.Src)
			frame := setFrame(top)
			ui, err := readOutsiderObservation(context.Background(), frame)
			if err != nil {
				t.Fatal(err)
			}
			p.prestigeFrame(frame, ui, transcensionObservation{})
			submit(transcension.OpenReset)
			confirmation := exportFixture(t, "transcension-confirm.png", 1280)
			obscured := image.NewRGBA(confirmation.Bounds())
			draw.Draw(obscured, obscured.Bounds(), confirmation, image.Point{}, draw.Src)
			draw.Draw(obscured, image.Rect(529, 585, 568, 620), image.NewUniform(color.Black), image.Point{}, draw.Src)
			frame = setFrame(obscured)
			unknown, _ := readTranscensionObservation(context.Background(), frame)
			p.prestigeFrame(frame, outsiderObservation{}, unknown)
			if p.prestige.controller.Next(time.Now()).Action != transcension.NoAction || clicks != 2 {
				t.Fatal("unknown respec control authorized reset")
			}
			frame = setFrame(confirmation)
			known, err := readTranscensionObservation(context.Background(), frame)
			if err != nil {
				t.Fatal(err)
			}
			p.prestigeFrame(frame, outsiderObservation{}, known)
			submit(transcension.ConfirmReset)
			if p.prestige.controller.Stage() != transcension.AwaitResetReceipt {
				t.Fatal("reset input did not retain durable pending ownership")
			}
			after := optInPrestigeSave(t, 2, 0, 79, 112, "0", 0)
			if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &after}, time.Now()); err != nil {
				t.Fatal(err)
			}
			// Synthetic affordable caption/header variation tests the reader;
			// the supplied real FEED screenshots all have a zero wallet.
			funded := image.NewRGBA(top.Bounds())
			draw.Draw(funded, funded.Bounds(), top, image.Point{}, draw.Src)
			r = image.Rect(420, 180, 605, 198)
			draw.Draw(funded, r, image.NewUniform(color.RGBA{86, 52, 12, 255}), image.Point{}, draw.Src)
			d := font.Drawer{Dst: funded, Src: image.NewUniform(color.White), Face: basicfont.Face7x13, Dot: fixed.P(424, 194)}
			d.DrawString("79 Ancient Souls")
			plan, err := ancientcalc.PlanOutsiders(context.Background(), after.State.AncientSoulsTotal, after.State.AncientSouls, 0, after.State.Outsiders)
			if err != nil {
				t.Fatal(err)
			}
			var target ancientcalc.OutsiderTarget
			for _, row := range plan.Now.Additions {
				if row.Target > row.Current {
					target = row
					break
				}
			}
			// Put the current target in a complete native card. The value/name
			// variation is synthetic, as is the affordable wallet above.
			cardTop := outsiderCardTops(funded)[1]
			cost, err := ancientcalc.OutsiderFeedCost(target.ID, target.Current, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, label := range []struct {
				r     image.Rectangle
				text  string
				color color.Color
			}{
				{image.Rect(238, cardTop+10, 390, cardTop+36), target.Name, color.White},
				{image.Rect(442, cardTop+12, 520, cardTop+45), fmt.Sprintf("Lvl %d", target.Current), color.RGBA{255, 220, 40, 255}},
				{image.Rect(480, cardTop+94, 530, cardTop+117), fmt.Sprintf("x%d", cost), color.RGBA{255, 220, 160, 255}},
			} {
				draw.Draw(funded, label.r, image.NewUniform(color.Black), image.Point{}, draw.Src)
				d := font.Drawer{Dst: funded, Src: image.NewUniform(label.color), Face: basicfont.Face7x13, Dot: fixed.P(label.r.Min.X+2, label.r.Max.Y-3)}
				d.DrawString(label.text)
			}
			frame = setFrame(funded)
			ui, err = readOutsiderObservation(context.Background(), frame)
			if err != nil {
				t.Fatal(err)
			}
			p.prestigeFrame(frame, ui, transcensionObservation{})
			if cmd := p.prestige.controller.Next(time.Now()); cmd.Action != transcension.NoAction {
				t.Fatal("affordable wallet authorized dim FEED", cmd)
			}
			for _, top := range outsiderCardTops(funded) {
				r := image.Rect(449, top+73, 513, top+94)
				// Preserve native artwork and antialiasing while varying its
				// brightness. Flattening only letter cores changes the shape.
				for y := r.Min.Y; y < r.Max.Y; y++ {
					for x := r.Min.X; x < r.Max.X; x++ {
						red, green, blue := rgb(funded.At(x, y))
						funded.Set(x, y, color.RGBA{uint8(min(255, red*2)), uint8(min(255, green*2)), uint8(min(255, blue*2)), 255})
					}
				}
			}
			frame = setFrame(funded)
			ui, err = readOutsiderObservation(context.Background(), frame)
			if err != nil {
				t.Fatal(err)
			}
			p.prestigeFrame(frame, ui, transcensionObservation{})
			cmd := submit(transcension.FeedOutsider)
			if cmd.Quantity != 1 || clicks != 4 || p.prestige.controller.Stage() != transcension.AwaitFeedReceipt {
				t.Fatal("FEED not reserved exactly once", cmd, clicks)
			}
			receipt := optInPrestigeSave(t, 2, 0, 79-cmd.Cost, 112, "0", cmd.ID)
			if err := p.acceptPrestigeSave(context.Background(), exportResult{transcension: &receipt}, time.Now()); err != nil || p.prestige.controller.Stage() != transcension.SpendOutsiders {
				t.Fatal("exact FEED receipt rejected", err)
			}
		})
	}
}

func TestConcreteOutsiderControls(t *testing.T) {
	for _, width := range []int{1280, 1920, 2560} {
		for _, name := range []string{"hero-economy-x1.png", "outsiders-top.png", "outsiders-bottom.png"} {
			screen := exportFixture(t, name, width)
			if _, known := outsiderEntryPoint(screen); !known {
				t.Fatal("native Outsiders tab missing", name, width)
			}
			if name != "hero-economy-x1.png" && !outsiderQuantityControlsKnown(screen) {
				t.Fatal("native fixed-quantity controls missing", name, width)
			}
		}
	}
	screen := exportFixture(t, "outsiders-top.png", 1280)
	// Selected backgrounds are a synthetic variation; their fixed captions
	// still have to match. MAX never supplies a bounded FEED quantity.
	for selected := 0; selected < 5; selected++ {
		changed := image.NewRGBA(screen.Bounds())
		draw.Draw(changed, changed.Bounds(), screen, image.Point{}, draw.Src)
		for i, left := range []int{112, 211, 311, 411, 510} {
			for y := 259; y < 284; y++ {
				for x := left; x < left+87; x++ {
					red, green, blue := rgb(changed.At(x, y))
					if red >= 180 && green >= 90 && green < 240 && blue < 100 {
						value := color.RGBA{255, 220, 43, 255}
						if i == selected {
							value = color.RGBA{238, 149, 32, 255}
						}
						changed.Set(x, y, value)
					}
				}
			}
		}
		if !outsiderQuantityControlsKnown(changed) || outsiderQuantity(changed) != []string{"x1", "x10", "x100", "x1000", "MAX"}[selected] {
			t.Fatal("selected quantity changed control recognition", selected)
		}
	}
	unknown := image.NewRGBA(screen.Bounds())
	draw.Draw(unknown, unknown.Bounds(), screen, image.Point{}, draw.Src)
	draw.Draw(unknown, vision.Rect(screen, image.Rect(539, 130, 576, 159)), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if _, known := outsiderEntryPoint(unknown); known {
		t.Fatal("missing tab icon admitted entry")
	}
	draw.Draw(unknown, image.Rect(425, 259, 485, 284), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if outsiderQuantityControlsKnown(unknown) {
		t.Fatal("unknown quantity controls admitted selection")
	}
}
