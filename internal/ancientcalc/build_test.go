package ancientcalc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
)

type hybridReference struct {
	Goals []struct {
		Name, Base, Old, Ratio, Alpha, Rate, Expected string
		Beyond8k                                      bool
	}
	Cases []struct {
		Name, Save, Ratio, Reserve, Spent, Remaining string
		SkillRate                                    float64
		Beyond8k                                     bool
		Rows                                         []Purchase
	}
}

func readHybridReference(t *testing.T) hybridReference {
	t.Helper()
	data, err := os.ReadFile("testdata/hybrid-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref hybridReference
	if err = json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Goals) != 104 || len(ref.Cases) != 4 {
		t.Fatalf("missing Hybrid vectors: %d/%d", len(ref.Goals), len(ref.Cases))
	}
	return ref
}

func TestHybridReferenceGoals(t *testing.T) {
	for _, tc := range readHybridReference(t).Goals {
		t.Run(tc.Name+"/"+tc.Base+"/"+tc.Ratio+"/"+tc.Alpha, func(t *testing.T) {
			got := hybridAncientGoal(tc.Name, aConst(tc.Base), aConst(tc.Old), aConst(tc.Alpha), aConst(tc.Rate), aConst(tc.Ratio), tc.Beyond8k)
			expected := aConst(tc.Expected)
			if got == nil {
				t.Fatal("reference goal missing", tc.Name)
			}
			if aNew().Abs(aSub(got, expected)).Cmp(aMul(aMax(aNew().Abs(expected), aConst("1")), aConst("1e-90"))) > 0 {
				t.Fatalf("%s != %s", aString(got), tc.Expected)
			}
		})
	}
}

func TestHybridReferenceAllocation(t *testing.T) {
	for _, tc := range readHybridReference(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			save, err := decodeAncientSave(context.Background(), []byte(tc.Save))
			if err != nil {
				t.Fatal(err)
			}
			p, err := planAncientsBuildWithPrice(context.Background(), save, tc.Reserve, tc.SkillRate, tc.Beyond8k, HybridBuild, aConst(tc.Ratio), legacyAncientPrice)
			if err != nil {
				t.Fatal(err)
			}
			if p.Spent != tc.Spent || p.Remaining != tc.Remaining || len(p.Rows) != len(tc.Rows) {
				t.Fatalf("different reference ledger/roster: %s/%s/%d want %s/%s/%d", p.Spent, p.Remaining, len(p.Rows), tc.Spent, tc.Remaining, len(tc.Rows))
			}
			for i, row := range p.Rows {
				want := tc.Rows[i]
				if row.ID != want.ID || row.Current != want.Current || row.Target != want.Target || row.Cost != want.Cost {
					t.Fatalf("row %s: %+v != %+v", row.Name, row, want)
				}
			}
		})
	}
}

func TestCalculateBuildKeepsActiveDefault(t *testing.T) {
	data, err := os.ReadFile("testdata/ancient-calculator-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Cases []struct {
			Name  string
			Input struct {
				Save, Reserve string
				SkillRate     float64
				Beyond8k      bool
			}
		}
	}
	if err = json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	for _, tc := range ref.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			exported := []byte(tc.Input.Save)
			want, err := Calculate(context.Background(), exported, tc.Input.Reserve, tc.Input.SkillRate, tc.Input.Beyond8k)
			if err != nil {
				t.Fatal(err)
			}
			got, err := CalculateBuild(nil, exported, BuildOptions{Reserve: tc.Input.Reserve, SkillRate: tc.Input.SkillRate, Beyond8k: tc.Input.Beyond8k})
			if err != nil || got.Mode != ActiveBuild || got.RequiredUnassignedAutoClickers != 0 || !reflect.DeepEqual(got.Plan, want) {
				t.Fatal("Active default changed", err)
			}
		})
	}
}

