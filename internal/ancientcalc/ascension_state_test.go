package ancientcalc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func ascensionStateFixture(t *testing.T, change func(map[string]any)) []byte {
	t.Helper()
	save := map[string]any{
		"uniqueId": "synthetic-ascension-profile", "version": 7, "readPatchNumber": "1.0e12-6144",
		"transcendent": true, "numberOfTranscensions": 2, "numWorldResets": 4,
		"numAscensionsThisTranscension": 0, "highestFinishedZonePersist": 131, "currentZoneHeight": 131,
		"primalSouls": "3", "totalHeroLevels": 4999, "heroSouls": "17", "heroSoulsSacrificed": "1e50",
		"stats": map[string]any{
			"currentTranscension": map[string]any{"id": 2},
			"currentAscension":    map[string]any{"id": 0, "transcensionId": 2},
		},
	}
	if change != nil {
		change(save)
	}
	plain, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(base64.StdEncoding.EncodeToString(plain))
}

func TestReadAscensionStateIndependentFacts(t *testing.T) {
	input := ascensionStateFixture(t, func(s map[string]any) {
		s["ancients"] = map[string]any{"ancients": map[string]any{
			"4":   map[string]any{"level": 1, "spentHeroSouls": "20"},
			"999": map[string]any{"level": 1, "spentHeroSouls": "5"},
			"5":   map[string]any{"level": 0},
		}}
	})
	before := append([]byte(nil), input...)
	state, err := ReadAscensionState(nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if state.PendingHeroSouls != "5" || state.HeroSouls != "17" || !state.CapitalKnown || state.SoulCapital != "42" {
		t.Fatalf("incorrect current-run economy: %+v", state)
	}
	if !state.Transcendent || state.Transcensions != 2 || state.Ascensions != 4 || state.AscensionsThisTranscension != 0 || state.HighestZone != 131 || state.CurrentZone == nil || *state.CurrentZone != 131 || state.SaveVersion != 7 || state.Build != "1.0e12-6144" {
		t.Fatalf("incorrect Ascension identity: %+v", state)
	}
	if !validStateDigest(state.ProfileID) || !validStateDigest(state.SaveHash) || *state.CurrentTranscensionID != 2 || *state.CurrentAscensionID != 0 || *state.CurrentAscensionCycleID != 2 || !bytes.Equal(before, input) {
		t.Fatal("missing identity or mutated input")
	}
	encoded, err := json.Marshal(state)
	if err != nil || bytes.Contains(encoded, []byte("synthetic-ascension-profile")) || bytes.Contains(encoded, []byte("uniqueId")) {
		t.Fatal("raw profile identity escaped reader")
	}
	other, err := ReadAscensionState(nil, ascensionStateFixture(t, func(s map[string]any) { s["uniqueId"] = "different-profile" }))
	if err != nil || other.ProfileID == state.ProfileID {
		t.Fatal("profiles were not separated", err)
	}
}

func TestReadAscensionStateMissingOptionalFacts(t *testing.T) {
	state, err := ReadAscensionState(nil, ascensionStateFixture(t, func(s map[string]any) {
		delete(s, "uniqueId")
		delete(s, "currentZoneHeight")
		delete(s, "stats")
	}))
	if err != nil || state.PendingHeroSouls != "5" || state.CapitalKnown || state.SoulCapital != "" || state.ProfileID != "" || state.CurrentZone != nil || state.CurrentAscensionID != nil || state.CurrentTranscensionID != nil || state.CurrentAscensionCycleID != nil {
		t.Fatalf("optional data invented or mandatory reward blocked: %+v, %v", state, err)
	}
	for _, tc := range []struct {
		name   string
		roster map[string]any
		known  bool
	}{
		{"empty roster", map[string]any{}, true},
		{"missing owned investment", map[string]any{"4": map[string]any{"level": 1}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, err := ReadAscensionState(nil, ascensionStateFixture(t, func(s map[string]any) { s["ancients"] = map[string]any{"ancients": tc.roster} }))
			if err != nil || state.CapitalKnown != tc.known || state.PendingHeroSouls != "5" || tc.known && state.SoulCapital != "17" || !tc.known && state.SoulCapital != "" {
				t.Fatalf("incorrect optional investment handling: %+v, %v", state, err)
			}
		})
	}
}

func TestReadAscensionStateRewardBoundariesAndLargeAmounts(t *testing.T) {
	for _, tc := range []struct {
		levels int
		primal string
		want   string
	}{
		{1999, "0", "0"}, {2000, "0", "1"}, {16777999, "0", "8388"}, {3999, "3", "4"}, {4000, "3", "5"},
		{2000, "1e1000", "1e+1000"}, {2000, "1e99999", "1e+99999"},
	} {
		state, err := ReadAscensionState(nil, ascensionStateFixture(t, func(s map[string]any) {
			s["totalHeroLevels"], s["primalSouls"] = tc.levels, tc.primal
		}))
		if err != nil || state.PendingHeroSouls != tc.want {
			t.Fatalf("reward(%d,%s)=%s, want %s (%v)", tc.levels, tc.primal, state.PendingHeroSouls, tc.want, err)
		}
		if len(state.PendingHeroSouls) > 100 || strings.Contains(state.PendingHeroSouls, "/") {
			t.Fatal("unbounded or nondecimal reward output")
		}
		if _, err := Value(state.PendingHeroSouls); err != nil {
			t.Fatal("reward cannot be consumed by soul policy:", err)
		}
	}
}

func TestReadAscensionStateRejectsInvalidRequiredFacts(t *testing.T) {
	for _, field := range []string{"version", "readPatchNumber", "transcendent", "numberOfTranscensions", "numWorldResets", "numAscensionsThisTranscension", "highestFinishedZonePersist", "primalSouls", "totalHeroLevels", "heroSouls"} {
		t.Run("missing "+field, func(t *testing.T) {
			if _, err := ReadAscensionState(nil, ascensionStateFixture(t, func(s map[string]any) { delete(s, field) })); err == nil {
				t.Fatal("missing mandatory fact accepted")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"negative pending", func(s map[string]any) { s["primalSouls"] = "-1" }},
		{"malformed pending", func(s map[string]any) { s["primalSouls"] = "NaN" }},
		{"excessive exponent", func(s map[string]any) { s["primalSouls"] = "1e100000" }},
		{"fractional hero levels", func(s map[string]any) { s["totalHeroLevels"] = "1999.5" }},
		{"excessive hero levels", func(s map[string]any) { s["totalHeroLevels"] = "9007199254740992" }},
		{"negative wallet", func(s map[string]any) { s["heroSouls"] = -1 }},
		{"unsupported build", func(s map[string]any) { s["readPatchNumber"] = "1.0e13-1" }},
		{"unsupported version", func(s map[string]any) { s["version"] = 8 }},
		{"invalid profile", func(s map[string]any) { s["uniqueId"] = "  " }},
		{"invalid zone", func(s map[string]any) { s["currentZoneHeight"] = 0 }},
		{"Transcension cycle mismatch", func(s map[string]any) { s["stats"].(map[string]any)["currentTranscension"] = map[string]any{"id": 3} }},
		{"Ascension cycle mismatch", func(s map[string]any) {
			s["stats"].(map[string]any)["currentAscension"] = map[string]any{"id": 1, "transcensionId": 2}
		}},
		{"Ascension Transcension mismatch", func(s map[string]any) {
			s["stats"].(map[string]any)["currentAscension"] = map[string]any{"id": 0, "transcensionId": 1}
		}},
		{"invalid investment", func(s map[string]any) {
			s["ancients"] = map[string]any{"ancients": map[string]any{"4": map[string]any{"level": 1, "spentHeroSouls": "-1"}}}
		}},
		{"unowned investment", func(s map[string]any) {
			s["ancients"] = map[string]any{"ancients": map[string]any{"4": map[string]any{"level": 0, "spentHeroSouls": 1}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ReadAscensionState(nil, ascensionStateFixture(t, tc.change)); err == nil {
				t.Fatal("invalid mandatory fact accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadAscensionState(ctx, ascensionStateFixture(t, nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation", err)
	}
	if _, err := ReadAscensionState(nil, []byte("not a save")); err == nil {
		t.Fatal("invalid encoding accepted")
	}
	if _, err := ReadAscensionState(nil, bytes.Repeat([]byte("x"), MaxSaveInput+1)); err == nil {
		t.Fatal("input bound lost")
	}
}
