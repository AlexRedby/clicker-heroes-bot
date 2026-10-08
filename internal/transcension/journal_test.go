package transcension

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func reopenController(t *testing.T, c *Controller) *Controller {
	t.Helper()
	path, profile := c.journal.path, c.journal.profile
	if err := c.journal.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := OpenJournal(path, profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	next := NewController(c.policy, c.native)
	if err := next.BindJournal(j); err != nil {
		t.Fatal(err)
	}
	return next
}

func TestJournalRestartReconcilesPendingResetAndFeed(t *testing.T) {
	c, before, now := beforeReset(t)
	c = reopenController(t, c)
	if c.Stage() != Uncertain || c.Next(now).Action != NoAction {
		t.Fatal("pending reset reopened as executable")
	}
	missed := cloneSnapshot(before)
	missed.Generation, missed.ExportedAt, missed.State.SaveHash = 2, now.Add(time.Second), strings.Repeat("d", 64)
	if c.Recover(context.Background(), missed.ExportedAt, missed) == nil || c.Stage() != Uncertain {
		t.Fatal("unchanged reset outcome permitted replay")
	}
	s := resetSnapshot(before, now.Add(2*time.Second))
	s.Generation = 2
	if err := c.Recover(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != SpendOutsiders {
		t.Fatal("pending reset recovery", err, c.Stage())
	}
	// Committed receipt and fixed targets also survive a second restart.
	c = reopenController(t, c)
	s.ExportedAt = s.ExportedAt.Add(time.Second)
	s.Generation = 3
	if err := c.Recover(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal("accepted reset receipt recovery", err)
	}
	c.Observe(outsiderObservation(s, 1, s.ExportedAt))
	cmd := reserve(t, c, s.ExportedAt, FeedOutsider)
	c = reopenController(t, c)
	if c.Next(s.ExportedAt).Action != NoAction || c.pending != cmd {
		t.Fatal("pending FEED was discarded on restart")
	}
	s = cloneSnapshot(s)
	for i := range s.State.Outsiders {
		if s.State.Outsiders[i].ID == cmd.ID {
			s.State.Outsiders[i].Level = "1"
		}
	}
	s.State.AncientSouls -= cmd.Cost
	s.State.SaveHash = strings.Repeat("e", 64)
	s.ExportedAt = s.ExportedAt.Add(time.Second)
	s.Generation = 4
	if err := c.Recover(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != SpendOutsiders {
		t.Fatal("pending FEED recovery", err)
	}
}

func TestNonpendingF8AndRestartRecoveryStages(t *testing.T) {
	for _, phase := range []Stage{AwaitOutsiders, AwaitConfirmation, SpendOutsiders, RestoreHeroes, AwaitFirstSouls, AwaitAncients, ReadyForAllocation} {
		for _, restart := range []bool{false, true} {
			t.Run(string(rune('A'+phase))+map[bool]string{false: "F8", true: "restart"}[restart], func(t *testing.T) {
				s, _, native := preparation()
				c := newTestController(t, Policy{Enabled: true, SkillRate: 1}, native)
				now := s.ExportedAt
				if phase >= SpendOutsiders {
					s = resetSnapshot(s, now)
					plans, err := ancientcalc.PlanOutsiders(context.Background(), s.State.AncientSoulsTotal, s.State.AncientSouls, 0, s.State.Outsiders)
					if err != nil {
						t.Fatal(err)
					}
					c.targets = plans.Now.Additions
					if phase == RestoreHeroes {
						for i := range c.targets {
							c.targets[i].Target = c.targets[i].Current
						}
					}
				}
				if phase == AwaitAncients || phase == ReadyForAllocation {
					s.State.HeroSouls, s.State.AscensionsThisTranscension, *s.State.CurrentAscensionID = "100", 1, 1
				}
				if phase == ReadyForAllocation {
					missing, err := ancientcalc.MissingActiveAncients(context.Background(), s.State, c.policy.SkillRate, c.policy.Beyond8k)
					if err != nil {
						t.Fatal(err)
					}
					for _, ancient := range missing {
						s.State.Ancients = append(s.State.Ancients, ancientcalc.Level{ID: ancient.ID, Name: ancient.Name, Level: "1"})
					}
				}
				c.snapshot, c.stage, c.inputAt = cloneSnapshot(s), phase, now
				if err := c.commit(); err != nil {
					t.Fatal(err)
				}
				if restart {
					c = reopenController(t, c)
				} else {
					c.Interrupt()
				}
				if c.Stage() != Uncertain {
					t.Fatal("interrupted phase did not stop")
				}
				fresh := cloneSnapshot(s)
				fresh.Generation, fresh.ExportedAt = 2, now.Add(time.Second)
				if err := c.Recover(context.Background(), fresh.ExportedAt, fresh); err != nil {
					t.Fatal("nonpending recovery", phase, err)
				}
				want := phase
				if phase == AwaitOutsiders || phase == AwaitConfirmation {
					want = Ordinary
				}
				if c.Stage() != want || c.latest.Frame != 0 {
					t.Fatal("wrong recovery handoff", c.Stage(), want)
				}
			})
		}
	}
}

func TestJournalFailurePreventsInputAndPoisonsUntilReopen(t *testing.T) {
	c, _, now := beforeReset(t)
	// beforeReset reserved reset once. First prove a failed later persistence
	// does not clear its durable pending ownership.
	original := journalSyncFile
	journalSyncFile = func(*os.File) error { return errors.New("injected disk failure") }
	defer func() { journalSyncFile = original }()
	if c.commit() == nil || c.Stage() != Uncertain || !c.journal.poisoned {
		t.Fatal("disk failure did not stop")
	}
	if c.Next(now).Action != NoAction || !errors.Is(c.journal.save(c.journal.state), ErrJournalPoisoned) {
		t.Fatal("poisoned journal granted another input")
	}
	journalSyncFile = original
	c = reopenController(t, c)
	if c.pending.Action != ConfirmReset || c.Next(now).Action != NoAction {
		t.Fatal("failed write lost pending reset")
	}
}

func TestJournalFailureBeforeInputNeverAuthorizesReplay(t *testing.T) {
	for _, failure := range []string{"sync", "rename", "directory"} {
		t.Run(failure, func(t *testing.T) {
			c, _, now := beforeConfirmation(t)
			cmd := c.Next(now)
			if cmd.Action != ConfirmReset {
				t.Fatal("confirmation unavailable")
			}
			syncFile, rename, syncDirectory := journalSyncFile, journalRename, journalSyncDirectory
			restore := func() { journalSyncFile, journalRename, journalSyncDirectory = syncFile, rename, syncDirectory }
			defer restore()
			injected := errors.New("injected persistence failure")
			switch failure {
			case "sync":
				journalSyncFile = func(*os.File) error { return injected }
			case "rename":
				journalRename = func(string, string) error { return injected }
			case "directory":
				journalSyncDirectory = func(string) error { return injected }
			}
			if c.Reserve(now, cmd) == nil || c.Stage() != Uncertain || c.Allowed(now, cmd) {
				t.Fatal("failed persistence authorized reset input")
			}
			restore()
			c = reopenController(t, c)
			if c.Next(now).Action != NoAction {
				t.Fatal("restarted failure authorized replay")
			}
			if failure == "directory" {
				if c.pending.Action != ConfirmReset {
					t.Fatal("replaced checkpoint lost uncertain reset ownership")
				}
			} else if c.pending != (Command{}) || c.recoveryStage != AwaitConfirmation {
				t.Fatal("failed replacement damaged prior checkpoint")
			}
		})
	}
}

func TestJournalLocksProfileIntegrityAndFilesystemBounds(t *testing.T) {
	profile, path := strings.Repeat("b", 64), filepath.Join(t.TempDir(), "state.json")
	j, err := OpenJournal(path, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenJournal(path, profile); !errors.Is(err, ErrJournalLocked) {
		t.Fatal("second process lock", err)
	}
	j.Close()
	if _, err := OpenJournal(path, strings.Repeat("c", 64)); err == nil {
		t.Fatal("profile mismatch accepted")
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, corrupt := range [][]byte{[]byte("{}"), append(append([]byte(nil), original...), []byte("\n{}")...), []byte(strings.Repeat("x", maxJournalBytes+1))} {
		if err := os.WriteFile(path, corrupt, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenJournal(path, profile); err == nil {
			t.Fatal("corrupt/oversized journal accepted")
		}
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0666); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenJournal(path, profile); err == nil {
			t.Fatal("public journal accepted")
		}
	}
}

func TestRecoveredCommandsRequireCurrentAdmissionAndJournal(t *testing.T) {
	c, before, now := beforeReset(t)
	c = reopenController(t, c)
	c.native = NativeEvidence{Build: before.State.Build}
	s := resetSnapshot(before, now.Add(time.Second))
	s.Generation = 2
	if err := c.Recover(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal("read-only receipt recovery should remain available", err)
	}
	c.Observe(outsiderObservation(s, 1, s.ExportedAt))
	if c.Next(s.ExportedAt).Action != NoAction {
		t.Fatal("old journal bypassed current native acceptance")
	}
	s, timing, native := preparation()
	noJournal := NewController(Policy{Enabled: true}, native)
	if noJournal.Begin(context.Background(), s.ExportedAt, s, timing) == nil {
		t.Fatal("reset admitted without durable ownership")
	}
}

func TestJournalRejectsBootstrapWithUnfinishedTargets(t *testing.T) {
	c, before, now := beforeReset(t)
	s := resetSnapshot(before, now.Add(time.Second))
	if err := c.AcceptExport(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal(err)
	}
	invalid := c.journal.state
	invalid.Stage = RestoreHeroes
	if err := validateJournalState(invalid, invalid.ProfileID); err == nil {
		t.Fatal("unfinished targets were discarded by a bootstrap checkpoint")
	}
}

func TestContinueEarningWaitsForAnotherFundedAscension(t *testing.T) {
	s, _, native := preparation()
	c := newTestController(t, Policy{Enabled: true, SkillRate: 1}, native)
	now := s.ExportedAt
	s = resetSnapshot(s, now)
	s.State.HeroSouls, s.State.AscensionsThisTranscension, *s.State.CurrentAscensionID = "1", 1, 1
	c.snapshot, c.stage = cloneSnapshot(s), AwaitAncients
	if err := c.ContinueEarning(); err != nil || c.Stage() != AwaitFirstSouls || c.HoldsGameplay() {
		t.Fatal("insufficient summon budget did not yield", err)
	}
	c = reopenController(t, c)
	now = now.Add(time.Second)
	s.ExportedAt = now
	s.Generation++
	if err := c.Recover(context.Background(), now, s); err != nil || c.Stage() != AwaitFirstSouls {
		t.Fatal("same banked souls immediately reopened summon", err)
	}
	now = now.Add(time.Second)
	s.ExportedAt, s.State.SaveHash, s.State.HeroSouls = now, strings.Repeat("e", 64), "2"
	s.State.AscensionsThisTranscension, *s.State.CurrentAscensionID = 2, 2
	if err := c.AcceptExport(context.Background(), now, s); err != nil || c.Stage() != AwaitAncients {
		t.Fatal("new funded Ascension did not resume summon", err)
	}
}
