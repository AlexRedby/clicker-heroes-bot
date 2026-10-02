package main

import (
	"bytes"
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
	SaveHash  string    `json:"saveHash"`
	CreatedAt time.Time `json:"createdAt"`
	savePath  string
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
	done                               map[int]bool
	latest                             ancientObservation
	pending                            *gameAction
	selected                           int
	quantity                           string
	failure, waiting                   string
	target                             string
	deadline, nextRead, nextAction     time.Time
	jobFrame                           uint64
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
	return fmt.Sprintf("stage=%s, Ancient=%s, expected quantity=%q, read quantity=%q, dialog=%t, OK=%t, visible rows=%d", stage, name, p.quantity, p.latest.quantity, p.latest.frame.context.ancientDialog, p.latest.okay, len(p.latest.rows))
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
		p.fail(err.Error())
		return
	}
	if p.pending != nil && out.frame.id <= p.pending.frame.id {
		return
	}
	p.latest = out
	if p.pending == nil {
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
		if absDiff(out.thumb.Y, a.point.Y) < max(2, out.frame.context.bounds.Dy()/1000) {
			return
		}
	case openAncientQuantity:
		if !out.frame.context.ancientDialog || !out.okay {
			return
		}
	case fillAncientQuantity:
		if !out.frame.context.ancientDialog || !out.okay || !ancientQuantityMatches(out.quantity, p.quantity) {
			return
		}
	case confirmAncientQuantity:
		if !out.frame.context.ancients {
			return
		}
		name := p.plan.Rows[p.selected].Name
		for _, row := range out.rows {
			if row.name == name && ancientDisplayMatches(row.level, p.target) {
				p.done[p.selected] = true
				fmt.Printf("leveled Ancient %s by %s\n", name, p.quantity)
				p.selected = -1
				p.quantity = ""
				p.target = ""
				p.pending = nil
				p.deadline = time.Time{}
				return
			}
		}
		return
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
		if p.latest.quantity == "" || !ancientQuantityMatches(p.latest.quantity, p.quantity) {
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
	if !p.topChecked {
		if p.latest.hasThumb && p.latest.thumb.Y-p.latest.thumbHeight/2 > frame.context.bounds.Min.Y+frame.context.bounds.Dy()*425/1000 {
			a, _ := makeAction(scrollAncients, p.latest.thumb)
			a.target = image.Pt(a.point.X, frame.context.bounds.Min.Y+frame.context.bounds.Dy()*40/100)
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
			current, _ := new(big.Rat).SetString(buy.Current)
			delta, _ := new(big.Rat).SetString(p.quantity)
			target := new(big.Rat).Add(current, delta)
			p.target = target.Num().Quo(target.Num(), target.Denom()).String()
			if ancientDisplayMatches(row.level, p.target) {
				p.fail("purchase too small to verify visibly for " + buy.Name)
				return gameAction{}, false
			}
			p.selected = i
			return makeAction(openAncientQuantity, row.point)
		}
	}
	if len(p.done) == len(p.plan.Rows) {
		return makeAction(returnAncientHeroes, ancientTabPoint(frame.image, true))
	}
	if !p.latest.hasThumb || p.latest.thumb.Y+p.latest.thumbHeight/2 >= frame.context.bounds.Min.Y+frame.context.bounds.Dy()*96/100 {
		p.fail("not all planned Ancients were found in the expanded list")
		return gameAction{}, false
	}
	a, _ := makeAction(scrollAncients, p.latest.thumb)
	a.target = image.Pt(a.point.X, a.point.Y+max(20, p.latest.thumbHeight*3/4))
	if a.target.Y+p.latest.thumbHeight/2 >= frame.context.bounds.Min.Y+frame.context.bounds.Dy()*94/100 {
		a.target.Y = frame.context.bounds.Max.Y - 1
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
		if a.ancient.step == confirmAncientQuantity {
			before, after := ancientQuantityMask(a.frame.image), ancientQuantityMask(current.image)
			if !bytes.Equal(before.Pix, after.Pix) {
				return false
			}
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
	return input.click(p)
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

func ancientQuantityMatches(got, want string) bool {
	if _, err := ancientcalc.Value(got); err != nil {
		return false
	}
	a, ok := new(big.Rat).SetString(got)
	if !ok {
		return false
	}
	b, ok := new(big.Rat).SetString(want)
	return ok && a.Cmp(b) == 0
}
