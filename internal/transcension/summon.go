package transcension

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

type SummonAction uint8

const (
	NoSummonAction SummonAction = iota
	OpenAncientTab
	ScrollSummonAncientsUp
	OpenSummonOffers
	ChooseAncientOffer
	ConfirmAncientSummon
	ExportSummonReceipt
)

type SummonScreen uint8

const (
	UnknownSummonScreen SummonScreen = iota
	SummonGameScreen
	SummonAncientsScreen
	SummonOffersScreen
	SummonConfirmationScreen
)

// Known means the installed UI identifies this exact Ancient and Hero Souls
// price. InitialLevel requires verified client/receipt evidence; it is never
// inferred from an owned-Ancient leveling button or an allocation plan.
type SummonOffer struct {
	ID                       int
	Name, Cost, InitialLevel string
	Known                    bool
}

type SummonObservation struct {
	Frame, Generation, Layout uint64
	At                        time.Time
	Screen                    SummonScreen
	Known                     bool
	Wallet                    string
	Offers                    []SummonOffer
	OpenKnown, UpKnown        bool
	ConfirmKnown, CancelKnown bool
}

type SummonCommand struct {
	Action                    SummonAction
	Frame, Generation, Layout uint64
	SaveHash                  string
	Offer                     SummonOffer
}

// SummonPlanner is an independent, required-only restoration transaction. It
// emits commands for the existing shared input owner, never captures or inputs.
// There is no reroll, ruby purchase, respec, or retry of an uncertain summon.
type SummonPlanner struct {
	policy            Policy
	native            NativeEvidence
	snapshot          Snapshot
	missing           []ancientcalc.AncientRequirement
	latest            SummonObservation
	selected          SummonOffer
	pending           SummonCommand
	lastFrame         uint64
	scrolls           int
	inputAt           time.Time
	exportRequested   bool
	active, uncertain bool
	reason            string
}

func NewSummonPlanner(policy Policy, native NativeEvidence) *SummonPlanner {
	return &SummonPlanner{policy: policy, native: native}
}

func (p *SummonPlanner) Reason() string { return p.reason }
func (p *SummonPlanner) Complete() bool {
	return p.active && !p.uncertain && p.pending.Action == NoSummonAction && len(p.missing) == 0
}
func (p *SummonPlanner) MissingAncients() []ancientcalc.AncientRequirement {
	return append([]ancientcalc.AncientRequirement(nil), p.missing...)
}

// NeedsMoreSouls yields to ordinary earning only when current exact offer
// evidence proves every observed required offer unaffordable. Unknown prices
// remain a blocker; missing offers never acquire an invented aggregate cost.
func (p *SummonPlanner) NeedsMoreSouls(now time.Time) bool {
	o := p.latest
	if !p.active || p.uncertain || p.pending.Action != NoSummonAction || p.selected != (SummonOffer{}) || !p.native.AncientSummon || p.native.Build != p.snapshot.State.Build || !o.Known || o.Screen != SummonOffersScreen || o.Frame == 0 || o.Frame <= p.lastFrame || o.Generation != p.snapshot.Generation || !fresh(now, o.At, 5*time.Second) || !fresh(now, p.snapshot.ExportedAt, 30*time.Second) {
		return false
	}
	wallet, we := exactSummonAmount(o.Wallet, false)
	saved, se := exactSummonAmount(p.snapshot.State.HeroSouls, false)
	if we != nil || se != nil || wallet.Cmp(saved) != 0 {
		return false
	}
	seen, required := make(map[int]bool, len(o.Offers)), 0
	for _, offer := range o.Offers {
		if seen[offer.ID] {
			return false
		}
		seen[offer.ID] = true
		for _, missing := range p.missing {
			if offer.ID != missing.ID {
				continue
			}
			if !p.required(offer) {
				return false
			}
			cost, _ := exactSummonAmount(offer.Cost, false)
			if cost.Cmp(wallet) <= 0 {
				return false
			}
			required++
		}
	}
	return required > 0
}

// ContinueEarning is a handoff, not a cancel click or paid reroll. The shared
// navigation owner must leave the offer panel using a verified free control.
func (p *SummonPlanner) ContinueEarning(now time.Time) error {
	if !p.NeedsMoreSouls(now) {
		return errors.New("fresh required offers do not establish a Hero Souls shortfall")
	}
	p.active, p.reason = false, "earn more Hero Souls through ordinary play and Ascension, then restore the remaining Ancients"
	return nil
}

func (p *SummonPlanner) Begin(ctx context.Context, now time.Time, s Snapshot) error {
	if p.active || p.uncertain || p.pending.Action != NoSummonAction {
		return errors.New("Ancient restoration already active; reconcile pending input")
	}
	if !p.policy.Enabled {
		return errors.New("Transcension disabled")
	}
	if err := validateSnapshot(ctx, s); err != nil {
		return err
	}
	if !fresh(now, s.ExportedAt, 30*time.Second) || !s.State.Transcendent || s.State.AscensionsThisTranscension <= 0 {
		return errors.New("fresh post-Transcension first-Ascension export required")
	}
	wallet, err := exactSummonAmount(s.State.HeroSouls, false)
	if err != nil || wallet.Sign() <= 0 {
		return errors.New("first earned Hero Souls required")
	}
	missing, err := ancientcalc.MissingActiveAncients(ctx, s.State, p.policy.SkillRate, p.policy.Beyond8k)
	if err != nil {
		return err
	}
	*p = SummonPlanner{policy: p.policy, native: p.native, snapshot: cloneSnapshot(s), missing: missing, active: true}
	return nil
}

