package ancientcalc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func receiptFixture(t *testing.T, reset bool, change func(map[string]any)) []byte {
	t.Helper()
	return prestigeFixture(t, func(s map[string]any) {
		s["uniqueId"] = "synthetic-receipt-profile"
		s["currentZoneHeight"] = 17439
		if reset {
			s["numberOfTranscensions"], s["numAscensionsThisTranscension"], s["numWorldResets"] = 9, 0, 0
			s["ancientSoulsTotal"], s["ancientSouls"], s["currentZoneHeight"] = 328, 18, 1
			s["heroSouls"] = "0"
			s["ancients"] = map[string]any{"ancients": map[string]any{}}
			s["stats"] = map[string]any{
				"currentAscension":    map[string]any{"id": 0, "transcensionId": 9},
				"currentTranscension": map[string]any{"id": 9},
			}
		}
		if change != nil {
			change(s)
		}
	})
}

func readReceiptState(t *testing.T, reset bool, change func(map[string]any)) TranscensionState {
	t.Helper()
	s, err := ReadTranscensionState(context.Background(), receiptFixture(t, reset, change))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func copyReceiptState(t *testing.T, state TranscensionState) TranscensionState {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var out TranscensionState
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReadTranscensionStateFactsAndMissingMetadata(t *testing.T) {
	input := prestigeFixture(t, nil)
	unchanged := append([]byte(nil), input...)
	s, err := ReadTranscensionState(nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if s.Transcensions != 8 || s.Ascensions != 93 || s.AscensionsThisTranscension != 13 || s.AncientSouls != 0 || s.AncientSoulsTotal != 310 || s.HighestZone != 17439 || s.SaveVersion != 7 || s.Build != "1.0e12-6144" {
		t.Fatalf("incorrect source facts: %+v", s)
	}
	if s.CurrentZone != nil || s.ProfileID != "" || len(s.Outsiders) != 9 || len(s.Ancients) == 0 || *s.CurrentTranscensionID != 8 || *s.CurrentAscensionID != 13 || *s.CurrentAscensionCycleID != 8 {
		t.Fatalf("missing metadata invented or roster lost: %+v", s)
	}
	if !bytes.Equal(input, unchanged) || !validStateDigest(s.SaveHash) {
		t.Fatal("snapshot changed input or omitted source hash")
	}
	for i, want := range []int{0, 6, 36, 15, 14, 5, 1, 3, 3} {
		if s.Outsiders[i].ID != outsiderIDs[i] || s.Outsiders[i].Level != strconv.Itoa(want) {
			t.Fatal("noncanonical Outsider mapping", s.Outsiders)
		}
	}
	bound := readReceiptState(t, false, nil)
	if bound.ProfileID == "" || bound.CurrentZone == nil || *bound.CurrentZone != 17439 {
		t.Fatal("present profile/zone metadata lost")
	}
	encoded, err := json.Marshal(bound)
	if err != nil || bytes.Contains(encoded, []byte("synthetic-receipt-profile")) || bytes.Contains(encoded, []byte("uniqueId")) {
		t.Fatal("snapshot exposed raw profile identity")
	}
	withoutStats := readReceiptState(t, true, func(s map[string]any) { delete(s, "stats") })
	if withoutStats.CurrentAscensionID != nil || withoutStats.CurrentTranscensionID != nil {
		t.Fatal("snapshot invented cycle metadata")
	}
	// Empty Ancient ownership is valid for reset inspection, while Calculate's
	// existing base-Ancient requirement remains unchanged.
	if _, err := Calculate(context.Background(), receiptFixture(t, true, nil), "0", 0, false); err == nil {
		t.Fatal("Calculate accepted an empty Ancient roster")
	}
}

func TestReadTranscensionStateRejectsInvalidLedgerAndSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unsupported build", func(s map[string]any) { s["readPatchNumber"] = "1.0e13-6144" }},
		{"unsupported version", func(s map[string]any) { s["version"] = 8 }},
		{"missing AS wallet", func(s map[string]any) { delete(s, "ancientSouls") }},
		{"negative AS wallet", func(s map[string]any) { s["ancientSouls"] = -1 }},
		{"negative AS total", func(s map[string]any) { s["ancientSoulsTotal"] = -1 }},
		{"negative Transcension count", func(s map[string]any) { s["numberOfTranscensions"] = -1 }},
		{"negative Ascension count", func(s map[string]any) { s["numWorldResets"] = -1 }},
		{"negative current Ascension count", func(s map[string]any) { s["numAscensionsThisTranscension"] = -1 }},
		{"AS ledger gap", func(s map[string]any) { s["ancientSoulsTotal"] = 311 }},
		{"overspent ledger", func(s map[string]any) { s["ancientSouls"] = 1 }},
		{"missing investment", func(s map[string]any) {
			delete(s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any), "spentAncientSouls")
		}},
		{"fractional Outsider", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any)["level"] = "6.5"
		}},
		{"negative Outsider", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any)["level"] = -1
		}},
		{"negative Outsider investment", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any)["spentAncientSouls"] = -1
		}},
		{"wrong Outsider investment", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["2"].(map[string]any)["spentAncientSouls"] = 22
		}},
		{"obsolete Outsider owned", func(s map[string]any) {
			s["outsiders"].(map[string]any)["outsiders"].(map[string]any)["4"].(map[string]any)["level"] = 1
		}},
		{"unknown Ancient", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["99"] = map[string]any{"level": 1}
		}},
		{"noncanonical Ancient ID", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["019"] = map[string]any{"level": 1}
		}},
		{"fractional Ancient", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["19"].(map[string]any)["level"] = "1.5"
		}},
		{"negative Ancient", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["19"].(map[string]any)["level"] = -1
		}},
		{"negative Ancient investment", func(s map[string]any) {
			s["ancients"].(map[string]any)["ancients"].(map[string]any)["19"].(map[string]any)["spentHeroSouls"] = "-1"
		}},
		{"negative HS", func(s map[string]any) { s["heroSouls"] = "-1" }},
		{"negative sacrificed HS", func(s map[string]any) { s["heroSoulsSacrificed"] = "-1" }},
		{"invalid current zone", func(s map[string]any) { s["currentZoneHeight"] = 0 }},
		{"cycle mismatch", func(s map[string]any) {
			s["stats"].(map[string]any)["currentAscension"].(map[string]any)["transcensionId"] = 7
		}},
		{"negative cycle metadata", func(s map[string]any) {
			s["stats"].(map[string]any)["currentTranscension"].(map[string]any)["id"] = -1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadTranscensionState(context.Background(), receiptFixture(t, false, tc.change)); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestVerifyTranscensionReceipt(t *testing.T) {
	before, after := readReceiptState(t, false, nil), readReceiptState(t, true, nil)
	if err := VerifyTranscensionReceipt(nil, before, after, 18); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*TranscensionState)
		reward int
	}{
		{"zero reward", func(s *TranscensionState) {}, 0},
		{"negative reward", func(s *TranscensionState) {}, -1},
		{"wrong reward", func(s *TranscensionState) {}, 17},
		{"stale hash", func(s *TranscensionState) { s.SaveHash = before.SaveHash }, 18},
		{"missing profile", func(s *TranscensionState) { s.ProfileID = "" }, 18},
		{"wrong profile", func(s *TranscensionState) { s.ProfileID = strings.Repeat("1", 64) }, 18},
		{"missing zone", func(s *TranscensionState) { s.CurrentZone = nil }, 18},
		{"not reset zone", func(s *TranscensionState) { *s.CurrentZone = 2 }, 18},
		{"missing cycle", func(s *TranscensionState) { s.CurrentAscensionID = nil }, 18},
		{"stale cycle", func(s *TranscensionState) {
			s.Transcensions = 8
			*s.CurrentTranscensionID = 8
			*s.CurrentAscensionCycleID = 8
		}, 18},
		{"skipped cycle", func(s *TranscensionState) {
			s.Transcensions = 10
			*s.CurrentTranscensionID = 10
			*s.CurrentAscensionCycleID = 10
		}, 18},
		{"wrong current Ascension ID", func(s *TranscensionState) { *s.CurrentAscensionID = 1 }, 18},
		{"wrong current Ascension cycle", func(s *TranscensionState) { *s.CurrentAscensionCycleID = 8 }, 18},
		{"ascended after reset", func(s *TranscensionState) { s.AscensionsThisTranscension = 1; *s.CurrentAscensionID = 1 }, 18},
		{"Hero Souls retained", func(s *TranscensionState) { s.HeroSouls = "1" }, 18},
		{"tiny Hero Souls retained", func(s *TranscensionState) { s.HeroSouls = "1e-1000" }, 18},
		{"Ancient already summoned", func(s *TranscensionState) { s.Ancients = []Level{{19, "Fragsworth", "1"}} }, 18},
		{"respec occurred", func(s *TranscensionState) { s.Outsiders[1].Level = "5"; s.AncientSouls += 6 }, 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := copyReceiptState(t, after)
			tc.change(&changed)
			if err := VerifyTranscensionReceipt(context.Background(), before, changed, tc.reward); err == nil {
				t.Fatal("invalid reset receipt accepted")
			}
		})
	}
	missingZone := copyReceiptState(t, after)
	missingZone.CurrentZone = nil
	if err := VerifyTranscensionReceipt(nil, before, missingZone, 18); err == nil || !strings.Contains(err.Error(), "currentZoneHeight") {
		t.Fatal("missing reset metadata not named", err)
	}
}

