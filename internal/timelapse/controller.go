package timelapse

import (
	"fmt"
	"time"

	"clicker-heroes-bot/internal/rubybudget"
)

// NativeRecognitionAvailable stays false until real Timelapse shop, confirmation,
// cancellation and result fixtures calibrate the shared-frame reader. No guessed
// text, color, coordinates or ordinary idle countdown can authorize payment.
const NativeRecognitionAvailable = false

type Stage uint8

const (
	Ordinary Stage = iota
	AwaitShop
	AwaitConfirmation
	AwaitOutcome
	AwaitFreshPreparation
	Uncertain
)

type Action uint8

const (
	NoAction Action = iota
	OpenShop
	SelectOffer
	ConfirmPurchase
	CancelOffer
	CloseShop
)

// Preparation is a contract for the other feature owners and the main pipeline.
// These fields must come from a fresh export and newer recognized shared frames,
// not from the ideal forecast or an existing advisory gild plan.
type Preparation struct {
	ProfileID, AscensionID, Build, Scene                          string
	Generation                                                    uint64
	ExportedAt, ObservedAt                                        time.Time
	Zone                                                          int
	Rubies                                                        uint64
	HybridVerified, HeroVerified, GildsVerified, UpgradesVerified bool
	IdleMechanicsVerified                                         bool // Includes native Nogardnit behavior; no invented wait rule.
	ClickerReservationActive                                      bool
	FreeClickers                                                  int
	ClickerPlacementUncertain                                     bool
	Hero                                                          string
	HeroLevel                                                     int
}

type Policy struct {
	Enabled         bool
	ProtectedBuild  string
	MinZones        int
	MinZonesPerRuby float64
	MinSavedTime    time.Duration
	// Observed active return speed, not the calculator's ideal 8050 zones/hour.
	ActiveZonesPerHour float64
	MaxEvidenceAge     time.Duration
}

type NativeEvidence struct {
	Build                                                                  string
	OffersAndPricesVerified, ConfirmationAndCancelVerified, ResultVerified bool
}

func (n NativeEvidence) verified(build string) bool {
	return n.Build != "" && n.Build == build && n.OffersAndPricesVerified && n.ConfirmationAndCancelVerified && n.ResultVerified
}

type Readiness struct {
	Ready     bool
	Reason    string
	SavedTime time.Duration
}

func Assess(now time.Time, policy Policy, prep Preparation, row Row) Readiness {
	fail := func(reason string) Readiness { return Readiness{Reason: reason} }
	if !policy.Enabled {
		return fail("Timelapse disabled")
	}
	if row.Hours == 0 {
		return fail("ordinary progression row")
	}
	if row.Hours != 8 && row.Hours != 24 && row.Hours != 48 && row.Hours != 168 {
		return fail("unsupported duration")
	}
	// Duration pricing is checked directly; forecast heuristics choose duration,
	// while a short final jump could be uneconomic even with a valid price.
	prices := map[int]int{8: 100, 24: 200, 48: 300, 168: 500}
	if row.Rubies != prices[row.Hours] || row.StartZone != prep.Zone || row.Zone <= prep.Zone || row.Zone-prep.Zone > row.Hours*4500 {
		return fail("invalid or stale forecast row")
	}
	age := policy.MaxEvidenceAge
	if age <= 0 {
		age = 30 * time.Second
	}
	if prep.ProfileID == "" || prep.AscensionID == "" || prep.Scene == "" || prep.Generation == 0 || prep.ExportedAt.IsZero() || prep.ObservedAt.IsZero() || prep.ExportedAt.After(now) || prep.ObservedAt.After(now) || now.Sub(prep.ExportedAt) > age || now.Sub(prep.ObservedAt) > 5*time.Second {
		return fail("fresh export and shared-frame preparation required")
	}
	if policy.ProtectedBuild == "" || prep.Build != policy.ProtectedBuild {
		return fail("unsupported game build")
	}
	if !prep.HybridVerified || !prep.HeroVerified || !prep.GildsVerified || !prep.UpgradesVerified || prep.Hero != row.Hero || prep.HeroLevel < row.Level {
		return fail("Hybrid Ancient, hero, upgrade and gild preparation required")
	}
	if !prep.IdleMechanicsVerified {
		return fail("native Timelapse idle mechanics unverified")
	}
	if !prep.ClickerReservationActive || prep.FreeClickers < 1 || prep.ClickerPlacementUncertain {
		return fail("positively recognized reserved free Auto Clicker required")
	}
	minimum := policy.MinZones
	if minimum <= 0 {
		minimum = 20000
	}
	if row.Zone-prep.Zone <= minimum {
		return fail("insufficient zones saved")
	}
	if !validFloat(policy.ActiveZonesPerHour) || policy.ActiveZonesPerHour <= 0 || !validFloat(policy.MinZonesPerRuby) || policy.MinZonesPerRuby < 0 || policy.MinSavedTime < 0 {
		return fail("measured active return speed and valid economics required")
	}
	if float64(row.Zone-prep.Zone)/float64(row.Rubies) < policy.MinZonesPerRuby {
		return fail("poor zones per ruby")
	}
	seconds := float64(row.Zone-prep.Zone) / policy.ActiveZonesPerHour * 3600
	if seconds > float64((24*365*time.Hour)/time.Second) {
		return fail("unsupported active-time estimate")
	}
	saved := time.Duration(seconds * float64(time.Second))
	if saved < policy.MinSavedTime {
		return fail("insufficient active time saved")
	}
	return Readiness{Ready: true, SavedTime: saved}
}

