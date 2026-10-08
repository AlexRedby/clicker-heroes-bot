package transcension

import (
	"context"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func summonFixture() (Snapshot, SummonOffer) {
	s, _, _ := preparation()
	s.State.Transcendent, s.State.Transcensions = true, 1
	s.State.Ascensions, s.State.AscensionsThisTranscension = 7, 1
	*s.State.CurrentTranscensionID, *s.State.CurrentAscensionCycleID, *s.State.CurrentAscensionID = 1, 1, 1
	// Synthetic offer exercises policy only; it is not native acceptance evidence.
	return s, SummonOffer{ID: 19, Name: "Fragsworth", Cost: "2", InitialLevel: "1", Known: true}
}

func newSummon(t *testing.T, s Snapshot, accepted bool) *SummonPlanner {
	t.Helper()
	p := NewSummonPlanner(Policy{Enabled: true, SkillRate: 1}, NativeEvidence{Build: s.State.Build, AncientSummon: accepted})
	if err := p.Begin(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal(err)
	}
	return p
}

func offerObservation(s Snapshot, offer SummonOffer) SummonObservation {
	return SummonObservation{Frame: 1, Generation: s.Generation, Layout: 3, At: s.ExportedAt, Screen: SummonOffersScreen, Known: true, Wallet: s.State.HeroSouls, Offers: []SummonOffer{offer}}
}

func reserveSummon(t *testing.T, p *SummonPlanner, now time.Time, action SummonAction) SummonCommand {
	t.Helper()
	cmd := p.Next(now)
	if cmd.Action != action || !p.Allowed(now, cmd) {
		t.Fatalf("wanted %d got %+v", action, cmd)
	}
	if err := p.Reserve(now, cmd); err != nil || p.Allowed(now, cmd) {
		t.Fatal("one-shot reservation failed", cmd, err)
	}
	return cmd
}

func pendingSummon(t *testing.T) (*SummonPlanner, Snapshot, SummonOffer) {
	t.Helper()
	s, offer := summonFixture()
	p := newSummon(t, s, true)
	o := offerObservation(s, offer)
	p.Observe(o)
	reserveSummon(t, p, s.ExportedAt, ChooseAncientOffer)
	o.Frame, o.Screen, o.ConfirmKnown, o.CancelKnown = 2, SummonConfirmationScreen, true, true
	p.Observe(o)
	reserveSummon(t, p, s.ExportedAt, ConfirmAncientSummon)
	return p, s, offer
}

func summonReceipt(s Snapshot, offer SummonOffer) Snapshot {
	s = cloneSnapshot(s)
	s.ExportedAt = s.ExportedAt.Add(time.Second)
	s.State.SaveHash = strings.Repeat("f", 64)
	wallet, _ := exactSummonAmount(s.State.HeroSouls, false)
	cost, _ := exactSummonAmount(offer.Cost, false)
	s.State.HeroSouls = wallet.Sub(wallet, cost).FloatString(10)
	s.State.Ancients = append(s.State.Ancients, ancientcalc.Level{ID: offer.ID, Name: offer.Name, Level: offer.InitialLevel})
	return s
}

func TestSummonOnlyRequiredKnownAffordableOffers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Snapshot, *SummonOffer, *SummonObservation)
	}{
		{"unrequired", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.ID, a.Name = 5, "Siyalatas" }},
		{"wrong name", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Name = "Bhaal" }},
		{"unknown offer", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Known = false }},
		{"unknown price", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Cost = "" }},
		{"rounded price", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Cost = "2K" }},
		{"zero price", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Cost = "0" }},
		{"negative price", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Cost = "-1" }},
		{"unaffordable", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.Cost = "100.000000000000000000000001" }},
		{"unknown initial level", func(_ *Snapshot, a *SummonOffer, _ *SummonObservation) { a.InitialLevel = "" }},
		{"mismatched wallet", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.Wallet = "101" }},
		{"rounded wallet", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.Wallet = "100K" }},
		{"unknown popup", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.Known = false }},
		{"stale frame", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.Frame = 0 }},
		{"stale generation", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.Generation++ }},
		{"stale timestamp", func(_ *Snapshot, _ *SummonOffer, o *SummonObservation) { o.At = o.At.Add(-6 * time.Second) }},
		{"duplicate offer", func(_ *Snapshot, a *SummonOffer, o *SummonObservation) { o.Offers = append(o.Offers, *a) }},
		{"already owned", func(s *Snapshot, a *SummonOffer, _ *SummonObservation) {
			s.State.Ancients = []ancientcalc.Level{{ID: a.ID, Name: a.Name, Level: "1"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, offer := summonFixture()
			o := offerObservation(s, offer)
			tc.change(&s, &offer, &o)
			o.Offers[0] = offer
			p := newSummon(t, s, true)
			p.Observe(o)
			if cmd := p.Next(s.ExportedAt); cmd.Action != NoSummonAction {
				t.Fatal("unsafe offer authorized", cmd)
			}
		})
	}
	s, offer := summonFixture()
	s.State.HeroSouls, offer.Cost = "2.000000000000000000000001", "2.000000000000000000000001"
	p := newSummon(t, s, true)
	p.Observe(offerObservation(s, offer))
	reserveSummon(t, p, s.ExportedAt, ChooseAncientOffer)
	if _, err := exactSummonAmount("1e1000", false); err != nil {
		t.Fatal("exact large export amount", err)
	}
}