func feedReceipt(t *testing.T, id, quantity int) (TranscensionState, TranscensionState) {
	t.Helper()
	before := readReceiptState(t, true, nil)
	after := readReceiptState(t, true, func(s map[string]any) {
		entries := s["outsiders"].(map[string]any)["outsiders"].(map[string]any)
		row := entries[strconv.Itoa(id)].(map[string]any)
		for i, current := range before.Outsiders {
			if current.ID == id {
				level, _ := strconv.Atoi(current.Level)
				cost, err := OutsiderFeedCost(id, level, quantity)
				if err != nil {
					t.Fatal(err)
				}
				row["level"], row["spentAncientSouls"] = level+quantity, outsiderCost(i, level)+cost
				s["ancientSouls"] = before.AncientSouls - cost
			}
		}
	})
	return before, after
}

func TestVerifyOutsiderFeedReceipt(t *testing.T) {
	for _, tc := range []struct{ id, quantity int }{{2, 1}, {2, 2}, {3, 3}} {
		before, after := feedReceipt(t, tc.id, tc.quantity)
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(after)
		if err := VerifyOutsiderFeedReceipt(nil, before, after, tc.id, tc.quantity); err != nil {
			t.Fatal(tc, err)
		}
		beforeNow, _ := json.Marshal(before)
		afterNow, _ := json.Marshal(after)
		if !bytes.Equal(beforeJSON, beforeNow) || !bytes.Equal(afterJSON, afterNow) {
			t.Fatal("successful FEED verification mutated source state")
		}
	}
	before, after := feedReceipt(t, 2, 1)
	for _, tc := range []struct {
		name         string
		change       func(*TranscensionState)
		id, quantity int
	}{
		{"wrong ID", func(s *TranscensionState) {}, 3, 1},
		{"unknown ID", func(s *TranscensionState) {}, 4, 1},
		{"wrong quantity", func(s *TranscensionState) {}, 2, 2},
		{"zero quantity", func(s *TranscensionState) {}, 2, 0},
		{"negative quantity", func(s *TranscensionState) {}, 2, -1},
		{"stale hash", func(s *TranscensionState) { s.SaveHash = before.SaveHash }, 2, 1},
		{"wrong profile", func(s *TranscensionState) { s.ProfileID = strings.Repeat("2", 64) }, 2, 1},
		{"reward during feed", func(s *TranscensionState) { s.AncientSoulsTotal++; s.AncientSouls++ }, 2, 1},
		{"other row fed", func(s *TranscensionState) { s.Outsiders[2].Level = "37"; s.AncientSouls-- }, 2, 1},
		{"manual HS change", func(s *TranscensionState) { s.HeroSouls = "1" }, 2, 1},
		{"manual Ancient change", func(s *TranscensionState) { s.Ancients = []Level{{19, "Fragsworth", "1"}} }, 2, 1},
		{"zone changed", func(s *TranscensionState) { *s.CurrentZone = 2 }, 2, 1},
		{"cycle changed", func(s *TranscensionState) { s.AscensionsThisTranscension++; *s.CurrentAscensionID++ }, 2, 1},
		{"lifetime Ascension counter changed", func(s *TranscensionState) { s.Ascensions++ }, 2, 1},
		{"invalid wallet", func(s *TranscensionState) { s.AncientSouls++ }, 2, 1},
		{"duplicate roster", func(s *TranscensionState) { s.Outsiders[0] = s.Outsiders[1] }, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := copyReceiptState(t, after)
			tc.change(&changed)
			beforeJSON, _ := json.Marshal(before)
			afterJSON, _ := json.Marshal(changed)
			if err := VerifyOutsiderFeedReceipt(context.Background(), before, changed, tc.id, tc.quantity); err == nil {
				t.Fatal("invalid feed receipt accepted")
			}
			beforeNow, _ := json.Marshal(before)
			afterNow, _ := json.Marshal(changed)
			if !bytes.Equal(beforeJSON, beforeNow) || !bytes.Equal(afterJSON, afterNow) {
				t.Fatal("failed FEED verification mutated source state")
			}
		})
	}
}

