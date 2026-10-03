package main

import (
	"bytes"
	"clicker-heroes-bot/internal/ancientcalc"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-vgo/robotgo"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func testIntegratedGildSave(t *testing.T, includeGild bool) []byte {
	t.Helper()
	payload := map[string]any{
		"heroSouls": "160", "heroSoulsSacrificed": 0, "highestFinishedZonePersist": "42",
		"ancientSoulsTotal": 1, "numWorldResets": 3, "transcendent": true,
		"ancients":  map[string]any{"ancients": map[string]any{"19": map[string]any{"level": "1", "spentHeroSouls": "1"}}},
		"outsiders": map[string]any{"outsiders": map[string]any{"1": map[string]any{"level": "0"}}},
	}
	if includeGild {
		heroes := make(map[string]any, 54)
		for id := 1; id <= 54; id++ {
			heroes[fmt.Sprint(id)] = map[string]any{"id": id, "uid": id, "level": 0, "epicLevel": 0, "locked": false}
		}
		heroes["42"] = map[string]any{"id": 42, "uid": 42, "level": 1000, "epicLevel": 0, "locked": false}
		heroes["43"] = map[string]any{"id": 43, "uid": 43, "level": 0, "epicLevel": 2, "locked": true}
		payload["numberOfTranscensions"] = 1
		payload["numAscensionsThisTranscension"] = 3
		payload["version"] = 7
		payload["readPatchNumber"] = "1.0e12-6144"
		payload["ancientSouls"] = 0
		payload["primalSouls"] = 0
		payload["totalHeroLevels"] = 1000
		payload["transcensionTimestamp"] = 100
		payload["heroCollection"] = map[string]any{"heroes": heroes}
		payload["upgrades"] = map[string]bool{"200": true}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte("7a990d405d2c6fb93aa8fbb0ec1a3b23" + base64.StdEncoding.EncodeToString(compressed.Bytes()))
}

func TestAncientNavigationControlsAndClipping(t *testing.T) {
	requireAncientOCR(t)
	for _, file := range []string{"ascension-ancients.png", "ancient-v-held.png", "ancient-dora-submitted.png", "ancient-energon-leveled.png", "ancient-kumawakamaru-leveled.png"} {
		for _, width := range []int{1280, 2560} {
			t.Run(fmt.Sprintf("%s/%d", file, width), func(t *testing.T) {
				original := loadTestImage(t, "testdata/"+file)
				screen := image.NewRGBA(image.Rect(0, 0, width, width*9/16))
				xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
				for _, direction := range []int{-1, 1} {
					point, found, err := ancientScrollArrow(screen, direction)
					if !found || err != nil || point.X != width*586/1280 {
						t.Fatal("native arrow", direction, point, found, err)
					}
				}
				out, err := readAncientNames(context.Background(), gameFrame{image: screen, context: gameContext{ancients: true}})
				if err != nil || len(out.anchors) < 2 {
					t.Fatal("readable navigation names", out.anchors, err)
				}
				if file == "ancient-energon-leveled.png" {
					clipped := "Fortuna"
					for _, point := range ancientButtons(screen) {
						region := ancientNameRegion(screen, point)
						for _, anchor := range out.anchors {
							if anchor.name == clipped && anchor.y >= region.Min.Y && anchor.y < region.Max.Y {
								t.Fatal("clipped button became a purchase candidate", anchor, point)
							}
						}
					}
				}
				if file == "ancient-dora-submitted.png" {
					found := false
					for _, point := range ancientButtons(screen) {
						region := ancientNameRegion(screen, point)
						for _, anchor := range out.anchors {
							if anchor.name == "Dora" && anchor.y >= region.Min.Y && anchor.y < region.Max.Y {
								found = true
							}
						}
					}
					if !found {
						t.Fatal("Dora's complete button was rejected because its portrait is clipped")
					}
				}
				if file == "ascension-ancients.png" {
					out, err = readAncientObservation(context.Background(), gameFrame{image: screen, context: gameContext{ancients: true}})
					if err != nil {
						t.Fatal(err)
					}
					for _, row := range out.rows {
						if row.name == "Atman" {
							r := ancientLevelRegion(screen, row.point)
							draw.Draw(screen, r, image.NewUniform(color.Black), image.Point{}, draw.Src)
						}
					}
					out, err = readAncientObservation(context.Background(), gameFrame{image: screen, context: gameContext{ancients: true}})
					found := false
					for _, row := range out.rows {
						if row.name == "Atman" && row.level == "" {
							found = true
						}
					}
					if err != nil || !found {
						t.Fatal("numeric OCR failure erased the navigation name", out, err)
					}
				}
			})
		}
	}
	covered := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for _, direction := range []int{-1, 1} {
		if _, found, err := ancientScrollArrow(covered, direction); found || err != nil {
			t.Fatal("covered arrow accepted", direction, err)
		}
	}
}

func TestAncientAlphabeticalSeek(t *testing.T) {
	exported, err := calculateAncients(context.Background(), "internal/ancientcalc/testdata/ancient-save.txt", "1%", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(exported.Owned))
	for _, owned := range exported.Owned {
		names = append(names, owned.Name)
	}
	sort.Strings(names)
	original := loadTestImage(t, "testdata/ascension-ancients.png")
	base := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	xdraw.CatmullRom.Scale(base, base.Bounds(), original, original.Bounds(), draw.Src, nil)
	// Displacements deliberately differ from card pitch; these are model values,
	// not a measurement of the installed game's wheel or arrow increment.
	for _, tc := range []struct {
		name                string
		start, wheel, arrow int
		missing, stalled    bool
		only                string
	}{
		{"top", 0, 163, 8, false, false, ""},
		{"middle", 1500, 181, 11, false, false, ""},
		{"bottom", 9999, 169, 9, false, false, ""},
		{"missing", 0, 163, 8, true, false, ""},
		{"no-motion", 0, 163, 8, false, true, ""},
		{"wheel-no-motion", 0, 0, 8, false, false, ""},
		{"Fragsworth-wheel-no-motion", sort.SearchStrings(names, "Chawedo")*148 + 16, 0, 8, false, false, "Fragsworth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "1000", Reserve: "1", Owned: exported.Owned}}
			for i := len(names) - 1; i >= 0; i-- {
				if tc.only != "" && names[i] != tc.only {
					continue
				}
				plan.Rows = append(plan.Rows, ancientcalc.Purchase{Name: names[i], Current: "1", Quantity: "1", Cost: "1"})
			}
			roster := append([]string(nil), names...)
			if tc.missing {
				roster = append(roster[:8], roster[9:]...)
			}
			limit := (len(roster) - 3) * 148
			offset := min(tc.start, limit)
			p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{}, seekTarget: -1}
			frame := gameFrame{image: base, context: gameContext{known: true, ancients: true, bounds: base.Bounds(), window: "game"}}
			now := time.Now()
			fine, coarse := 0, 0
			for n := 0; n < 1000 && !p.finished && !p.blocked; n++ {
				now = now.Add(time.Second)
				frame.id++
				frame.at = now
				screen := image.NewRGBA(base.Bounds())
				draw.Draw(screen, screen.Bounds(), base, image.Point{}, draw.Src)
				strip := image.Rect(83, 288, 90, 691)
				draw.Draw(screen, strip, image.NewUniform(color.Black), image.Point{}, draw.Src)
				out := ancientObservation{frame: frame, namesOnly: p.budgetChecked && p.selected < 0 && !p.needFullRead}
				for i, name := range roster {
					y := 300 + i*148 - offset
					button := image.Rect(83, y+31, 90, y+89).Intersect(strip)
					draw.Draw(screen, button, image.NewUniform(color.RGBA{40, 150, 230, 255}), image.Point{}, draw.Src)
					if y >= 275 && y < 699 {
						out.anchors = append(out.anchors, ancientNameAnchor{name, y})
					}
				}
				frame.image, out.frame.image = screen, screen
				if !out.namesOnly {
					out.souls = fmt.Sprint(1000 - len(p.done))
					for _, point := range ancientButtons(screen) {
						region := ancientNameRegion(screen, point)
						for _, anchor := range out.anchors {
							if anchor.y >= region.Min.Y && anchor.y < region.Max.Y {
								out.rows = append(out.rows, ancientScreenRow{anchor.name, "1", point})
							}
						}
					}
				}
				p.observe(out, nil, now)
				a, ok := p.action(frame, now)
				if !ok {
					continue
				}
				switch a.ancient.step {
				case scrollAncients:
					if !ancientActionStable(a, frame) {
						t.Fatal("invalid native navigation control", a)
					}
					step := tc.wheel
					if a.ancient.fine {
						fine++
						step = tc.arrow
					} else {
						coarse++
					}
					if !tc.stalled {
						offset = max(0, min(limit, offset+a.ancient.direction*step))
					}
					p.sent(a, now)
				case openAncientQuantity:
					if out.namesOnly || p.done[p.selected] {
						t.Fatal("name-only or repeated purchase", p.selected)
					}
					// The separate transaction tests cover V/entry/OK; feed its owned
					// one-shot submission here, then require a fresh wallet next time.
					owner := frame
					owner.context.ancients, owner.context.ancientDialog = false, true
					p.sent(gameAction{frame: owner, ancient: ancientCommand{step: confirmAncientQuantity}}, now)
					frame.id++
					p.observe(ancientObservation{frame: frame}, nil, now)
					if !p.needFullRead || p.selected != -1 {
						t.Fatal("submission did not require the next full read")
					}
				case returnAncientHeroes:
					p.finished = true
				default:
					t.Fatal("unexpected navigation action", a)
				}
			}
			if tc.missing || tc.stalled {
				if !p.blocked || !strings.Contains(p.failure, "target=") || !strings.Contains(p.failure, "neighbors=") || p.seekClicks > 128 {
					t.Fatal("missing/no-motion navigation did not stop with diagnostics", p.failure)
				}
			} else if !p.finished || len(p.done) != len(plan.Rows) || coarse == 0 || tc.name != "bottom" && fine == 0 {
				t.Fatal("alphabetical batch incomplete", len(p.done), fine, coarse, p.failure)
			}
			if tc.only == "Fragsworth" && (coarse != 2 || fine <= 24) {
				t.Fatal("distant target did not get one bounded arrow recovery", coarse, fine)
			}
			if tc.stalled && (coarse != 2 || fine != 3) {
				t.Fatal("stalled arrows inherited wheel failures or retried forever", coarse, fine)
			}
		})
	}
}