// Screen is a native-reader contract; Known must only be set by a calibrated
// reader on the existing shared capture. Unknown/modal screens suppress input.
type Screen uint8

const (
	UnknownScreen Screen = iota
	GameScreen
	ShopScreen
	ConfirmationScreen
	ResultScreen
)

type Observation struct {
	Frame, Generation uint64
	ObservedAt        time.Time
	Scene, Build      string
	Screen            Screen
	// The offer and confirmation independently recognize exact duration, price
	// and current wallet. Buttons must be identified as this transaction's controls.
	Hours                                                         int
	Price, Balance                                                uint64
	EntryKnown, OfferKnown, ConfirmKnown, CancelKnown, CloseKnown bool
	ResultKnown                                                   bool
	ResultZone                                                    int
}

type Command struct {
	Action            Action
	Frame, Generation uint64
	Hours             int
	Price             uint64
}

type Budget interface {
	Snapshot() (rubybudget.Snapshot, error)
	Reserve(uint64, string, uint64) (rubybudget.Reservation, error)
	ConfirmSpent(string) error
}

// Controller only returns abstract commands. The pipeline owns coordinate
// recognition and serialized input, independent fish analysis, modal click
// suppression and F8. Call Allowed again at dequeue and immediately before input.
// All methods must run under that serialized dispatch owner.
type Controller struct {
	reserveHeld             bool
	inputCount              int
	stage                   Stage
	policy                  Policy
	native                  NativeEvidence
	budget                  Budget
	row                     Row
	prep                    Preparation
	latest                  Observation
	lastInput, minimumFrame uint64
	deadline                time.Time
	reservation             string
	reason                  string
	// Tests exercise future native stages without turning on production recognition.
	recognition bool
}

func NewController(policy Policy, native NativeEvidence, budget Budget) *Controller {
	c := &Controller{policy: policy, native: native, budget: budget, recognition: NativeRecognitionAvailable}
	if budget != nil {
		if state, err := budget.Snapshot(); err == nil && state.Pending != nil {
			c.reserveHeld = true
			c.stage, c.reservation, c.reason = Uncertain, state.Pending.ID, "pending ruby debit requires explicit reconciliation"
		}
	}
	return c
}
func (c *Controller) Stage() Stage   { return c.stage }
func (c *Controller) Reason() string { return c.reason }

// HoldsInputs suppresses ordinary/fish input under a Timelapse modal. Analysis
// can continue independently. Unknown outcome keeps the clicker reserve intact.
func (c *Controller) HoldsInputs() bool {
	return c.stage == AwaitShop || c.stage == AwaitConfirmation || c.stage == AwaitOutcome || c.stage == Uncertain
}
func (c *Controller) HoldsClickerReserve() bool { return c.reserveHeld }

