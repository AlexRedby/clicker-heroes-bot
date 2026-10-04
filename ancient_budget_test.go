package main

import (
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"
)

func TestAncientArgaivBudgetScreenshot(t *testing.T) {
	requireAncientOCR(t)
	for _, divisor := range []int{1, 2} {
		for _, hideBudget := range []bool{false, true} {
			s := mercenaryScaled(t, "testdata/ancient-budget-argaiv.png", divisor)
			if hideBudget {
				blank := image.NewRGBA(s.Bounds())
				draw.Draw(blank, blank.Bounds(), s, s.Bounds().Min, draw.Src)
				draw.Draw(blank, controlRect(s, image.Rect(410, 172, 591, 199)), image.NewUniform(color.Black), image.Point{}, draw.Src)
				s = blank
			}
			f := gameFrame{id: 1, image: s, context: gameContext{known: true, ancients: true, bounds: s.Bounds()}}
			out, err := readAncientObservation(context.Background(), f)
			if err != nil {
				t.Fatal("native Ancient OCR", divisor, hideBudget, err)
			}
			found := false
			for _, row := range out.rows {
				if row.name == "Argaiv" {
					found = row.level == "2.107e34"
				}
			}
			if !found {
				t.Fatal("native Argaiv level OCR", divisor, out.rows)
			}
			// The other 21 submissions are represented by their combined guarded cost.
			plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "8.48693546959918e72", Reserve: "0", Rows: []ancientcalc.Purchase{
				{Name: "Mammon", Cost: "7484277222087832988824714216195316020614018418425505307133715653553820035"},
				{Name: "Argaiv", Current: "2.10778207420936e34", Quantity: "1.63071e36", Cost: "1.002658245732111629859313641117261295324449327296749422685307122362310565e72"},
			}}}
			p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{0: true}}
			now := time.Now()
			p.observe(out, nil, now)
			a, ok := p.action(f, now)
			if !ok || p.blocked || a.ancient.step != openAncientQuantity || p.selected != 1 || a.ancient.quantity != "1.63071e36" {
				t.Fatal("affordable Argaiv blocked or a submission replayed", divisor, a, p.failure)
			}
		}
	}
}

func TestAncientInsufficientBudgetDiagnostics(t *testing.T) {
	f := testPipelineFrame()
	f.context.heroes, f.context.ancients = false, true
	plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "1.1e72", Reserve: "1e71", Rows: []ancientcalc.Purchase{{ID: 1, Name: "Argaiv", Current: "1e28", Target: "2e28", Quantity: "1e28", Cost: "1.1e72"}}}}
	p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{}}
	now := time.Now()
	p.observe(ancientObservation{frame: f, rows: []ancientScreenRow{{"Argaiv", "1.000e28", image.Pt(204, 500)}}}, nil, now)
	if _, ok := p.action(f, now); ok || !p.blocked {
		t.Fatal("genuine insufficient funds did not stop")
	}
	for _, value := range []string{"remaining=", "1.1e72", "1e71"} {
		if !strings.Contains(p.failure, value) {
			t.Fatal("missing budget operand", value, p.failure)
		}
	}
}

func TestAncientRemainingBudgetGuards(t *testing.T) {
	for _, tc := range []struct {
		name, souls, spent, reserve, cost string
		allow                             bool
	}{
		{"precise save budget", "1.0026e72", "0", "0", "1.0024e72", true},
		{"precise save insufficient", "1.0016e72", "0", "0", "1.0017e72", false},
		{"reserve exactly retained", "100", "70", "5", "25", true},
		{"reserve exceeded by one", "100", "70", "6", "25", false},
		{"fractional reserve exactly retained", "10.1", "7", "0.1", "3", true},
		{"all previous costs deducted", "100", "80", "5", "25", false},
		{"guarded spending", "100", "3", "90", "3", true},
		{"invalid save budget", "bad", "0", "0", "1", false},
		{"invalid submitted cost", "100", "bad", "0", "1", false},
		{"negative remaining budget", "100", "101", "0", "1", false},
		{"invalid next cost", "100", "70", "0", "bad", false},
		{"invalid reserve", "100", "70", "bad", "1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testPipelineFrame()
			f.context.heroes, f.context.ancients = false, true
			plan := ancientPlan{Plan: ancientcalc.Plan{Souls: tc.souls, Reserve: tc.reserve, Rows: []ancientcalc.Purchase{
				{Name: "Atman", Cost: tc.spent},
				{Name: "Argaiv", Current: "1", Quantity: "1", Cost: tc.cost},
			}}}
			p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{0: true}}
			now := time.Now()
			p.observe(ancientObservation{frame: f, rows: []ancientScreenRow{{"Argaiv", "1", image.Pt(204, 500)}, {"Atman", "999", image.Pt(204, 800)}}}, nil, now)
			a, ok := p.action(f, now)
			if tc.allow {
				if !ok || p.blocked || p.selected != 1 || a.ancient.step != openAncientQuantity {
					t.Fatal("affordable pending purchase rejected", a, p.failure)
				}
			} else if ok || !p.blocked {
				t.Fatal("unsafe budget accepted", a, p.failure)
			}
		})
	}
}