func TestAncientFineSeekUsesNamesWithoutRepeatedWalletOCR(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/ancient-dora-submitted.png")
	frame := gameFrame{id: 1, image: screen, context: gameContext{known: true, ancients: true, bounds: screen.Bounds()}}
	// Shift the real card crop down so Dora's button, rather than its portrait,
	// is clipped at the physical list edge. Keep the native arrow untouched.
	shifted := image.NewRGBA(screen.Bounds())
	draw.Draw(shifted, shifted.Bounds(), screen, image.Point{}, draw.Src)
	panel := controlRect(screen, image.Rect(40, 275, 570, 718))
	draw.Draw(shifted, panel, screen, panel.Min.Sub(image.Pt(0, 60)), draw.Src)
	frame.image = shifted
	out, err := readAncientNames(context.Background(), frame)
	if err != nil {
		t.Fatal(err)
	}
	plan := ancientPlan{Plan: ancientcalc.Plan{Owned: []ancientcalc.Level{{Name: "Chronos"}, {Name: "Dogcog"}, {Name: "Dora"}}, Rows: []ancientcalc.Purchase{{Name: "Dora"}}}}
	p := ancientPlanner{plan: &plan, active: true, budgetChecked: true, selected: -1, done: map[int]bool{}}
	now := time.Now()
	for range 2 {
		p.observe(out, nil, now)
		a, ok := p.action(out.frame, now)
		if !ok || a.ancient.step != scrollAncients || !a.ancient.fine || a.ancient.direction != 1 || p.needFullRead {
			t.Fatal("clipped name waited for wallet/level OCR", a, p.needFullRead)
		}
		p.sent(a, now)
		now = now.Add(time.Second)
		out.frame.id++
		for i := range out.anchors {
			out.anchors[i].y -= 6
		}
	}
	p.observe(out, nil, now)
	if p.pending != nil || p.seekStalls != 0 {
		t.Fatal("small content movement with unchanged names was not acknowledged")
	}
}

