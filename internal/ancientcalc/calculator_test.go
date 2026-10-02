package ancientcalc

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"
)

func TestAncientCalculatorReference(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-calculator-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Cases []struct {
			Name  string
			Input struct {
				Save, Reserve string
				SkillRate     float64
				Beyond8k      bool
			}
			Expected Plan
		}
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference.Cases) != 12 {
		t.Fatal("missing reference scenarios")
	}
	for _, tc := range reference.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := Calculate(context.Background(), []byte(tc.Input.Save), tc.Input.Reserve, tc.Input.SkillRate, tc.Input.Beyond8k)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.Expected
			if len(got.Rows) != len(want.Rows) || len(got.Owned) != len(want.Owned) || got.Ascensions != want.Ascensions {
				t.Fatalf("different roster: got %d/%d want %d/%d", len(got.Rows), len(got.Owned), len(want.Rows), len(want.Owned))
			}
			compare := func(label, a, b string) {
				t.Helper()
				x, e1 := Value(a)
				y, e2 := Value(b)
				if e1 != nil || e2 != nil {
					t.Fatalf("%s invalid amounts %q %q", label, a, b)
				}
				diff := aNew().Abs(aSub(x, y))
				tolerance := aMul(y, aConst("1e-12"))
				if y.Cmp(aConst("1e9")) < 0 {
					tolerance = aConst("0")
				}
				if diff.Cmp(tolerance) > 0 {
					t.Errorf("%s: %s != %s", label, a, b)
				}
			}
			compare("souls", got.Souls, want.Souls)
			compare("reserve", got.Reserve, want.Reserve)
			compare("spent", got.Spent, want.Spent)
			compare("remaining", got.Remaining, want.Remaining)
			if (got.Invested == "") != (want.Invested == "") {
				t.Fatal("invested knowledge differs")
			}
			if got.Invested != "" {
				compare("invested", got.Invested, want.Invested)
			}
			for i, row := range got.Rows {
				old := want.Rows[i]
				if row.ID != old.ID || row.Name != old.Name {
					t.Fatalf("row identity differs: %s %s", row.Name, old.Name)
				}
				compare(row.Name+" current", row.Current, old.Current)
				compare(row.Name+" target", row.Target, old.Target)
				compare(row.Name+" cost", row.Cost, old.Cost)
				// Input quantities are truncated, never rounded up to exceed the goal.
				q, _ := new(big.Rat).SetString(row.Quantity)
				current, _ := new(big.Rat).SetString(row.Current)
				target, _ := new(big.Rat).SetString(row.Target)
				if q.Sign() <= 0 || q.Cmp(new(big.Rat).Sub(target, current)) > 0 {
					t.Fatal("unsafe purchase quantity", row.Name)
				}
				wantQuantity, err := InputQuantity(old.Quantity)
				if err != nil {
					t.Fatalf("format reference quantity %q: %v", old.Quantity, err)
				}
				if row.Quantity != wantQuantity {
					t.Errorf("%s quantity: %s != %s", row.Name, row.Quantity, wantQuantity)
				}
			}
		})
	}
}

func TestAncientCalculatorRejectsUnsafeInputs(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeAncientSave(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		reserve string
		rate    float64
	}{{"-1", 1}, {"101%", 1}, {"1e1000000", 1}, {"1%", 2}} {
		if _, err := planAncients(context.Background(), decoded, tc.reserve, tc.rate, false); err == nil {
			t.Fatal("unsafe settings accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planAncients(ctx, decoded, "1%", 1, false); err != context.Canceled {
		t.Fatal("cancel ignored", err)
	}
	if _, err := aInput("1"+strings.Repeat("0", 100)+".5", "level", true); err == nil {
		t.Fatal("fractional large level rounded to an integer")
	}
	decoded.Ancients.Ancients["999"] = ancientSaveEntry{Level: "1"}
	if _, err := planAncients(context.Background(), decoded, "1%", 1, false); err == nil {
		t.Fatal("unknown owned Ancient accepted")
	}
}

func TestAncientRoundingAndExcludedGoals(t *testing.T) {
	for _, tc := range []struct{ x, floor, ceil string }{{"-1.5", "-2", "-1"}, {"1.5", "1", "2"}, {"-2", "-2", "-2"}} {
		if aString(aRound(aConst(tc.x), false)) != tc.floor || aString(aRound(aConst(tc.x), true)) != tc.ceil {
			t.Fatal("incorrect integer rounding", tc.x)
		}
	}
	for _, name := range []string{"Khrysos", "Thusia", "Iris", "Siyalatas", "Libertas", "Nogardnit", "Revolc"} {
		if ancientGoal(name, aConst("10"), aConst("1"), aConst("1"), aConst("1"), false) != nil {
			t.Fatal("excluded goal selected", name)
		}
	}
	if aString(aConst("1e1000")) != "1e+1000" {
		t.Fatal("large values lost scientific formatting")
	}
}
