package transcension

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func newTestController(t *testing.T, policy Policy, native NativeEvidence) *Controller {
	t.Helper()
	c := NewController(policy, native)
	j, err := OpenJournal(filepath.Join(t.TempDir(), "transcension.json"), strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	if err := c.BindJournal(j); err != nil {
		t.Fatal(err)
	}
	return c
}

func preparation() (Snapshot, Timing, NativeEvidence) {
	now := time.Unix(1000, 0)
	zone, cycle, asc := 300, 0, 6
	s := Snapshot{Generation: 1, ExportedAt: now, State: ancientcalc.TranscensionState{
		SaveHash: strings.Repeat("a", 64), ProfileID: strings.Repeat("b", 64), Build: "1.0e12-6144", SaveVersion: 7,
		Ascensions: 6, AscensionsThisTranscension: 6, HighestZone: 300, CurrentZone: &zone,
		CurrentTranscensionID: &cycle, CurrentAscensionCycleID: &cycle, CurrentAscensionID: &asc,
		HeroSouls: "100", Ancients: []ancientcalc.Level{},
	}}
	ids := []int{1, 2, 3, 5, 6, 7, 8, 9, 10}
	names := []string{"Xyliqil", "Chor'gorloth", "Phandoryss", "Ponyboy", "Borb", "Rhageist", "K'Ariqua", "Orphalas", "Sen-Akhan"}
	for i, id := range ids {
		s.State.Outsiders = append(s.State.Outsiders, ancientcalc.Level{ID: id, Name: names[i], Level: "0"})
	}
	gain := 20
	s.Preview = ancientcalc.TranscensionPreview{SaveHash: s.State.SaveHash, Build: s.State.Build, SaveVersion: 7, Ascensions: 6, HighestZone: 300, EstimatedASGain: &gain}
	timing := Timing{At: now, Generation: 1, SaveHash: s.State.SaveHash, CompletedLoops: 2, WallConfirmed: true, PreviousASPerHour: 10, ASPerHour: 5}
	native := NativeEvidence{Build: s.State.Build, ResetRecovery: true, Feed: true, AncientSummon: true}
	return s, timing, native
}

func reserve(t *testing.T, c *Controller, now time.Time, action Action) Command {
	t.Helper()
	cmd := c.Next(now)
	if cmd.Action != action || !c.Allowed(now, cmd) {
		t.Fatalf("stage=%d wanted action=%d got=%+v reason=%s", c.Stage(), action, cmd, c.Reason())
	}
	if err := c.Reserve(now, cmd); err != nil {
		t.Fatal(err)
	}
	if c.Allowed(now, cmd) {
		t.Fatal("reserved command can be replayed", cmd)
	}
	return cmd
}

func beforeConfirmation(t *testing.T) (*Controller, Snapshot, time.Time) {
	t.Helper()
	s, timing, native := preparation()
	c := newTestController(t, Policy{Enabled: true, SkillRate: 1}, native)
	now := s.ExportedAt
	if err := c.Begin(context.Background(), now, s, timing); err != nil {
		t.Fatal(err)
	}
	c.Observe(Observation{Frame: 1, Generation: 1, At: now, Screen: GameScreen, Known: true, EntryKnown: true})
	reserve(t, c, now, OpenOutsiders)
	c.Observe(Observation{Frame: 2, Generation: 1, At: now, Screen: OutsidersScreen, Known: true, OpenKnown: true, Reward: 20, Quantity: 1, Rows: []Row{{ID: 1, Name: "Xyliqil", Cost: 1}}})
	reserve(t, c, now, OpenReset)
	c.Observe(Observation{Frame: 3, Generation: 1, At: now, Screen: ConfirmationScreen, Known: true, ConfirmKnown: true, CancelKnown: true, RespecKnown: true, Reward: 20})
	return c, s, now
}

func beforeReset(t *testing.T) (*Controller, Snapshot, time.Time) {
	t.Helper()
	c, s, now := beforeConfirmation(t)
	reserve(t, c, now, ConfirmReset)
	return c, s, now
}

func resetSnapshot(s Snapshot, now time.Time) Snapshot {
	s = cloneSnapshot(s)
	s.State.SaveHash = strings.Repeat("c", 64)
	s.State.Transcendent, s.State.Transcensions, s.State.AscensionsThisTranscension = true, 1, 0
	*s.State.CurrentTranscensionID, *s.State.CurrentAscensionCycleID, *s.State.CurrentAscensionID = 1, 1, 0
	*s.State.CurrentZone = 1
	s.State.AncientSouls, s.State.AncientSoulsTotal, s.State.HeroSouls = 20, 20, "0"
	s.ExportedAt = now
	return s
}

func outsiderObservation(s Snapshot, frame uint64, now time.Time) Observation {
	o := Observation{Frame: frame, Generation: s.Generation, At: now, Screen: OutsidersScreen, Known: true, Wallet: s.State.AncientSouls, Quantity: 1, QuantityKnown: true}
	for _, row := range s.State.Outsiders {
		level, _ := strconv.Atoi(row.Level)
		cost, _ := ancientcalc.OutsiderFeedCost(row.ID, level, 1)
		o.Rows = append(o.Rows, Row{ID: row.ID, Name: row.Name, Level: level, Cost: cost, FeedKnown: true})
	}
	return o
}

func TestResetFeedBootstrapAndAncientRestoration(t *testing.T) {
	c, before, now := beforeReset(t)
	reserve(t, c, now, FreshExport)
	now = now.Add(time.Second)
	s := resetSnapshot(before, now)
	if err := c.AcceptExport(context.Background(), now, s); err != nil || c.Stage() != SpendOutsiders {
		t.Fatal("reset receipt", err, c.Stage())
	}
	frame, inputs, quantity := uint64(4), 0, 1
	for inputs < 21 {
		o := outsiderObservation(s, frame, now)
		o.Quantity = quantity
		for i := range o.Rows {
			o.Rows[i].Cost, _ = ancientcalc.OutsiderFeedCost(o.Rows[i].ID, o.Rows[i].Level, quantity)
		}
		c.Observe(o)
		cmd := c.Next(now)
		if cmd.Action == BootstrapHeroes {
			reserve(t, c, now, BootstrapHeroes)
			break
		}
		if cmd.Action == SelectQuantity {
			cmd = reserve(t, c, now, SelectQuantity)
			quantity = cmd.Quantity
			frame++
			continue
		}
		cmd = reserve(t, c, now, FeedOutsider)
		if c.Next(now).Action != FreshExport || cmd.Cost > s.State.AncientSouls || cmd.Quantity != quantity {
			t.Fatal("feed did not require an exact affordable fixed-quantity receipt", cmd)
		}
		reserve(t, c, now, FreshExport)
		s = cloneSnapshot(s)
		for i := range s.State.Outsiders {
			if s.State.Outsiders[i].ID == cmd.ID {
				level, _ := strconv.Atoi(s.State.Outsiders[i].Level)
				s.State.Outsiders[i].Level = strconv.Itoa(level + cmd.Quantity)
			}
		}
		s.State.AncientSouls -= cmd.Cost
		s.State.SaveHash = fmt.Sprintf("%064x", inputs+100)
		now = now.Add(time.Second)
		s.ExportedAt = now
		if err := c.AcceptExport(context.Background(), now, s); err != nil {
			t.Fatal("feed receipt", err)
		}
		frame++
		inputs++
	}
	if inputs == 0 || c.Stage() != AwaitFirstSouls || c.HoldsGameplay() || !c.HoldsAncientAllocation() {
		t.Fatal("bootstrap did not hand off to ordinary hero/Ascension flow", inputs, c.Stage())
	}
	// A reset HUD or pending primal souls do not prove banked first-run souls.
	now = now.Add(time.Second)
	s.State.SaveHash, s.ExportedAt = strings.Repeat("d", 64), now
	if err := c.AcceptExport(context.Background(), now, s); err != nil || c.Stage() != AwaitFirstSouls {
		t.Fatal("zero Hero Souls progressed to summon", err)
	}
	s.State.AscensionsThisTranscension, *s.State.CurrentAscensionID = 1, 1
	s.State.HeroSouls, s.State.SaveHash = "1000", strings.Repeat("e", 64)
	now = now.Add(time.Second)
	s.ExportedAt = now
	if err := c.AcceptExport(context.Background(), now, s); err != nil || c.Stage() != AwaitAncients {
		t.Fatal("first souls did not request summon", err, c.Stage())
	}
	missing, err := c.MissingAncients(context.Background())
	if err != nil || len(missing) == 0 || c.Next(now).Action != NoAction {
		t.Fatal("missing Ancients are not a purchase in an owned-Ancient list", missing, err)
	}
	for _, ancient := range missing {
		s.State.Ancients = append(s.State.Ancients, ancientcalc.Level{ID: ancient.ID, Name: ancient.Name, Level: "1"})
	}
	now = now.Add(time.Second)
	s.State.HeroSouls, s.State.SaveHash, s.ExportedAt = "100", strings.Repeat("f", 64), now
	if err := c.AcceptExport(context.Background(), now, s); err != nil || c.Stage() != ReadyForAllocation || c.HoldsAncientAllocation() {
		t.Fatal("restored Ancient receipt", err, c.Stage())
	}
	if c.CompleteAllocation(before.State.SaveHash) == nil || c.CompleteAllocation(s.State.SaveHash) != nil || c.Stage() != Ordinary {
		t.Fatal("allocation handoff must use the restored snapshot")
	}
}

func TestEligibilityAndUnverifiedNativeGates(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Snapshot, *Timing, *NativeEvidence, *Policy)
	}{
		{"disabled", func(_ *Snapshot, _ *Timing, _ *NativeEvidence, p *Policy) { p.Enabled = false }},
		{"no profile", func(s *Snapshot, _ *Timing, _ *NativeEvidence, _ *Policy) { s.State.ProfileID = "" }},
		{"no current zone", func(s *Snapshot, _ *Timing, _ *NativeEvidence, _ *Policy) { s.State.CurrentZone = nil }},
		{"stale export", func(s *Snapshot, _ *Timing, _ *NativeEvidence, _ *Policy) {
			s.ExportedAt = s.ExportedAt.Add(-time.Minute)
		}},
		{"unowned timing", func(_ *Snapshot, timing *Timing, _ *NativeEvidence, _ *Policy) { timing.Generation++ }},
		{"no wall", func(_ *Snapshot, timing *Timing, _ *NativeEvidence, _ *Policy) { timing.WallConfirmed = false }},
		{"different native build", func(_ *Snapshot, _ *Timing, n *NativeEvidence, _ *Policy) { n.Build = "other" }},
		{"different preview", func(s *Snapshot, _ *Timing, _ *NativeEvidence, _ *Policy) { s.Preview.SaveHash = "wrong" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, timing, native := preparation()
			now, policy := s.ExportedAt, Policy{Enabled: true}
			test.edit(&s, &timing, &native, &policy)
			c := newTestController(t, policy, native)
			if c.Begin(context.Background(), now, s, timing) == nil || c.Stage() != Ordinary || c.Next(now).Action != NoAction {
				t.Fatal("ineligible reset started")
			}
		})
	}
}