func TestAncientUnknownNeighborsAreBounded(t *testing.T) {
	for _, anchors := range [][]ancientNameAnchor{
		{{"garbled", 700}},
		{{"Dora", 700}, {"Atman", 1000}},
	} {
		plan := ancientPlan{Plan: ancientcalc.Plan{Owned: []ancientcalc.Level{{Name: "Atman"}, {Name: "Dora"}}, Rows: []ancientcalc.Purchase{{Name: "Atman"}}}}
		p := ancientPlanner{plan: &plan, active: true, budgetChecked: true, selected: -1, done: map[int]bool{}}
		now := time.Now()
		for i := 1; i <= 5 && !p.blocked; i++ {
			frame := gameFrame{id: uint64(i), context: gameContext{known: true, ancients: true}}
			p.observe(ancientObservation{frame: frame, namesOnly: true, anchors: anchors}, nil, now)
			if _, ok := p.action(frame, now); ok {
				t.Fatal("unknown or misordered names authorized input")
			}
			now = now.Add(time.Second)
		}
		if !p.blocked || !strings.Contains(p.failure, "target=Atman") {
			t.Fatal("unknown names did not stop with target diagnostics", p.failure)
		}
	}
}

func TestAncientNavigationDispatchAndF8(t *testing.T) {
	screen := loadTestImage(t, "testdata/ascension-ancients.png")
	for _, fine := range []bool{false, true} {
		controls := pauseControl{}
		wheel, clicks, parks := 0, 0, 0
		frame := gameFrame{image: screen, context: gameContext{known: true, ancients: true, bounds: screen.Bounds()}}
		point := controlRect(screen, image.Rect(350, 510, 351, 511)).Min
		if fine {
			point, _, _ = ancientScrollArrow(screen, 1)
		}
		a := gameAction{kind: handleAncient, frame: frame, point: point, ancient: ancientCommand{step: scrollAncients, direction: 1, fine: fine}}
		p := newGamePipeline(&controls, heroInput{
			click: func(point image.Point) error { clicks++; return nil },
			scroll: func(point image.Point, direction int) error {
				if direction != 1 || point != a.point {
					t.Fatal("wheel primitive changed")
				}
				wheel++
				return nil
			},
			move: func(image.Point) error { parks++; return nil },
		}, pipelineReaders{}, pipelineOptions{})
		if acted, err := p.execute(context.Background(), a); !acted || err != nil || wheel+clicks != 1 || parks != 1 || (fine && wheel != 0) || (!fine && clicks != 0) {
			t.Fatal("navigation did not emit exactly one wheel/arrow primitive", acted, err, wheel, clicks, parks)
		}
		controls.toggle()
		controls.toggle()
		if acted, err := p.execute(context.Background(), a); acted || err != nil || wheel+clicks != 1 {
			t.Fatal("pre-F8 navigation input was replayed", acted, err)
		}
		changed := frame
		changed.context.window = "foreign"
		if ancientActionStable(a, changed) {
			t.Fatal("foreign context retained navigation action")
		}
		a.ancient.direction = 2
		if ancientActionStable(a, frame) {
			t.Fatal("non-unit wheel direction accepted")
		}
	}
}

func requireAncientOCR(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
}

func TestAncientRealScreen(t *testing.T) {
	requireAncientOCR(t)
	for _, name := range []string{"ascension-ancients.png", "ancient-v-held.png", "ancient-quantity.png"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			img, err := png.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if ancientQuantityDialog(img) != (name == "ancient-quantity.png") {
				t.Fatal("quantity dialog classification")
			}
			if name == "ancient-quantity.png" {
				if _, found, err := ancientControl(img, 1); err != nil || !found {
					t.Fatal("OK button not recognized", err)
				}
				out, err := readAncientObservation(context.Background(), gameFrame{image: img, context: gameContext{ancientDialog: true}})
				if err != nil || !out.okay {
					t.Fatalf("empty dialog: %+v %v", out, err)
				}
				return
			}
			if !ancientTabSelected(img) {
				t.Fatal("Ancients tab not recognized")
			}
			out, err := readAncientObservation(context.Background(), gameFrame{image: img, context: gameContext{ancients: true}})
			if err != nil {
				t.Fatal(err)
			}
			if out.souls != "1.755e58" {
				t.Fatal("current wallet confused with pending Ascension souls", out.souls)
			}
			t.Logf("souls=%s rows=%+v anchors=%v", out.souls, out.rows, out.anchors)
			if len(out.rows) != 3 || out.rows[0].name != "Argaiv" || out.rows[0].level != "1.000e28" || out.rows[1].name != "Atman" || out.rows[1].level != "164" {
				t.Fatal("Ancient row recognition", out.rows)
			}

		})
	}
}

func TestAncientPlanExport(t *testing.T) {
	plan, err := calculateAncients(context.Background(), "internal/ancientcalc/testdata/ancient-save.txt", "1%", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Owned) != 26 || len(plan.Rows) != 22 || plan.Ascensions != 93 || plan.SaveHash == "" {
		t.Fatalf("unexpected exported state: owned=%d purchases=%d ascensions=%d", len(plan.Owned), len(plan.Rows), plan.Ascensions)
	}
	if err := writeAncientPlan("internal/ancientcalc/testdata/ancient-save.txt", plan); err == nil {
		t.Fatal("export could be overwritten")
	}
	if err := writeAncientPlan(filepath.Join(t.TempDir(), "plan.json"), plan); err != nil {
		t.Fatal(err)
	}
	for _, row := range plan.Rows {
		quantity, err := ancientcalc.InputQuantity(row.Quantity)
		if err != nil || len(quantity) > 24 {
			t.Fatalf("unusable quantity %q: %v", quantity, err)
		}
	}
}

