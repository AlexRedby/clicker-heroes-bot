package ancientcalc

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strconv"
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
			save, err := decodeAncientSave(context.Background(), []byte(tc.Input.Save))
			if err != nil {
				t.Fatal(err)
			}
			guarded, err := planAncients(context.Background(), save, tc.Input.Reserve, tc.Input.SkillRate, tc.Input.Beyond8k)
			if err != nil {
				t.Fatal(err)
			}
			assertAncientPlanLedger(t, guarded)
			got, err := planAncientsWithPrice(context.Background(), save, tc.Input.Reserve, tc.Input.SkillRate, tc.Input.Beyond8k, legacyAncientPrice)
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

func assertAncientPlanLedger(t *testing.T, p Plan) {
	t.Helper()
	sum := new(big.Rat)
	for _, row := range p.Rows {
		sum.Add(sum, exactPlanAmount(t, row.Cost))
	}
	wallet, spent := exactPlanAmount(t, p.Souls), exactPlanAmount(t, p.Spent)
	remaining, reserve := exactPlanAmount(t, p.Remaining), exactPlanAmount(t, p.Reserve)
	tolerance := new(big.Rat).Mul(new(big.Rat).Quo(wallet, exactPlanAmount(t, "1e99")), new(big.Rat).SetInt64(int64(len(p.Rows)+3)))
	if new(big.Rat).Abs(new(big.Rat).Sub(sum, spent)).Cmp(tolerance) > 0 || new(big.Rat).Abs(new(big.Rat).Sub(new(big.Rat).Sub(wallet, spent), remaining)).Cmp(tolerance) > 0 || remaining.Cmp(reserve) < 0 || spent.Cmp(new(big.Rat).Sub(wallet, reserve)) > 0 {
		t.Fatal("incorrect guarded plan ledger", p)
	}
}

func legacyAncientPrice(formula string, old, target, multiplier *big.Float) (*big.Float, error) {
	return aRound(aMul(aSub(ancientCostSum(formula, target), ancientCostSum(formula, old)), multiplier), true), nil
}

