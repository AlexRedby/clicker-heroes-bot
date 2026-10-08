package transcension

import (
	"context"
	"testing"
	"time"
)

func TestOptInRequiresObservedControlsWithoutPriorLiveAcceptance(t *testing.T) {
	s, timing, _ := preparation()
	c := newTestController(t, Policy{Enabled: true}, NativeEvidence{Build: s.State.Build})
	now := s.ExportedAt
	if err := c.Begin(context.Background(), now, s, timing); err != nil {
		t.Fatal(err)
	}
	o := Observation{Frame: 1, Generation: s.Generation, At: now, Known: true, Screen: GameScreen}
	c.Observe(o)
	if c.Next(now).Action != NoAction {
		t.Fatal("source build alone authorized a missing entry control")
	}
	o.Frame, o.EntryKnown = 2, true
	c.Observe(o)
	reserve(t, c, now, OpenOutsiders)
	o = outsiderObservation(s, 3, now)
	o.Reward, o.AtTop = 20, true
	c.Observe(o)
	if c.Next(now).Action != NoAction {
		t.Fatal("missing Transcend control authorized reset entry")
	}
	o.Frame, o.OpenKnown = 4, true
	c.Observe(o)
	reserve(t, c, now, OpenReset)
	o = Observation{Frame: 5, Generation: s.Generation, At: now, Known: true, Screen: ConfirmationScreen, ConfirmKnown: true, CancelKnown: true, Reward: 20}
	c.Observe(o)
	if c.Next(now).Action != NoAction {
		t.Fatal("unknown respec checkbox authorized reset")
	}
	o.Frame, o.RespecKnown, o.Respec = 6, true, true
	c.Observe(o)
	if c.Next(now).Action != NoAction {
		t.Fatal("respec was admitted")
	}
	o.Frame, o.Respec = 7, false
	c.Observe(o)
	reserve(t, c, now, ConfirmReset)
	after := resetSnapshot(s, now.Add(time.Second))
	if err := c.AcceptExport(context.Background(), after.ExportedAt, after); err != nil {
		t.Fatal(err)
	}
	o = outsiderObservation(after, 8, after.ExportedAt)
	for i := range o.Rows {
		o.Rows[i].FeedKnown = false
	}
	// Even prior acceptance cannot stand in for an actual visible button.
	c.native.ResetRecovery, c.native.Feed = true, true
	c.Observe(o)
	if c.Next(after.ExportedAt).Action != NoAction {
		t.Fatal("prior acceptance authorized an unknown FEED control")
	}
	o.Frame = 9
	for i := range o.Rows {
		o.Rows[i].FeedKnown = true
	}
	o.QuantityKnown = false
	c.Observe(o)
	if c.Next(after.ExportedAt).Action != NoAction {
		t.Fatal("quantity highlight alone authorized FEED")
	}
	o.Frame, o.QuantityKnown = 10, true
	c.Observe(o)
	reserve(t, c, after.ExportedAt, FeedOutsider)
}