func TestAncientPlanIncludesReadOnlyGildPreview(t *testing.T) {
	save := testIntegratedGildSave(t, true)
	path := filepath.Join(t.TempDir(), "clickerHeroSave.txt")
	plan, err := calculateAncientData(context.Background(), save, path, "10%", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Gilds == nil || plan.GildError != "" || plan.Gilds.Cost != "160" || plan.Gilds.Reserve != "16" || !plan.Gilds.PreviewOnly || plan.Gilds.Eligible {
		t.Fatalf("gild preview: %+v error=%q", plan.Gilds, plan.GildError)
	}
	base, err := ancientcalc.Calculate(context.Background(), save, "10%", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Souls != base.Souls || plan.Spent != base.Spent || plan.Remaining != base.Remaining {
		t.Fatalf("gild preview changed Ancient budget: plan=%+v base=%+v", plan.Plan, base)
	}
	wantHash := sha256.Sum256(save)
	if plan.Transcension == nil || plan.TranscensionError != "" || plan.Transcension.UIVerified || plan.Transcension.SaveHash != plan.SaveHash {
		t.Fatalf("Transcension preview: %+v error=%q", plan.Transcension, plan.TranscensionError)
	}
	if plan.SaveHash != fmt.Sprintf("%x", wantHash) || plan.Gilds.SaveHash != plan.SaveHash {
		t.Fatalf("save hash=%q", plan.SaveHash)
	}
	out := filepath.Join(t.TempDir(), "plan.json")
	if err := writeAncientPlan(out, plan); err != nil {
		t.Fatal(err)
	}
	encoded, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted["transcension"]; !ok {
		t.Fatal("persisted plan omitted Transcension preview")
	}
	if _, ok := persisted["gilds"]; !ok {
		t.Fatal("persisted plan omitted gild preview")
	}
}

func TestAncientPlanSurvivesMissingGildMetadata(t *testing.T) {
	plan, err := calculateAncientData(context.Background(), testIntegratedGildSave(t, false), "synthetic-save", "0", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Transcension != nil || plan.TranscensionError == "" {
		t.Fatal("missing prestige metadata did not remain advisory")
	}
	if plan.Gilds != nil || plan.GildError == "" || plan.Souls == "" {
		t.Fatalf("missing gild metadata: gilds=%+v error=%q souls=%q", plan.Gilds, plan.GildError, plan.Souls)
	}
}

func TestAncientPlanGildPreviewHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := calculateAncientData(ctx, testIntegratedGildSave(t, true), "synthetic-save", "0", 1, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}
}

func TestAncientTransaction(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/ascension-ancients.png")
	dialog := loadTestImage(t, "testdata/ancient-quantity.png")
	plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "1.7554538149018e58", Reserve: "1e56", Rows: []ancientcalc.Purchase{{ID: 1, Name: "Argaiv", Current: "1e28", Target: "2e28", Quantity: "1e28", Cost: "1e50"}}}}
	p := ancientPlanner{plan: &plan}
	frame := gameFrame{id: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	a, ok := p.action(frame, now)
	if !ok || a.ancient.step != visitAncients {
		t.Fatal("did not visit Ancients")
	}
	p.sent(a, now)
	frame.id++
	frame.context.heroes = false
	frame.context.ancients = true
	out := ancientObservation{frame: frame, souls: "1.755e58", rows: []ancientScreenRow{{"Argaiv", "1.000e28", image.Pt(243, 711)}}}
	p.observe(out, nil, now.Add(time.Second))
	a, ok = p.action(frame, now.Add(time.Second))
	if !ok || a.ancient.step != openAncientQuantity {
		t.Fatal("did not select owned Ancient", p.blocked)
	}
	p.sent(a, now)
	frame.id++
	frame.image = dialog
	frame.context.ancients = false
	frame.context.ancientDialog = true
	p.observe(ancientObservation{frame: frame, okay: true}, nil, now.Add(time.Second))
	a, ok = p.action(frame, now.Add(time.Second))
	if !ok || a.ancient.step != fillAncientQuantity {
		t.Fatal("did not fill quantity")
	}
	p.sent(a, now)
	frame.id++
	p.observe(ancientObservation{frame: frame, okay: true}, nil, now.Add(time.Second))
	a, ok = p.action(frame, now.Add(time.Second))
	if !ok || a.ancient.step != confirmAncientQuantity {
		t.Fatal("quantity did not authorize confirmation")
	}
	p.sent(a, now)
	frame.id++
	p.observe(ancientObservation{frame: frame, okay: true}, nil, now.Add(time.Second))
	if p.pending == nil || len(p.done) != 0 {
		t.Fatal("open dialog acknowledged")
	}
	if _, ok := p.action(frame, now.Add(2*time.Second)); ok {
		t.Fatal("OK was repeated")
	}
	out.frame.id = frame.id + 1
	p.observe(ancientObservation{frame: out.frame}, nil, now.Add(2*time.Second))
	if p.pending != nil || !p.done[0] {
		t.Fatal("closed owned dialog not acknowledged")
	}
	a, ok = p.action(out.frame, now.Add(2*time.Second))
	if !ok || a.ancient.step != returnAncientHeroes {
		t.Fatal("did not return to Heroes")
	}
	p.sent(a, now)
	frame = out.frame
	frame.id++
	frame.context.ancients = false
	frame.context.heroes = true
	p.observe(ancientObservation{frame: frame}, nil, now.Add(3*time.Second))
	if !p.finished || p.active {
		t.Fatal("batch did not finish")
	}
	p.interrupt()
	if _, ok := p.action(frame, now.Add(4*time.Second)); ok {
		t.Fatal("finished batch replayed")
	}

	for _, fault := range []string{"souls", "level", "pause"} {
		t.Run(fault, func(t *testing.T) {
			q := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{}}
			bad := out
			bad.rows = []ancientScreenRow{{"Argaiv", "1.000e28", image.Pt(243, 711)}}
			if fault == "souls" {
				bad.souls = "1e57"
			}
			if fault == "level" {
				bad.rows[0].level = "3e28"
			}
			q.observe(bad, nil, now)
			if fault == "pause" {
				q.sent(a, now)
				q.interrupt()
			}
			if _, ok := q.action(bad.frame, now.Add(time.Second)); ok || !q.blocked {
				t.Fatal("unsafe plan continued")
			}
		})
	}
}

