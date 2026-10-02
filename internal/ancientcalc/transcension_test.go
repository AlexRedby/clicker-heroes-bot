package ancientcalc

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"os"
	"strings"
	"testing"
)

func prestigeFixture(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/prestige-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	if change == nil {
		return data
	}
	compressed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data[32:])))
	if err != nil {
		t.Fatal(err)
	}
	r := flate.NewReader(bytes.NewReader(compressed))
	defer r.Close()
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	var save map[string]any
	if err := decoder.Decode(&save); err != nil {
		t.Fatal(err)
	}
	change(save)
	plain, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(plain))
}

func TestTranscensionPreviewStateAndRecovery(t *testing.T) {
	data := prestigeFixture(t, nil)
	p, err := PreviewTranscension(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if p.AncientSoulsTotal != 310 || p.AncientSouls != 0 || *p.EstimatedASGain != 18 || *p.ProjectedASWallet != 18 || *p.EstimatedASWithoutRun != 310 {
		t.Fatalf("incorrect AS preview: %+v", p)
	}
	if p.UIVerified || p.Recommendation != "continue_ascending" || p.History.CompletedAscensions != 13 || p.History.ASGrowingAscensions != 0 || p.History.LastCompleted[2].ID != 12 {
		t.Fatalf("live Ascension counted as closed: %+v", p)
	}
	if math.Abs(*p.TPBeforePercent-4.042549493188769) > 1e-10 || math.Abs(*p.TPAfterPercent-4.155414715562781) > 1e-10 {
		t.Fatal("incorrect TP estimate")
	}
	if p.Outsiders == nil || !p.Outsiders.RequiresRespec || p.Outsiders.Spent > 18 {
		t.Fatal("unsafe Outsider preview")
	}
	if !bytes.Equal(data, prestigeFixture(t, nil)) {
		t.Fatal("preview changed input")
	}
	bonus := prestigeFixture(t, func(s map[string]any) {
		s["ancients"] = map[string]any{"ancients": map[string]any{}}
		s["outsiders"] = map[string]any{"outsiders": map[string]any{}}
		delete(s, "stats")
		s["transcendent"] = false
		s["heroSouls"], s["heroSoulsSacrificed"], s["primalSouls"] = "0", "0", "0"
		s["ancientSoulsTotal"], s["ancientSouls"], s["totalHeroLevels"] = 0, 0, 20000
	})
	p, err = PreviewTranscension(context.Background(), bonus)
	if err != nil || *p.EstimatedASGain != 5 {
		t.Fatal("hero-level Ascension reward missing", err)
	}

	for _, first := range []bool{false, true} {
		input := prestigeFixture(t, func(s map[string]any) {
			s["ancients"] = map[string]any{"ancients": map[string]any{}}
			s["outsiders"] = map[string]any{"outsiders": map[string]any{}}
			delete(s, "stats")
			s["transcendent"] = !first
			s["heroSouls"], s["heroSoulsSacrificed"], s["primalSouls"] = "0", "0", "0"
			s["totalHeroLevels"], s["numberOfTranscensions"], s["numAscensionsThisTranscension"] = 0, 0, 0
			s["ancientSoulsTotal"], s["ancientSouls"], s["highestFinishedZonePersist"] = 0, 0, 0
			if first {
				s["primalSouls"], s["highestFinishedZonePersist"] = "1000", 300
			}
		})
		preview, err := PreviewTranscension(context.Background(), input)
		if err != nil {
			t.Fatal("empty Ancient roster:", err)
		}
		if first && (*preview.EstimatedASGain != 15 || preview.Recommendation != "candidate" || *preview.TPBeforePercent != 0) {
			t.Fatal("first Transcension preview")
		}
		if !first && *preview.EstimatedASGain != 0 {
			t.Fatal("invented post-reset reward")
		}
	}
}

func TestTranscensionExactASThresholds(t *testing.T) {
	for _, tc := range []struct {
		souls string
		want  int
	}{
		{"0", 0}, {"0.1", 0}, {"1", 0}, {"9", 4}, {"10", 5},
		{"999." + strings.Repeat("9", 90), 14}, {"1000", 15},
		{"1000." + strings.Repeat("0", 90) + "1", 15}, {"1e1000", 5000},
	} {
		r, ok := new(big.Rat).SetString(tc.souls)
		if !ok {
			t.Fatal(tc.souls)
		}
		if got := soulsToAS(r); got != tc.want {
			t.Fatalf("AS(%s)=%d, want %d", tc.souls, got, tc.want)
		}
	}
}

func TestTranscensionRejectsUnsafeAndUnknownState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing pending", func(s map[string]any) { delete(s, "primalSouls") }},
		{"missing state", func(s map[string]any) { delete(s, "transcendent") }},
		{"missing Ancient roster", func(s map[string]any) { delete(s, "ancients") }},
		{"negative wallet", func(s map[string]any) { s["ancientSouls"] = -1 }},
		{"fractional levels", func(s map[string]any) { s["totalHeroLevels"] = 1.5 }},
		{"overspent ledger", func(s map[string]any) { s["ancientSouls"] = 310 }},
		{"history identity", func(s map[string]any) { s["numAscensionsThisTranscension"] = 14 }},
		{"unowned negative investment", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["4"] = map[string]any{"level": 0, "spentHeroSouls": "-1"}
		}},
		{"unowned positive investment", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["4"] = map[string]any{"level": 0, "spentHeroSouls": "1"}
		}},
		{"unknown Outsider", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["11"] = map[string]any{"level": 1, "spentAncientSouls": 1}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PreviewTranscension(context.Background(), prestigeFixture(t, tc.change)); err == nil {
				t.Fatal("unsafe state accepted")
			}
		})
	}
	unknown := prestigeFixture(t, func(s map[string]any) { s["readPatchNumber"] = "1.0e13-9999" })
	p, err := PreviewTranscension(context.Background(), unknown)
	if err != nil || p.EstimatedASGain != nil || p.TPBeforePercent != nil || p.TPAfterPercent != nil || p.Outsiders != nil {
		t.Fatal("unknown mechanics produced a plan")
	}
	missingCosts := prestigeFixture(t, func(s map[string]any) {
		delete(s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any), "spentAncientSouls")
	})
	p, err = PreviewTranscension(context.Background(), missingCosts)
	if err != nil || p.Outsiders != nil {
		t.Fatal("missing accounting produced a plan")
	}
	partialHistory := prestigeFixture(t, func(s map[string]any) {
		delete(s["stats"].(map[string]any)["currentTranscension"].(map[string]any)["ascensions"].(map[string]any), "0")
	})
	p, err = PreviewTranscension(context.Background(), partialHistory)
	if err != nil || p.History.Available || p.Recommendation != "insufficient_evidence" {
		t.Fatal("incomplete history treated as zero AS growth", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PreviewTranscension(ctx, prestigeFixture(t, nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation", err)
	}
	if _, err := PreviewTranscension(context.Background(), []byte("not a save")); err == nil {
		t.Fatal("invalid save accepted")
	}
}

func TestPrestigeFixtureContainsOnlyWhitelistedData(t *testing.T) {
	data := prestigeFixture(t, nil)
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data[32:])))
	if err != nil {
		t.Fatal(err)
	}
	r := flate.NewReader(bytes.NewReader(b))
	defer r.Close()
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "email", "account", "purchaseHash", "uniqueId"} {
		if bytes.Contains(bytes.ToLower(plain), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("sensitive fixture field: %s", forbidden)
		}
	}
}