func TestAncientClientPriceBounds(t *testing.T) {
	if _, err := ancientPrice("exponential", aConst("1"), aConst("4000001"), aConst("1")); err == nil {
		t.Fatal("unsupported exponential range accepted")
	}
	for _, file := range []struct {
		path  string
		count int
	}{{"ancient-client-polynomial-prices.json", 80}, {"ancient-client-other-prices.json", 96}} {
		data, err := os.ReadFile("testdata/" + file.path)
		if err != nil {
			t.Fatal(err)
		}
		var reference struct {
			Cases []struct {
				Formula, Current, Target, Chor string
				Mantissa                       float64
				Exponent                       int
				Collapsed                      bool
			}
		}
		if err := json.Unmarshal(data, &reference); err != nil || len(reference.Cases) != file.count {
			t.Fatal("invalid client price vectors", err)
		}
		for _, tc := range reference.Cases {
			if tc.Formula == "" {
				tc.Formula = "polynomial1_5"
			}
			t.Run(tc.Formula+"/"+tc.Current+"-"+tc.Target+"/"+tc.Chor, func(t *testing.T) {
				price, err := ancientPrice(tc.Formula, aConst(tc.Current), aConst(tc.Target), aPowInteger(aConst("0.95"), aConst(tc.Chor)))
				if tc.Collapsed {
					if err == nil {
						t.Fatal("unresolved price accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				// Exact binary64 mantissa and decimal exponent from original WASM,
				// not the host's rounded multiplication or the native price model.
				client := new(big.Rat).Mul(new(big.Rat).SetFloat64(tc.Mantissa), exactPlanAmount(t, "1e"+strconv.Itoa(tc.Exponent)))
				bound, _ := price.Rat(nil)
				if bound.Cmp(client) < 0 {
					t.Fatal("underpriced client purchase", price, client)
				}
				allowance := "1.01"
				current, target := exactPlanAmount(t, tc.Current), exactPlanAmount(t, tc.Target)
				if current.Cmp(exactPlanAmount(t, "1e9")) >= 0 && new(big.Rat).Sub(target, current).Cmp(new(big.Rat).Mul(current, exactPlanAmount(t, "1e-6"))) < 0 {
					allowance = "2" // Endpoint intervals overlap for still smaller increments.
				} else if current.Cmp(exactPlanAmount(t, "1e20")) >= 0 {
					allowance = "1.00000001"
				}
				ceiling := new(big.Rat).Add(new(big.Rat).Mul(client, exactPlanAmount(t, allowance)), exactPlanAmount(t, "4"))
				if bound.Cmp(ceiling) > 0 {
					t.Fatal("excessive rounding allowance", price, client)
				}
			})
		}
	}
}

func TestAncientGuardedPolynomialAllocation(t *testing.T) {
	exact, err := planAncients(context.Background(), smallAllocationSave(5, 0, true), "0", 0, false)
	if err != nil || len(exact.Rows) != 2 || exact.Spent != "5" || exact.Remaining != "0" {
		t.Fatal("exact budget rejected", exact, err)
	}
	for _, tc := range []struct{ wallet, reserve, target, spent, remaining string }{
		{"90", "0", "5", "63", "27"},
		{"91", "0", "6", "91", "0"},
		{"92", "1", "6", "91", "1"},
		{"91", "1", "5", "63", "28"},
	} {
		wallet, _ := strconv.Atoi(tc.wallet)
		save := smallAllocationSave(wallet, 0, true)
		save.Ancients.Ancients["29"] = ancientSaveEntry{Level: "1"}
		p, err := planAncients(context.Background(), save, tc.reserve, 0, false)
		if err != nil {
			t.Fatal(err)
		}
		assertAncientPlanLedger(t, p)
		if len(p.Rows) != 3 || p.Rows[1].ID != 19 || p.Rows[1].Target != tc.target || p.Spent != tc.spent || p.Remaining != tc.remaining {
			t.Fatal("incorrect guarded allocation", tc, p)
		}
	}
	for _, tc := range []struct{ old, base string }{
		{"999999999", "177827940781"},
		{"1000000000", "177827941003"},
		{"1e20", "1e25"},
	} {
		save := smallAllocationSave(1, 0, true)
		base := aConst(tc.base)
		save.HeroSouls = json.Number(aString(aMul(base, aConst("10"))))
		save.Ancients.Ancients["19"] = ancientSaveEntry{Level: json.Number(aString(base))}
		save.Ancients.Ancients["16"] = ancientSaveEntry{Level: json.Number(aString(aSquare(base)))}
		save.Ancients.Ancients["29"] = ancientSaveEntry{Level: json.Number(tc.old)}
		p, err := planAncients(context.Background(), save, "0", 0, false)
		if err != nil {
			t.Fatal("unresolved increment stopped allocation", tc.old, err)
		}
		assertAncientPlanLedger(t, p)
		for _, row := range p.Rows {
			if row.ID == 29 {
				t.Fatal("unresolved polynomial increment planned", row)
			}
		}
	}
}

func TestAncientMeaningfulPurchaseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, current, delta string
		keepFrags, atman     bool
	}{
		{"below-relative-minimum", "1000000000", "999999", false, false},
		{"exact-relative-minimum", "1000000000", "1000000", true, false},
		{"input-truncates-below-minimum", "1234567890", "1234568", false, false},
		{"input-meets-minimum", "1234567890", "1234570", true, false},
		{"small-and-exponential-plus-one", "1000000", "1", true, true},
		{"huge-micro", "1e500", "1e496", false, false},
		{"huge-meaningful", "1e500", "2e497", true, false},
		{"huge-exact-entered-minimum", "1e167", "1.000001e164", true, false},
		{"huge-exact-entered-minimum-170", "1e170", "1.000001e167", true, false},
		{"huge-exact-entered-minimum-172", "1e172", "1.000001e169", true, false},
		{"huge-exact-entered-minimum-1000", "1e1000", "1.000001e997", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, delta := exactPlanAmount(t, tc.current), exactPlanAmount(t, tc.delta)
			target := new(big.Rat).Add(old, delta)
			// Independent exact integer prices select the allocator's desired base.
			fragsCost := new(big.Rat).Quo(new(big.Rat).Sub(
				new(big.Rat).Mul(target, new(big.Rat).Add(target, big.NewRat(1, 1))),
				new(big.Rat).Mul(old, new(big.Rat).Add(old, big.NewRat(1, 1)))), big.NewRat(2, 1))
			oldMorg := new(big.Rat).Mul(old, old)
			morgCost := new(big.Rat).Sub(new(big.Rat).Mul(target, target), oldMorg)
			wallet := new(big.Rat).Add(new(big.Rat).Add(fragsCost, morgCost), big.NewRat(100, 1))
			save := smallAllocationSave(1, 0, true)
			save.Ancients.Ancients["19"] = ancientSaveEntry{Level: json.Number(old.Num().String())}
			save.Ancients.Ancients["16"] = ancientSaveEntry{Level: json.Number(oldMorg.Num().String())}
			if tc.atman {
				save.Ancients.Ancients["13"] = ancientSaveEntry{Level: "19"}
				wallet.Add(wallet, big.NewRat(1048576, 1)) // 2^20 pays for Atman 19 -> 20.
			}
			save.HeroSouls = json.Number(wallet.Num().String())
			p, err := planAncientsWithPrice(context.Background(), save, "100", 0, false, legacyAncientPrice)
			if err != nil {
				t.Fatal(err)
			}
			assertAncientPlanLedger(t, p)
			foundFrags, foundAtman := false, false
			for _, row := range p.Rows {
				if row.ID == 19 {
					foundFrags = true
					if !strings.HasPrefix(tc.current, "1e") && (exactPlanAmount(t, row.Target).Cmp(target) != 0 || exactPlanAmount(t, row.Cost).Cmp(fragsCost) != 0) {
						t.Fatal("retained allocator target or conservative price changed", row)
					}
					if strings.HasPrefix(tc.name, "huge-exact-entered-minimum") {
						minimum := new(big.Rat).Quo(old, big.NewRat(1000, 1))
						text, err := InputQuantity(minimum.Num().String())
						if err != nil || row.Quantity != text {
							t.Fatal("test did not reach the exact entered 0.1% boundary", row.Quantity, text, err)
						}
					}
				}
				if row.ID == 13 {
					foundAtman = row.Quantity == "1" && row.Target == "20" && row.Cost == "1048576"
				}
			}
			if foundFrags != tc.keepFrags || foundAtman != tc.atman {
				t.Fatal("incorrect micro-purchase policy", p.Rows)
			}
			if tc.name == "huge-micro" && (len(p.Rows) != 0 || p.Spent != "0" || p.Remaining != p.Souls) {
				t.Fatal("empty plan retained skipped spend or cancellation error", p)
			}
			if tc.name == "below-relative-minimum" {
				if len(p.Rows) != 1 || p.Rows[0].ID != 16 || exactPlanAmount(t, p.Spent).Cmp(morgCost) != 0 || exactPlanAmount(t, p.Remaining).Cmp(new(big.Rat).Add(fragsCost, big.NewRat(100, 1))) != 0 {
					t.Fatal("skipped estimate did not remain in the wallet", p)
				}
			}
		})
	}
}