func TestAncientQueueIsolation(t *testing.T) {
	now := time.Now()
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{ancientPlan: &ancientPlan{}})
	p.frame = testPipelineFrame()
	p.frame.context.heroes = true
	p.layout = p.frame.layout
	p.enqueue(gameAction{kind: collectFish, frame: p.frame, point: image.Pt(1, 1)}, now)
	p.enqueue(gameAction{kind: castSkill, frame: p.frame}, now)
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleAncient || a.ancient.step != visitAncients {
		t.Fatal("background action crossed Ancient visit")
	}
	p.actionCompleted(actionResult{action: a}, now)
	if len(p.queue) != 0 || p.ancient.pending == nil {
		t.Fatal("queued input survived transition")
	}
	p.enqueue(a, now)
	if _, ok := p.nextAction(now); ok {
		t.Fatal("input repeated before observation")
	}
	p.plan(now.Add(21 * time.Second))
	if !p.ancient.blocked || !controls.paused {
		t.Fatal("missing confirmation did not pause")
	}
	p.reset(controls.snapshot())
	if _, ok := p.ancient.action(p.frame, now.Add(time.Minute)); ok {
		t.Fatal("interrupted batch replayed")
	}

	q := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	q.frame = testPipelineFrame()
	q.frame.context.ancientDialog = true
	q.layout = q.frame.layout
	q.enqueue(gameAction{kind: collectFish, frame: q.frame}, now)
	if _, ok := q.nextAction(now); ok {
		t.Fatal("manual dialog received background input")
	}
}

func TestAncientHeldInputsReleaseOnFailure(t *testing.T) {
	var keys []string
	input := heroInput{keyToggle: func(k, s string) error { keys = append(keys, k+":"+s); return nil }, click: func(image.Point) error { return errors.New("missed input") }}
	if clickAncientCustom(context.Background(), input, image.Point{}) == nil || strings.Join(keys, ",") != "v:down,v:up" {
		t.Fatal("V was not released", keys)
	}
	keys = nil
	input.keyTap = func(string) error { return errors.New("missed selection") }
	if fillAncientCustom(context.Background(), input, "1e28") == nil || strings.Join(keys, ",") != robotgo.CmdCtrl()+":down,"+robotgo.CmdCtrl()+":up" {
		t.Fatal("selection modifier was not released", keys)
	}
}

func TestAncientHalfSizeScreen(t *testing.T) {
	requireAncientOCR(t)
	for _, name := range []string{"ascension-ancients.png", "ancient-quantity.png"} {
		t.Run(name, func(t *testing.T) {
			original := loadTestImage(t, "testdata/"+name)
			screen := image.NewRGBA(image.Rect(0, 0, 1280, 720))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			c, err := recognizedGame(screen)
			if err != nil || !c.known {
				t.Fatal("context", c, err)
			}
			out, err := readAncientObservation(context.Background(), gameFrame{image: screen, context: c})
			if err != nil {
				t.Fatal(err)
			}
			if name == "ancient-quantity.png" {
				if !c.ancientDialog || !out.okay {
					t.Fatal("quantity dialog", c, out)
				}
			} else if !c.ancients || out.souls != "1.755e58" || len(out.rows) != 3 || out.rows[0].name != "Argaiv" || out.rows[1].name != "Atman" {
				t.Fatal("Ancient rows", c, out.rows)
			}
		})
	}
}

func TestAncientFilledQuantityReachesOKWithoutOCR(t *testing.T) {
	for _, tc := range []struct{ image, quantity string }{
		{"ancient-quantity-filled.png", "20"},
		{"ancient-quantity-scrolled.png", "4.5579e32"},
	} {
		original := loadTestImage(t, "testdata/"+tc.image)
		for _, width := range []int{original.Bounds().Dx(), 1280} {
			t.Run(tc.image+fmt.Sprint(width), func(t *testing.T) {
				screen := image.NewRGBA(image.Rect(0, 0, width, width*original.Bounds().Dy()/original.Bounds().Dx()))
				xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
				c, err := recognizedGame(screen)
				if err != nil || !c.ancientDialog {
					t.Fatal("filled dialog not recognized", c, err)
				}
				now := time.Now()
				frame := gameFrame{id: 2, layout: 1, at: now, image: screen, context: c}
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Reading a quantity dialog must not launch Tesseract.
				out, err := readAncientObservation(ctx, frame)
				if err != nil || !out.okay {
					t.Fatal("dialog controls required OCR", err)
				}
				plan := ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Atman", Current: "164", Target: "184", Quantity: tc.quantity}}}}
				p := newGamePipeline(&pauseControl{}, heroInput{click: func(point image.Point) error {
					if !point.In(controlRect(screen, image.Rect(581, 370, 695, 420))) {
						t.Fatal("clicked outside OK", point)
					}
					return nil
				}, move: func(image.Point) error { return nil }}, pipelineReaders{}, pipelineOptions{ancientPlan: &plan})
				p.frame, p.layout = frame, frame.layout
				p.ancient.active, p.ancient.started = true, true
				p.ancient.selected, p.ancient.quantity = 0, tc.quantity
				p.ancient.sent(gameAction{frame: gameFrame{id: 1}, ancient: ancientCommand{step: fillAncientQuantity, quantity: tc.quantity}}, now)
				p.ancient.observe(out, nil, now)
				if p.ancient.pending != nil || !p.ancient.quantityEntered {
					t.Fatal("completed entry did not advance")
				}
				p.planAncients(now.Add(199 * time.Millisecond))
				if _, ok := p.nextAction(now.Add(199 * time.Millisecond)); ok {
					t.Fatal("OK scheduled before entry settled")
				}
				p.planAncients(now.Add(200 * time.Millisecond))
				a, ok := p.nextAction(now.Add(200 * time.Millisecond))
				if !ok || a.ancient.step != confirmAncientQuantity {
					t.Fatal("OK not scheduled", a.ancient.step, ok)
				}
				if acted, err := p.execute(context.Background(), a); err != nil || !acted {
					t.Fatal("OK not executed", acted, err)
				}
				changed := frame
				changed.image = loadTestImage(t, "testdata/ascension-ancients.png")
				if ancientActionStable(a, changed) {
					t.Fatal("OK accepted after dialog disappeared")
				}
			})
		}
	}
}

