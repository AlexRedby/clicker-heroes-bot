package ancientcalc

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func TestTimelapseGildPreparationUsesForecastAndKeepsGuards(t *testing.T) {
	for _, name := range []string{"earlier forecast hero", "needs levels", "missing upgrade", "not hired", "no gain", "nil history", "earlier receipt", "same receipt", "pending receipt", "epoch changed", "no move", "insufficient reserve", "unsupported late hero", "stale forecast", "invalid forecast", "canceled"} {
		t.Run(name, func(t *testing.T) {
			f := newGildFixture()
			setGildHero(f, 41, 1000, 0, false)
			setGildHero(f, 42, 1000, 0, false)
			f.Upgrades["196"] = true
			history := currentGildHistory()
			forecast := TimelapseGildForecast{HeroID: 41, Level: 1000, WithoutTransferZones: 100, WithTransferZones: 200}
			reserve, reason, wantError := "0", "", false
			switch name {
			case "needs levels":
				forecast.Level = 1001
				reason = "required preparation level"
			case "missing upgrade":
				f.Upgrades["196"] = false
				reason = "upgrades are not purchased"
			case "not hired":
				setGildHero(f, 41, 0, 0, true)
				reason = "not purchased"
			case "no gain":
				forecast.WithTransferZones = forecast.WithoutTransferZones
				reason = "no zone gain"
			case "nil history":
				history = nil
				reason = "history is required"
			case "earlier receipt":
				history.LastTargetID = 42
				reason = "not later"
			case "same receipt":
				history.LastTargetID = 41
				reason = "not later"
			case "pending receipt":
				history.PendingTargetID = 41
				reason = "unresolved"
			case "epoch changed":
				history.Transcensions++
				reason = "identity changed"
			case "no move":
				setGildHero(f, 41, 1000, 2, false)
				setGildHero(f, 43, 0, 0, true)
				reason = "already on the target"
			case "insufficient reserve":
				reserve = "1"
				reason = "protected reserve"
			case "unsupported late hero":
				forecast.HeroID = 47
				setGildHero(f, 47, 1000, 0, false)
				reason = "outside the supported"
			case "stale forecast", "invalid forecast", "canceled":
				wantError = true
			}
			save := f.save(t, false)
			forecast.SaveHash = fmt.Sprintf("%x", sha256.Sum256(save))
			if name == "stale forecast" {
				f.WorldResets = 4
				save = f.save(t, false)
			}
			if name == "invalid forecast" {
				forecast.WithoutTransferZones = -1
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "canceled" {
				cancel()
			}
			p, err := CalculateTimelapseGilds(ctx, save, reserve, history, forecast)
			if wantError {
				if err == nil || p.Eligible || !p.PreviewOnly {
					t.Fatalf("invalid forecast accepted: %+v %v", p, err)
				}
				if name == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation error: %v", err)
				}
				return
			}
			if err != nil || !p.PreviewOnly || p.Target.ID != forecast.HeroID || p.SaveHash != forecast.SaveHash {
				t.Fatalf("forecast preview: %+v %v", p, err)
			}
			if reason == "" {
				if !p.Eligible || p.Cost != "160" || p.Remaining != "0" {
					t.Fatalf("exact-fit forecast target: %+v", p)
				}
				active, err := CalculateGilds(context.Background(), save, reserve, history)
				if err != nil || active.Target.ID != 42 {
					t.Fatalf("ordinary latest-hero policy changed: %+v %v", active, err)
				}
			} else if p.Eligible || !strings.Contains(strings.Join(p.Reasons, ";"), reason) {
				t.Fatalf("missing block %q: %+v", reason, p)
			}
		})
	}
}

func TestTimelapseGildPreparationBoundsInputBeforeHashing(t *testing.T) {
	for _, save := range [][]byte{nil, make([]byte, MaxSaveInput+1)} {
		p, err := CalculateTimelapseGilds(context.Background(), save, "0", nil, TimelapseGildForecast{})
		if err == nil || !strings.Contains(err.Error(), "4 MiB") || !p.PreviewOnly || p.Eligible {
			t.Fatalf("unbounded save accepted: %+v %v", p, err)
		}
	}
}

type gildFixture struct {
	Souls         any
	WorldResets   any
	Transcensions any
	Timestamp     any
	Heroes        map[string]any
	Upgrades      map[string]bool
}

