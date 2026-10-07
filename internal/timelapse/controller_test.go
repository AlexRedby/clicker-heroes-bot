package timelapse

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/rubybudget"
)

func prepared(now time.Time) (Policy, Preparation, Row) {
	p := Policy{Enabled: true, ProtectedBuild: "1.0e12-6144", ActiveZonesPerHour: 8050, MinZonesPerRuby: 200, MinSavedTime: time.Hour}
	prep := Preparation{ProfileID: "p1", AscensionID: "a1", Build: p.ProtectedBuild, Scene: "game1", Generation: 1, ExportedAt: now, ObservedAt: now, Zone: 40, Rubies: 500, HybridVerified: true, HeroVerified: true, GildsVerified: true, UpgradesVerified: true, IdleMechanicsVerified: true, ClickerReservationActive: true, FreeClickers: 1, Hero: "Skogur", HeroLevel: 7299}
	row := Row{Hours: 8, Hero: "Skogur", Level: 7299, StartZone: 40, Zone: 36040, Rubies: 100}
	return p, prep, row
}
func nativeFor(prep Preparation) NativeEvidence {
	return NativeEvidence{Build: prep.Build, OffersAndPricesVerified: true, ConfirmationAndCancelVerified: true, ResultVerified: true}
}
func ledger(t *testing.T, allowance, cap uint64) *rubybudget.Ledger {
	t.Helper()
	l, err := rubybudget.Open(rubybudget.Config{Path: filepath.Join(t.TempDir(), "ruby.json"), ProfileID: "p1", InitialAllowance: allowance, ProtectedBalance: 200, PerAscensionCap: cap})
	if errors.Is(err, rubybudget.ErrUnsupportedDurability) {
		t.Skip("positive budgets intentionally unsupported on this platform")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
func observation(now time.Time, frame uint64, s Screen, prep Preparation, row Row) Observation {
	return Observation{Frame: frame, Generation: prep.Generation, ObservedAt: now, Scene: prep.Scene, Build: prep.Build, Screen: s, Hours: row.Hours, Price: uint64(row.Rubies), Balance: prep.Rubies, EntryKnown: true, OfferKnown: true, ConfirmKnown: true, CancelKnown: true, CloseKnown: true}
}
func fixtureController(t *testing.T, now time.Time) *Controller {
	p, prep, row := prepared(now)
	c := NewController(p, nativeFor(prep), ledger(t, 200, 100))
	c.recognition = true
	if r := c.Begin(now, prep, row); !r.Ready {
		t.Fatal(r)
	}
	return c
}
func advanceToConfirmation(t *testing.T, c *Controller, now time.Time, prep Preparation, row Row) Command {
	t.Helper()
	c.Observe(now, observation(now, 1, GameScreen, prep, row))
	cmd := c.Command(now)
	if cmd.Action != OpenShop {
		t.Fatal(cmd)
	}
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	c.Observe(now, observation(now, 2, ShopScreen, prep, row))
	cmd = c.Command(now)
	if cmd.Action != SelectOffer {
		t.Fatal(cmd)
	}
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	c.Observe(now, observation(now, 3, ConfirmationScreen, prep, row))
	return c.Command(now)
}
func TestReadinessRequiresActualPreparationAndEconomics(t *testing.T) {
	now := time.Now()
	p, prep, row := prepared(now)
	if !Assess(now, p, prep, row).Ready {
		t.Fatal("prepared case failed")
	}
	cases := []struct {
		name   string
		mutate func(*Policy, *Preparation, *Row)
	}{
		{"default-off", func(p *Policy, _ *Preparation, _ *Row) { p.Enabled = false }},
		{"ordinary", func(_ *Policy, _ *Preparation, r *Row) { r.Hours = 0 }},
		{"Active", func(_ *Policy, s *Preparation, _ *Row) { s.HybridVerified = false }},
		{"gild-preview", func(_ *Policy, s *Preparation, _ *Row) { s.GildsVerified = false }},
		{"no-free", func(_ *Policy, s *Preparation, _ *Row) { s.FreeClickers = 0 }},
		{"uncertain-clicker", func(_ *Policy, s *Preparation, _ *Row) { s.ClickerPlacementUncertain = true }},
		{"unreserved", func(_ *Policy, s *Preparation, _ *Row) { s.ClickerReservationActive = false }},
		{"stale-save", func(_ *Policy, s *Preparation, _ *Row) { s.ExportedAt = now.Add(-time.Minute) }},
		{"stale-frame", func(_ *Policy, s *Preparation, _ *Row) { s.ObservedAt = now.Add(-6 * time.Second) }},
		{"wrong-build", func(_ *Policy, s *Preparation, _ *Row) { s.Build = "mobile" }},
		{"underleveled", func(_ *Policy, s *Preparation, _ *Row) { s.HeroLevel = 1 }},
		{"price-change", func(_ *Policy, _ *Preparation, r *Row) { r.Rubies = 10 }},
		{"poor-value", func(p *Policy, _ *Preparation, _ *Row) { p.MinZonesPerRuby = 1000 }},
		{"unmeasured-time", func(p *Policy, _ *Preparation, _ *Row) { p.ActiveZonesPerHour = 0 }},
		{"idle-rule-unverified", func(_ *Policy, s *Preparation, _ *Row) { s.IdleMechanicsVerified = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, s, r := prepared(now)
			tc.mutate(&p, &s, &r)
			if Assess(now, p, s, r).Ready {
				t.Fatal("unsafe readiness")
			}
		})
	}
}
func TestNativeRecognitionStaysOffAndZeroNeverStarts(t *testing.T) {
	now := time.Now()
	p, prep, row := prepared(now)
	c := NewController(p, nativeFor(prep), ledger(t, 200, 100))
	if r := c.Begin(now, prep, row); r.Ready || !strings.Contains(r.Reason, "recognition") || c.Stage() != Ordinary {
		t.Fatal(r)
	}
	for _, limits := range [][2]uint64{{0, 100}, {99, 100}, {200, 99}} {
		c := NewController(p, nativeFor(prep), ledger(t, limits[0], limits[1]))
		c.recognition = true
		if c.Begin(now, prep, row).Ready {
			t.Fatal("budget bypass")
		}
	}
}
func TestPaidInputReservesBeforeSubmissionAndNeverReplays(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	cmd := advanceToConfirmation(t, c, now, prep, row)
	if cmd.Action != ConfirmPurchase {
		t.Fatal(cmd)
	}
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	s, err := c.budget.Snapshot()
	if err != nil || s.Pending == nil || s.Pending.Amount != 100 {
		t.Fatalf("missing durable debit %+v %v", s, err)
	}
	if c.Allowed(now, cmd, prep) || c.Command(now).Action != NoAction {
		t.Fatal("paid command replay")
	}
	c.Interrupt()
	if c.Stage() != Uncertain || !c.HoldsClickerReserve() {
		t.Fatal(c.Stage())
	}
	if c.Begin(now, prep, row).Ready {
		t.Fatal("F8 reset uncertain action")
	}
}
func TestChangedOfferCancelsWithoutDebit(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	advanceToConfirmation(t, c, now, prep, row)
	o := observation(now, 4, ConfirmationScreen, prep, row)
	o.Price = 200
	c.Observe(now, o)
	cmd := c.Command(now)
	if cmd.Action != CancelOffer {
		t.Fatal(cmd)
	}
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	s, _ := c.budget.Snapshot()
	if s.Pending != nil || s.Spent != 0 || c.Stage() != Ordinary {
		t.Fatal(s)
	}
}
func TestFreshDispatchAndBalancePreventPayment(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	cmd := advanceToConfirmation(t, c, now, prep, row)
	changed := prep
	changed.FreeClickers = 0
	if c.Allowed(now, cmd, changed) {
		t.Fatal("old queue consumed reserved clicker")
	}
	if c.Allowed(now.Add(6*time.Second), cmd, prep) {
		t.Fatal("stale input")
	}
	o := observation(now, 4, ConfirmationScreen, prep, row)
	o.Balance = 250
	c.Observe(now, o)
	cmd = c.Command(now)
	if err := c.BeforeInput(now, cmd, prep); err == nil {
		t.Fatal("other ruby spender breached reserve")
	}
	s, _ := c.budget.Snapshot()
	if s.Pending != nil || s.Spent != 0 {
		t.Fatal(s)
	}
}
func TestUnknownScreensSuppressAndResultNeedsFreshExport(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	c.Observe(now, observation(now, 1, UnknownScreen, prep, row))
	if c.Command(now).Action != NoAction {
		t.Fatal("unknown UI input")
	}
	// Use a separate transaction to retain monotonically increasing frames.
	c = fixtureController(t, now)
	cmd := advanceToConfirmation(t, c, now, prep, row)
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	c.Observe(now, observation(now, 4, GameScreen, prep, row))
	if c.Stage() != AwaitOutcome {
		t.Fatal("dialog closure counted as purchase")
	}
	o := observation(now, 5, ResultScreen, prep, row)
	o.ResultKnown = true
	o.ResultZone = row.Zone
	c.Observe(now, o)
	if c.Stage() != AwaitFreshPreparation {
		t.Fatal(c.Stage(), c.Reason())
	}
	s, _ := c.budget.Snapshot()
	if s.Spent != 100 || s.Pending != nil {
		t.Fatal(s)
	}
	if err := c.CompletePreparation(prep); err == nil {
		t.Fatal("stale outcome export accepted")
	}
	prep.ExportedAt = now.Add(time.Second)
	prep.Zone = row.Zone
	if err := c.CompletePreparation(prep); err != nil {
		t.Fatal(err)
	}
	if c.HoldsClickerReserve() || c.HoldsInputs() {
		t.Fatal("ordinary play did not resume")
	}
}

func TestPendingRestartKeepsReserveAndNeverReplays(t *testing.T) {
	now := time.Now()
	p, prep, row := prepared(now)
	l := ledger(t, 200, 100)
	if _, err := l.Reserve(100, prep.AscensionID, 500); err != nil {
		t.Fatal(err)
	}
	c := NewController(p, nativeFor(prep), l)
	if c.Stage() != Uncertain || !c.HoldsClickerReserve() || c.Command(now).Action != NoAction || c.Begin(now, prep, row).Ready {
		t.Fatal("restart replayed or released pending preparation")
	}
}
func TestGenerationChangeAndTimeoutDoNotReplay(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	o := observation(now, 1, GameScreen, prep, row)
	o.Generation++
	c.Observe(now, o)
	if c.Stage() != Ordinary || c.Command(now).Action != NoAction {
		t.Fatal("changed geometry received input")
	}
	c = fixtureController(t, now)
	cmd := advanceToConfirmation(t, c, now, prep, row)
	if err := c.BeforeInput(now, cmd, prep); err != nil {
		t.Fatal(err)
	}
	later := now.Add(21 * time.Second)
	c.Observe(later, observation(later, 4, GameScreen, prep, row))
	if c.Stage() != Uncertain || c.Command(later).Action != NoAction {
		t.Fatal("timeout replayed paid action")
	}
}

func TestMissedShopInputsAreBounded(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	for i := uint64(1); i <= 7; i++ {
		c.Observe(now, observation(now, i, GameScreen, prep, row))
		cmd := c.Command(now)
		err := c.BeforeInput(now, cmd, prep)
		if i < 7 && err != nil {
			t.Fatal(err)
		}
		if i == 7 && (err == nil || c.Stage() != Ordinary) {
			t.Fatal("unbounded missed shop clicks")
		}
	}
	s, _ := c.budget.Snapshot()
	if s.Pending != nil || s.Spent != 0 {
		t.Fatal(s)
	}
}

func TestF8BeforePaymentPreservesReserveUntilFreshOrdinaryFrame(t *testing.T) {
	now := time.Now()
	_, prep, row := prepared(now)
	c := fixtureController(t, now)
	c.Observe(now, observation(now, 1, GameScreen, prep, row))
	c.Interrupt()
	if !c.HoldsClickerReserve() {
		t.Fatal("F8 released reserve")
	}
	if err := c.ReleaseUnpaidPreparation(now, observation(now, 1, GameScreen, prep, row)); err == nil {
		t.Fatal("old frame released reserve")
	}
	if err := c.ReleaseUnpaidPreparation(now, observation(now, 2, UnknownScreen, prep, row)); err == nil {
		t.Fatal("modal/unknown released reserve")
	}
	if err := c.ReleaseUnpaidPreparation(now, observation(now, 2, GameScreen, prep, row)); err != nil {
		t.Fatal(err)
	}
	if c.HoldsClickerReserve() {
		t.Fatal("ordinary allocation not released")
	}
}