func TestAncientBlockedResumeExplainsFailure(t *testing.T) {
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{ancientPlan: &ancientPlan{}})
	p.frame = testPipelineFrame()
	p.frame.context.ancientDialog = true
	p.ancient.active = true
	p.ancient.started = true
	p.ancient.quantity = "20"
	p.ancient.pending = &gameAction{frame: p.frame, ancient: ancientCommand{step: fillAncientQuantity}}
	p.ancient.deadline = time.Now().Add(-time.Second)
	p.planAncients(time.Now())
	if !controls.isPaused() || !p.ancient.blocked {
		t.Fatal("unconfirmed purchase not blocked")
	}
	message := controls.message()
	if !strings.Contains(message, "stage=type quantity") || !strings.Contains(message, "quantity=\"20\"") || !strings.Contains(message, "restart") {
		t.Fatal("missing failure details", message)
	}
	generation := controls.snapshot()
	for i := 0; i < 3; i++ {
		if !controls.toggle() || controls.message() != message || controls.snapshot() != generation {
			t.Fatal("blocked F8 announced a resume or lost reason")
		}
	}
}

func TestAncientDisplayPrecision(t *testing.T) {
	for _, tc := range []struct {
		display, exact string
		matches        bool
	}{
		{"6.556e65", "6.55659294544822e65", true}, // Reported plan total, truncated HUD.
		{"6.557e65", "6.55659294544822e65", true}, // Rounded HUD remains supported.
		{"6.556e65", "6.556999e65", true},
		{"6.556e65", "6.557e65", false},
		{"6.556e65", "6.5554e65", false},
		{"6.556e65", "6.55659294544822e64", false},
		{"6.556e65", "6.558e65", false},
		{"1.755e58", "6.55659294544822e65", false}, // Bank must not match pending souls.
		{"1.00E+03", "1009", true},
		{"1.00e00003", "1009", true},
		{"1.00E+03", "1010", false},
		{"1.25", "1.259", true},
		{"184", "184", true},
		{"184", "185", false},
		{"ee", "184", false},
		{"184", "invalid", false},
	} {
		if got := ancientDisplayMatches(tc.display, tc.exact); got != tc.matches {
			t.Errorf("display=%q exact=%q: match=%t, want %t", tc.display, tc.exact, got, tc.matches)
		}
	}
}

func TestAncientWalletGateUsesDisplayedPrecision(t *testing.T) {
	frame := testPipelineFrame()
	frame.context.heroes, frame.context.ancients = false, true
	plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "6.55659294544822e65", Reserve: "6.55659294544822e63", Rows: []ancientcalc.Purchase{{ID: 1, Name: "Argaiv", Current: "1e28", Target: "2e28", Quantity: "1e28", Cost: "1e56"}}}}
	for _, read := range []string{"6.556e65", "1.755e58"} {
		t.Run(read, func(t *testing.T) {
			p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{}}
			p.observe(ancientObservation{frame: frame, souls: read, rows: []ancientScreenRow{{"Argaiv", "1.000e28", image.Pt(204, 500)}}}, nil, time.Now())
			a, ok := p.action(frame, time.Now())
			if read == "6.556e65" {
				if !ok || a.ancient.step != openAncientQuantity || p.blocked {
					t.Fatalf("fresh truncated wallet blocked: %s", p.failure)
				}
			} else if ok || !p.blocked || !strings.Contains(p.failure, plan.Souls) || !strings.Contains(p.failure, read) {
				t.Fatalf("stale wallet allowed or missing diagnostics: action=%t failure=%s", ok, p.failure)
			}
		})
	}
}

func TestAncientCustomInputTiming(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var downAt, clickAt, upAt time.Time
			held := false
			input := heroInput{
				keyToggle: func(key, state string) error {
					if key != "v" {
						t.Fatalf("unexpected key %q", key)
					}
					held = state == "down"
					if held {
						downAt = time.Now()
						if cancelled {
							cancel()
						}
					} else {
						upAt = time.Now()
					}
					return nil
				},
				click: func(image.Point) error {
					clickAt = time.Now()
					if !held || clickAt.Sub(downAt) < 100*time.Millisecond {
						t.Fatal("clicked before V settled")
					}
					return clickLeft(ctx, func(...interface{}) error {
						if !held {
							t.Fatal("V released during the click")
						}
						return nil
					})
				},
			}
			err := clickAncientCustom(ctx, input, image.Point{})
			if held || upAt.IsZero() {
				t.Fatal("V was not released")
			}
			if cancelled {
				if !errors.Is(err, context.Canceled) || !clickAt.IsZero() {
					t.Fatal("cancelled modifier triggered a purchase", err)
				}
			} else if err != nil || upAt.Sub(clickAt) < 200*time.Millisecond {
				t.Fatal("V was not held through mouse-up settling", err)
			}
		})
	}
}