func newGildFixture() gildFixture {
	heroes := make(map[string]any, 54)
	for id := 1; id <= 54; id++ {
		heroes[strconv.Itoa(id)] = map[string]any{
			"id": id, "uid": id, "level": 0, "epicLevel": 0, "locked": false,
		}
	}
	f := gildFixture{Souls: "160", WorldResets: 3, Transcensions: 1, Timestamp: 100, Heroes: heroes, Upgrades: map[string]bool{"200": true}}
	setGildHero(f, 43, 0, 2, true)
	return f
}

func (f gildFixture) save(t *testing.T, raw bool) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"heroSouls": f.Souls, "numWorldResets": f.WorldResets,
		"numberOfTranscensions": f.Transcensions, "transcensionTimestamp": f.Timestamp,
		"heroCollection": map[string]any{"heroes": f.Heroes}, "upgrades": f.Upgrades,
	})
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	var writer io.WriteCloser
	if raw {
		writer, err = flate.NewWriter(&compressed, flate.DefaultCompression)
	} else {
		writer = zlib.NewWriter(&compressed)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	header := "7a990d405d2c6fb93aa8fbb0ec1a3b23"
	if raw {
		header = "7e8bb5a89f2842ac4af01b3b7e228592"
	}
	return []byte(header + base64.StdEncoding.EncodeToString(compressed.Bytes()))
}

func setGildHero(f gildFixture, id, level, gilds int, locked bool) {
	f.Heroes[strconv.Itoa(id)] = map[string]any{
		"id": id, "uid": id, "level": level, "epicLevel": gilds, "locked": locked,
	}
}

func gildPlan(t *testing.T, f gildFixture, reserve string, history *GildHistory) GildPlan {
	t.Helper()
	plan, err := CalculateGilds(context.Background(), f.save(t, false), reserve, history)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func currentGildHistory() *GildHistory {
	return &GildHistory{Transcensions: 1, TranscensionTimestamp: 100}
}

func hasGildReason(plan GildPlan, want string) bool {
	for _, reason := range plan.Reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestCalculateGildsMovesLockedGildsAndAcceptsRawAndZlib(t *testing.T) {
	for _, raw := range []bool{false, true} {
		f := newGildFixture()
		setGildHero(f, 42, 1000, 0, false)
		plan, err := CalculateGilds(context.Background(), f.save(t, raw), "0", currentGildHistory())
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Eligible || !plan.PreviewOnly || plan.Target.ID != 42 || plan.MoveGilds != 2 || plan.TotalGilds != 2 {
			t.Fatalf("raw=%v plan=%+v", raw, plan)
		}
		if plan.Cost != "160" || plan.Remaining != "0" || plan.Reserve != "0" || plan.Allowance != "160" {
			t.Fatalf("raw=%v amounts: %+v", raw, plan)
		}
	}
}

func TestCalculateGildsBudgetReserveAndHugeWallet(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 43, 0, 2, true)
	setGildHero(f, 42, 1000, 0, false)
	f.Souls = "159"
	short := gildPlan(t, f, "0", currentGildHistory())
	if short.Eligible || short.Remaining != "-1" || !hasGildReason(short, "transfer would exceed the wallet or protected reserve") {
		t.Fatalf("one-soul shortage: %+v", short)
	}
	f.Souls = "200"
	reserved := gildPlan(t, f, "100", currentGildHistory())
	if reserved.Eligible || reserved.Remaining != "40" || reserved.Reserve != "100" {
		t.Fatalf("reserve should block purchase: %+v", reserved)
	}
	f.Souls = "1e1000"
	huge := gildPlan(t, f, "0", currentGildHistory())
	if !huge.Eligible || huge.Cost != "160" || strings.ContainsAny(huge.Cost, "eE") {
		t.Fatalf("huge wallet formatting: %+v", huge)
	}
}

func TestCalculateGildsUpgradeThresholds(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 42, 4000, 0, false)
	f.Upgrades = map[string]bool{"200": true, "201": true}
	blocked := gildPlan(t, f, "0", currentGildHistory())
	if blocked.Eligible || len(blocked.MissingUpgrades) != 1 || blocked.MissingUpgrades[0] != 202 {
		t.Fatalf("missing threshold upgrade: %+v", blocked)
	}
	f.Upgrades["202"] = true
	allowed := gildPlan(t, f, "0", currentGildHistory())
	if !allowed.Eligible {
		t.Fatalf("threshold upgrades should allow plan: %+v", allowed)
	}
}