func TestAncientRepeatedInputStopsMicroPurchases(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	save, err := decodeAncientSave(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	var definitions struct {
		Ancients []ancientDefinition `json:"ancients"`
	}
	if err := json.Unmarshal(ancientDataJSON, &definitions); err != nil {
		t.Fatal(err)
	}
	chor, err := aInput(string(save.Outsiders.Outsiders["2"].Level), "discount", true)
	if err != nil {
		t.Fatal(err)
	}
	multiplier := aPowInteger(aConst("0.95"), chor)
	for run := 0; run < 4; run++ {
		p, err := planAncients(context.Background(), save, "1%", 1, false)
		if err != nil {
			t.Fatal(err)
		}
		assertAncientPlanLedger(t, p)
		if run >= 2 {
			if len(p.Rows) != 0 || p.Spent != "0" || p.Remaining != p.Souls {
				t.Fatal("repeat run still requests micro-purchases", run, p)
			}
			continue
		}
		if len(p.Rows) == 0 {
			t.Fatal("meaningful initial allocation was skipped", run)
		}
		// Model six-digit entered integers and legacy charge estimates. This is
		// a repeat-run regression, not installed-game balance calibration.
		paid := aConst("0")
		for _, row := range p.Rows {
			old, _ := aInput(row.Current, "current", true)
			entered, _ := aInput(row.Quantity, "quantity", true)
			target := aAdd(old, entered)
			for _, def := range definitions.Ancients {
				if def.ID != row.ID {
					continue
				}
				cost, err := legacyAncientPrice(def.Formula, old, target, multiplier)
				if err != nil {
					t.Fatal(err)
				}
				paid = aAdd(paid, cost)
			}
			key := strconv.Itoa(row.ID)
			entry := save.Ancients.Ancients[key]
			integer, _ := target.Int(nil)
			entry.Level = json.Number(integer.String())
			save.Ancients.Ancients[key] = entry
		}
		wallet, _ := aInput(p.Souls, "wallet", false)
		save.HeroSouls = json.Number(aString(aSub(wallet, paid)))
	}
}

func smallAllocationSave(wallet int, chor int, morgulis bool) ancientSave {
	save := ancientSave{
		HeroSouls: json.Number(strconv.Itoa(wallet)), HeroSoulsSacrificed: "0",
		HighestFinishedZonePersist: "0", AncientSoulsTotal: "0", NumWorldResets: "0",
		Ancients:  ancientSaveAncients{map[string]ancientSaveEntry{"19": {Level: "1"}}},
		Outsiders: ancientSaveOutsiders{map[string]ancientSaveEntry{"2": {Level: json.Number(strconv.Itoa(chor))}}},
	}
	if morgulis {
		save.Ancients.Ancients["16"] = ancientSaveEntry{Level: "1"}
	}
	return save
}

func exactPlanAmount(t *testing.T, text string) *big.Rat {
	t.Helper()
	n, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("invalid plan amount %q", text)
	}
	return n
}