func (c *Controller) Begin(now time.Time, prep Preparation, row Row) Readiness {
	if c.stage != Ordinary {
		return Readiness{Reason: "Timelapse transaction already active"}
	}
	r := Assess(now, c.policy, prep, row)
	if !r.Ready {
		c.reason = r.Reason
		return r
	}
	if !c.recognition || !c.native.verified(prep.Build) {
		c.reason = "native Timelapse recognition/fixtures unverified"
		return Readiness{Reason: c.reason}
	}
	if c.budget == nil {
		c.reason = "durable ruby allowance required"
		return Readiness{Reason: c.reason}
	}
	s, err := c.budget.Snapshot()
	if err != nil {
		c.reason = err.Error()
		return Readiness{Reason: c.reason}
	}
	if s.ProfileID != prep.ProfileID || s.Pending != nil || s.InitialAllowance <= s.Spent || uint64(row.Rubies) > s.InitialAllowance-s.Spent || prep.Rubies < s.ProtectedBalance || uint64(row.Rubies) > prep.Rubies-s.ProtectedBalance || s.SpentByAscension[prep.AscensionID] > s.PerAscensionCap || uint64(row.Rubies) > s.PerAscensionCap-s.SpentByAscension[prep.AscensionID] {
		c.reason = "ruby allowance, reserve, pending debit or Ascension cap blocks purchase"
		return Readiness{Reason: c.reason}
	}
	c.reserveHeld = true
	c.inputCount = 0
	c.prep, c.row = prep, row
	c.stage = AwaitShop
	c.deadline = now.Add(20 * time.Second)
	c.reason = ""
	return r
}

func (c *Controller) Observe(now time.Time, o Observation) {
	if c.stage == Ordinary || c.stage == AwaitFreshPreparation {
		return
	}
	if !now.Before(c.deadline) {
		c.stop("Timelapse stage timeout")
		return
	}
	if o.Frame <= c.latest.Frame || o.Frame <= c.minimumFrame || o.ObservedAt.After(now) || now.Sub(o.ObservedAt) > 5*time.Second {
		return
	}
	if o.Generation != c.prep.Generation || o.Scene != c.prep.Scene || o.Build != c.prep.Build {
		c.stop("Timelapse scene/build changed")
		return
	}
	c.latest = o
	if c.stage == AwaitOutcome && o.Frame > c.lastInput && o.Screen == ResultScreen && o.ResultKnown && o.ResultZone > c.prep.Zone && o.ResultZone <= c.row.Zone && o.Hours == c.row.Hours && o.Price == uint64(c.row.Rubies) {
		// A recognized result acknowledges the one-shot payment. A fresh export
		// still gates another preparation; closing a dialog alone is insufficient.
		if err := c.budget.ConfirmSpent(c.reservation); err != nil {
			c.stop(fmt.Sprintf("Timelapse debit acknowledgement failed: %v", err))
			return
		}
		c.stage = AwaitFreshPreparation
		c.reason = "fresh outcome export/preparation required"
		return
	}
	if c.stage == AwaitShop && o.Screen == ConfirmationScreen {
		c.stop("foreign or unexpected Timelapse confirmation")
	}
}

func (c *Controller) Command(now time.Time) Command {
	o := c.latest
	if c.stage == Ordinary || c.stage == Uncertain || c.stage == AwaitFreshPreparation || o.Frame == 0 || o.Frame <= c.lastInput || now.Sub(o.ObservedAt) > 5*time.Second || !now.Before(c.deadline) {
		return Command{}
	}
	cmd := Command{Frame: o.Frame, Generation: o.Generation, Hours: c.row.Hours, Price: uint64(c.row.Rubies)}
	switch c.stage {
	case AwaitShop:
		if o.Screen == GameScreen && o.EntryKnown {
			cmd.Action = OpenShop
		} else if o.Screen == ShopScreen && o.OfferKnown && o.Hours == cmd.Hours && o.Price == cmd.Price {
			cmd.Action = SelectOffer
		} else if o.Screen == ShopScreen && o.CloseKnown {
			cmd.Action = CloseShop
		}
	case AwaitConfirmation:
		if o.Screen == ConfirmationScreen && o.CancelKnown {
			if o.ConfirmKnown && o.Hours == cmd.Hours && o.Price == cmd.Price {
				cmd.Action = ConfirmPurchase
			} else {
				cmd.Action = CancelOffer
			}
		}
	}
	return cmd
}