func TestF8ReconcilesReceiptWithoutReplayingReset(t *testing.T) {
	c, before, now := beforeReset(t)
	c.Interrupt()
	if c.Stage() != Uncertain || c.Next(now).Action != NoAction {
		t.Fatal("F8 did not suppress input")
	}
	bad := cloneSnapshot(before)
	bad.Generation, bad.ExportedAt, bad.State.SaveHash = 2, now.Add(time.Second), strings.Repeat("d", 64)
	if c.Reconcile(context.Background(), bad.ExportedAt, bad) == nil || c.Stage() != Uncertain {
		t.Fatal("a missed reset became replayable")
	}
	s := resetSnapshot(before, now.Add(2*time.Second))
	s.Generation = 2
	if err := c.Reconcile(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != SpendOutsiders {
		t.Fatal("valid post-F8 receipt", err)
	}
	c.Observe(outsiderObservation(before, 100, s.ExportedAt))
	if c.Next(s.ExportedAt).Action != NoAction {
		t.Fatal("old generation supplied input after reconciliation")
	}
}

func TestChangedModalAndStaleFeedCannotAuthorizeInput(t *testing.T) {
	c, s, now := beforeReset(t)
	now = now.Add(time.Second)
	s = resetSnapshot(s, now)
	if err := c.AcceptExport(context.Background(), now, s); err != nil {
		t.Fatal(err)
	}
	c.Observe(outsiderObservation(s, 4, now))
	cmd := c.Next(now)
	o := outsiderObservation(s, 5, now)
	o.Wallet++
	c.Observe(o)
	if c.Allowed(now, cmd) || c.Reserve(now, cmd) == nil {
		t.Fatal("changed wallet allowed queued FEED")
	}
	c.Observe(outsiderObservation(s, 6, now))
	reserve(t, c, now, FeedOutsider)
	if c.Next(now.Add(time.Minute)).Action != NoAction || c.Stage() != Uncertain {
		t.Fatal("unknown FEED outcome did not stop")
	}
	// Exact confirmation reward and positively recognized unchecked respec matter.
	for _, edit := range []func(*Observation){
		func(o *Observation) { o.Reward++ }, func(o *Observation) { o.Respec = true },
		func(o *Observation) { o.RespecKnown = false }, func(o *Observation) { o.ConfirmKnown = false },
	} {
		c, _, now := beforeReset(t)
		c.stage, c.pending = AwaitConfirmation, Command{}
		c.lastFrame = 2
		o := Observation{Frame: 4, Generation: 1, At: now, Screen: ConfirmationScreen, Known: true, ConfirmKnown: true, CancelKnown: true, RespecKnown: true, Reward: 20}
		edit(&o)
		c.Observe(o)
		if c.Next(now).Action != NoAction {
			t.Fatal("changed confirmation authorized reset", o)
		}
	}
}

func TestRestorationCannotSkipFirstSoulsOrAcceptOlderExports(t *testing.T) {
	_, before, now := beforeReset(t)
	s := resetSnapshot(before, now.Add(time.Second))
	_, _, native := preparation()
	c := newTestController(t, Policy{Enabled: true, SkillRate: 1}, native)
	c.snapshot, c.stage, c.inputAt = cloneSnapshot(s), AwaitFirstSouls, now
	missing, _ := c.MissingAncients(context.Background())
	for _, row := range missing {
		s.State.Ancients = append(s.State.Ancients, ancientcalc.Level{ID: row.ID, Name: row.Name, Level: "1"})
	}
	s.State.SaveHash, s.ExportedAt = strings.Repeat("e", 64), now.Add(2*time.Second)
	if err := c.AcceptExport(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != AwaitFirstSouls {
		t.Fatal("owned roster skipped first banked souls", err, c.Stage())
	}
	s.State.HeroSouls, s.State.AscensionsThisTranscension, *s.State.CurrentAscensionID = "100", 1, 1
	s.State.SaveHash, s.ExportedAt = strings.Repeat("f", 64), now.Add(3*time.Second)
	if err := c.AcceptExport(context.Background(), s.ExportedAt, s); err != nil || c.Stage() != ReadyForAllocation {
		t.Fatal(err, c.Stage())
	}
	c.stage = AwaitAncients
	s.State.SaveHash, s.ExportedAt = strings.Repeat("d", 64), now.Add(2*time.Second)
	if c.AcceptExport(context.Background(), now.Add(4*time.Second), s) == nil || c.Stage() != Uncertain {
		t.Fatal("older restoration export was accepted")
	}
}

func TestNewCycleDoesNotReuseOldGenerationFrame(t *testing.T) {
	s, timing, native := preparation()
	c := newTestController(t, Policy{Enabled: true}, native)
	c.latest = Observation{Frame: 100, Generation: 1, At: s.ExportedAt, Screen: GameScreen, Known: true, EntryKnown: true}
	c.lastFrame = 99
	s.Generation, timing.Generation = 2, 2
	if err := c.Begin(context.Background(), s.ExportedAt, s, timing); err != nil || c.Next(s.ExportedAt).Action != NoAction {
		t.Fatal("new cycle used old input state", err)
	}
	c.Observe(Observation{Frame: 1, Generation: 2, At: s.ExportedAt, Screen: GameScreen, Known: true, EntryKnown: true})
	if c.Next(s.ExportedAt).Action != OpenOutsiders {
		t.Fatal("new generation frame was rejected")
	}
}

func TestRepeatResetNeedsCompletedGrowingHistory(t *testing.T) {
	for _, name := range []string{"ready", "no history", "only two AS-growing Ascensions"} {
		t.Run(name, func(t *testing.T) {
			s, timing, native := preparation()
			s.State.Transcendent, s.State.Transcensions = true, 1
			*s.State.CurrentTranscensionID, *s.State.CurrentAscensionCycleID = 1, 1
			s.Preview.Transcendent, s.Preview.Transcensions = true, 1
			s.Preview.History.Available, s.Preview.History.ASGrowingAscensions = true, 3
			switch name {
			case "no history":
				s.Preview.History.Available = false
			case "only two AS-growing Ascensions":
				s.Preview.History.ASGrowingAscensions = 2
			}
			c := newTestController(t, Policy{Enabled: true}, native)
			err := c.Begin(context.Background(), s.ExportedAt, s, timing)
			if (err == nil) != (name == "ready") {
				t.Fatal("repeat timing decision", err)
			}
		})
	}
}
