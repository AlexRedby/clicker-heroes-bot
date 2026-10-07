package ancientcalc

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
)

func achievementExport(t *testing.T, payload string, raw bool) []byte {
	t.Helper()
	var b bytes.Buffer
	var w io.WriteCloser
	header := "7a990d405d2c6fb93aa8fbb0ec1a3b23"
	if raw {
		w, _ = flate.NewWriter(&b, flate.DefaultCompression)
		header = "7e8bb5a89f2842ac4af01b3b7e228592"
	} else {
		w = zlib.NewWriter(&b)
	}
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return []byte(header + base64.StdEncoding.EncodeToString(b.Bytes()))
}

const achievementJSON = `{"uniqueId":"synthetic-profile","readPatchNumber":"1.0e12-6144","achievements":{"113":true,"114":false},"collectedAchievements":{"999":true},"goldQuestsCompleted":0,"relicQuestsCompleted":4,"rubyQuestsCompleted":"123","skillQuestsCompleted":6,"heroSoulQuestsCompleted":7,"total5MinuteQuests":8,"totalClicks":1000.0,"totalBossKills":1e3,"totalHeroLevels":99,"highestFinishedZone":20,"highestFinishedZonePersist":999,"transcendentHighestFinishedZone":5000}`

func TestReadAchievementState(t *testing.T) {
	for _, raw := range []bool{false, true} {
		s, err := ReadAchievementState(nil, achievementExport(t, achievementJSON, raw))
		if err != nil || len(s.ProfileID) != 64 || !s.OwnershipKnown || !s.Earned[113] || s.Earned[114] || s.Earned[999] {
			t.Fatalf("ownership: %+v %v", s, err)
		}
		for _, c := range s.Counters {
			if !c.Known {
				t.Fatalf("counter unavailable: %+v", c)
			}
		}
		if c := s.Counters["goldQuestsCompleted"]; !c.Known || c.Value != 0 {
			t.Fatalf("actual zero: %+v", c)
		}
		if s.Counters["highestFinishedZone"].Value != 20 || s.Counters["totalBossKills"].Value != 1000 {
			t.Fatal("counter path/decimal representation changed")
		}
	}
	for _, bad := range []string{`null`, `true`, `-1`, `0.5`, `"NaN"`, `9007199254740992`, `{}`} {
		payload := strings.Replace(achievementJSON, `"goldQuestsCompleted":0`, `"goldQuestsCompleted":`+bad, 1)
		s, err := ReadAchievementState(nil, achievementExport(t, payload, false))
		if err != nil || s.Counters["goldQuestsCompleted"].Known || s.Counters["goldQuestsCompleted"].Reason != "malformed counter" || !s.Counters["total5MinuteQuests"].Known {
			t.Fatalf("invalid %s: %+v %v", bad, s, err)
		}
	}
	for _, tc := range []struct{ from, to, reason string }{
		{`"goldQuestsCompleted":0,`, ``, "missing counter"},
		{`1.0e12-6144`, `1.0e12-9999`, "unsupported save build"},
	} {
		s, err := ReadAchievementState(nil, achievementExport(t, strings.Replace(achievementJSON, tc.from, tc.to, 1), false))
		if err != nil || s.Counters["goldQuestsCompleted"].Known || s.Counters["goldQuestsCompleted"].Reason != tc.reason || !s.OwnershipKnown {
			t.Fatalf("unavailable: %+v %v", s, err)
		}
	}
	for _, bad := range []string{`null`, `{"113":null}`, `{"0113":true}`, `{"113":"true"}`} {
		payload := strings.Replace(achievementJSON, `{"113":true,"114":false}`, bad, 1)
		s, err := ReadAchievementState(nil, achievementExport(t, payload, false))
		if err != nil || s.OwnershipKnown || len(s.Earned) != 0 || s.OwnershipReason == "" {
			t.Fatalf("bad ownership: %+v %v", s, err)
		}
	}
	for _, payload := range []string{`null`, `[]`, `{`, strings.Replace(achievementJSON, `"uniqueId":"synthetic-profile",`, ``, 1)} {
		if _, err := ReadAchievementState(nil, achievementExport(t, payload, false)); err == nil {
			t.Fatalf("invalid payload %s accepted", payload)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadAchievementState(ctx, achievementExport(t, achievementJSON, true)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