func TestAncientLegacyExactBudgetAndAllocationInvariants(t *testing.T) {
	for _, morgulis := range []bool{true, false} {
		for _, chor := range []int{0, 1, 6} {
			// Independent rational discount and integer row ceiling; no calculator helpers.
			discount := new(big.Rat).SetFrac(
				new(big.Int).Exp(big.NewInt(19), big.NewInt(int64(chor)), nil),
				new(big.Int).Exp(big.NewInt(20), big.NewInt(int64(chor)), nil))
			price := func(raw int) int64 {
				n := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(raw)), discount)
				return new(big.Int).Quo(new(big.Int).Add(n.Num(), new(big.Int).Sub(n.Denom(), big.NewInt(1))), n.Denom()).Int64()
			}
			for wallet := 0; wallet <= 200; wallet++ {
				for _, reserve := range []string{"0", "1", "1%", "100%"} {
					if wallet == 0 && reserve == "1" {
						continue
					}
					p, err := planAncientsWithPrice(context.Background(), smallAllocationSave(wallet, chor, morgulis), reserve, 0, false, legacyAncientPrice)
					if err != nil {
						t.Fatal(wallet, reserve, chor, morgulis, err)
					}
					reserved := new(big.Rat)
					switch reserve {
					case "1":
						reserved.SetInt64(1)
					case "1%":
						reserved.SetFrac64(int64(wallet), 100)
					case "100%":
						reserved.SetInt64(int64(wallet))
					}
					available := new(big.Rat).Sub(new(big.Rat).SetInt64(int64(wallet)), reserved)
					wantBase := 1
					for base := 2; base <= 201; base++ {
						cost := price(base*(base+1)/2 - 1)
						if morgulis {
							cost += price(base*base - 1)
						} else {
							cost += int64(base * base) // The synthetic soul bank is not discounted.
						}
						if new(big.Rat).SetInt64(cost).Cmp(available) > 0 {
							break
						}
						wantBase = base
					}
					wantSpend := price(wantBase*(wantBase+1)/2 - 1)
					wantRows := 0
					if wantBase > 1 {
						wantRows++
						if morgulis {
							wantRows++
						}
					}
					if morgulis {
						wantSpend += price(wantBase*wantBase - 1)
					}
					if len(p.Rows) != wantRows {
						t.Fatalf("wallet=%d reserve=%s chor=%d morgulis=%t: rows=%+v, want base %d", wallet, reserve, chor, morgulis, p.Rows, wantBase)
					}
					sum := new(big.Rat)
					for _, row := range p.Rows {
						wantTarget, wantCost := wantBase, price(wantBase*(wantBase+1)/2-1)
						if row.ID == 16 && morgulis {
							wantTarget, wantCost = wantBase*wantBase, price(wantBase*wantBase-1)
						} else if row.ID != 19 {
							t.Fatal("unexpected purchase", row)
						}
						if row.Current != "1" || row.Target != strconv.Itoa(wantTarget) || row.Quantity != strconv.Itoa(wantTarget-1) || row.Cost != strconv.FormatInt(wantCost, 10) {
							t.Fatalf("wallet=%d reserve=%s chor=%d: incorrect row %+v", wallet, reserve, chor, row)
						}
						sum.Add(sum, exactPlanAmount(t, row.Cost))
					}
					remaining := new(big.Rat).Sub(new(big.Rat).SetInt64(int64(wallet)), sum)
					if p.Souls != strconv.Itoa(wallet) || sum.Cmp(new(big.Rat).SetInt64(wantSpend)) != 0 || sum.Cmp(exactPlanAmount(t, p.Spent)) != 0 || remaining.Cmp(exactPlanAmount(t, p.Remaining)) != 0 || reserved.Cmp(exactPlanAmount(t, p.Reserve)) != 0 || remaining.Cmp(reserved) < 0 {
						t.Fatal("incorrect plan ledger", p)
					}
					if !morgulis && wantBase > 1 && new(big.Rat).Sub(remaining, reserved).Cmp(new(big.Rat).SetInt64(int64(wantBase*wantBase))) < 0 {
						t.Fatal("synthetic soul bank was spent", p)
					}
					guarded, err := planAncients(context.Background(), smallAllocationSave(wallet, chor, morgulis), reserve, 0, false)
					if err != nil {
						t.Fatal(wallet, reserve, chor, morgulis, err)
					}
					assertAncientPlanLedger(t, guarded)
					for _, row := range guarded.Rows {
						target, err := strconv.Atoi(row.Target)
						if err != nil || row.Current != "1" || row.Quantity != strconv.Itoa(target-1) {
							t.Fatal("incorrect guarded quantity", row, err)
						}
						maximum, raw := wantBase, target*(target+1)/2-1
						if row.ID == 16 && morgulis {
							maximum, raw = wantBase*wantBase, target-1
						} else if row.ID != 19 {
							t.Fatal("unexpected guarded purchase", row)
						}
						if target <= 1 || target > maximum || exactPlanAmount(t, row.Cost).Cmp(new(big.Rat).SetInt64(price(raw))) < 0 {
							t.Fatal("unsafe guarded allocation", wallet, reserve, chor, row)
						}
					}
				}
			}
		}
	}
}