func TestSummonNativeGateAndSafeBoundedNavigation(t *testing.T) {
	s, offer := summonFixture()
	p := newSummon(t, s, false)
	p.Observe(offerObservation(s, offer))
	if p.Next(s.ExportedAt).Action != NoSummonAction || len(p.MissingAncients()) == 0 {
		t.Fatal("missing acceptance must block spending but preserve policy dependencies")
	}
	p.Observe(SummonObservation{Frame: 2, Generation: s.Generation, At: s.ExportedAt, Screen: SummonGameScreen, Known: true})
	reserveSummon(t, p, s.ExportedAt, OpenAncientTab)
	for i := uint64(3); i < 15; i++ {
		p.Observe(SummonObservation{Frame: i, Generation: s.Generation, At: s.ExportedAt, Screen: SummonAncientsScreen, Known: true, UpKnown: true})
		reserveSummon(t, p, s.ExportedAt, ScrollSummonAncientsUp)
	}
	p.Observe(SummonObservation{Frame: 15, Generation: s.Generation, At: s.ExportedAt, Screen: SummonAncientsScreen, Known: true, UpKnown: true})
	if p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("unknown summon entry caused unbounded navigation")
	}
}

func TestSummonShortfallHandsBackToOrdinaryEarning(t *testing.T) {
	s, offer := summonFixture()
	s.State.HeroSouls = "1"
	p := newSummon(t, s, true)
	o := offerObservation(s, offer)
	p.Observe(o)
	if !p.NeedsMoreSouls(s.ExportedAt) || p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("first Ascension shortfall did not yield ordinary earning")
	}
	if p.NeedsMoreSouls(s.ExportedAt.Add(6 * time.Second)) {
		t.Fatal("stale offer supplied a shortfall")
	}
	unknown := SummonOffer{ID: 15, Name: "Bhaal", Known: true, InitialLevel: "1"}
	o.Frame, o.Offers = 2, []SummonOffer{offer, unknown}
	p.Observe(o)
	if p.NeedsMoreSouls(s.ExportedAt) {
		t.Fatal("unknown required price was converted into ordinary earning")
	}
	unknown.Cost = "1"
	o.Frame, o.Offers = 3, []SummonOffer{offer, unknown}
	p.Observe(o)
	if p.NeedsMoreSouls(s.ExportedAt) || p.Next(s.ExportedAt).Offer.ID != unknown.ID {
		t.Fatal("affordable subset was skipped")
	}
	o.Frame, o.Offers = 4, []SummonOffer{offer}
	p.Observe(o)
	if err := p.ContinueEarning(s.ExportedAt); err != nil || p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("ordinary earning handoff", err)
	}
	s.ExportedAt = s.ExportedAt.Add(time.Second)
	s.State.SaveHash, s.State.HeroSouls = strings.Repeat("d", 64), "3"
	if err := p.Begin(context.Background(), s.ExportedAt, s); err != nil {
		t.Fatal("fresh earning export could not resume restoration", err)
	}
	p.Observe(offerObservation(s, offer))
	reserveSummon(t, p, s.ExportedAt, ChooseAncientOffer)
}

