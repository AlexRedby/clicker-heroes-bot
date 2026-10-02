package ancientcalc

import (
	"context"
	"reflect"
	"strconv"
	"testing"
)

func testOwned(levels ...int) []Level {
	out := make([]Level, len(outsiderIDs))
	for i, id := range outsiderIDs {
		out[i] = Level{ID: id, Name: outsiderNames[i], Level: strconv.Itoa(levels[i])}
	}
	return out
}

func TestPlanOutsidersLedgerAndReward(t *testing.T) {
	owned := testOwned(0, 6, 36, 15, 14, 5, 1, 3, 3)
	p, err := PlanOutsiders(context.Background(), 310, 0, 20, owned)
	if err != nil || p.Now.Budget != 0 || p.AfterReward.Budget != 20 {
		t.Fatalf("plans=%+v err=%v", p, err)
	}
	if got := levelsOf(p.Now.Additions); !reflect.DeepEqual(got, []int{0, 6, 36, 15, 14, 5, 1, 3, 3}) {
		t.Fatalf("owned levels changed: %v", got)
	}
	if p.Now.Spent != 0 || p.AfterReward.Spent != 20 {
		t.Fatalf("spent now=%d after=%d", p.Now.Spent, p.AfterReward.Spent)
	}
	if got := levelsOf(p.AfterReward.Additions); got[3] != 16 || got[2] != 40 {
		t.Fatalf("projected levels=%v", got)
	}
	if p.AfterReward.Additions[3].Cost != 16 || p.AfterReward.Additions[2].Cost != 4 {
		t.Fatalf("projected costs=%+v", p.AfterReward.Additions)
	}
}

func TestPlanOutsidersNonzeroWalletAndNilContext(t *testing.T) {
	owned := testOwned(0, 6, 36, 15, 14, 5, 1, 3, 3)
	p, err := PlanOutsiders(nil, 330, 20, 0, owned)
	if err != nil {
		t.Fatal(err)
	}
	if p.Now.Spent != 20 || p.Now.Remaining != 0 {
		t.Fatalf("allocation=%+v", p.Now)
	}
	if p.Now.Additions[7].Target != 3 || p.Now.Additions[7].Cost != 0 {
		t.Fatalf("Orphalas was refunded: %+v", p.Now.Additions[7])
	}
}

func TestPlanOutsidersValidation(t *testing.T) {
	owned := testOwned(0, 0, 0, 0, 0, 0, 0, 0, 0)
	for _, mutate := range []func([]Level){
		func(x []Level) { x[0].ID = x[1].ID },
		func(x []Level) { x[0].Name = "wrong" },
		func(x []Level) { x[0].Level = "1.0" },
		func(x []Level) { x[0].Level = "-1" },
		func(x []Level) { x[0].Level = "+1" },
		func(x []Level) { x[0].Level = "01" },
	} {
		copyOf := append([]Level(nil), owned...)
		mutate(copyOf)
		if _, err := PlanOutsiders(context.Background(), 0, 0, 0, copyOf); err == nil {
			t.Fatal("expected roster validation error")
		}
	}
	if _, err := PlanOutsiders(context.Background(), 0, 0, 0, owned[:8]); err == nil {
		t.Fatal("expected incomplete roster error")
	}
	if _, err := PlanOutsiders(context.Background(), 1, 0, 0, owned); err == nil {
		t.Fatal("expected ledger error")
	}
}

func TestOutsiderPlanBoundsCancellationAndUnsupported(t *testing.T) {
	owned := testOwned(0, 0, 0, 0, 0, 0, 0, 0, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanOutsiders(ctx, 0, 0, 0, owned); err == nil {
		t.Fatal("expected cancellation")
	}
	if _, err := PlanOutsiders(context.Background(), 21000, 21000, 1, owned); err != nil {
		t.Fatal(err)
	}
	p, err := PlanOutsiders(context.Background(), 21000, 21000, 1, owned)
	if err != nil {
		t.Fatal(err)
	}
	if p.Now.Status != "unsupported" || p.AfterReward.Status != "unsupported" {
		t.Fatal(p)
	}
	if _, err := PlanOutsiders(context.Background(), int(maxOutsiderInteger), int(maxOutsiderInteger), 1, owned); err == nil {
		t.Fatal("expected projected overflow")
	}
	for _, tc := range [][3]int{{-1, 0, 0}} {
		if _, err := PlanOutsiders(context.Background(), tc[0], tc[1], tc[2], owned); err == nil {
			t.Fatalf("expected negative total error: %v", tc)
		}
	}
	for _, tc := range [][3]int{{0, -1, 0}, {0, 0, -1}} {
		if _, err := PlanOutsiders(context.Background(), tc[0], tc[1], tc[2], owned); err == nil {
			t.Fatalf("expected negative budget error: %v", tc)
		}
	}
}

func TestOutsiderFeedCost(t *testing.T) {
	got, err := OutsiderFeedCost(3, 36, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("Phandoryss cost=%d", got)
	}
	got, err = OutsiderFeedCost(1, 6, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got != 15 {
		t.Fatalf("triangular cost=%d", got)
	}
	for _, tc := range [][3]int{{4, 0, 1}, {99, 0, 1}, {1, 0, 0}, {1, -1, 1}} {
		if _, err := OutsiderFeedCost(tc[0], tc[1], tc[2]); err == nil {
			t.Fatalf("expected feed error for %v", tc)
		}
	}
	for _, tc := range [][3]int{{1, int(maxOutsiderInteger), 1}, {1, 0, int(maxOutsiderInteger)}, {1, int(maxOutsiderInteger - 1), 2}} {
		if _, err := OutsiderFeedCost(tc[0], tc[1], tc[2]); err == nil {
			t.Fatalf("expected huge feed error for %v", tc)
		}
	}
}