func exactSummonAmount(value string, integer bool) (*big.Rat, error) {
	// Export Hero Souls and a fully read price are decimal numbers, never a
	// rounded suffix or float64. Bound exponents before constructing a rational.
	if _, err := ancientcalc.Value(value); err != nil {
		return nil, err
	}
	x, ok := new(big.Rat).SetString(value)
	if !ok || x.Sign() < 0 || integer && !x.IsInt() {
		return nil, errors.New("invalid exact Ancient summon amount")
	}
	return x, nil
}

func (p *SummonPlanner) Observe(o SummonObservation) {
	if o.Generation != p.snapshot.Generation || o.Frame <= p.latest.Frame || o.Frame <= p.lastFrame {
		return
	}
	o.Offers = append([]SummonOffer(nil), o.Offers...)
	p.latest = o
}

func (p *SummonPlanner) required(offer SummonOffer) bool {
	if !offer.Known {
		return false
	}
	for _, missing := range p.missing {
		if missing.ID == offer.ID && missing.Name == offer.Name {
			cost, ce := exactSummonAmount(offer.Cost, false)
			level, le := exactSummonAmount(offer.InitialLevel, true)
			return ce == nil && le == nil && cost.Sign() > 0 && level.Sign() > 0
		}
	}
	return false
}

func (p *SummonPlanner) Next(now time.Time) SummonCommand {
	o := p.latest
	cmd := SummonCommand{Frame: o.Frame, Generation: o.Generation, Layout: o.Layout, SaveHash: p.snapshot.State.SaveHash}
	if !p.active || p.uncertain || len(p.missing) == 0 {
		return cmd
	}
	if p.pending.Action == ConfirmAncientSummon {
		if !p.exportRequested {
			cmd.Action = ExportSummonReceipt
		}
		return cmd
	}
	if o.Frame == 0 || o.Frame <= p.lastFrame || !o.Known || o.Generation != p.snapshot.Generation || !fresh(now, o.At, 5*time.Second) || !fresh(now, p.snapshot.ExportedAt, 30*time.Second) {
		return cmd
	}
	switch o.Screen {
	case SummonGameScreen:
		cmd.Action = OpenAncientTab
	case SummonAncientsScreen:
		if o.OpenKnown {
			cmd.Action = OpenSummonOffers
		} else if o.UpKnown && p.scrolls < 12 {
			cmd.Action = ScrollSummonAncientsUp
		}
	case SummonOffersScreen, SummonConfirmationScreen:
		if !p.native.AncientSummon || p.native.Build != p.snapshot.State.Build {
			return cmd
		}
		wallet, we := exactSummonAmount(o.Wallet, false)
		saved, se := exactSummonAmount(p.snapshot.State.HeroSouls, false)
		if we != nil || se != nil || wallet.Cmp(saved) != 0 {
			return cmd
		}
		seen := make(map[int]bool, len(o.Offers))
		for _, offer := range o.Offers {
			if seen[offer.ID] {
				return cmd
			}
			seen[offer.ID] = true
		}
		for _, offer := range o.Offers {
			if !p.required(offer) {
				continue
			}
			cost, _ := exactSummonAmount(offer.Cost, false)
			if cost.Cmp(wallet) > 0 {
				continue
			}
			if o.Screen == SummonOffersScreen && p.selected == (SummonOffer{}) {
				cmd.Action, cmd.Offer = ChooseAncientOffer, offer
			} else if o.Screen == SummonConfirmationScreen && len(o.Offers) == 1 && offer == p.selected && o.ConfirmKnown && o.CancelKnown {
				cmd.Action, cmd.Offer = ConfirmAncientSummon, offer
			}
			return cmd
		}
	}
	return cmd
}

func (p *SummonPlanner) Allowed(now time.Time, cmd SummonCommand) bool {
	return cmd.Action != NoSummonAction && cmd == p.Next(now)
}

// Reserve precedes shared-queue input. Selection cannot also authorize a
// purchase; only a later fully identified confirmation can reserve the debit.
func (p *SummonPlanner) Reserve(now time.Time, cmd SummonCommand) error {
	if !p.Allowed(now, cmd) {
		return errors.New("stale or unauthorized Ancient summon command")
	}
	p.lastFrame = cmd.Frame
	switch cmd.Action {
	case ScrollSummonAncientsUp:
		p.scrolls++
	case ChooseAncientOffer:
		p.selected = cmd.Offer
	case ConfirmAncientSummon:
		p.pending, p.inputAt, p.exportRequested = cmd, now, false
	case ExportSummonReceipt:
		p.exportRequested = true
	}
	return nil
}