func TestMissingActiveAncientsAndCancellation(t *testing.T) {
	state := readReceiptState(t, true, nil)
	missing, err := MissingActiveAncients(nil, state, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, row := range missing {
		seen[row.Name] = true
	}
	for _, name := range []string{"Fragsworth", "Bhaal", "Juggernaut", "Mammon", "Kumawakamaru", "Morgulis"} {
		if !seen[name] {
			t.Fatal("missing Active dependency", name)
		}
	}
	for _, name := range []string{"Libertas", "Siyalatas", "Nogardnit", "Revolc", "Berserker", "Vaagur"} {
		if seen[name] {
			t.Fatal("irrelevant or disabled dependency", name)
		}
	}
	state.Ancients = []Level{{19, "Fragsworth", "1"}}
	withSkills, err := MissingActiveAncients(nil, state, .5, true)
	if err != nil {
		t.Fatal(err)
	}
	seen = make(map[string]bool)
	for _, row := range withSkills {
		seen[row.Name] = true
	}
	if seen["Fragsworth"] || !seen["Berserker"] || !seen["Vaagur"] {
		t.Fatal("owned or skill-rate policy lost", withSkills)
	}
	before, after := readReceiptState(t, false, nil), readReceiptState(t, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, err := range map[string]error{
		"reset": VerifyTranscensionReceipt(ctx, before, after, 18),
		"feed":  VerifyOutsiderFeedReceipt(ctx, after, after, 3, 1),
	} {
		if !errors.Is(err, context.Canceled) {
			t.Fatal(name, err)
		}
	}
	if _, err := ReadTranscensionState(ctx, prestigeFixture(t, nil)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := MissingActiveAncients(ctx, state, .5, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
