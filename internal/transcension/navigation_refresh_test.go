package transcension

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func navigationController(t *testing.T, stage Stage) (*Controller, Snapshot, time.Time) {
	t.Helper()
	if stage == AwaitOutsiders {
		s, timing, native := preparation()
		c := newTestController(t, Policy{Enabled: true, SkillRate: 1}, native)
		if err := c.Begin(context.Background(), s.ExportedAt, s, timing); err != nil {
			t.Fatal(err)
		}
		return c, s, s.ExportedAt
	}
	c, s, now := beforeReset(t)
	s = resetSnapshot(s, now.Add(time.Second))
	if err := c.AcceptExport(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal(err)
	}
	return c, s, s.ExportedAt
}

func TestNavigationRequestsFreshExportAndPreservesOwnership(t *testing.T) {
	for _, stage := range []Stage{AwaitOutsiders, SpendOutsiders} {
		for _, generation := range []uint64{1, 2} {
			t.Run(string(rune('A'+stage))+string(rune('0'+generation)), func(t *testing.T) {
				c, s, start := navigationController(t, stage)
				targets := slices.Clone(c.targets)
				c.scrolls = 5
				for i := 1; i <= 2; i++ {
					now := start.Add(time.Duration(i) * 10 * time.Second)
					c.Observe(Observation{Frame: uint64(i + 10), Generation: s.Generation, At: now, Known: true, Screen: GameScreen})
					reserve(t, c, now, OpenOutsiders)
				}
				now := start.Add(30 * time.Second)
				oldObservation := Observation{Frame: 13, Generation: s.Generation, At: now, Known: true, Screen: GameScreen}
				c.Observe(oldObservation)
				oldCommand := c.Next(now)
				now = now.Add(time.Second)
				reserve(t, c, now, FreshExport)
				if c.Next(now).Action != NoAction {
					t.Fatal("duplicate export request")
				}
				fresh := cloneSnapshot(s)
				fresh.Generation, fresh.ExportedAt = generation, now.Add(time.Second)
				// Saving unchanged state may keep its hash. A later generation
				// may instead encode incidental save changes.
				if generation == 2 {
					fresh.State.SaveHash = strings.Repeat("d", 64)
				}
				if err := c.AcceptExport(context.Background(), fresh.ExportedAt, fresh); err != nil {
					t.Fatal(err)
				}
				if c.Stage() != stage || c.scrolls != 5 || !slices.Equal(c.targets, targets) || c.pending != (Command{}) || c.exportRequested {
					t.Fatal("refresh lost stage, bound, targets or ownership")
				}
				oldObservation.At = fresh.ExportedAt
				c.Observe(oldObservation)
				if c.Allowed(fresh.ExportedAt, oldCommand) || c.Next(fresh.ExportedAt).Action != NoAction {
					t.Fatal("old capture permitted replay")
				}
				frame := uint64(14)
				if generation == 2 {
					frame = 1
				}
				c.Observe(Observation{Frame: frame, Generation: generation, At: fresh.ExportedAt, Known: true, Screen: GameScreen})
				reserve(t, c, fresh.ExportedAt, OpenOutsiders)
				// The refreshed source and original fixed plan survive restart.
				c = reopenController(t, c)
				fresh.ExportedAt = fresh.ExportedAt.Add(time.Second)
				if err := c.Recover(context.Background(), fresh.ExportedAt, fresh); err != nil {
					t.Fatal(err)
				}
				want := stage
				if stage == AwaitOutsiders {
					want = Ordinary // Restart abandons a decision before reset.
				}
				if c.Stage() != want || c.snapshot.State.SaveHash != fresh.State.SaveHash || !slices.Equal(c.targets, targets) {
					t.Fatal("refreshed source or fixed plan lost on restart")
				}
			})
		}
	}
}