func TestAncientOrdinaryPurchaseWithoutDialog(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/ancient-missed-v.png")
	out, err := readAncientObservation(context.Background(), gameFrame{id: 2, image: screen, context: gameContext{known: true, ancients: true, bounds: screen.Bounds()}})
	if err != nil || ancientQuantityDialog(screen) {
		t.Fatal("reported failure frame", err)
	}
	found := false
	for _, row := range out.rows {
		if row.name == "Atman" {
			found = row.level == "185"
		}
	}
	if !found {
		t.Fatal("Atman level 185 not recognized", out.rows)
	}
	plan := ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Atman", Current: "184", Target: "209", Quantity: "25"}}}}
	p := ancientPlanner{plan: &plan, active: true, selected: 0, quantity: "25"}
	p.sent(gameAction{kind: handleAncient, frame: gameFrame{id: 1}, ancient: ancientCommand{step: openAncientQuantity, quantity: "25"}}, time.Now())
	unchanged := out
	unchanged.rows = []ancientScreenRow{{name: "Atman", level: "184"}}
	p.observe(unchanged, nil, time.Now())
	if p.blocked || p.pending == nil {
		t.Fatal("unchanged level was treated as a purchase")
	}
	p.observe(out, nil, time.Now())
	if !p.blocked || !strings.Contains(p.failure, `saved="184", read="185"`) {
		t.Fatal("ordinary purchase did not block the stale plan", p.failure)
	}
	if _, ok := p.action(out.frame, time.Now().Add(time.Second)); ok {
		t.Fatal("stale plan repeated the purchase")
	}
}

func TestAncientEnergonLevelRecognition(t *testing.T) {
	requireAncientOCR(t)
	original := loadTestImage(t, "testdata/ancient-energon-leveled.png")
	for _, width := range []int{original.Bounds().Dx(), 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := image.NewRGBA(image.Rect(0, 0, width, width*original.Bounds().Dy()/original.Bounds().Dx()))
			xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
			c, err := recognizedGame(screen)
			if err != nil || !c.known || !c.ancients || c.ancientDialog {
				t.Fatal("post-purchase context", c, err)
			}
			out, err := readAncientObservation(context.Background(), gameFrame{id: 2, image: screen, context: c})
			if err != nil {
				t.Fatal(err)
			}
			energon := -1
			for i, row := range out.rows {
				if row.name == "Energon" && row.level == "201" {
					energon = i
				}
			}
			if energon < 0 {
				t.Fatal("Energon level 201 not recognized", out.rows)
			}

		})
	}
}

func TestAncientLevelLabelPreservesDigits(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"Lvl201", "201"}, {"LvIl201", "201"}, {"Lvi161", "161"},
		{"vl185", "185"}, {"Lvl149", "149"}, {"Lvl1201", "1201"},
		{"Lv1149", "149"}, {"Lvl1.000e28", "1.000e28"},
	} {
		if got := ancientLevelLabel.ReplaceAllString(tc.raw, ""); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestAncientSubmissionSkipsOCR(t *testing.T) {
	for _, tc := range []struct{ file, name, current, quantity string }{
		{"testdata/ancient-dora-submitted.png", "Dora", "161", "48"},
		{"testdata/ancient-kumawakamaru-leveled.png", "Kumawakamaru", "140", "71"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			screen := loadTestImage(t, tc.file)
			c, err := recognizedGame(screen)
			if err != nil || !c.known || !c.ancients || c.ancientDialog {
				t.Fatal("ordinary Ancients context", c, err)
			}
			c.window = "game"
			controls := pauseControl{}
			plan := ancientPlan{Plan: ancientcalc.Plan{Reserve: "1", Rows: []ancientcalc.Purchase{
				{Name: tc.name, Current: tc.current, Quantity: tc.quantity},
				{Name: "Dogcog", Current: "100", Quantity: "1", Cost: "1"},
			}}}
			p := newGamePipeline(&controls, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
				context: recognizedGame, window: func() string { return "game" },
				ancients: func(context.Context, gameFrame) (ancientObservation, error) { panic("post-OK OCR must not run") },
			}, pipelineOptions{ancientPlan: &plan})
			owner := c
			owner.ancients, owner.ancientDialog = false, true
			p.frame = gameFrame{id: 1, layout: 1, image: screen, context: owner}
			p.layout = 1
			p.ancient.active, p.ancient.started = true, true
			p.ancient.budgetChecked = true
			p.ancient.selected, p.ancient.quantity = 0, tc.quantity
			p.ancient.done = map[int]bool{}
			p.actionCompleted(actionResult{action: gameAction{kind: handleAncient, frame: p.frame, ancient: ancientCommand{step: confirmAncientQuantity, quantity: tc.quantity}}}, now)
			jobs := make([]chan analysisJob, analysisCount)
			for i := range jobs {
				jobs[i] = make(chan analysisJob, 1)
			}
			if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
				t.Fatal(err)
			}
			job := <-jobs[ancientAnalysis]
			if !job.modeOnly || job.frame.id <= 1 || job.frame.layout == 1 {
				t.Fatal("closed-dialog capture did not schedule a new context-only frame", job)
			}
			out := p.analyze(context.Background(), ancientAnalysis, job)
			if out.err != nil || len(out.ancient.rows) != 0 || out.ancient.souls != "" {
				t.Fatal("acknowledgement read purchase results", out)
			}
			stale := out
			stale.frame.layout--
			p.accept(context.Background(), stale, now.Add(time.Second))
			if p.ancient.pending == nil || p.ancient.done[0] {
				t.Fatal("obsolete layout acknowledged submission")
			}
			p.accept(context.Background(), out, now.Add(time.Second))
			if p.ancient.pending != nil || !p.ancient.done[0] || p.ancient.selected != -1 || p.ancient.quantity != "" || !p.ancient.deadline.IsZero() {
				t.Fatal("closed dialog did not retire submitted item", p.ancient)
			}
			reads := 0
			p.readers.ancients = func(_ context.Context, frame gameFrame) (ancientObservation, error) {
				reads++
				return ancientObservation{frame: frame, souls: "10", rows: []ancientScreenRow{{name: "Dogcog", level: "100", point: image.Pt(243, 711)}}}, nil
			}
			if err := p.capture(context.Background(), now.Add(2*time.Second), jobs); err != nil {
				t.Fatal(err)
			}
			job = <-jobs[ancientAnalysis]
			if job.modeOnly {
				t.Fatal("next purchase skipped pre-purchase OCR")
			}
			p.accept(context.Background(), p.analyze(context.Background(), ancientAnalysis, job), now.Add(2*time.Second))
			a, ok := p.ancient.action(p.frame, now.Add(2*time.Second))
			if reads != 1 || !ok || a.ancient.step != openAncientQuantity || p.ancient.selected != 1 {
				t.Fatal("next checked item did not proceed", reads, a, ok)
			}
			p.ancient.sent(a, now.Add(2*time.Second))
			p.accept(context.Background(), out, now.Add(3*time.Second))
			if _, ok := p.ancient.action(p.frame, now.Add(3*time.Second)); ok || p.ancient.done[1] {
				t.Fatal("stale acknowledgement replayed input or retired the next item")
			}
			controls.toggle()
			p.reset(controls.snapshot())
			p.accept(context.Background(), out, now.Add(4*time.Second))
			if p.ancient.active || !p.ancient.blocked || p.ancient.done[1] {
				t.Fatal("F8 accepted old work")
			}
			if executed, err := controls.runClick(context.Background(), a.frame.generation, func() error { t.Fatal("old input executed after F8"); return nil }); executed || err != nil {
				t.Fatal("old input generation survived F8", executed, err)
			}
		})
	}
}

