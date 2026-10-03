package ancientcalc

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strconv"
	"testing"
)

func TestAncientResidualMorgulis(t *testing.T) {
	for _, tc := range []struct {
		name, wallet, reserve, quantity, remaining string
		owned                                      bool
	}{
		{"fallback", "4", "0", "4", "0", true},
		{"exact-floor", "4", "3", "1", "3", true},
		{"fractional-floor", "4.5", "0.5", "", "4.5", true},
		{"all-reserved", "4", "4", "", "4", true},
		{"empty-wallet", "0", "0", "", "0", true},
		{"absent", "4", "0", "", "4", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			save := smallAllocationSave(4, 0, tc.owned)
			save.HeroSouls = json.Number(tc.wallet)
			p, err := planAncients(context.Background(), save, tc.reserve, 0, false)
			if err != nil {
				t.Fatal(err)
			}
			assertAncientPlanLedger(t, p)
			if p.Remaining != tc.remaining {
				t.Fatal("incorrect balance", p)
			}
			if tc.quantity == "" {
				if len(p.Rows) != 0 {
					t.Fatal("unexpected purchase", p)
				}
				return
			}
			if len(p.Rows) != 1 || p.Rows[0].ID != 16 || p.Rows[0].Quantity != tc.quantity {
				t.Fatal("missing profitable fallback", p)
			}
			// Exact comparison of the complete native Soul multiplier before/after.
			before := new(big.Rat).Add(big.NewRat(111, 100), new(big.Rat).Quo(exactPlanAmount(t, p.Souls), big.NewRat(10, 1)))
			after := new(big.Rat).Add(big.NewRat(1, 1), new(big.Rat).Quo(exactPlanAmount(t, p.Remaining), big.NewRat(10, 1)))
			after.Add(after, new(big.Rat).Mul(exactPlanAmount(t, p.Rows[0].Target), big.NewRat(11, 100)))
			if after.Cmp(before) <= 0 {
				t.Fatal("purchase loses native Soul damage", p)
			}
		})
	}
}

func TestMorgulisMergedInputAndBenefit(t *testing.T) {
	for _, expensive := range []bool{false, true} {
		rawQuantity, wallet := "1000007", "1000017"
		quantity, err := InputQuantity(rawQuantity)
		if err != nil {
			t.Fatal(err)
		}
		// The actual six-digit input is 1e6, while the old estimate prices 1000007.
		// Released estimate headroom must not count as added Soul damage benefit.
		save := smallAllocationSave(25, 0, true)
		save.HeroSouls = json.Number(wallet)
		p := Plan{Souls: wallet, Reserve: "0", Spent: rawQuantity, Remaining: "10", Owned: []Level{{16, "Morgulis", "1"}, {19, "Fragsworth", "1"}}, Rows: []Purchase{{16, "Morgulis", "1", "1000008", quantity, rawQuantity}}}
		price := func(_ string, old, target, m *big.Float) (*big.Float, error) {
			q := aSub(target, old)
			if expensive && q.Cmp(aConst(quantity)) > 0 {
				return aAdd(q, aConst("2")), nil
			}
			return q, nil
		}
		got, err := addProfitableMorgulis(context.Background(), save, p, price)
		if err != nil {
			t.Fatal(err)
		}
		assertAncientPlanLedger(t, got)
		if expensive {
			if got.Rows[0].Quantity != quantity || got.Spent != rawQuantity {
				t.Fatal("credited unentered estimated levels as benefit", got)
			}
			continue
		}
		if len(got.Rows) != 1 || got.Rows[0].Quantity != "1.00001e6" || got.Spent != "1000010" || got.Remaining != "7" {
			t.Fatal("six-digit merge or ledger incorrect", got)
		}
	}
}

func TestMorgulisCanonicalLevelsAndTinyBudget(t *testing.T) {
	for _, raw := range []string{"10000000000000000000000000", "1e25", "1.0000000000000000000000000e25"} {
		for _, wallet := range []string{"4", "1e23"} {
			save := smallAllocationSave(4, 0, true)
			save.Ancients.Ancients["16"] = ancientSaveEntry{Level: json.Number(raw)}
			save.HeroSouls = json.Number(wallet)
			p, err := planAncients(context.Background(), save, "0", 0, false)
			if err != nil {
				t.Fatal(raw, wallet, err)
			}
			assertAncientPlanLedger(t, p)
			if wallet == "4" {
				for _, row := range p.Rows {
					if row.ID == 16 {
						t.Fatal("tiny budget bought unresolved level", p)
					}
				}
				continue
			}
			// Isolate a fallback-only plan to exercise the new row's canonical
			// identity independently of whether RoT already proposed Morgulis.
			p = Plan{Souls: wallet, Reserve: "0", Spent: "0", Remaining: wallet, Owned: []Level{{16, "Morgulis", aString(aConst(raw))}, {19, "Fragsworth", "1"}}}
			p, err = addProfitableMorgulis(context.Background(), save, p, ancientPrice)
			if err != nil {
				t.Fatal(raw, wallet, err)
			}
			assertAncientPlanLedger(t, p)
			if len(p.Rows) != 1 || p.Rows[0].ID != 16 || p.Rows[0].Current != aString(aConst(raw)) {
				t.Fatal("meaningful canonical Morgulis fallback missing", p)
			}
		}
	}
}

