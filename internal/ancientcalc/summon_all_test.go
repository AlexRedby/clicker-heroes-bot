package ancientcalc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func summonAllState(t *testing.T) TranscensionState {
	t.Helper()
	s := readReceiptState(t, true, nil)
	s.HeroSouls = "1264000"
	s.Ascensions, s.AscensionsThisTranscension = 1, 1
	id := 1
	s.CurrentAscensionID = &id
	return s
}

func TestPlanSummonAllClient6144(t *testing.T) {
	s := summonAllState(t)
	p, err := PlanSummonAll(nil, s)
	if err != nil || !p.Eligible || p.Cost != "184161" || len(p.Missing) != 26 {
		t.Fatalf("empty roster: %+v, %v", p, err)
	}
	wantIDs := []int{4, 5, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 31, 32}
	for i, row := range p.Missing {
		if row.ID != wantIDs[i] || row.Level != "1" || row.Name == "" {
			t.Fatalf("noncanonical client roster: %+v", p.Missing)
		}
	}
	for _, tc := range []struct {
		wallet string
		want   bool
	}{{"184160.999999", false}, {"184161", true}, {"1.84161e5", true}} {
		s.HeroSouls = tc.wallet
		plan, err := PlanSummonAll(nil, s)
		if err != nil || plan.Eligible != tc.want {
			t.Fatalf("wallet %s: %+v, %v", tc.wallet, plan, err)
		}
	}
	s.Ancients = []Level{{ID: 19, Name: "Fragsworth", Level: "20"}, {ID: 5, Name: "Siyalatas", Level: "1"}}
	partial, err := PlanSummonAll(nil, s)
	if err != nil || partial.Cost != "184158" || len(partial.Missing) != 24 {
		t.Fatalf("partial roster: %+v, %v", partial, err)
	}
	s.Ancients = append(s.Ancients, partial.Missing...)
	complete, err := PlanSummonAll(nil, s)
	if err != nil || complete.Eligible || complete.Cost != "0" || len(complete.Missing) != 0 {
		t.Fatalf("complete roster: %+v, %v", complete, err)
	}
}

func TestPlanSummonAllRejectsUnverifiedSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*TranscensionState)
	}{
		{"different build", func(s *TranscensionState) { s.Build = "1.0e12-9999" }},
		{"not transcendent", func(s *TranscensionState) { s.Transcendent = false }},
		{"obsolete owned", func(s *TranscensionState) { s.Ancients = []Level{{ID: 3, Name: "Solomon", Level: "1"}} }},
		{"zero owned level", func(s *TranscensionState) { s.Ancients = []Level{{ID: 19, Name: "Fragsworth", Level: "0"}} }},
		{"duplicate owned", func(s *TranscensionState) {
			s.Ancients = []Level{{ID: 19, Name: "Fragsworth", Level: "1"}, {ID: 19, Name: "Fragsworth", Level: "1"}}
		}},
		{"unknown owned", func(s *TranscensionState) { s.Ancients = []Level{{ID: 99, Name: "Unknown", Level: "1"}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := summonAllState(t)
			tc.change(&s)
			if _, err := PlanSummonAll(nil, s); err == nil {
				t.Fatal("accepted unverified source")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanSummonAll(ctx, summonAllState(t)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestVerifySummonAllReceipt(t *testing.T) {
	before := summonAllState(t)
	before.Ancients = []Level{{ID: 19, Name: "Fragsworth", Level: "20"}}
	plan, err := PlanSummonAll(nil, before)
	if err != nil {
		t.Fatal(err)
	}
	after := copyReceiptState(t, before)
	after.SaveHash = strings.Repeat("a", 64)
	after.HeroSouls = "1079840"
	after.Ancients = append(after.Ancients, plan.Missing...)
	*after.CurrentZone = 5
	after.HighestZone++
	if err := VerifySummonAllReceipt(nil, before, after, plan); err != nil {
		t.Fatal("valid receipt with combat progress:", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*TranscensionState, *SummonAllPlan)
	}{
		{"stale", func(s *TranscensionState, p *SummonAllPlan) { s.SaveHash = before.SaveHash }},
		{"incorrect debit", func(s *TranscensionState, p *SummonAllPlan) { s.HeroSouls = "1079841" }},
		{"partial roster", func(s *TranscensionState, p *SummonAllPlan) { s.Ancients = s.Ancients[:len(s.Ancients)-1] }},
		{"old level changed", func(s *TranscensionState, p *SummonAllPlan) { s.Ancients[0].Level = "21" }},
		{"new level wrong", func(s *TranscensionState, p *SummonAllPlan) { s.Ancients[1].Level = "2" }},
		{"cycle changed", func(s *TranscensionState, p *SummonAllPlan) { s.Ascensions++ }},
		{"outsider changed", func(s *TranscensionState, p *SummonAllPlan) { s.Outsiders[0].Name = "different" }},
		{"highest decreased", func(s *TranscensionState, p *SummonAllPlan) { s.HighestZone = before.HighestZone - 1 }},
		{"wrong reservation", func(s *TranscensionState, p *SummonAllPlan) { p.Cost = "1" }},
		{"missing reservation row", func(s *TranscensionState, p *SummonAllPlan) { p.Missing = p.Missing[:len(p.Missing)-1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := copyReceiptState(t, after)
			reserved := plan
			reserved.Missing = append([]Level(nil), plan.Missing...)
			tc.change(&s, &reserved)
			if err := VerifySummonAllReceipt(nil, before, s, reserved); err == nil {
				t.Fatal("accepted unrelated or incomplete receipt")
			}
		})
	}
}

func TestBulkRosterCompletesTranscendentDependencies(t *testing.T) {
	s := summonAllState(t)
	plan, err := PlanSummonAll(nil, s)
	if err != nil {
		t.Fatal(err)
	}
	s.Ancients = plan.Missing
	missing, err := MissingActiveAncients(nil, s, 1, true)
	if err != nil || len(missing) != 0 {
		t.Fatal("bulk roster still waiting for unavailable Ancients", missing, err)
	}
	s.Transcendent = false
	s.Ancients = []Level{}
	missing, err = MissingActiveAncients(nil, s, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range missing {
		if row.ID == 3 {
			return
		}
	}
	t.Fatal("non-transcendent Solomon dependency removed")
}
