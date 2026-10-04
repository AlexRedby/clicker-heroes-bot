package main

import (
	"math"
	"testing"
)

func TestMercenaryRecoveryPolicy(t *testing.T) {
	funded := mercenaryRecoveryBudget{Known: true, Cost: 100, Rubies: 1000}
	cases := []struct {
		name   string
		traits mercenaryRecoveryTraits
		budget mercenaryRecoveryBudget
		want   mercenaryRecoveryMethod
	}{
		{"Emma level 10 with rounded uncommon ruby bonus", mercenaryRecoveryTraits{true, 10, 8, 0}, funded, mercenaryRecoveryBury},
		{"new ordinary recruit", mercenaryRecoveryTraits{true, 1, 0, 0}, funded, mercenaryRecoveryBury},
		{"young ordinary mercenary", mercenaryRecoveryTraits{true, 2, 0, 0}, funded, mercenaryRecoveryRubies},
		{"ordinary paid upper boundary", mercenaryRecoveryTraits{true, 8, 0, 0}, funded, mercenaryRecoveryRubies},
		{"ordinary beyond paid boundary", mercenaryRecoveryTraits{true, 9, 0, 0}, funded, mercenaryRecoveryBury},
		{"rare non-ruby trait does not extend range", mercenaryRecoveryTraits{true, 10, 0, 0}, funded, mercenaryRecoveryBury},
		{"uncommon ruby upper boundary", mercenaryRecoveryTraits{true, 9, 8, 0}, funded, mercenaryRecoveryRubies},
		{"epic ruby upper boundary", mercenaryRecoveryTraits{true, 10, 20, 0}, funded, mercenaryRecoveryRubies},
		{"epic ruby beyond boundary", mercenaryRecoveryTraits{true, 11, 20, 0}, funded, mercenaryRecoveryBury},
		{"mythical ruby upper boundary", mercenaryRecoveryTraits{true, 13, 200, 0}, funded, mercenaryRecoveryRubies},
		{"transcendent ruby upper boundary", mercenaryRecoveryTraits{true, 19, 2000, 0}, funded, mercenaryRecoveryRubies},
		{"transcendent ruby beyond boundary", mercenaryRecoveryTraits{true, 20, 2000, 0}, funded, mercenaryRecoveryBury},
		{"preserve life while paid revive worthwhile", mercenaryRecoveryTraits{true, 10, 0, 1}, funded, mercenaryRecoveryRubies},
		{"use life above paid range", mercenaryRecoveryTraits{true, 11, 0, 1}, funded, mercenaryRecoveryExtraLife},
		{"six remaining lives paid boundary", mercenaryRecoveryTraits{true, 16, 0, 6}, funded, mercenaryRecoveryRubies},
		{"six remaining lives beyond paid boundary", mercenaryRecoveryTraits{true, 17, 0, 6}, funded, mercenaryRecoveryExtraLife},
		{"life on a young recruit", mercenaryRecoveryTraits{true, 1, 0, 1}, funded, mercenaryRecoveryRubies},
		{"missing actual cost cannot authorize payment", mercenaryRecoveryTraits{true, 8, 0, 0}, mercenaryRecoveryBudget{}, mercenaryRecoveryInspect},
		{"exact available balance", mercenaryRecoveryTraits{true, 8, 0, 0}, mercenaryRecoveryBudget{true, 360, 360}, mercenaryRecoveryRubies},
		{"keep worthwhile but unaffordable mercenary", mercenaryRecoveryTraits{true, 8, 0, 0}, mercenaryRecoveryBudget{true, 360, 359}, mercenaryRecoveryWait},
		{"use life if unaffordable", mercenaryRecoveryTraits{true, 8, 0, 1}, mercenaryRecoveryBudget{true, 360, 359}, mercenaryRecoveryExtraLife},
		{"unknown trait must not bury", mercenaryRecoveryTraits{}, funded, mercenaryRecoverySkip},
		{"invalid level", mercenaryRecoveryTraits{true, 0, 0, 0}, funded, mercenaryRecoverySkip},
		{"invalid ruby trait", mercenaryRecoveryTraits{true, 10, math.NaN(), 0}, funded, mercenaryRecoverySkip},
		{"unsupported future bonus", mercenaryRecoveryTraits{true, 10, 3000, 0}, funded, mercenaryRecoverySkip},
		{"negative life count", mercenaryRecoveryTraits{true, 10, 0, -1}, funded, mercenaryRecoverySkip},
		{"zero paid cost", mercenaryRecoveryTraits{true, 8, 0, 0}, mercenaryRecoveryBudget{true, 0, 1000}, mercenaryRecoverySkip},
		{"unknown ruby balance", mercenaryRecoveryTraits{true, 8, 0, 0}, mercenaryRecoveryBudget{true, 360, -1}, mercenaryRecoverySkip},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := chooseMercenaryRecovery(tc.traits, tc.budget)
			if got.method != tc.want || got.reason == "" {
				t.Fatalf("decision=%+v, want method %v", got, tc.want)
			}
		})
	}
}

func TestMercenaryRecoveryUsesDisplayedCurrencyScale(t *testing.T) {
	traits := mercenaryRecoveryTraits{Known: true, Level: 8}
	for _, scale := range []int64{1, 10} {
		if got := chooseMercenaryRecovery(traits, mercenaryRecoveryBudget{true, 36 * scale, 35 * scale}); got.method != mercenaryRecoveryWait {
			t.Fatalf("scale %d: unaffordable revive decision=%+v", scale, got)
		}
		if got := chooseMercenaryRecovery(traits, mercenaryRecoveryBudget{true, 36 * scale, 36 * scale}); got.method != mercenaryRecoveryRubies {
			t.Fatalf("scale %d: affordable revive decision=%+v", scale, got)
		}
	}
}