func TestAncientSubmissionRejectsUnownedFrames(t *testing.T) {
	now := time.Now()
	frame := gameFrame{id: 1, generation: 7, layout: 2, context: gameContext{known: true, ancientDialog: true, window: "game", bounds: image.Rect(0, 0, 1280, 720)}}
	closed := frame
	closed.id++
	closed.context.ancientDialog, closed.context.ancients = false, true
	for _, fault := range []string{"open", "unknown", "heroes", "mercenaries", "gild", "save", "quest", "ascension", "foreign", "outside", "bounds", "geometry", "generation", "stale", "error", "unowned"} {
		t.Run(fault, func(t *testing.T) {
			plan := ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Dora", Quantity: "48"}}}}
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{
				ancients: func(context.Context, gameFrame) (ancientObservation, error) { panic("pending OK must not read OCR") },
			}, pipelineOptions{ancientPlan: &plan})
			p.ancient.active, p.ancient.started = true, true
			p.ancient.selected, p.ancient.quantity, p.ancient.done = 0, "48", map[int]bool{}
			p.ancient.sent(gameAction{frame: frame, ancient: ancientCommand{step: confirmAncientQuantity, quantity: "48"}}, now)
			bad := closed
			switch fault {
			case "open":
				bad.context = frame.context
			case "unknown":
				bad.context.known = false
			case "heroes":
				bad.context.heroes = true
			case "mercenaries":
				bad.context.mercenaries = true
			case "gild":
				bad.context.modal = unknownGildModal
			case "save":
				bad.context.saveMenu = true
			case "quest":
				bad.context.questDialog = true
			case "ascension":
				bad.context.ascension = true
			case "foreign":
				bad.context.window = "other game"
			case "outside":
				bad.context.window = "!outside-game"
			case "bounds":
				bad.context.bounds.Max.X++
			case "geometry":
				bad.context.geometry = viewportGeometry{Capture: image.Rect(1, 1, 2, 2)}
			case "generation":
				bad.generation++
			case "stale":
				bad.id = frame.id
			case "unowned":
				p.ancient.selected = -1
			}
			out := p.analyze(context.Background(), ancientAnalysis, analysisJob{frame: bad, modeOnly: true})
			if fault == "error" {
				out.err = errors.New("context read failed")
			}
			p.ancient.observe(out.ancient, out.err, now.Add(time.Second))
			if p.ancient.done[0] {
				t.Fatal("unowned frame acknowledged submission")
			}
			if _, ok := p.ancient.action(bad, now.Add(2*time.Second)); ok {
				t.Fatal("unconfirmed OK repeated")
			}
			p.planAncients(now.Add(21 * time.Second))
			if !p.ancient.blocked || p.ancient.pending != nil {
				t.Fatal("unacknowledged submission did not block at deadline")
			}
		})
	}
}

func TestAncientNextPurchaseGuards(t *testing.T) {
	for _, fault := range []string{"none", "case", "balance", "level", "quantity"} {
		t.Run(fault, func(t *testing.T) {
			plan := ancientPlan{Plan: ancientcalc.Plan{Reserve: "5", Rows: []ancientcalc.Purchase{
				{Name: "Dora"},
				{Name: "Dogcog", Current: "1e28", Target: "10000000000000000000000000001", Quantity: "1", Cost: "1"},
			}}}
			frame := gameFrame{id: 10, context: gameContext{known: true, ancients: true}}
			out := ancientObservation{frame: frame, souls: "10", rows: []ancientScreenRow{{name: "Dogcog", level: "1.000e28"}}}
			switch fault {
			case "case":
				out.rows[0].name = "dogcog"
				if !ancientRowReadable(out.rows, "Dogcog") {
					t.Fatal("case-only OCR difference erased a readable row")
				}
			case "balance":
				out.souls = "5"
			case "level":
				out.rows[0].level = "2.000e28"
			case "quantity":
				plan.Rows[1].Quantity = "0"
			}
			p := ancientPlanner{plan: &plan, active: true, started: true, budgetChecked: true, selected: -1, done: map[int]bool{0: true}}
			p.observe(out, nil, time.Now())
			a, ok := p.action(frame, time.Now())
			if fault == "none" || fault == "case" {
				if !ok || a.ancient.step != openAncientQuantity || p.selected != 1 || p.blocked {
					t.Fatal("valid positive purchase below displayed precision was rejected")
				}
			} else if ok || !p.blocked {
				t.Fatal("next purchase bypassed its pre-purchase guard", fault)
			}
		})
	}
}
