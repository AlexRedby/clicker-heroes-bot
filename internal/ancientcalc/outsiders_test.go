package ancientcalc

import (
	"context"
	"reflect"
	"testing"
)

func levelsOf(rows []OutsiderTarget) []int {
	out := make([]int, len(rows))
	for i := range rows {
		out[i] = rows[i].Target
	}
	return out
}

func TestOutsiderIdealFrozen(t *testing.T) {
	// Independent Python evaluation of the pinned upstream formulas, including
	// FANT and HZE branch boundaries. No upstream JavaScript runtime is used.
	for _, tc := range []struct {
		total int
		want  []int
	}{
		{0, []int{0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{1, []int{0, 0, 0, 1, 0, 0, 0, 0, 0}},
		{12, []int{0, 0, 3, 2, 3, 0, 0, 0, 0}},
		{49, []int{0, 1, 5, 6, 6, 1, 0, 0, 0}},
		{50, []int{0, 1, 6, 6, 6, 1, 0, 0, 0}},
		{99, []int{0, 2, 10, 9, 7, 4, 0, 0, 2}},
		{100, []int{0, 2, 11, 9, 7, 4, 0, 0, 2}},
		{309, []int{0, 7, 34, 15, 14, 5, 1, 0, 3}},
		{310, []int{0, 7, 35, 15, 14, 5, 1, 0, 3}},
		{311, []int{0, 7, 36, 15, 14, 5, 1, 0, 3}},
		{327, []int{0, 7, 36, 16, 14, 5, 1, 0, 3}},
		{328, []int{0, 7, 37, 16, 14, 5, 1, 0, 3}},
		{329, []int{0, 7, 38, 16, 14, 5, 1, 0, 3}},
		{2000, []int{0, 38, 139, 38, 18, 19, 5, 0, 2}},
		{2001, []int{0, 38, 140, 38, 18, 19, 5, 0, 2}},
		{10499, []int{0, 110, 366, 66, 40, 41, 13, 0, 9}},
		{10500, []int{0, 109, 310, 65, 42, 44, 13, 0, 11}},
		{20000, []int{0, 150, 664, 95, 53, 56, 18, 0, 22}},
		{20999, []int{0, 150, 817, 102, 54, 57, 19, 0, 23}},
	} {
		a, err := allocateOutsiders(context.Background(), tc.total, tc.total, [9]int{})
		if err != nil {
			t.Fatal(err)
		}
		got := levelsOf(a.Ideal)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("AS %d: got %v, want %v", tc.total, got, tc.want)
		}
		if !reflect.DeepEqual(levelsOf(a.Additions), tc.want) {
			t.Fatalf("AS %d: empty roster additions differ from ideal: %v", tc.total, levelsOf(a.Additions))
		}
	}
}

func TestOutsiderAdditionsAreAffordable(t *testing.T) {
	current := [9]int{0, 6, 36, 15, 14, 5, 1, 3, 3}
	a, err := allocateOutsiders(context.Background(), 328, 18, current)
	if err != nil {
		t.Fatal(err)
	}
	if a.Spent != 18 || a.Remaining != 0 {
		t.Fatalf("spent=%d remaining=%d", a.Spent, a.Remaining)
	}
	spent := 0
	for i, row := range a.Additions {
		if row.Current != current[i] || row.Target < current[i] || row.Cost < 0 {
			t.Fatalf("row %d decreased or refunded: %+v", i, row)
		}
		cost := func(level int) int {
			if i == 2 {
				return level
			}
			return level * (level + 1) / 2
		}
		if row.Cost != cost(row.Target)-cost(row.Current) {
			t.Fatalf("wrong purchase cost: %+v", row)
		}
		spent += row.Cost
	}
	if spent != a.Spent || !a.RequiresRespec {
		t.Fatalf("incorrect totals/respec flag: %+v", a)
	}
}

func TestOutsiderCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := allocateOutsiders(ctx, 328, 18, [9]int{}); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestOutsiderUnsupportedAndBounds(t *testing.T) {
	a, err := allocateOutsiders(context.Background(), 21000, 10, [9]int{})
	if err != nil || a.Status != "unsupported" || a.Spent != 0 {
		t.Fatalf("got %+v, err=%v", a, err)
	}
	for total := 0; total < 21000; total += 41 {
		a, err := allocateOutsiders(context.Background(), total, total, [9]int{})
		if err != nil || a.Status != "ok" || a.Spent > total || a.Remaining < 0 {
			t.Fatalf("AS %d: %+v, err=%v", total, a, err)
		}
		spent := 0
		for i, row := range a.Ideal {
			cost := row.Target
			if i != 2 {
				cost = row.Target * (row.Target + 1) / 2
			}
			if row.Target < 0 || row.Cost != cost {
				t.Fatalf("AS %d: invalid ideal row %+v", total, row)
			}
			spent += cost
		}
		if spent != total || a.Spent != total || a.RequiresRespec {
			t.Fatalf("AS %d: budget accounting failed: %+v", total, a)
		}
	}
}