func TestCalculateGildsBlocksNilHistoryAndUnsupportedMaxTarget(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 42, 1000, 0, false)
	if plan, err := CalculateGilds(context.Background(), f.save(t, false), "0", nil); err != nil || plan.Eligible || !hasGildReason(plan, "transfer history is required; reconcile before application") {
		t.Fatalf("nil history: plan=%+v err=%v", plan, err)
	}
	setGildHero(f, 47, 2000, 0, false)
	plan := gildPlan(t, f, "0", currentGildHistory())
	if plan.Target.ID != 47 || plan.Eligible || !hasGildReason(plan, "latest hero is outside the supported Atlas-Xavira progression") {
		t.Fatalf("unsupported max target fell back: %+v", plan)
	}
}

func TestCalculateGildsAcceptsIntegerNumericForms(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 42, 1000, 0, false)
	f.WorldResets, f.Transcensions, f.Timestamp = "3.0", "1", "100.0"
	plan := gildPlan(t, f, "0", currentGildHistory())
	if !plan.Eligible {
		t.Fatalf("integer numeric forms rejected: %+v", plan)
	}
}

func TestCalculateGildsHistoryBlocksTargetAndEpoch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		history GildHistory
		mutate  func(*gildFixture)
	}{
		{"same target", GildHistory{LastTargetID: 42}, nil},
		{"pending target", GildHistory{PendingTargetID: 42}, nil},
		{"lower target", GildHistory{LastTargetID: 42}, func(f *gildFixture) {
			setGildHero(*f, 41, 1000, 0, false)
			f.Heroes["42"].(map[string]any)["level"] = 0
			f.Upgrades["196"] = true
		}},
		{"transcension epoch", GildHistory{Transcensions: 1, TranscensionTimestamp: 100}, func(f *gildFixture) { f.Transcensions = 2 }},
		{"timestamp epoch", GildHistory{Transcensions: 1, TranscensionTimestamp: 100}, func(f *gildFixture) { f.Timestamp = 101 }},
		{"ascension keeps lock", GildHistory{LastTargetID: 42}, func(f *gildFixture) { f.WorldResets = 4 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGildFixture()
			setGildHero(f, 42, 1000, 0, false)
			tc.history.Transcensions, tc.history.TranscensionTimestamp = 1, 100
			if tc.mutate != nil {
				tc.mutate(&f)
			}
			plan := gildPlan(t, f, "0", &tc.history)
			if plan.Eligible || !plan.PreviewOnly {
				t.Fatalf("history guard not applied: %+v", plan)
			}
			want := map[string]string{
				"same target":          "target is not later than the last transferred hero",
				"pending target":       "an attempted transfer is unresolved",
				"lower target":         "target is not later than the last transferred hero",
				"transcension epoch":   "Transcension identity changed; reconcile transfer history",
				"timestamp epoch":      "Transcension identity changed; reconcile transfer history",
				"ascension keeps lock": "target is not later than the last transferred hero",
			}[tc.name]
			if !hasGildReason(plan, want) {
				t.Fatalf("missing reason %q: %+v", want, plan)
			}
		})
	}
}

func TestCalculateGildsRejectsMalformedRosterAndNumbers(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*gildFixture)
	}{
		{"unknown hero key", func(f *gildFixture) { f.Heroes["55"] = f.Heroes["1"] }},
		{"missing hero key", func(f *gildFixture) { delete(f.Heroes, "54") }},
		{"mismatched hero id", func(f *gildFixture) { f.Heroes["1"].(map[string]any)["id"] = 2 }},
		{"negative level", func(f *gildFixture) { f.Heroes["1"].(map[string]any)["level"] = -1 }},
		{"fractional world reset", func(f *gildFixture) { f.WorldResets = 3.5 }},
		{"missing required field", func(f *gildFixture) { f.Souls = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGildFixture()
			tc.mutate(&f)
			if _, err := CalculateGilds(context.Background(), f.save(t, false), "0", &GildHistory{}); err == nil {
				t.Fatal("malformed save accepted")
			}
		})
	}
}

