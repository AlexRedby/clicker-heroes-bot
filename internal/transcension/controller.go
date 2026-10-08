package transcension

import (
	"context"
	"errors"
	"math"
	"math/big"
	"slices"
	"strconv"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

type Stage uint8

const (
	Ordinary Stage = iota
	AwaitOutsiders
	AwaitConfirmation
	AwaitResetReceipt
	SpendOutsiders
	AwaitFeedReceipt
	RestoreHeroes
	AwaitFirstSouls
	AwaitAncients
	ReadyForAllocation
	Uncertain
)

type Action uint8

const (
	NoAction Action = iota
	OpenOutsiders
	ScrollOutsidersTop
	ScrollOutsidersDown
	SelectQuantity
	OpenReset
	ConfirmReset
	FeedOutsider
	FreshExport
	BootstrapHeroes
)

type Policy struct {
	Enabled   bool
	SkillRate float64
	Beyond8k  bool
}

// NativeEvidence is installed-client acceptance, not a CLI override. The bot
// adapter must leave missing FEED/reset-recovery/summon evidence false.
type NativeEvidence struct {
	Build                              string
	ResetRecovery, Feed, AncientSummon bool
}

type Snapshot struct {
	State      ancientcalc.TranscensionState
	Preview    ancientcalc.TranscensionPreview
	ExportedAt time.Time
	Generation uint64
}

// ReadSnapshot works before Ancient summoning. The existing file-acquisition
// worker supplies freshness; no owned-Ancient allocation runs on an empty roster.
func ReadSnapshot(ctx context.Context, exported []byte, at time.Time, generation uint64) (Snapshot, error) {
	s := Snapshot{ExportedAt: at, Generation: generation}
	var err error
	s.State, err = ancientcalc.ReadTranscensionState(ctx, exported)
	if err != nil {
		return s, err
	}
	s.Preview, err = ancientcalc.PreviewTranscension(ctx, exported)
	return s, err
}

// Timing comes from completed live loops and measured active play. Save dates,
// offline time and an advisory preview cannot establish these facts.
type Timing struct {
	At                           time.Time
	Generation                   uint64
	SaveHash                     string
	CompletedLoops               int
	WallConfirmed                bool
	PreviousASPerHour, ASPerHour float64
}

type Screen uint8

const (
	UnknownScreen Screen = iota
	GameScreen
	OutsidersScreen
	ConfirmationScreen
)

type Row struct {
	ID          int
	Name        string
	Level, Cost int
}

type Observation struct {
	Frame, Generation, Layout  uint64
	At                         time.Time
	Screen                     Screen
	Known                      bool
	Wallet, Reward             int
	Quantity                   int // x1/x10/x100/x1000; MAX has no fixed bound.
	Rows                       []Row
	OpenKnown, AtTop, AtBottom bool
	ConfirmKnown, CancelKnown  bool
	RespecKnown, Respec        bool
}

type Command struct {
	Action                    Action
	Frame, Generation, Layout uint64
	SaveHash                  string
	ID                        int
	Name                      string
	Quantity, Cost            int
}

// Controller runs under the shared input-queue owner. Reserve is called before
// input, after rechecking the command against the latest shared frame. An input
// error or F8 calls Interrupt; neither reset nor FEED is replayed automatically.
type Controller struct {
	policy            Policy
	native            NativeEvidence
	stage             Stage
	snapshot          Snapshot
	latest            Observation
	targets           []ancientcalc.OutsiderTarget
	pending           Command
	inputAt, deadline time.Time
	lastFrame         uint64
	exportRequested   bool
	reward, scrolls   int
	reason            string
}

func NewController(policy Policy, native NativeEvidence) *Controller {
	return &Controller{policy: policy, native: native}
}

func (c *Controller) Stage() Stage   { return c.stage }
func (c *Controller) Reason() string { return c.reason }

// Hero bootstrap and its first ordinary Ascension use the existing controller;
// Ancient allocation must remain suspended until ReadyForAllocation.
func (c *Controller) HoldsGameplay() bool {
	return c.stage != Ordinary && c.stage != AwaitFirstSouls && c.stage != ReadyForAllocation
}
func (c *Controller) HoldsAncientAllocation() bool {
	return c.stage != Ordinary && c.stage != ReadyForAllocation
}

func fresh(now, at time.Time, age time.Duration) bool {
	return !at.IsZero() && !at.After(now) && now.Sub(at) <= age
}

func validateSnapshot(ctx context.Context, s Snapshot) error {
	if _, err := ancientcalc.MissingActiveAncients(ctx, s.State, 0, false); err != nil {
		return err
	}
	state := s.State
	if s.Generation == 0 || state.ProfileID == "" || state.CurrentZone == nil || state.CurrentTranscensionID == nil || state.CurrentAscensionID == nil || state.CurrentAscensionCycleID == nil {
		return errors.New("fresh profile/current-zone/cycle metadata required")
	}
	return nil
}

func (c *Controller) Begin(ctx context.Context, now time.Time, s Snapshot, timing Timing) error {
	if c.stage != Ordinary {
		return errors.New("Transcension transaction already active")
	}
	if !c.policy.Enabled {
		return errors.New("Transcension disabled")
	}
	if !(c.policy.SkillRate >= 0 && c.policy.SkillRate <= 1) {
		return errors.New("invalid Ancient skill rate")
	}
	if err := validateSnapshot(ctx, s); err != nil {
		return err
	}
	if !fresh(now, s.ExportedAt, 30*time.Second) || !fresh(now, timing.At, 5*time.Second) || timing.Generation != s.Generation || timing.SaveHash != s.State.SaveHash {
		return errors.New("fresh export and owned live timing required")
	}
	p, state := s.Preview, s.State
	if p.SaveHash != state.SaveHash || p.Build != state.Build || p.SaveVersion != state.SaveVersion || p.Transcensions != state.Transcensions || p.Ascensions != state.AscensionsThisTranscension || p.AncientSouls != state.AncientSouls || p.AncientSoulsTotal != state.AncientSoulsTotal || p.Transcendent != state.Transcendent || p.HighestZone != state.HighestZone || p.EstimatedASGain == nil || *p.EstimatedASGain <= 0 || state.HighestZone < 300 {
		return errors.New("preview and fresh state do not establish positive reset eligibility")
	}
	if timing.CompletedLoops < 2 || !timing.WallConfirmed {
		return errors.New("two completed Ascension loops and a confirmed wall required")
	}
	if state.Transcendent {
		if !p.History.Available || p.History.ASGrowingAscensions < 3 {
			return errors.New("three completed AS-growing Ascensions required by the conservative policy")
		}
		if math.IsNaN(timing.PreviousASPerHour) || math.IsInf(timing.PreviousASPerHour, 0) || math.IsNaN(timing.ASPerHour) || math.IsInf(timing.ASPerHour, 0) || timing.PreviousASPerHour <= 0 || timing.ASPerHour < 0 || timing.ASPerHour >= timing.PreviousASPerHour {
			return errors.New("declining measured AS per active-play hour required")
		}
	}
	if c.native.Build != state.Build || !c.native.ResetRecovery || !c.native.Feed || !c.native.AncientSummon {
		return errors.New("native reset recovery, affordable FEED and Ancient summon acceptance required")
	}
	if plans, err := ancientcalc.PlanOutsiders(ctx, state.AncientSoulsTotal, state.AncientSouls, *p.EstimatedASGain, state.Outsiders); err != nil || plans.AfterReward.Status != "ok" {
		return errors.New("unsupported projected Outsider allocation")
	}
	*c = Controller{policy: c.policy, native: c.native, snapshot: cloneSnapshot(s), stage: AwaitOutsiders, deadline: now.Add(30 * time.Second)}
	return nil
}

func (c *Controller) Observe(o Observation) {
	if o.Generation != c.snapshot.Generation || o.Frame <= c.latest.Frame || o.Frame <= c.lastFrame {
		return
	}
	o.Rows = append([]Row(nil), o.Rows...)
	c.latest = o
}

func (c *Controller) stop(reason string) {
	c.stage, c.reason = Uncertain, reason
}

func (c *Controller) Interrupt() {
	if c.stage != Ordinary && c.stage != ReadyForAllocation {
		c.stop("Transcension interrupted; reconcile a fresh export before any further input")
	}
}

func (c *Controller) Next(now time.Time) Command {
	o := c.latest
	cmd := Command{Frame: o.Frame, Generation: o.Generation, Layout: o.Layout, SaveHash: c.snapshot.State.SaveHash}
	if c.stage >= AwaitOutsiders && c.stage <= RestoreHeroes && now.After(c.deadline) {
		c.stop("Transcension native stage timed out")
		return cmd
	}
	if c.stage == AwaitResetReceipt || c.stage == AwaitFeedReceipt {
		if !c.exportRequested {
			cmd.Action = FreshExport
		}
		return cmd
	}
	if c.stage == RestoreHeroes {
		cmd.Action = BootstrapHeroes
		return cmd
	}
	if c.stage != AwaitOutsiders && c.stage != AwaitConfirmation && c.stage != SpendOutsiders {
		return cmd
	}
	if !o.Known || o.Generation != c.snapshot.Generation || o.Frame == 0 || o.Frame <= c.lastFrame || !fresh(now, o.At, 5*time.Second) || !fresh(now, c.snapshot.ExportedAt, 30*time.Second) {
		return cmd
	}
	if c.stage == AwaitConfirmation {
		if o.Screen == ConfirmationScreen && o.ConfirmKnown && o.CancelKnown && o.RespecKnown && !o.Respec && o.Reward == c.reward && o.Reward > 0 {
			cmd.Action = ConfirmReset
		}
		return cmd
	}
	if o.Screen == GameScreen {
		cmd.Action = OpenOutsiders
		return cmd
	}
	if o.Screen != OutsidersScreen || !c.rowsMatch(o) {
		return cmd
	}
	if c.stage == AwaitOutsiders {
		if o.Reward <= 0 {
			return cmd
		}
		if o.OpenKnown {
			cmd.Action = OpenReset
		} else if !o.AtTop {
			cmd.Action = ScrollOutsidersTop
		}
		return cmd
	}
	for _, target := range c.targets {
		current := c.level(target.ID)
		if current >= target.Target {
			continue
		}
		quantity := 1
		for _, candidate := range []int{1000, 100, 10} {
			if target.Target-current >= candidate {
				quantity = candidate
				break
			}
		}
		if o.Quantity != quantity {
			cmd.Action, cmd.Quantity = SelectQuantity, quantity
			return cmd
		}
		for _, row := range o.Rows {
			if row.ID == target.ID {
				cost, err := ancientcalc.OutsiderFeedCost(target.ID, current, quantity)
				if err == nil && cost == row.Cost && cost <= o.Wallet {
					cmd.Action, cmd.ID, cmd.Name, cmd.Quantity, cmd.Cost = FeedOutsider, target.ID, target.Name, quantity, cost
				}
				return cmd
			}
		}
		if c.scrolls >= 12 {
			c.stop("Outsider target not found in bounded roster sweep")
		} else if o.AtBottom {
			cmd.Action = ScrollOutsidersTop
		} else {
			cmd.Action = ScrollOutsidersDown
		}
		return cmd
	}
	c.stage = RestoreHeroes
	cmd.Action = BootstrapHeroes
	return cmd
}

func (c *Controller) level(id int) int {
	for _, row := range c.snapshot.State.Outsiders {
		if row.ID == id {
			level, _ := strconv.Atoi(row.Level)
			return level
		}
	}
	return -1
}

func (c *Controller) rowsMatch(o Observation) bool {
	if o.Wallet != c.snapshot.State.AncientSouls || len(o.Rows) == 0 {
		return false
	}
	seen := make(map[int]bool, len(o.Rows))
	for _, row := range o.Rows {
		if seen[row.ID] || c.level(row.ID) != row.Level || row.Cost <= 0 {
			return false
		}
		seen[row.ID] = true
		name := ""
		for _, saved := range c.snapshot.State.Outsiders {
			if saved.ID == row.ID {
				name = saved.Name
			}
		}
		if row.Name != name || name == "" {
			return false
		}
		if o.Quantity > 0 {
			cost, err := ancientcalc.OutsiderFeedCost(row.ID, row.Level, o.Quantity)
			if err != nil || cost != row.Cost {
				return false
			}
		}
	}
	return true
}

// Allowed must be rechecked at dequeue with the current frame. FreshExport and
// BootstrapHeroes are handoffs, not input at this frame's old coordinates.
func (c *Controller) Allowed(now time.Time, cmd Command) bool {
	return cmd.Action != NoAction && cmd == c.Next(now)
}

func (c *Controller) Reserve(now time.Time, cmd Command) error {
	if !c.Allowed(now, cmd) {
		return errors.New("stale or unauthorized Transcension command")
	}
	if cmd.Action == OpenReset {
		plans, err := ancientcalc.PlanOutsiders(context.Background(), c.snapshot.State.AncientSoulsTotal, c.snapshot.State.AncientSouls, c.latest.Reward, c.snapshot.State.Outsiders)
		if err != nil || plans.AfterReward.Status != "ok" {
			return errors.New("observed reward exceeds the supported allocation model")
		}
	}
	c.lastFrame = cmd.Frame
	switch cmd.Action {
	case OpenReset:
		c.reward, c.stage = c.latest.Reward, AwaitConfirmation
	case ConfirmReset:
		c.pending, c.stage, c.exportRequested = cmd, AwaitResetReceipt, false
	case FeedOutsider:
		c.pending, c.stage, c.exportRequested = cmd, AwaitFeedReceipt, false
	case FreshExport:
		c.exportRequested = true
	case BootstrapHeroes:
		c.stage = AwaitFirstSouls
	case ScrollOutsidersTop, ScrollOutsidersDown:
		c.scrolls++
	}
	// The receipt must follow the reset/FEED input, not the later export request.
	if cmd.Action != FreshExport {
		c.inputAt = now
		c.deadline = now.Add(30 * time.Second)
	}
	return nil
}

func sameCycle(a, b ancientcalc.TranscensionState) bool {
	return a.ProfileID == b.ProfileID && a.Build == b.Build && a.SaveVersion == b.SaveVersion && a.Transcensions == b.Transcensions && a.Transcendent == b.Transcendent && a.AncientSoulsTotal == b.AncientSoulsTotal && a.AncientSouls == b.AncientSouls
}

func (c *Controller) AcceptExport(ctx context.Context, now time.Time, s Snapshot) error {
	fail := func(err error) error { c.stop(err.Error()); return err }
	if c.stage != AwaitResetReceipt && c.stage != AwaitFeedReceipt && c.stage != AwaitFirstSouls && c.stage != AwaitAncients {
		return errors.New("no Transcension export is expected")
	}
	if err := validateSnapshot(ctx, s); err != nil {
		return fail(err)
	}
	if s.Generation != c.snapshot.Generation || !fresh(now, s.ExportedAt, 30*time.Second) || !s.ExportedAt.After(c.inputAt) || !s.ExportedAt.After(c.snapshot.ExportedAt) || s.State.SaveHash == c.snapshot.State.SaveHash {
		return fail(errors.New("stale Transcension receipt/export"))
	}
	switch c.stage {
	case AwaitResetReceipt:
		if err := ancientcalc.VerifyTranscensionReceipt(ctx, c.snapshot.State, s.State, c.reward); err != nil {
			return fail(err)
		}
		plans, err := ancientcalc.PlanOutsiders(ctx, s.State.AncientSoulsTotal, s.State.AncientSouls, 0, s.State.Outsiders)
		if err != nil || plans.Now.Status != "ok" {
			return fail(errors.New("post-reset allocation unavailable"))
		}
		c.targets, c.stage = append([]ancientcalc.OutsiderTarget(nil), plans.Now.Additions...), SpendOutsiders
	case AwaitFeedReceipt:
		if err := ancientcalc.VerifyOutsiderFeedReceipt(ctx, c.snapshot.State, s.State, c.pending.ID, c.pending.Quantity); err != nil {
			return fail(err)
		}
		c.stage = SpendOutsiders
	case AwaitFirstSouls, AwaitAncients:
		if !sameCycle(c.snapshot.State, s.State) || !slices.Equal(c.snapshot.State.Outsiders, s.State.Outsiders) || s.State.Ascensions < c.snapshot.State.Ascensions || s.State.AscensionsThisTranscension < c.snapshot.State.AscensionsThisTranscension || s.State.HighestZone < c.snapshot.State.HighestZone {
			return fail(errors.New("profile/cycle or Outsider ledger changed during restoration"))
		}
		wallet, ok := new(big.Rat).SetString(s.State.HeroSouls)
		if !ok {
			return fail(errors.New("invalid restored Hero Souls"))
		}
		if c.stage == AwaitFirstSouls && (wallet.Sign() <= 0 || s.State.AscensionsThisTranscension == 0) {
			break
		}
		missing, err := ancientcalc.MissingActiveAncients(ctx, s.State, c.policy.SkillRate, c.policy.Beyond8k)
		if err != nil {
			return fail(err)
		}
		if len(missing) == 0 {
			c.stage = ReadyForAllocation
		} else {
			c.stage, c.reason = AwaitAncients, "summon missing Active Ancients using verified native controls, then export again"
		}
	}
	c.snapshot, c.pending, c.exportRequested = cloneSnapshot(s), Command{}, false
	c.scrolls, c.deadline = 0, now.Add(30*time.Second)
	return nil
}

func (c *Controller) MissingAncients(ctx context.Context) ([]ancientcalc.AncientRequirement, error) {
	return ancientcalc.MissingActiveAncients(ctx, c.snapshot.State, c.policy.SkillRate, c.policy.Beyond8k)
}

// Reconcile never retries a pending destructive input. It only accepts its exact
// fresh receipt, including after F8 changes the capture generation.
func (c *Controller) Reconcile(ctx context.Context, now time.Time, s Snapshot) error {
	if c.stage != Uncertain || c.pending.Action != ConfirmReset && c.pending.Action != FeedOutsider {
		return errors.New("no pending reset/FEED receipt to reconcile")
	}
	next := *c
	next.snapshot = cloneSnapshot(c.snapshot)
	next.snapshot.Generation = s.Generation
	if c.pending.Action == ConfirmReset {
		next.stage = AwaitResetReceipt
	} else {
		next.stage = AwaitFeedReceipt
	}
	if err := next.AcceptExport(ctx, now, s); err != nil {
		return err
	}
	next.latest, next.lastFrame, next.reason = Observation{}, 0, ""
	*c = next
	return nil
}

// CompleteAllocation is called only after the ordinary fresh-save Ancient
// allocator and its purchase/return handoff finish; it does not grant summoning.
func (c *Controller) CompleteAllocation(saveHash string) error {
	if c.stage != ReadyForAllocation || saveHash != c.snapshot.State.SaveHash {
		return errors.New("Ancient restoration is incomplete")
	}
	c.stage, c.reason = Ordinary, ""
	return nil
}

func cloneSnapshot(s Snapshot) Snapshot {
	s.State.Ancients = append([]ancientcalc.Level{}, s.State.Ancients...)
	s.State.Outsiders = append([]ancientcalc.Level(nil), s.State.Outsiders...)
	for _, field := range []**int{&s.State.CurrentZone, &s.State.CurrentTranscensionID, &s.State.CurrentAscensionID, &s.State.CurrentAscensionCycleID} {
		if *field != nil {
			value := **field
			*field = &value
		}
	}
	return s
}