func applyEnteredAncientPlan(t *testing.T, save *ancientSave, p Plan) {
	t.Helper()
	var defs struct {
		Ancients []ancientDefinition `json:"ancients"`
	}
	if err := json.Unmarshal(ancientDataJSON, &defs); err != nil {
		t.Fatal(err)
	}
	chor := aConst("0")
	if o, ok := save.Outsiders.Outsiders["2"]; ok {
		chor = aConst(string(o.Level))
	}
	multiplier := aPowInteger(aConst("0.95"), chor)
	paid := aConst("0")
	for _, row := range p.Rows {
		old := aConst(row.Current)
		target := aAdd(old, aConst(row.Quantity))
		for _, def := range defs.Ancients {
			if def.ID == row.ID {
				c, err := legacyAncientPrice(def.Formula, old, target, multiplier)
				if err != nil {
					t.Fatal(err)
				}
				paid = aAdd(paid, c)
			}
		}
		integer, _ := target.Int(nil)
		key := strconv.Itoa(row.ID)
		entry := save.Ancients.Ancients[key]
		entry.Level = json.Number(integer.String())
		save.Ancients.Ancients[key] = entry
	}
	save.HeroSouls = json.Number(aString(aSub(aConst(p.Souls), paid)))
}

func TestAncientResidualRepeatInput(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, large := range []bool{false, true} {
		for _, owned := range []bool{false, true} {
			for _, chor := range []int{0, 4} {
				for _, reserve := range []string{"0", "1%", "100%"} {
					name := strconv.FormatBool(large) + "/" + strconv.FormatBool(owned) + "/" + strconv.Itoa(chor) + "/" + reserve
					t.Run(name, func(t *testing.T) {
						save := smallAllocationSave(1000, chor, owned)
						if large {
							save, err = decodeAncientSave(context.Background(), data)
							if err != nil {
								t.Fatal(err)
							}
							save.Outsiders.Outsiders["2"] = ancientSaveEntry{Level: json.Number(strconv.Itoa(chor))}
							if !owned {
								delete(save.Ancients.Ancients, "16")
							}
						}
						stopped := false
						for run := 0; run < 5; run++ {
							p, e := planAncients(context.Background(), save, reserve, 1, false)
							if e != nil {
								t.Fatal(e)
							}
							assertAncientPlanLedger(t, p)
							if len(p.Rows) == 0 {
								stopped = true
								again, e := planAncients(context.Background(), save, reserve, 1, false)
								if e != nil || len(again.Rows) != 0 {
									t.Fatal("unchanged export requests new purchase", again, e)
								}
								break
							}
							if reserve == "100%" {
								t.Fatal("spent protected wallet", p)
							}
							applyEnteredAncientPlan(t, &save, p)
						}
						if !stopped {
							t.Fatal("entered plans did not converge without new souls")
						}
					})
				}
			}
		}
	}
}

func TestMorgulisRetainsGoldAndSkills(t *testing.T) {
	save := smallAllocationSave(1000000, 0, true)
	save.Ancients.Ancients["8"] = ancientSaveEntry{Level: "1"}
	save.Ancients.Ancients["22"] = ancientSaveEntry{Level: "1"}
	p, err := planAncients(context.Background(), save, "0", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	assertAncientPlanLedger(t, p)
	seen := map[int]bool{}
	for _, row := range p.Rows {
		seen[row.ID] = true
	}
	if !seen[8] || !seen[22] || !seen[16] {
		t.Fatal("native Soul comparison discarded useful gold/skill allocation", p)
	}
}

func TestMorgulisSearchNearMinimumInput(t *testing.T) {
	old, minimum := aConst("1e25"), aConst("1e22")
	minimumCost, err := ancientPrice("one", old, aAdd(old, minimum), aConst("1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, wallet string
		want         bool
	}{
		{"upper-interval", "1.5e22", true},
		{"just-below-entered-floor", "9999999999999999999999", false},
		{"ideal-floor-without-price-allowance", "1e22", false},
		{"exact-guarded-floor", aString(minimumCost), true},
		{"just-below-guarded-floor", aString(aSub(minimumCost, aConst("1"))), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			save := smallAllocationSave(4, 0, true)
			save.Ancients.Ancients["16"] = ancientSaveEntry{Level: "1e25"}
			save.HeroSouls = json.Number(tc.wallet)
			p := Plan{Souls: tc.wallet, Reserve: "0", Spent: "0", Remaining: tc.wallet, Owned: []Level{{16, "Morgulis", "1e+25"}, {19, "Fragsworth", "1"}}}
			got, err := addProfitableMorgulis(context.Background(), save, p, ancientPrice)
			if err != nil {
				t.Fatal(err)
			}
			assertAncientPlanLedger(t, got)
			if (len(got.Rows) == 1) != tc.want {
				t.Fatal("discarded eligible upper interval or exceeded floor", got)
			}
			if !tc.want {
				return
			}
			row := got.Rows[0]
			q := exactPlanAmount(t, row.Quantity)
			if row.ID != 16 || q.Cmp(exactPlanAmount(t, "1e22")) < 0 {
				t.Fatal("entered purchase below 0.1%", row)
			}
			if new(big.Rat).Mul(q, big.NewRat(11, 1)).Cmp(new(big.Rat).Mul(exactPlanAmount(t, row.Cost), big.NewRat(10, 1))) <= 0 {
				t.Fatal("not profitable under real guarded pricing", row)
			}
			if tc.name == "exact-guarded-floor" && (row.Quantity != "1e22" || got.Remaining != "0") {
				t.Fatal("exact guarded threshold rejected", got)
			}
		})
	}
}
