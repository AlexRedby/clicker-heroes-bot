package ancientcalc

import (
	"context"
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

const tlStateJSON = `{"uniqueId":"sanitized-profile-1","readPatchNumber":"1.0e12-6144","currentZoneHeight":40,"highestFinishedZonePersist":20000,"rubies":3303.0,"autoclickers":3,"numWorldResets":93.0,"numberOfTranscensions":4,"heroSouls":"1","primalSouls":"1e100000","stats":{"currentAscension":{"heroSoulsStart":"2.339e58"}},"ancients":{"ancients":{"4":{"level":"2e28"},"5":{"level":"3e28"},"32":{"level":"9e22"}}},"outsiders":{"outsiders":{"1":{"level":0},"2":{"level":6},"6":{"level":14}}}}`

func tlSave(raw string) []byte { return []byte(base64.StdEncoding.EncodeToString([]byte(raw))) }

func TestReadTimelapseStateUsesAscensionCapitalAndHash(t *testing.T) {
	s, err := ReadTimelapseState(context.Background(), tlSave(tlStateJSON))
	if err != nil {
		t.Fatal(err)
	}
	if s.Build != "1.0e12-6144" || s.Zone != 40 || s.HighestZone != 20000 || s.Rubies != 3303 || s.AutoClickers != 3 || s.AscensionID != "4/93" || s.Ancients["32"] != "9e22" || s.Outsiders["2"] != "6" {
		t.Fatal(s)
	}
	if len(s.ProfileID) != 64 || strings.Contains(s.ProfileID, "sanitized") || math.Abs(s.LogAscensionStartSouls-(58+math.Log10(2.339))) > 1e-12 {
		t.Fatal(s)
	}
	again, err := ReadTimelapseState(nil, tlSave(strings.Replace(tlStateJSON, `"rubies":3303.0`, `"rubies":4000`, 1)))
	if err != nil || again.ProfileID != s.ProfileID {
		t.Fatalf("unstable identity %+v %v", again, err)
	}
}
func TestReadTimelapseStateRejectsMissingOrInvalidEvidence(t *testing.T) {
	for _, bad := range []string{
		strings.Replace(tlStateJSON, `"uniqueId":"sanitized-profile-1"`, `"uniqueId":""`, 1),
		strings.Replace(tlStateJSON, `"heroSoulsStart":"2.339e58"`, `"heroSoulsStart":"0"`, 1),
		strings.Replace(tlStateJSON, `"rubies":3303.0`, `"rubies":3303.5`, 1),
		strings.Replace(tlStateJSON, `"rubies":3303.0`, `"rubies":-1`, 1),
		strings.Replace(tlStateJSON, `"currentZoneHeight":40`, `"currentZoneHeight":20002`, 1),
		strings.Replace(tlStateJSON, `"autoclickers":3`, `"autoclickers":2000000001`, 1),
		strings.Replace(tlStateJSON, `"2e28"`, `"NaN"`, 1),
	} {
		if _, err := ReadTimelapseState(nil, tlSave(bad)); err == nil {
			t.Fatal("accepted invalid state")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadTimelapseState(ctx, tlSave(tlStateJSON)); err != context.Canceled {
		t.Fatal(err)
	}
}
