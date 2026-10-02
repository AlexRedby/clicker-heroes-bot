package main

import (
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"errors"
	"fmt"
	"github.com/go-vgo/robotgo"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
			t.Logf("souls=%s rows=%+v thumb=%v/%d", out.souls, out.rows, out.thumb, out.thumbHeight)
			if len(out.rows) != 3 || out.rows[0].name != "Argaiv" || out.rows[0].level != "1.000e28" || out.rows[1].name != "Atman" || out.rows[1].level != "164" {
				t.Fatal("Ancient row recognition", out.rows)
			}
			if !out.hasThumb || out.thumb.X < img.Bounds().Dx()*44/100 || out.thumb.X > img.Bounds().Dx()*48/100 {
				t.Fatal("scrollbar thumb recognition")
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
	out := ancientObservation{frame: frame, souls: "1.755e58", rows: []ancientScreenRow{{"Argaiv", "1.000e28", image.Pt(243, 711)}}, hasThumb: true, thumb: image.Pt(1171, 659), thumbHeight: 120}
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
	out.frame.id = frame.id + 1
	p.observe(out, nil, now.Add(time.Second))
	if p.pending == nil || len(p.done) != 0 {
		t.Fatal("unchanged level confirmed")
	}
	if _, ok := p.action(out.frame, now.Add(2*time.Second)); ok {
		t.Fatal("OK was repeated")
	}
	out.frame.id++
	out.rows[0].level = "2.000e28"
	p.observe(out, nil, now.Add(2*time.Second))
	if p.pending != nil || !p.done[0] {
		t.Fatal("increased level not confirmed")
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
	p := ancientPlanner{plan: &plan, active: true, selected: 0, quantity: "25", target: "209"}
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

func TestAncientEnergonPurchaseConfirmation(t *testing.T) {
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
			plan := ancientPlan{Plan: ancientcalc.Plan{Rows: []ancientcalc.Purchase{{Name: "Energon", Current: "149", Target: "201", Quantity: "52"}}}}
			p := ancientPlanner{plan: &plan, active: true, selected: 0, quantity: "52", target: "201", done: map[int]bool{}}
			now := time.Now()
			p.sent(gameAction{frame: gameFrame{id: 1}, ancient: ancientCommand{step: confirmAncientQuantity, quantity: "52"}}, now)
			unchanged := out
			unchanged.rows = append([]ancientScreenRow(nil), out.rows...)
			unchanged.rows[energon].level = "149"
			p.observe(unchanged, nil, now)
			if p.pending == nil || p.done[0] {
				t.Fatal("unchanged level confirmed")
			}
			if _, ok := p.action(out.frame, now.Add(time.Second)); ok {
				t.Fatal("unconfirmed OK repeated")
			}
			out.frame.id++
			p.observe(out, nil, now.Add(time.Second))
			if p.pending != nil || !p.done[0] || p.blocked {
				t.Fatal("successful Energon purchase not confirmed", p.failure)
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