func TestHybridOwnedPurchasesAndSourceFacts(t *testing.T) {
	for _, tc := range readHybridReference(t).Cases {
		t.Run(tc.Name, func(t *testing.T) {
			exported := []byte(tc.Save)
			out, err := CalculateBuild(nil, exported, BuildOptions{Mode: HybridBuild, Reserve: tc.Reserve, SkillRate: tc.SkillRate, Beyond8k: tc.Beyond8k, HybridRatio: tc.Ratio})
			if err != nil {
				t.Fatal(err)
			}
			assertAncientPlanLedger(t, out.Plan)
			if err = out.Plan.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(out.SummoningRequired) != 0 || out.RequiredUnassignedAutoClickers != 1 || out.HighestZone != "20000" || out.AncientSoulsTotal != "0" || len(out.SaveHash) != 64 || len(out.Outsiders) != 9 || out.Outsiders[0].Level != "10" {
				t.Fatal("missing source/dependency facts", out)
			}
			for _, name := range []string{"Siyalatas", "Libertas", "Nogardnit"} {
				found := false
				for _, row := range out.Plan.Rows {
					if row.Name == name {
						found = true
					}
				}
				if !found {
					t.Fatal("missing owned idle purchase", name)
				}
			}
			morg := false
			for _, row := range out.Plan.Rows {
				if row.ID == 16 {
					morg = true
					if new(big.Rat).Mul(exactPlanAmount(t, row.Quantity), big.NewRat(11, 1)).Cmp(new(big.Rat).Mul(exactPlanAmount(t, row.Cost), big.NewRat(10, 1))) <= 0 {
						t.Fatal("Morgulis is worse than retained souls")
					}
				}
			}
			if tc.Name == "owned-morgulis" && !morg {
				t.Fatal("owned Morgulis omitted")
			}
			if tc.Name != "owned-morgulis" && (morg || exactPlanAmount(t, out.Plan.Remaining).Cmp(exactPlanAmount(t, out.Plan.Reserve)) <= 0) {
				t.Fatal("virtual soul bank spent")
			}
		})
	}
}

func changeHybridSave(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(readHybridReference(t).Cases[0].Save)
	if err != nil {
		t.Fatal(err)
	}
	var save map[string]any
	if err = json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	change(save)
	raw, err = json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(raw))
}

func TestHybridMissingAncientsDoNotSummon(t *testing.T) {
	exported := changeHybridSave(t, func(s map[string]any) {
		ancients := s["ancients"].(map[string]any)["ancients"].(map[string]any)
		delete(ancients, "4")
		ancients["32"].(map[string]any)["level"] = "0e2"
		delete(ancients, "19")
	})
	out, err := CalculateBuild(nil, exported, BuildOptions{Mode: HybridBuild, SkillRate: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.HybridRatio != "0.5" || len(out.SummoningRequired) != 3 {
		t.Fatal("missing summoning dependencies", out)
	}
	for _, row := range out.Plan.Rows {
		if row.ID == 4 || row.ID == 32 || row.ID == 19 {
			t.Fatal("attempted to purchase absent Ancient", row)
		}
	}
	exported = changeHybridSave(t, func(s map[string]any) { delete(s["ancients"].(map[string]any)["ancients"].(map[string]any), "5") })
	out, err = CalculateBuild(nil, exported, BuildOptions{Mode: HybridBuild, SkillRate: 1})
	if err == nil || !strings.Contains(err.Error(), "Siyalatas must be owned") || len(out.SummoningRequired) != 1 || len(out.Plan.Rows) != 0 {
		t.Fatal("missing base allowed a purchase plan", out, err)
	}
}

func TestHybridInvalidInputAndCancellation(t *testing.T) {
	exported := []byte(readHybridReference(t).Cases[0].Save)
	for _, options := range []BuildOptions{{Mode: "idle"}, {Mode: ActiveBuild, HybridRatio: "0.5"}, {Mode: HybridBuild, HybridRatio: "0"}, {Mode: HybridBuild, HybridRatio: "-1"}, {Mode: HybridBuild, HybridRatio: "NaN"}, {Mode: HybridBuild, Reserve: "101%"}, {Mode: HybridBuild, SkillRate: 2}} {
		if _, err := CalculateBuild(nil, exported, options); err == nil {
			t.Fatal("invalid build accepted", options)
		}
	}
	for _, change := range []func(map[string]any){func(s map[string]any) { delete(s, "heroSouls") }, func(s map[string]any) {
		s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["1"].(map[string]any)["level"] = "1.5"
	}, func(s map[string]any) {
		s["ancients"].(map[string]any)["ancients"].(map[string]any)["32"].(map[string]any)["level"] = "bad"
	}} {
		if _, err := CalculateBuild(nil, changeHybridSave(t, change), BuildOptions{Mode: HybridBuild, SkillRate: 1}); err == nil {
			t.Fatal("malformed save accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CalculateBuild(ctx, exported, BuildOptions{Mode: HybridBuild}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
}

func TestHybridLargeBudgetAndReserve(t *testing.T) {
	exported := changeHybridSave(t, func(s map[string]any) { s["heroSouls"] = "1e1000" })
	out, err := CalculateBuild(nil, exported, BuildOptions{Mode: HybridBuild, Reserve: "1%", SkillRate: 1})
	if err != nil {
		t.Fatal(err)
	}
	assertAncientPlanLedger(t, out.Plan)
	if out.Plan.Reserve != "1e+998" {
		t.Fatal("reserve changed", out.Plan.Reserve)
	}
	out, err = CalculateBuild(nil, exported, BuildOptions{Mode: HybridBuild, Reserve: "100%", SkillRate: 1})
	if err != nil || len(out.Plan.Rows) != 0 || out.Plan.Remaining != out.Plan.Souls {
		t.Fatal("spent protected budget", out, err)
	}
}