func (p *SummonPlanner) Interrupt() {
	if p.active && !p.Complete() {
		p.uncertain, p.reason = true, "Ancient summon interrupted; reconcile fresh ownership and wallet before further input"
	}
}

func (p *SummonPlanner) AcceptExport(ctx context.Context, now time.Time, after Snapshot) error {
	fail := func(err error) error { p.uncertain, p.reason = true, err.Error(); return err }
	if p.pending.Action != ConfirmAncientSummon {
		return errors.New("no reserved Ancient summon receipt expected")
	}
	if err := validateSnapshot(ctx, after); err != nil {
		return fail(err)
	}
	if after.Generation != p.snapshot.Generation || !fresh(now, after.ExportedAt, 30*time.Second) || !after.ExportedAt.After(p.inputAt) || !after.ExportedAt.After(p.snapshot.ExportedAt) || after.State.SaveHash == p.snapshot.State.SaveHash {
		return fail(errors.New("stale Ancient summon receipt"))
	}
	if err := VerifyAncientSummonReceipt(ctx, p.snapshot.State, after.State, p.pending.Offer); err != nil {
		return fail(err)
	}
	missing, err := ancientcalc.MissingActiveAncients(ctx, after.State, p.policy.SkillRate, p.policy.Beyond8k)
	if err != nil {
		return fail(err)
	}
	p.snapshot, p.missing = cloneSnapshot(after), missing
	p.selected, p.pending, p.latest = SummonOffer{}, SummonCommand{}, SummonObservation{}
	p.exportRequested, p.uncertain, p.reason = false, false, ""
	return nil
}

// Reconcile proves a pending one-shot debit after an interruption. A missing or
// rejected receipt leaves the planner uncertain; the purchase is never replayed.
func (p *SummonPlanner) Reconcile(ctx context.Context, now time.Time, after Snapshot) error {
	if !p.uncertain || p.pending.Action != ConfirmAncientSummon {
		return errors.New("no pending Ancient summon to reconcile")
	}
	next := *p
	next.snapshot = cloneSnapshot(p.snapshot)
	next.snapshot.Generation = after.Generation
	if err := next.AcceptExport(ctx, now, after); err != nil {
		return err
	}
	next.lastFrame = 0
	*p = next
	return nil
}

// VerifyAncientSummonReceipt proves exactly one canonical new ownership entry,
// unchanged previous levels, and the exact observed Hero Souls debit. No
// allocation, Outsider input, Ascension, or profile change may share the receipt.
func VerifyAncientSummonReceipt(ctx context.Context, before, after ancientcalc.TranscensionState, offer SummonOffer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, s := range []ancientcalc.TranscensionState{before, after} {
		if _, err := ancientcalc.MissingActiveAncients(ctx, s, 0, false); err != nil {
			return err
		}
		if s.ProfileID == "" || s.CurrentZone == nil || s.CurrentTranscensionID == nil || s.CurrentAscensionID == nil || s.CurrentAscensionCycleID == nil {
			return errors.New("Ancient summon receipt lacks profile/zone/cycle metadata")
		}
	}
	if !offer.Known || before.SaveHash == after.SaveHash || !sameCycle(before, after) || before.Ascensions != after.Ascensions || before.AscensionsThisTranscension != after.AscensionsThisTranscension || before.HighestZone != after.HighestZone || *before.CurrentZone != *after.CurrentZone || !slices.Equal(before.Outsiders, after.Outsiders) {
		return errors.New("Ancient summon receipt contains stale or unrelated state changes")
	}
	wallet, we := exactSummonAmount(before.HeroSouls, false)
	remaining, re := exactSummonAmount(after.HeroSouls, false)
	cost, ce := exactSummonAmount(offer.Cost, false)
	level, le := exactSummonAmount(offer.InitialLevel, true)
	if we != nil || re != nil || ce != nil || le != nil || cost.Sign() <= 0 || level.Sign() <= 0 || wallet.Cmp(cost) < 0 || new(big.Rat).Sub(wallet, cost).Cmp(remaining) != 0 {
		return errors.New("Ancient summon receipt wallet does not match exact observed cost")
	}
	expected := make(map[int]ancientcalc.Level, len(before.Ancients)+1)
	for _, row := range before.Ancients {
		if row.ID == offer.ID {
			return errors.New("Ancient summon receipt target was already owned")
		}
		expected[row.ID] = row
	}
	expected[offer.ID] = ancientcalc.Level{ID: offer.ID, Name: offer.Name, Level: offer.InitialLevel}
	if len(after.Ancients) != len(expected) {
		return errors.New("Ancient summon receipt did not add exactly one ownership entry")
	}
	for _, row := range after.Ancients {
		old, ok := expected[row.ID]
		actual, ae := exactSummonAmount(row.Level, true)
		want, ve := exactSummonAmount(old.Level, true)
		if !ok || row.Name != old.Name || ae != nil || ve != nil || actual.Cmp(want) != 0 {
			return errors.New("Ancient summon receipt ownership or level mismatch")
		}
	}
	return ctx.Err()
}