func (c *Controller) Allowed(now time.Time, cmd Command, prep Preparation) bool {
	if cmd.Action == NoAction || cmd != c.Command(now) || prep.ProfileID != c.prep.ProfileID || prep.AscensionID != c.prep.AscensionID || prep.Generation != c.prep.Generation || prep.Scene != c.prep.Scene {
		return false
	}
	if cmd.Action == CancelOffer || cmd.Action == CloseShop {
		return true
	}
	return Assess(now, c.policy, prep, c.row).Ready
}

// BeforeInput reserves the debit and advances BEFORE the irreversible click.
// If input is missed, canceled, paused or its acknowledgement is lost, never
// replay it. No pending debit is refunded automatically from a balance delta.
func (c *Controller) BeforeInput(now time.Time, cmd Command, prep Preparation) error {
	if c.inputCount >= 6 {
		c.stop("Timelapse input attempt bound exceeded")
		return fmt.Errorf("Timelapse input attempt bound exceeded")
	}
	if !c.Allowed(now, cmd, prep) {
		return fmt.Errorf("stale or unready Timelapse command")
	}
	if cmd.Action == ConfirmPurchase {
		r, err := c.budget.Reserve(cmd.Price, prep.AscensionID, c.latest.Balance)
		if err != nil {
			c.stop(err.Error())
			return err
		}
		c.reservation = r.ID
		c.stage = AwaitOutcome
	} else if cmd.Action == SelectOffer {
		c.stage = AwaitConfirmation
	} else if cmd.Action == CancelOffer || cmd.Action == CloseShop {
		c.stage = Ordinary
		c.reserveHeld = false
		c.reason = "Timelapse offer canceled"
	}
	c.inputCount++
	c.lastInput = cmd.Frame
	c.deadline = now.Add(20 * time.Second)
	return nil
}

// Interrupt handles F8 without clearing an uncertain debit. Resume must obtain
// newer frames and fresh preparation; an uncertain payment requires manual review.
func (c *Controller) Interrupt() { c.stop("Timelapse interrupted") }
func (c *Controller) stop(reason string) {
	c.reason = reason
	if c.reservation != "" || c.stage == AwaitOutcome {
		c.stage = Uncertain
	} else {
		c.stage = Ordinary
	}
	c.minimumFrame = c.latest.Frame
	c.latest = Observation{}
}

// CompletePreparation releases this controller after a newly exported result.
// The clicker owner may then endIdlePreparation and resume ordinary Active play.
func (c *Controller) CompletePreparation(prep Preparation) error {
	if c.stage != AwaitFreshPreparation || prep.ProfileID != c.prep.ProfileID || prep.AscensionID != c.prep.AscensionID || !prep.ExportedAt.After(c.latest.ObservedAt) || prep.Zone < c.latest.ResultZone {
		return fmt.Errorf("fresh Timelapse outcome export required")
	}
	c.stage = Ordinary
	c.reserveHeld = false
	c.reservation = ""
	c.latest = Observation{}
	return nil
}

// ReleaseUnpaidPreparation lets the dispatch owner resume Active allocation
// after a skipped/canceled preparation only on a newer ordinary game frame.
// F8 alone never releases the reserve, and pending payment cannot use this path.
func (c *Controller) ReleaseUnpaidPreparation(now time.Time, o Observation) error {
	if c.stage != Ordinary || c.reservation != "" || o.Screen != GameScreen || o.Frame <= c.minimumFrame || o.Frame <= c.lastInput || o.Generation != c.prep.Generation || o.Scene != c.prep.Scene || o.Build != c.prep.Build || o.ObservedAt.IsZero() || o.ObservedAt.After(now) || now.Sub(o.ObservedAt) > 5*time.Second {
		return fmt.Errorf("new ordinary game frame required to release unpaid preparation")
	}
	c.reserveHeld = false
	return nil
}