func TestCalculateGildsHonorsCancellation(t *testing.T) {
	f := newGildFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CalculateGilds(ctx, f.save(t, false), "0", currentGildHistory())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestCalculateGildsZeroAndAlreadyOnTarget(t *testing.T) {
	f := newGildFixture()
	f.Heroes["43"].(map[string]any)["epicLevel"] = 0
	setGildHero(f, 42, 1000, 2, false)
	plan := gildPlan(t, f, "0", currentGildHistory())
	if plan.Eligible || plan.TotalGilds != 2 || plan.MoveGilds != 0 || !hasGildReason(plan, "all gilds are already on the target") {
		t.Fatalf("all gilds on target: %+v", plan)
	}
	f.Heroes["43"].(map[string]any)["epicLevel"] = 0
	f.Heroes["42"].(map[string]any)["epicLevel"] = 0
	plan = gildPlan(t, f, "0", currentGildHistory())
	if plan.Eligible || !hasGildReason(plan, "no gilds") {
		t.Fatalf("zero gilds: %+v", plan)
	}
}

func TestCalculateGildsTargetLevelAndReservePercentage(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 42, 999, 0, false)
	plan := gildPlan(t, f, "0", currentGildHistory())
	if plan.Eligible || !hasGildReason(plan, "latest hero has not reached level 1000") {
		t.Fatalf("level 999: %+v", plan)
	}
	f.Heroes["42"].(map[string]any)["level"] = 1000
	f.Souls = "200"
	plan = gildPlan(t, f, "20%", currentGildHistory())
	if !plan.Eligible || plan.Reserve != "40" || plan.Remaining != "40" {
		t.Fatalf("percentage exact fit: %+v", plan)
	}
	plan = gildPlan(t, f, "20.5%", currentGildHistory())
	if plan.Eligible || plan.Remaining != "40" || !hasGildReason(plan, "transfer would exceed the wallet or protected reserve") {
		t.Fatalf("percentage shortage: %+v", plan)
	}
}

func TestCalculateGildsRejectsInvalidGildFieldsAndReserve(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*gildFixture)
		reserve string
	}{
		{"negative epic level", func(f *gildFixture) { f.Heroes["43"].(map[string]any)["epicLevel"] = -1 }, "0"},
		{"fractional epic level", func(f *gildFixture) { f.Heroes["43"].(map[string]any)["epicLevel"] = 1.5 }, "0"},
		{"missing locked", func(f *gildFixture) { delete(f.Heroes["43"].(map[string]any), "locked") }, "0"},
		{"invalid reserve", func(*gildFixture) {}, "-1"},
		{"reserve percentage over 100", func(*gildFixture) {}, "100.1%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGildFixture()
			tc.mutate(&f)
			if _, err := CalculateGilds(context.Background(), f.save(t, false), tc.reserve, currentGildHistory()); err == nil {
				t.Fatal("invalid gild input accepted")
			}
		})
	}
}

func TestCalculateGildsRejectsDuplicateHeroKey(t *testing.T) {
	f := newGildFixture()
	payload, err := json.Marshal(map[string]any{
		"heroSouls": f.Souls, "numWorldResets": f.WorldResets,
		"numberOfTranscensions": f.Transcensions, "transcensionTimestamp": f.Timestamp,
		"heroCollection": map[string]any{"heroes": json.RawMessage(`{"1":{"id":1,"uid":1,"level":0,"epicLevel":0,"locked":false},"1":{"id":1,"uid":1,"level":0,"epicLevel":0,"locked":false}}`)},
		"upgrades":       f.Upgrades,
	})
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	exported := []byte("7a990d405d2c6fb93aa8fbb0ec1a3b23" + base64.StdEncoding.EncodeToString(compressed.Bytes()))
	if _, err := CalculateGilds(context.Background(), exported, "0", currentGildHistory()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate hero key: %v", err)
	}
}

func TestCalculateGildsExactHugeWalletRemaining(t *testing.T) {
	f := newGildFixture()
	setGildHero(f, 42, 1000, 0, false)
	f.Souls = "1e1000"
	plan := gildPlan(t, f, "0", currentGildHistory())
	wallet, ok := new(big.Int).SetString("1"+strings.Repeat("0", 1000), 10)
	if !ok {
		t.Fatal("cannot build expected wallet")
	}
	want := new(big.Int).Sub(wallet, big.NewInt(160)).String()
	if !plan.Eligible || plan.Remaining != want {
		t.Fatalf("huge wallet remaining = %q, want %q", plan.Remaining, want)
	}
}