func TestSummonConfirmationRequiresSelectedOffer(t *testing.T) {
	s, offer := summonFixture()
	p := newSummon(t, s, true)
	o := offerObservation(s, offer)
	p.Observe(o)
	cmd := reserveSummon(t, p, s.ExportedAt, ChooseAncientOffer)
	cmd.Layout++
	if p.Allowed(s.ExportedAt, cmd) {
		t.Fatal("layout change retained authority")
	}
	o.Frame, o.Screen, o.ConfirmKnown = 2, SummonConfirmationScreen, true
	p.Observe(o)
	if p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("unknown cancel authorized confirmation")
	}
	o.Frame, o.CancelKnown, o.Offers[0].Cost = 3, true, "3"
	p.Observe(o)
	if p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("changed price authorized confirmation")
	}
	o.Frame, o.Offers[0] = 4, offer
	p.Observe(o)
	reserveSummon(t, p, s.ExportedAt, ConfirmAncientSummon)
	reserveSummon(t, p, s.ExportedAt, ExportSummonReceipt)
	if p.Next(s.ExportedAt).Action != NoSummonAction {
		t.Fatal("pending summon replayed input or receipt request")
	}
}

func TestSummonReceiptsRejectUnrelatedChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Snapshot)
	}{
		{"stale hash", func(s *Snapshot) { s.State.SaveHash = strings.Repeat("a", 64) }},
		{"stale export", func(s *Snapshot) { s.ExportedAt = s.ExportedAt.Add(-time.Second) }},
		{"generation", func(s *Snapshot) { s.Generation++ }},
		{"profile", func(s *Snapshot) { s.State.ProfileID = strings.Repeat("c", 64) }},
		{"missing cycle", func(s *Snapshot) { s.State.CurrentAscensionID = nil }},
		{"zone", func(s *Snapshot) { *s.State.CurrentZone++ }},
		{"ascension", func(s *Snapshot) { s.State.Ascensions++ }},
		{"outsider", func(s *Snapshot) { s.State.Outsiders[0].Level = "1"; s.State.AncientSoulsTotal++ }},
		{"no debit", func(s *Snapshot) { s.State.HeroSouls = "100" }},
		{"wrong debit", func(s *Snapshot) { s.State.HeroSouls = "97.999999999999999999999999" }},
		{"wrong ancient", func(s *Snapshot) { s.State.Ancients = []ancientcalc.Level{{ID: 15, Name: "Bhaal", Level: "1"}} }},
		{"wrong level", func(s *Snapshot) { s.State.Ancients[0].Level = "2" }},
		{"two summons", func(s *Snapshot) {
			s.State.Ancients = append(s.State.Ancients, ancientcalc.Level{ID: 15, Name: "Bhaal", Level: "1"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, before, offer := pendingSummon(t)
			after := summonReceipt(before, offer)
			tc.change(&after)
			if err := p.AcceptExport(context.Background(), before.ExportedAt.Add(time.Second), after); err == nil {
				t.Fatal("invalid receipt accepted")
			}
			if p.Next(after.ExportedAt).Action != NoSummonAction || p.Reason() == "" {
				t.Fatal("uncertain debit replayed")
			}
		})
	}
	p, before, offer := pendingSummon(t)
	after := summonReceipt(before, offer)
	p.Interrupt()
	after.Generation++
	if p.Next(after.ExportedAt).Action != NoSummonAction {
		t.Fatal("F8 retried pending summon")
	}
	if err := p.Reconcile(context.Background(), after.ExportedAt, after); err != nil {
		t.Fatal(err)
	}
	for _, row := range p.MissingAncients() {
		if row.ID == offer.ID {
			t.Fatal("verified new Ancient still missing")
		}
	}
	if err := p.AcceptExport(context.Background(), after.ExportedAt, after); err == nil {
		t.Fatal("receipt replay accepted")
	}
}

func TestSummonReceiptPreservesPreviouslyOwnedAncients(t *testing.T) {
	before, offer := summonFixture()
	before.State.Ancients = []ancientcalc.Level{{ID: 15, Name: "Bhaal", Level: "2"}}
	after := summonReceipt(before, offer)
	if err := VerifyAncientSummonReceipt(context.Background(), before.State, after.State, offer); err != nil {
		t.Fatal("one new Ancient with unchanged old ownership", err)
	}
	after.State.Ancients[0].Level = "3"
	if err := VerifyAncientSummonReceipt(context.Background(), before.State, after.State, offer); err == nil {
		t.Fatal("leveling an old Ancient shared the summon receipt")
	}
}
