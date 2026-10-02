package main

import (
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"errors"
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
				if err != nil || !out.okay || out.quantity != "" {
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
		if !ancientQuantityMatches(quantity, strings.ToUpper(quantity)) {
			t.Fatal("equivalent exponent rejected")
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
	p.observe(ancientObservation{frame: frame, okay: true, quantity: "1e28"}, nil, now.Add(time.Second))
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

func TestAncientQuantityOCRAndStaleConfirmation(t *testing.T) {
	requireAncientOCR(t)
	screen := loadTestImage(t, "testdata/ancient-quantity.png")
	typed := image.NewRGBA(screen.Bounds())
	draw.Draw(typed, typed.Bounds(), screen, screen.Bounds().Min, draw.Src)
	r := controlRect(screen, image.Rect(508, 325, 772, 350))
	// Synthetic Arial text checks input-field OCR; the blank dialog is a real frame.
	text := loadTestImage(t, "testdata/ancient-quantity-text.png")
	draw.Draw(typed, r, text, text.Bounds().Min, draw.Src)
	f := gameFrame{image: typed, context: gameContext{known: true, ancientDialog: true, bounds: screen.Bounds()}}
	out, err := readAncientObservation(context.Background(), f)
	if err != nil || !out.okay || !ancientQuantityMatches(out.quantity, "1.23456789012345e28") {
		t.Fatalf("quantity OCR %q: %v", out.quantity, err)
	}
	a := gameAction{kind: handleAncient, frame: f, ancient: ancientCommand{step: confirmAncientQuantity}}
	if !ancientActionStable(a, f) {
		t.Fatal("unchanged field rejected")
	}
	f.image = screen
	if ancientActionStable(a, f) {
		t.Fatal("changed field retained confirmation")
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
			} else if !c.ancients || len(out.rows) != 3 || out.rows[0].name != "Argaiv" || out.rows[1].name != "Atman" {
				t.Fatal("Ancient rows", c, out.rows)
			}
		})
	}
}
