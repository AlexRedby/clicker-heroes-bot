package transcension

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"
)

// These are synthetic exports using the installed schema, not native acceptance
// evidence. ReadSnapshot must derive reward/history instead of forged preview fields.
func timingPolicySnapshot(t *testing.T, cycle, ascensions int) Snapshot {
	t.Helper()
	history := map[string]any{}
	for id := 0; id <= ascensions; id++ {
		souls := "0"
		if id > 0 {
			souls = fmt.Sprintf("1e%d", cycle*4+id)
		}
		history[fmt.Sprint(id)] = map[string]any{
			"id": id, "transcensionId": cycle, "highestZoneEver": 300,
			"heroSoulsEnd": souls, "startTime": 1, "endTime": 2000000000000,
		}
	}
	sacrificed := "0"
	if cycle > 0 {
		sacrificed = fmt.Sprintf("1e%d", cycle*4)
	}
	input, err := json.Marshal(map[string]any{
		"version": 7, "readPatchNumber": "1.0e12-6144", "uniqueId": "synthetic-timing-profile",
		"transcendent": cycle > 0, "numberOfTranscensions": cycle,
		"numWorldResets": cycle*10 + ascensions, "numAscensionsThisTranscension": ascensions,
		"currentZoneHeight": 300, "highestFinishedZonePersist": 300,
		"ancientSoulsTotal": cycle * 20, "ancientSouls": cycle * 20,
		"heroSouls":           fmt.Sprintf("1e%d", cycle*4+ascensions+1),
		"heroSoulsSacrificed": sacrificed, "primalSouls": "1000", "totalHeroLevels": 0,
		"ancients":  map[string]any{"ancients": map[string]any{}},
		"outsiders": map[string]any{"outsiders": map[string]any{}},
		"stats": map[string]any{
			"currentAscension":    map[string]any{"id": ascensions, "transcensionId": cycle},
			"currentTranscension": map[string]any{"id": cycle, "ascensions": history},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := ReadSnapshot(context.Background(), []byte(base64.StdEncoding.EncodeToString(input)), time.Unix(2000000000, 0), 7)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTimingPolicyAdmitsInitialAndRepeatedCyclesWithoutFreshLoopWait(t *testing.T) {
	for _, cycle := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(cycle), func(t *testing.T) {
			ascensions := 0
			if cycle > 0 {
				ascensions = 4
			}
			s := timingPolicySnapshot(t, cycle, ascensions)
			if cycle > 0 && (!s.Preview.History.Available || s.Preview.History.ASGrowingAscensions != 3) {
				t.Fatal("closed growing history must come from the export", s.Preview.History)
			}
			c := NewController(Policy{Enabled: true}, NativeEvidence{Build: s.State.Build, ResetRecovery: true, Feed: true})
			journal, err := OpenJournal(filepath.Join(t.TempDir(), "transcension.json"), s.State.ProfileID)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { journal.Close() })
			if err := c.BindJournal(journal); err != nil {
				t.Fatal(err)
			}
			// Runtime startup has no new live loops or rate baseline. The
			// current wall and fresh export are owned; saved history survives.
			timing := Timing{At: s.ExportedAt, Generation: s.Generation, SaveHash: s.State.SaveHash, WallConfirmed: true}
			if cycle == 2 {
				// Advisory rates must not become an implicit blocking wait.
				timing.PreviousASPerHour, timing.ASPerHour = math.NaN(), math.Inf(1)
			}
			if err := c.Begin(context.Background(), s.ExportedAt, s, timing); err != nil {
				t.Fatal("useful wall reset blocked on missing loop/summon evidence", err)
			}
			c.Observe(Observation{Frame: 1, Generation: s.Generation, At: s.ExportedAt, Screen: GameScreen, Known: true, EntryKnown: true})
			if c.Next(s.ExportedAt).Action != OpenOutsiders {
				t.Fatal("missing summon UI blocked supported reset navigation")
			}
		})
	}
}

func TestSupportedResetAndFeedDoNotRequireSummonControls(t *testing.T) {
	c, before, now := beforeConfirmation(t)
	c.native.AncientSummon = false
	reserve(t, c, now, ConfirmReset)
	after := resetSnapshot(before, now.Add(time.Second))
	if err := c.AcceptExport(context.Background(), after.ExportedAt, after); err != nil {
		t.Fatal(err)
	}
	c.Observe(outsiderObservation(after, 4, after.ExportedAt))
	reserve(t, c, after.ExportedAt, FeedOutsider)
}
