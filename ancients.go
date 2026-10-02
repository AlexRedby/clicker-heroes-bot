package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"

	"github.com/go-vgo/robotgo"
)

type ancientPlan struct {
	ancientcalc.Plan
	SaveHash          string                           `json:"saveHash"`
	CreatedAt         time.Time                        `json:"createdAt"`
	Gilds             *ancientcalc.GildPlan            `json:"gilds,omitempty"`
	GildError         string                           `json:"gildError,omitempty"`
	Transcension      *ancientcalc.TranscensionPreview `json:"transcension,omitempty"`
	TranscensionError string                           `json:"transcensionError,omitempty"`

	savePath string
}

func calculateAncients(ctx context.Context, savePath, reserve string, skillRate float64, beyond8k bool) (ancientPlan, error) {
	var plan ancientPlan
	if _, err := ancientcalc.Value(strings.TrimSuffix(reserve, "%")); err != nil {
		return plan, fmt.Errorf("soul reserve: %w", err)
	}
	if !(skillRate >= 0 && skillRate <= 1) {
		return plan, errors.New("-ancient-skill-rate must be between 0 and 1")
	}
	file, err := os.Open(savePath)
	if err != nil {
		return plan, fmt.Errorf("read exported save: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return plan, fmt.Errorf("read exported save: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > ancientcalc.MaxSaveInput {
		return plan, errors.New("exported save must be a nonempty regular file no larger than 4 MiB")
	}
	save, err := io.ReadAll(io.LimitReader(file, ancientcalc.MaxSaveInput+1))
	if err != nil {
		return plan, fmt.Errorf("read exported save: %w", err)
	}
	return calculateAncientData(ctx, save, savePath, reserve, skillRate, beyond8k)
}

func calculateAncientData(ctx context.Context, save []byte, savePath, reserve string, skillRate float64, beyond8k bool) (ancientPlan, error) {
	var plan ancientPlan
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	plan.Plan, err = ancientcalc.Calculate(ctx, save, reserve, skillRate, beyond8k)
	if err != nil {
		return plan, fmt.Errorf("Ancient calculator: %w", err)
	}
	// Redistribution remains preview-only, so it does not reduce the purchase budget.
	gilds, err := ancientcalc.CalculateGilds(ctx, save, reserve, nil)
	if ctx.Err() != nil {
		return plan, ctx.Err()
	}
	if err != nil {
		// Ancient-only exports can still be planned; report unavailable gild metadata.
		plan.GildError = err.Error()
	} else {
		plan.Gilds = &gilds
	}
	prestige, err := ancientcalc.PreviewTranscension(ctx, save)
	if ctx.Err() != nil {
		return plan, ctx.Err()
	}
	if err != nil {
		plan.TranscensionError = err.Error()
	} else {
		plan.Transcension = &prestige
	}

	sum := sha256.Sum256(save)
	plan.SaveHash = hex.EncodeToString(sum[:])
	plan.CreatedAt = time.Now().UTC()
	plan.savePath, err = filepath.Abs(savePath)
	return plan, err
}

func writeAncientPlan(path string, plan ancientPlan) error {
	if plan.savePath != "" {
		output, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		sourceInfo, _ := os.Stat(plan.savePath)
		outputInfo, _ := os.Stat(output)
		if output == plan.savePath || (sourceInfo != nil && outputInfo != nil && os.SameFile(sourceInfo, outputInfo)) {
			return errors.New("plan output must not overwrite the exported save")
		}
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	fmt.Printf("Ancient plan: %d purchases, %s Hero Souls spent, %s remaining\n", len(plan.Rows), plan.Spent, plan.Remaining)
	for _, a := range plan.Rows {
		fmt.Printf("%s: %s -> %s (+%s), cost %s Hero Souls\n", a.Name, a.Current, a.Target, a.Quantity, a.Cost)
	}
	if plan.Gilds != nil {
		g := plan.Gilds
		fmt.Printf("Gild redistribution preview: %d gilds to %s, cost %s Hero Souls\n", g.MoveGilds, g.Target.Name, g.Cost)
		if len(g.Reasons) > 0 {
			fmt.Println("Gild redistribution blocked:", strings.Join(g.Reasons, "; "))
		}
	} else if plan.GildError != "" {
		fmt.Println("Gild redistribution preview unavailable:", plan.GildError)
	}
	if plan.Transcension != nil {
		p := plan.Transcension
		fmt.Printf("Transcension preview: recommendation=%s, Ancient Souls=%d; informational only\n", p.Recommendation, p.AncientSouls)
	} else if plan.TranscensionError != "" {
		fmt.Println("Transcension preview unavailable:", plan.TranscensionError)
	}
	fmt.Println("saved", path)
	return nil
}

// A decimal UI value may be rounded or truncated at its last displayed digit.
func ancientDisplayMatches(display, exact string) bool {
	shown, err := ancientcalc.Value(display)
	if err != nil {
		return false
	}
	wanted, err := ancientcalc.Value(exact)
	if err != nil {
		return false
	}
	if !strings.ContainsAny(display, "eE.") {
		return shown.Cmp(wanted) == 0
	}
	parts := strings.Split(strings.ToLower(display), "e")
	exponent := 0
	if len(parts) == 2 {
		exponent, err = strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
	}
	decimals := 0
	if i := strings.IndexByte(parts[0], '.'); i >= 0 {
		decimals = len(parts[0]) - i - 1
	}
	unit, err := ancientcalc.Value(fmt.Sprintf("1e%d", exponent-decimals))
	if err != nil {
		return false
	}
	diff := new(big.Float).SetPrec(256).Sub(wanted, shown)
	lower := new(big.Float).SetPrec(256).Quo(unit, big.NewFloat(2))
	lower.Neg(lower)
	// Rounding can raise the display by half a unit; truncation loses less than one.
	return diff.Cmp(lower) >= 0 && diff.Cmp(unit) < 0
}

type ancientStep uint8

const (
	visitAncients ancientStep = iota
	scrollAncients
	openAncientQuantity
	fillAncientQuantity
	confirmAncientQuantity
	returnAncientHeroes
)

type ancientCommand struct {
	step     ancientStep
	quantity string
}
type ancientPlanner struct {
	plan                               *ancientPlan
	active, started, finished, blocked bool
	budgetChecked, topChecked          bool
	quantityEntered, recovering        bool
	bottomChecked                      bool
	done                               map[int]bool
	latest                             ancientObservation
	pending                            *gameAction
	selected                           int
	quantity                           string
	failure, waiting                   string
	deadline, nextRead, nextAction     time.Time
	jobFrame                           uint64
}

// The native track excludes its arrow buttons; its thumb has a minimum height.
// A fraction of thumb height is not the same fraction of a page of cards.
func ancientScrollLimits(out ancientObservation) (int, int) {
	b := out.frame.context.bounds
	return b.Min.Y + b.Dy()*416/1000 + out.thumbHeight/2,
		b.Min.Y + b.Dy()*965/1000 - out.thumbHeight/2
}

func (p *ancientPlanner) interrupt() {
	if p.started && !p.finished && !p.blocked {
		p.fail("batch interrupted before completion")
	}
	p.active = false
	p.pending = nil
	p.latest = ancientObservation{}
	p.jobFrame = 0
}
func (step ancientStep) String() string {
	return [...]string{"visit Ancients", "scroll list", "open quantity", "type quantity", "click OK", "return to Heroes"}[step]
}
func (p *ancientPlanner) confirmationStatus() string {
	stage := "planning"
	if p.pending != nil {
		stage = p.pending.ancient.step.String()
	}
	name := "none"
	if p.plan != nil && p.selected >= 0 && p.selected < len(p.plan.Rows) {
		name = p.plan.Rows[p.selected].Name
	}
	return fmt.Sprintf("stage=%s, Ancient=%s, quantity=%q, dialog=%t, OK=%t, visible rows=%d", stage, name, p.quantity, p.latest.frame.context.ancientDialog, p.latest.okay, len(p.latest.rows))
}
func (p *ancientPlanner) pauseReason() string {
	return "Ancient batch blocked: " + p.failure + "; close the dialog, export a fresh save and restart before another batch"
}
func (p *ancientPlanner) fail(reason string) {
	if !p.blocked {
		p.failure = reason + " (" + p.confirmationStatus() + ")"
		fmt.Printf("Ancient purchases stopped: %s\n", p.failure)
	}
	p.blocked = true
	p.active = false
	p.pending = nil
}
func (p *ancientPlanner) observe(out ancientObservation, err error, now time.Time) {
	if !p.active {
		return
	}
	if err != nil {
		p.latest = ancientObservation{}
		p.nextRead = now.Add(time.Second)
		if p.deadline.IsZero() {
			p.deadline = now.Add(20 * time.Second)
		}
		fmt.Printf("Ancient panel unreadable: %v; retrying in 1s\n", err)
		return
	}
	if p.pending != nil && out.frame.id <= p.pending.frame.id {
		return
	}
	p.latest = out
	if p.pending == nil {
		if out.hasThumb || out.frame.context.ancientDialog {
			p.deadline = time.Time{}
		}
		return
	}
	a := p.pending
	defer func() {
		if p.pending == a {
			status := p.confirmationStatus()
			if status != p.waiting {
				fmt.Printf("Ancient confirmation pending: %s\n", status)
				p.waiting = status
			}
		}
	}()
	switch a.ancient.step {
	case visitAncients:
		if !out.frame.context.ancients {
			return
		}
	case scrollAncients:
		if !out.frame.context.ancients || !out.hasThumb {
			return
		}
		top, bottom := ancientScrollLimits(out)
		tolerance := max(2, out.frame.context.bounds.Dy()/1000)
		endPad := max(2, out.frame.context.bounds.Dy()*4/1000)
		move := out.thumb.Y - a.point.Y
		if a.target.Y < a.point.Y {
			if move > 0 || (absDiff(move, 0) < tolerance && !(a.target.Y <= top+tolerance && out.thumb.Y <= top+tolerance)) {
				return
			}
		} else if move < 0 || (move < tolerance && !(a.target.Y >= bottom-endPad && out.thumb.Y >= bottom-endPad)) {
			return
		}
		if a.target.Y >= bottom+endPad && out.thumb.Y >= bottom-endPad {
			p.bottomChecked = true
		}
	case openAncientQuantity:
		if !out.frame.context.ancientDialog {
			buy := p.plan.Rows[p.selected]
			for _, row := range out.rows {
				if row.name != buy.Name || ancientDisplayMatches(row.level, buy.Current) {
					continue
				}
				level, e1 := ancientcalc.Value(row.level)
				current, e2 := ancientcalc.Value(buy.Current)
				if e1 == nil && e2 == nil && level.Cmp(current) > 0 {
					p.fail(fmt.Sprintf("custom quantity dialog did not open, but %s level increased: saved=%q, read=%q; V modifier may not have registered", buy.Name, buy.Current, row.level))
					return
				}
			}
			return
		}
		if !out.okay {
			return
		}
	case fillAncientQuantity:
		if !out.frame.context.ancientDialog || !out.okay {
			return
		}
		p.quantityEntered = true
	case confirmAncientQuantity:
		c, owner := out.frame.context, a.frame.context
		if !owner.known || !owner.ancientDialog || !c.known || !c.ancients || c.ancientDialog || c.heroes || c.mercenaries || c.saveMenu || c.ascension || c.questDialog || c.modal != noGildModal || c.window == "!outside-game" || c.window != owner.window || c.bounds != owner.bounds || c.geometry != owner.geometry || out.frame.generation != a.frame.generation {
			return
		}
		if p.plan == nil || p.selected < 0 || p.selected >= len(p.plan.Rows) {
			p.fail("unowned quantity confirmation")
			return
		}
		// Dialog closure acknowledges submission; a silent failed buy can underbuy.
		p.done[p.selected] = true
		fmt.Printf("submitted Ancient %s quantity=%s; quantity dialog closed\n", p.plan.Rows[p.selected].Name, p.quantity)
		p.selected = -1
		p.quantity = ""
		p.quantityEntered = false
	case returnAncientHeroes:
		if !out.frame.context.heroes {
			return
		}
		p.active = false
		p.finished = true
		p.pending = nil
		return
	}
	p.pending = nil
	p.deadline = time.Time{}
}
func (p *ancientPlanner) action(frame gameFrame, now time.Time) (gameAction, bool) {
	if p.plan == nil || p.finished || p.blocked || p.pending != nil || now.Before(p.nextAction) {
		return gameAction{}, false
	}
	makeAction := func(step ancientStep, point image.Point) (gameAction, bool) {
		return gameAction{kind: handleAncient, frame: frame, point: point, ancient: ancientCommand{step, p.quantity}}, true
	}
	if !p.active {
		if !frame.context.heroes && !frame.context.ancients {
			return gameAction{}, false
		}
		p.active = true
		p.started = true
		p.done = map[int]bool{}
		p.selected = -1
		if frame.context.heroes {
			return makeAction(visitAncients, ancientTabPoint(frame.image, false))
		}
	}
	if p.latest.frame.id == 0 {
		return gameAction{}, false
	}
	frame = p.latest.frame
	if frame.context.ancientDialog {
		if p.selected < 0 || !p.latest.okay {
			p.fail("unowned quantity dialog")
			return gameAction{}, false
		}
		if !p.quantityEntered {
			return makeAction(fillAncientQuantity, controlRect(frame.image, image.Rect(640, 338, 641, 339)).Min)
		}
		point, found, err := ancientControl(frame.image, 1)
		if !found || err != nil {
			return gameAction{}, false
		}
		return makeAction(confirmAncientQuantity, point)
	}
	if !frame.context.ancients {
		return gameAction{}, false
	}
	if !p.budgetChecked {
		if !ancientDisplayMatches(p.latest.souls, p.plan.Souls) {
			p.fail(fmt.Sprintf("exported Hero Souls do not match the current game: saved=%q, read=%q", p.plan.Souls, p.latest.souls))
			return gameAction{}, false
		}
		p.budgetChecked = true
	}
	if !p.topChecked && !p.latest.hasThumb {
		// Short lists need no sweep when every remaining planned row is readable.
		visible := 0
		for i, buy := range p.plan.Rows {
			if p.done[i] {
				continue
			}
			for _, row := range p.latest.rows {
				if row.name == buy.Name {
					visible++
					break
				}
			}
		}
		if visible != len(p.plan.Rows)-len(p.done) {
			if p.deadline.IsZero() {
				p.deadline = now.Add(20 * time.Second)
			}
			p.nextAction = now.Add(300 * time.Millisecond)
			return gameAction{}, false
		}
		p.topChecked = true
	}
	if !p.topChecked && p.latest.hasThumb {
		top, _ := ancientScrollLimits(p.latest)
		if p.latest.thumb.Y > top+max(2, frame.context.bounds.Dy()/1000) {
			a, _ := makeAction(scrollAncients, p.latest.thumb)
			a.target = image.Pt(a.point.X, top)
			return a, true
		}
		p.topChecked = true
	}
	for i, buy := range p.plan.Rows {
		if p.done[i] {
			continue
		}
		for _, row := range p.latest.rows {
			if row.name != buy.Name {
				continue
			}
			if !ancientDisplayMatches(row.level, buy.Current) {
				p.fail("exported level differs for " + buy.Name)
				return gameAction{}, false
			}
			souls, e1 := ancientcalc.Value(p.latest.souls)
			reserve, e2 := ancientcalc.Value(p.plan.Reserve)
			cost, e3 := ancientcalc.Value(buy.Cost)
			if e1 != nil || e2 != nil || e3 != nil || souls.Cmp(new(big.Float).SetPrec(256).Add(reserve, cost)) < 0 {
				p.fail("insufficient observed Hero Souls for " + buy.Name)
				return gameAction{}, false
			}
			var err error
			p.quantity, err = ancientcalc.InputQuantity(buy.Quantity)
			if err != nil {
				p.fail(err.Error())
				return gameAction{}, false
			}
			p.selected = i
			p.quantityEntered = false
			return makeAction(openAncientQuantity, row.point)
		}
	}
	if len(p.done) == len(p.plan.Rows) {
		return makeAction(returnAncientHeroes, ancientTabPoint(frame.image, true))
	}
	if !p.latest.hasThumb {
		if p.deadline.IsZero() {
			p.deadline = now.Add(20 * time.Second)
		}
		p.nextAction = now.Add(300 * time.Millisecond)
		return gameAction{}, false
	}
	top, bottom := ancientScrollLimits(p.latest)
	tolerance := max(2, frame.context.bounds.Dy()/1000)
	var missing []string
	for i, row := range p.plan.Rows {
		if !p.done[i] {
			missing = append(missing, row.Name)
		}
	}
	if p.recovering && p.latest.thumb.Y <= top+tolerance {
		p.fail("planned Ancients still missing after recovery sweep: " + strings.Join(missing, ", "))
		return gameAction{}, false
	}
	if !p.recovering && p.bottomChecked {
		p.recovering = true
		fmt.Printf("Ancient list recovery for remaining rows: %s\n", strings.Join(missing, ", "))
	}
	a, _ := makeAction(scrollAncients, p.latest.thumb)
	// Quarter-thumb steps overlap expanded cards even with the native minimum thumb size.
	// Gold-run height can omit border pixels (4px in the native bottom fixture).
	// Request the endpoint with a small pad, then require a newer clamped frame.
	endPad := max(2, frame.context.bounds.Dy()*4/1000)
	a.target = image.Pt(a.point.X, min(bottom+endPad, a.point.Y+max(2, p.latest.thumbHeight/4)))
	if p.recovering {
		// One reverse sweep samples closer views; submitted rows stay excluded.
		a.target.Y = max(top, a.point.Y-max(2, p.latest.thumbHeight/8))
	}
	return a, true
}
func (p *ancientPlanner) sent(a gameAction, now time.Time) {
	fmt.Printf("Ancient action: %s at (%d, %d), quantity=%q\n", a.ancient.step, a.point.X, a.point.Y, a.ancient.quantity)
	p.waiting = ""
	p.pending = &a
	p.latest = ancientObservation{}
	p.jobFrame = 0
	p.nextRead = now.Add(200 * time.Millisecond)
	p.nextAction = p.nextRead
	p.deadline = now.Add(20 * time.Second)
}
func ancientActionStable(a gameAction, current gameFrame) bool {
	if a.frame.context != current.context {
		return false
	}
	if a.ancient.step == fillAncientQuantity || a.ancient.step == confirmAncientQuantity {
		if !current.context.ancientDialog || !ancientQuantityDialog(current.image) {
			return false
		}
		return true
	}
	if a.ancient.step == scrollAncients {
		thumb, _, found := listScrollbarThumb(current.image, 416)
		return found && absDiff(thumb.Y, a.point.Y) <= max(3, current.context.bounds.Dy()/100)
	}
	if a.ancient.step != openAncientQuantity {
		return true
	}
	r := ancientNameRegion(current.image, a.point).Intersect(current.image.Bounds())
	// Ignore animated art and hover effects; the text identifying the row must remain unchanged.
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			text := func(img image.Image) bool { red, g, b := rgb(img.At(x, y)); return red > 150 && b > 150 && g < 140 }
			if text(a.frame.image) != text(current.image) {
				return false
			}
		}
	}
	return true
}
func clickAncientCustom(ctx context.Context, input heroInput, p image.Point) (err error) {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer func() { err = errors.Join(err, input.keyToggle("v", "up")) }()
	if err = input.keyToggle("v", "down"); err != nil {
		return err
	}
	// Let the game process V before the mouse-down event, not just during the click.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return input.click(p)
	}
}
func fillAncientCustom(ctx context.Context, input heroInput, quantity string) (err error) {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	modifier := robotgo.CmdCtrl()
	defer func() { err = errors.Join(err, input.keyToggle(modifier, "up")) }()
	if err = input.keyToggle(modifier, "down"); err != nil {
		return err
	}
	if err = input.keyTap("a"); err != nil {
		return err
	}
	if err = input.keyToggle(modifier, "up"); err != nil {
		return err
	}
	return input.typeText(quantity)
}
