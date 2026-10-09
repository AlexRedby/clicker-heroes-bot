package bot

import (
	"context"
	"fmt"
	"image"
	"math"
	"math/big"
	"regexp"
	"strings"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/vision"
)

var ascensionRewardLabel = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?(?:[eE][0-9]+)?)\s+Hero Souls?$`)

const (
	ascensionSpiral = iota
	ascensionTitle
	ascensionYes
	ascensionNo
)

var ascensionPendingLabel = regexp.MustCompile(`^Ascend for \+([0-9]+(?:\.[0-9]+)?(?:[eE][0-9]+)?)(?:\s+Hero Souls?)?$`)
var ascensionSoulRegions = [...]image.Rectangle{image.Rect(410, 172, 591, 199), image.Rect(405, 198, 600, 218)}

var ascensionRewardRegion = image.Rect(675, 307, 887, 329)

func ascensionControl(screen image.Image, control int) (image.Point, bool, error) {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return image.Point{}, false, nil
	}
	regions := [...]image.Rectangle{
		image.Rect(1237, 247, 1251, 263), // Opaque center of the World Ascension spiral.
		image.Rect(502, 108, 778, 130),   // Specific Ascension dialog title.
		image.Rect(485, 468, 625, 530),   // Yes; Quick Ascension is a different button below it.
		image.Rect(655, 468, 796, 530),   // No.
	}
	names := [...]string{"ascension/spiral.png", "ascension/title.png", "ascension/confirm.png", "ascension/cancel.png"}
	found, err := vision.MatchControl(screen, regions[control], names[control])
	r := vision.Rect(screen, regions[control])
	return r.Min.Add(r.Size().Div(2)), found, err
}

func ascensionDialog(screen image.Image) (bool, error) {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false, nil
	}
	// Avoid template work on the ordinary HUD; these points are blank dialog corners.
	for _, point := range []image.Point{{370, 140}, {910, 140}} {
		p := vision.Rect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
		r, g, b := rgb(screen.At(p.X, p.Y))
		if r < 235 || g < 225 || b < 160 {
			return false, nil
		}
	}
	_, found, err := ascensionControl(screen, ascensionTitle)
	return found, err
}

type ascensionStep uint8

const (
	openAscension ascensionStep = iota
	confirmAscension
	cancelAscension
	waitAscensionReset
	salvageAscensionJunk
	waitAscensionJunk
)

type ascensionObservation struct {
	frame       gameFrame
	confirm, no bool
	junk        bool
	souls       float64
	zone        int
	economy     bool
	bank        float64
}

func readAscensionObservation(ctx context.Context, frame gameFrame) (ascensionObservation, error) {
	out := ascensionObservation{frame: frame}
	if relicJunkDialog(frame.image) {
		out.junk = true
		_, out.confirm, _ = relicJunkControl(frame.image, true)
		_, out.no, _ = relicJunkControl(frame.image, false)
		if !out.confirm || !out.no {
			return out, fmt.Errorf("Junk Pile confirmation controls not recognized")
		}
		return out, nil // The fixed native prompt authorizes Junk Pile only; no reward OCR.
	}
	if frame.context.ascension {
		_, out.confirm, _ = ascensionControl(frame.image, ascensionYes)
		_, out.no, _ = ascensionControl(frame.image, ascensionNo)
		if !out.confirm || !out.no {
			return out, fmt.Errorf("Ascension confirmation controls not recognized")
		}
		raw, err := readGameText(ctx, frame.image, vision.Rect(frame.image, ascensionRewardRegion), max(1, 2048/frame.image.Bounds().Dx()), 7, 0, "0123456789.eEabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
		if err != nil {
			return out, fmt.Errorf("Ascension reward: %w", err)
		}
		match := ascensionRewardLabel.FindStringSubmatch(strings.TrimSpace(raw))
		if match == nil {
			return out, fmt.Errorf("unreadable Ascension reward %q", strings.TrimSpace(raw))
		}
		value, ok := parseGameNumber(match[1])
		if !ok {
			return out, fmt.Errorf("%w in Ascension reward %q", errUnreadableGameNumber, match[1])
		}
		out.souls = value
		return out, nil
	}
	// The initial HUD can lack an unlocked progression control. Verify the hero
	// quantity bar before reading zone, rather than treating an unknown image as a reset.
	if heroQuantityBarPresent(frame.image) {
		var err error
		out.zone, err = readProgressionZone(ctx, frame.image)
		return out, err
	}
	return out, nil
}

// Read the two fixed HUD lines only when a reset candidate exists.
func readAscensionEconomy(ctx context.Context, frame gameFrame) (ascensionObservation, error) {
	out := ascensionObservation{frame: frame, economy: true}
	for i, region := range ascensionSoulRegions {
		raw, err := readGameText(ctx, frame.image, vision.Rect(frame.image, region), max(1, 2560/frame.image.Bounds().Dx()), 7, 180, "0123456789.eEabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ +")
		if err != nil {
			return out, err
		}
		label := ascensionRewardLabel
		if i == 1 {
			label = ascensionPendingLabel
		}
		match := label.FindStringSubmatch(strings.TrimSpace(raw))
		if match == nil {
			return out, fmt.Errorf("unreadable Ascension soul budget %q", strings.TrimSpace(raw))
		}
		value, ok := parseGameNumber(match[1])
		if !ok {
			return out, errUnreadableGameNumber
		}
		if i == 0 {
			out.bank = value
		} else {
			out.souls = value
		}
	}
	return out, nil
}

// All soul amounts are log10 values, including zero as -Inf.
func soulCapital(bank, invested float64) float64 {
	hi, lo := max(bank, invested), min(bank, invested)
	if math.IsInf(hi, -1) {
		return hi
	}
	return hi + math.Log10(1+math.Pow(10, lo-hi))
}

// Keep the exported total as a floor, including after a planned Ancient batch
// transfers the same souls from the wallet into Ancients. A fresh save is needed
// after manual spending or another Ascension/Transcension.
func ascensionSoulCapital(plan *ancientPlan) (float64, error) {
	total := math.Inf(-1)
	if plan == nil || plan.Invested == "" {
		return total, nil
	}
	for _, raw := range []string{plan.Souls, plan.Invested} {
		value, err := ancientcalc.Value(raw)
		if err != nil {
			return 0, err
		}
		mantissa := new(big.Float)
		exponent := value.MantExp(mantissa)
		m, _ := mantissa.Float64()
		total = soulCapital(total, math.Log10(m)+float64(exponent)*math.Log10(2))
	}
	return total, nil
}

type ascensionPlanner struct {
	highestZone, wallZone          int
	lastProgress, lastObservation  time.Time
	nextCheck                      time.Time
	active                         bool
	firstRun                       bool // Run identity from an accepted fresh save, not input permission.
	fullCombatFailed               bool
	relicsChecked                  bool // Advisory save check, not permission to move or salvage items.
	minimumReward                  float64
	step                           ascensionStep
	lastInputFrame, jobFrame       uint64
	nextRead, nextAction, deadline time.Time
	latest                         ascensionObservation
	junkSalvaged                   bool
	junkReopened                   bool
	junkCancelled                  bool
}

func (p *ascensionPlanner) interrupt() { *p = ascensionPlanner{} }

func (p *ascensionPlanner) resetAttempt() {
	firstRun, firstZone := p.firstRun, p.highestZone
	p.interrupt()
	p.firstRun = firstRun
	if firstRun {
		p.highestZone = firstZone
	}
}

func (p *ascensionPlanner) observeSave(out exportResult) {
	p.firstRun = false
	if out.transcension != nil && out.transcensionErr == nil {
		s := out.transcension.State
		p.firstRun = s.SaveHash != "" && s.Transcendent && s.AscensionsThisTranscension == 0
		return
	}
	preview := out.prestige
	if preview == nil && out.plan != nil {
		preview = out.plan.Transcension
	}
	if preview != nil {
		p.firstRun = preview.SaveHash != "" && preview.Transcendent && preview.Ascensions == 0
	}
}

func (p *ascensionPlanner) invalidate() {
	p.relicsChecked = false
	p.lastObservation = time.Time{}
	p.latest = ascensionObservation{}
	p.jobFrame = 0
}

func (p *ascensionPlanner) observeProgress(s progressionState, wall int, now time.Time, fullCombatFailed bool) {
	if p.active || !s.Known || s.Zone <= 0 {
		return
	}
	// First-run progression must not restart a completed preflight before its early Ascension.
	if !p.firstRun && (s.Enabled || s.Zone > p.highestZone || wall != p.wallZone) {
		p.relicsChecked = false
	}
	if !s.observedAt.IsZero() {
		now = s.observedAt
	}
	if p.highestZone == 0 || s.Zone < p.highestZone-1 {
		if p.highestZone != 0 {
			p.firstRun, p.relicsChecked = false, false
		}
		p.highestZone, p.wallZone = s.Zone, 0
		p.lastProgress = now
	}
	p.lastObservation = now
	if s.Zone > p.highestZone {
		p.highestZone = s.Zone
		p.lastProgress = now
	}
	p.wallZone, p.fullCombatFailed = 0, false
	// Require an observed boss and its recognized fallback, not a disabled toggle alone.
	if !s.Enabled && wall > 0 && p.highestZone >= wall && s.Zone >= wall-1 && s.Zone <= wall {
		p.wallZone = wall
		p.fullCombatFailed = fullCombatFailed
	}
}

func (p *ascensionPlanner) due(now time.Time, stall time.Duration) bool {
	return !p.active && p.wallZone > 0 && now.Sub(p.lastObservation) <= 10*time.Second &&
		(p.fullCombatFailed || !now.Before(p.lastProgress.Add(stall))) && !now.Before(p.nextCheck)
}

func (p *gamePipeline) ascensionCandidate(now time.Time) bool {
	if p.ascension.due(now, p.options.ascensionStall) {
		return true
	}
	progress := p.state[progressionAnalysis]
	return p.ascension.firstRun && !p.ascension.active && !now.Before(p.ascension.nextCheck) &&
		p.startup == noStartup && !p.startupCheck && p.frame.context.heroes &&
		progress.progression.Known && progress.progression.Zone > 130 &&
		progress.frame.id != 0 && progress.frame.id <= p.frame.id && progress.frame.id >= p.barriers[progressionAnalysis] &&
		progress.frame.generation == p.generation && progress.frame.layout == p.layout && progress.frame.context == p.frame.context &&
		!progress.frame.at.After(now) && now.Sub(progress.frame.at) <= 10*time.Second &&
		p.ascension.lastObservation.Equal(progress.frame.at)
}

func (p *ascensionPlanner) observe(out ascensionObservation, err error, now time.Time) (reset bool) {
	if !p.active {
		if out.junk && err == nil && out.no {
			// F8/restart does not inherit permission to destroy an orphaned Junk Pile.
			p.active, p.step, p.latest = true, cancelAscension, out
			p.deadline = now.Add(20 * time.Second)
			return false
		}
		if out.economy {
			p.latest = out
			if err != nil {
				p.latest = ascensionObservation{}
				p.nextRead = now.Add(30 * time.Second)
				fmt.Printf("Ascension soul budget unreadable: %v; retrying in 30s\n", err)
			}
		}
		return false
	}
	if out.frame.id <= p.lastInputFrame {
		return false
	}
	p.latest = out
	if out.junk {
		if p.step == cancelAscension && p.junkCancelled {
			p.latest = ascensionObservation{} // A delayed orphan No is not replayed.
		}
		if err == nil && out.confirm && out.no && (p.step == openAscension || p.step == confirmAscension || p.step == waitAscensionReset) {
			p.step = salvageAscensionJunk
			p.deadline = now.Add(20 * time.Second)
			if p.junkSalvaged {
				p.step = cancelAscension // Never replay salvage, even after reopening Ascension.
			}
		}
		return false // An unchanged prompt after Yes cannot advance or receive another Yes.
	}
	if p.step == waitAscensionJunk && err == nil && out.frame.context.known {
		if !out.frame.context.ascension && out.zone == 1 {
			*p = ascensionPlanner{}
			return true
		}
		if out.frame.context.ascension {
			p.step = openAscension // Fall through to independent Hero Souls reward validation.
		} else if out.frame.context.heroes && !p.junkReopened {
			p.step, p.junkReopened = openAscension, true
			return false
		}
	}
	if (p.step == openAscension || p.step == confirmAscension) && out.frame.context.ascension {
		if err != nil {
			p.latest = ascensionObservation{}
			p.nextRead = now.Add(time.Second)
			fmt.Printf("Ascension reward unreadable: %v; retrying on a fresh dialog frame\n", err)
			return false
		}
		if math.IsInf(out.souls, -1) || out.souls < p.minimumReward {
			fmt.Println("Ascension skipped: Hero Souls reward below the minimum useful gain")
			p.step = cancelAscension
		} else {
			p.step = confirmAscension
			fmt.Printf("Ascension reward verified: %.3fe%.0f Hero Souls\n", math.Pow(10, out.souls-math.Floor(out.souls)), math.Floor(out.souls))
		}
	}
	if p.step == waitAscensionReset && err == nil && !out.frame.context.ascension && out.zone == 1 {
		*p = ascensionPlanner{}
		return true
	}
	if p.step == cancelAscension && !out.frame.context.ascension && out.frame.context.known {
		p.active, p.latest = false, ascensionObservation{}
		p.deadline = time.Time{}
		p.nextCheck = now.Add(time.Minute)
	}
	return false
}

func (p *ascensionPlanner) sent(step ascensionStep, frameID uint64, now time.Time) {
	if step == cancelAscension && p.latest.junk {
		p.junkCancelled = true
	}
	p.active = true
	p.lastInputFrame = frameID
	p.latest = ascensionObservation{}
	p.nextAction, p.nextRead = now.Add(500*time.Millisecond), now.Add(500*time.Millisecond)
	if p.deadline.IsZero() {
		p.deadline = now.Add(20 * time.Second)
	}
	if step == confirmAscension {
		p.step = waitAscensionReset
	} else if step == salvageAscensionJunk {
		p.step, p.junkSalvaged = waitAscensionJunk, true
	} else {
		p.step = step
	}
}

// The shared queue uses this for normal confirmation, Junk Pile and orphan No.
// Waiting states deliberately have no input point.
func ascensionActionPoint(screen image.Image, step ascensionStep) (image.Point, bool, error) {
	if step == salvageAscensionJunk {
		return relicJunkControl(screen, true)
	}
	if step == cancelAscension && relicJunkDialog(screen) {
		return relicJunkControl(screen, false)
	}
	control := ascensionSpiral
	if step == confirmAscension || step == cancelAscension {
		known, err := ascensionDialog(screen)
		if err != nil || !known {
			return image.Point{}, false, err
		}
		control = ascensionYes
		if step == cancelAscension {
			control = ascensionNo
		}
	} else if step != openAscension {
		return image.Point{}, false, nil
	}
	return ascensionControl(screen, control)
}

func ascensionActionStable(a gameAction, current gameFrame) bool {
	if a.frame.image == nil || current.image == nil || a.frame.context != current.context || a.frame.generation != current.generation || a.frame.layout != current.layout {
		return false
	}
	point, found, err := ascensionActionPoint(current.image, a.ascension)
	if err != nil || !found || point != a.point {
		return false
	}
	if a.ascension != confirmAscension {
		return true
	}
	// The pending reward must still be visible, unchanged since its single OCR read.
	r := vision.Rect(current.image, ascensionRewardRegion)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			yellow := func(screen image.Image) bool {
				r, g, b := rgb(screen.At(x, y))
				return min(r, g) > 170 && r-b > 40 && g-b > 25
			}
			if yellow(a.frame.image) != yellow(current.image) {
				return false
			}
		}
	}
	return true
}

func ascensionBudgetStable(before, after image.Image) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	for _, region := range ascensionSoulRegions {
		r := vision.Rect(after, region)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				bright := func(im image.Image) bool { r, g, b := rgb(im.At(x, y)); return min(r, min(g, b)) > 180 }
				if bright(before) != bright(after) {
					return false
				}
			}
		}
	}
	return true
}