func TestNavigationRefreshRejectsChangesAndPendingInputs(t *testing.T) {
	for _, change := range []string{"profile", "cycle", "wallet", "levels", "ancients", "souls", "ascensions", "zone", "highest-zone", "old-generation", "old-export"} {
		t.Run(change, func(t *testing.T) {
			c, s, now := navigationController(t, SpendOutsiders)
			s = cloneSnapshot(s)
			s.ExportedAt = now.Add(time.Second)
			switch change {
			case "profile":
				s.State.ProfileID = strings.Repeat("e", 64)
			case "cycle":
				s.State.Transcensions++
			case "wallet":
				s.State.AncientSouls--
			case "levels":
				s.State.Outsiders[0].Level = "1"
			case "ancients":
				missing, err := c.MissingAncients(context.Background())
				if err != nil || len(missing) == 0 {
					t.Fatal("missing Ancient fixture", err)
				}
				s.State.Ancients = []ancientcalc.Level{{ID: missing[0].ID, Name: missing[0].Name, Level: "1"}}
			case "souls":
				s.State.HeroSouls = "1"
			case "ascensions":
				s.State.Ascensions++
			case "zone":
				*s.State.CurrentZone++
			case "highest-zone":
				s.State.HighestZone++
			case "old-generation":
				c.snapshot.Generation = 2
			case "old-export":
				s.ExportedAt = now
			}
			if c.Refresh(context.Background(), s.ExportedAt, s) == nil || c.Stage() != Uncertain {
				t.Fatal("changed/stale source admitted")
			}
		})
	}
	for _, action := range []Action{ConfirmReset, FeedOutsider} {
		c, s, now := beforeReset(t)
		if action == FeedOutsider {
			s = resetSnapshot(s, now.Add(time.Second))
			if err := c.AcceptExport(context.Background(), s.ExportedAt, s); err != nil {
				t.Fatal(err)
			}
			now = s.ExportedAt
			c.Observe(outsiderObservation(s, 4, now))
			reserve(t, c, now, FeedOutsider)
		}
		pending, stage := c.pending, c.stage
		s.ExportedAt = now.Add(time.Second)
		if c.Refresh(context.Background(), s.ExportedAt, s) == nil || c.pending != pending || c.stage != stage {
			t.Fatal("refresh consumed pending receipt ownership")
		}
	}
}

func TestNavigationRefreshDoesNotBypassNativeInactivityTimeout(t *testing.T) {
	c, s, now := navigationController(t, SpendOutsiders)
	now = now.Add(31 * time.Second)
	if c.Next(now).Action != NoAction || c.Stage() != Uncertain {
		t.Fatal("inactive native stage silently refreshed")
	}
	s.ExportedAt = now.Add(time.Second)
	if c.Refresh(context.Background(), s.ExportedAt, s) == nil {
		t.Fatal("refresh bypassed interrupted recovery")
	}
	if err := c.Recover(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != SpendOutsiders {
		t.Fatal("unchanged post-reset stage cannot recover", err)
	}
}

func TestNavigationRefreshKeepsNativeAdmissionAndFailsClosedOnJournalError(t *testing.T) {
	c, s, now := navigationController(t, SpendOutsiders)
	c.native = NativeEvidence{Build: s.State.Build}
	s.ExportedAt = now.Add(time.Second)
	if err := c.Refresh(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal(err)
	}
	c.Observe(outsiderObservation(s, 4, s.ExportedAt))
	if c.Next(s.ExportedAt).Action != NoAction {
		t.Fatal("refresh inferred native acceptance")
	}
	original := journalSyncFile
	journalSyncFile = func(*os.File) error { return errors.New("injected refresh sync failure") }
	defer func() { journalSyncFile = original }()
	s.ExportedAt = s.ExportedAt.Add(time.Second)
	if c.Refresh(context.Background(), s.ExportedAt, s) == nil || c.Stage() != Uncertain || !c.journal.poisoned {
		t.Fatal("failed persistence left navigation executable")
	}
}