func TestAncientLargeAllocationInvariants(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	save, err := decodeAncientSave(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	save.HeroSouls = "1e1000"
	p, err := planAncients(context.Background(), save, "1%", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) == 0 {
		t.Fatal("missing large purchases")
	}
	sum := new(big.Rat)
	for _, row := range p.Rows {
		delta := new(big.Rat).Sub(exactPlanAmount(t, row.Target), exactPlanAmount(t, row.Current))
		quantity := exactPlanAmount(t, row.Quantity)
		if !quantity.IsInt() || quantity.Sign() <= 0 || quantity.Cmp(delta) > 0 {
			t.Fatal("unsafe large quantity", row)
		}
		integer := new(big.Int).Quo(delta.Num(), delta.Denom())
		unit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(max(0, len(integer.String())-6))), nil)
		want := new(big.Int).Mul(new(big.Int).Quo(integer, unit), unit)
		if quantity.Num().Cmp(want) != 0 {
			t.Fatal("incorrect six-digit truncation", row)
		}
		sum.Add(sum, exactPlanAmount(t, row.Cost))
	}
	souls, reserved := exactPlanAmount(t, p.Souls), exactPlanAmount(t, p.Reserve)
	remaining, spent := exactPlanAmount(t, p.Remaining), exactPlanAmount(t, p.Spent)
	// Each serialized field has 100 significant digits. Bound their independent
	// rounding errors by one wallet-scale decimal unit per field, not a reference tolerance.
	tolerance := new(big.Rat).Mul(new(big.Rat).Quo(souls, exactPlanAmount(t, "1e99")), new(big.Rat).SetInt64(int64(len(p.Rows)+3)))
	ledgerError := new(big.Rat).Abs(new(big.Rat).Sub(sum, spent))
	balanceError := new(big.Rat).Abs(new(big.Rat).Sub(new(big.Rat).Sub(souls, spent), remaining))
	if ledgerError.Cmp(tolerance) > 0 || balanceError.Cmp(tolerance) > 0 || remaining.Cmp(reserved) < 0 || spent.Cmp(new(big.Rat).Sub(souls, reserved)) > 0 {
		t.Fatal("incorrect large plan ledger", p)
	}
	if souls.Cmp(exactPlanAmount(t, "1e1000")) != 0 || reserved.Cmp(exactPlanAmount(t, "1e998")) != 0 {
		t.Fatal("incorrect large wallet or percentage reserve", p)
	}
}

func TestAncientAllocationExponentialAndExcludedRows(t *testing.T) {
	save := smallAllocationSave(1000, 1, true)
	for id, level := range map[string]string{"13": "1", "5": "2", "27": "1", "28": "10000"} {
		save.Ancients.Ancients[id] = ancientSaveEntry{Level: json.Number(level)}
	}
	p, err := planAncients(context.Background(), save, "1%", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	foundAtman := false
	for _, row := range p.Rows {
		switch row.ID {
		case 13:
			foundAtman = true
			target, err := strconv.Atoi(row.Target)
			if err != nil || target < 2 || target > 10 {
				t.Fatal("unexpected small Atman target", row)
			}
			// Sum 2^n from level 2 through target, then apply the rational 19/20 discount.
			cost := ((1 << (target + 1)) - 4) * 19
			if exactPlanAmount(t, row.Cost).Cmp(new(big.Rat).SetInt64(int64((cost+19)/20))) < 0 || row.Quantity != strconv.Itoa(target-1) {
				t.Fatal("incorrect exponential range cost", row)
			}
		case 16, 19:
		default:
			t.Fatal("excluded or already higher Ancient was purchased", row)
		}
	}
	if !foundAtman {
		t.Fatal("missing affordable Atman purchase")
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
