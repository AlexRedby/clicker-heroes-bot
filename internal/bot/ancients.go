package bot

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
	"clicker-heroes-bot/internal/vision"

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
	gilds, err := ancientcalc.CalculateGilds(ctx, save, reserve)
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

// Decimal plan amounts are subtracted exactly; row costs already include wallet loss.
func ancientBudgetValue(value string) (*big.Rat, error) {
	if _, err := ancientcalc.Value(value); err != nil {
		return nil, err
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid Ancient budget amount %q", value)
	}
	return n, nil
}

func (p *ancientPlanner) remainingSouls() (*big.Rat, error) {
	remaining, err := ancientBudgetValue(p.plan.Souls)
	if err != nil {
		return nil, err
	}
	for i, row := range p.plan.Rows {
		if !p.done[i] {
			continue
		}
		cost, err := ancientBudgetValue(row.Cost)
		if err != nil {
			return nil, err
		}
		remaining.Sub(remaining, cost)
	}
	if remaining.Sign() < 0 {
		return nil, fmt.Errorf("submitted Ancient costs exceed exported Hero Souls")
	}
	return remaining, nil
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
	step      ancientStep
	quantity  string
	direction int
	fine      bool
	before    []ancientNameAnchor
}
type ancientPlanner struct {
	plan                                                                               *ancientPlan
	active, started, finished, blocked                                                 bool
	quantityEntered, needFullRead                                                      bool
	seekArrows                                                                         bool
	seekTarget, seekDirection, seekClicks, seekStalls, seekTurns, seekLocal, seekReads int
	seekReadFrame                                                                      uint64
	seekNames                                                                          []ancientNameAnchor
	done                                                                               map[int]bool
	latest                                                                             ancientObservation
	pending                                                                            *gameAction
	selected                                                                           int
	quantity                                                                           string
	failure, waiting                                                                   string
	deadline, nextRead, nextAction                                                     time.Time
	jobFrame                                                                           uint64
}

// The installed game orders owned Ancients alphabetically (confirmed by the user).
// Accept only fresh-save canonical names; arbitrary OCR strings cannot set direction.
func (p *ancientPlanner) canonicalAnchors(out ancientObservation) ([]ancientNameAnchor, bool) {
	anchors := out.anchors
	if len(anchors) == 0 {
		for _, row := range out.rows {
			anchors = append(anchors, ancientNameAnchor{row.name, row.point.Y})
		}
	}
	var known []ancientNameAnchor
	for _, anchor := range anchors {
		name := ""
		if p.plan != nil {
			for _, owned := range p.plan.Owned {
				if strings.EqualFold(anchor.name, owned.Name) {
					name = owned.Name
					break
				}
			}
			if name == "" {
				for _, buy := range p.plan.Rows {
					if strings.EqualFold(anchor.name, buy.Name) {
						name = buy.Name
						break
					}
				}
			}
		}
		if name == "" {
			continue
		}
		valid := len(known) == 0 || anchor.y > known[len(known)-1].y && strings.ToLower(name) > strings.ToLower(known[len(known)-1].name)
		known = append(known, ancientNameAnchor{name, anchor.y})
		if !valid {
			return known, false
		}
	}
	return known, len(known) > 0
}
func (p *ancientPlanner) beginSeek(index int) {
	if p.seekTarget == index {
		return
	}
	p.seekTarget = index
	p.seekDirection, p.seekClicks, p.seekStalls, p.seekTurns, p.seekLocal, p.seekReads = 0, 0, 0, 0, 0, 0
	p.seekReadFrame = 0
	p.seekArrows = false
}
func (p *ancientPlanner) seekStatus() string {
	name := "none"
	if p.plan != nil && p.seekTarget >= 0 && p.seekTarget < len(p.plan.Rows) {
		name = p.plan.Rows[p.seekTarget].Name
	}
	neighbors := make([]string, 0, len(p.seekNames))
	for _, anchor := range p.seekNames {
		neighbors = append(neighbors, fmt.Sprintf("%s@%d", anchor.name, anchor.y))
	}
	direction := "none"
	if p.seekDirection < 0 {
		direction = "up"
	}
	if p.seekDirection > 0 {
		direction = "down"
	}
	return fmt.Sprintf("target=%s, direction=%s, neighbors=%s, inputs=%d, no-motion=%d, fine=%d, turns=%d", name, direction, strings.Join(neighbors, ","), p.seekClicks, p.seekStalls, p.seekLocal, p.seekTurns)
}
func (p *ancientPlanner) retrySeek(now time.Time, full bool) {
	if p.latest.frame.id != p.seekReadFrame {
		p.seekReads++
		p.seekReadFrame = p.latest.frame.id
	}
	p.needFullRead = full
	p.latest = ancientObservation{}
	p.nextRead, p.nextAction = now.Add(300*time.Millisecond), now.Add(300*time.Millisecond)
	if p.deadline.IsZero() {
		p.deadline = now.Add(20 * time.Second)
	}
}

func ancientRowReadable(rows []ancientScreenRow, name string) bool {
	for _, row := range rows {
		if strings.EqualFold(row.name, name) {
			_, err := ancientcalc.Value(row.level)
			return err == nil
		}
	}
	return false
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
	return fmt.Sprintf("stage=%s, Ancient=%s, quantity=%q, dialog=%t, OK=%t, visible rows=%d; %s", stage, name, p.quantity, p.latest.frame.context.ancientDialog, p.latest.okay, len(p.latest.rows), p.seekStatus())
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
	fullPanel := !out.namesOnly && out.frame.context.ancients && !out.frame.context.ancientDialog
	if fullPanel {
		p.needFullRead = false
	}
	if p.pending == nil {
		if fullPanel || out.frame.context.ancientDialog {
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
		if !out.frame.context.known || !out.frame.context.ancients || out.frame.context.ancientDialog {
			return
		}
		anchors, valid := p.canonicalAnchors(out)
		if !valid {
			return
		}
		p.seekNames = anchors
		moved := false
		for _, before := range a.ancient.before {
			for _, after := range anchors {
				if before.name == after.name && a.ancient.direction*(before.y-after.y) >= max(2, out.frame.context.bounds.Dy()/1000) {
					moved = true
				}
			}
		}
		if len(a.ancient.before) > 0 && len(anchors) > 0 && a.ancient.direction*strings.Compare(strings.ToLower(anchors[0].name), strings.ToLower(a.ancient.before[0].name)) > 0 {
			moved = true
		}
		if moved {
			p.seekStalls = 0
		} else {
			p.seekStalls++
		}
		if !a.ancient.fine && p.seekStalls >= 2 {
			fmt.Printf("Ancient seek switching wheel -> arrow after repeated no-motion: %s\n", p.seekStatus())
			p.seekArrows = true
			p.seekStalls = 0
		} else if p.seekStalls >= 3 {
			p.fail("Ancient navigation made no progress: " + p.seekStatus())
			return
		}
	case openAncientQuantity:
		if !out.frame.context.ancientDialog {
			buy := p.plan.Rows[p.selected]
			for _, row := range out.rows {
				if !strings.EqualFold(row.name, buy.Name) || ancientDisplayMatches(row.level, buy.Current) {
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
		p.needFullRead = true
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
		return gameAction{kind: handleAncient, frame: frame, point: point, ancient: ancientCommand{step: step, quantity: p.quantity}}, true
	}
	if len(p.plan.Rows) == 0 && !p.active {
		if frame.context.heroes && bootstrapHeroes(frame.context) {
			p.finished = true
			fmt.Println("Ancient plan has no purchases; continuing on Heroes")
			return gameAction{}, false
		}
		if frame.context.ancients && frame.context.known && !frame.context.ancientDialog && !frame.context.saveMenu && frame.context.modal == noGildModal {
			p.active, p.started = true, true
			return makeAction(returnAncientHeroes, ancientTabPoint(frame.image, true))
		}
		return gameAction{}, false
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
			return makeAction(fillAncientQuantity, vision.Rect(frame.image, image.Rect(640, 338, 641, 339)).Min)
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
	if len(p.done) == len(p.plan.Rows) {
		return makeAction(returnAncientHeroes, ancientTabPoint(frame.image, true))
	}
	target := -1
	for i, buy := range p.plan.Rows {
		if !p.done[i] && (target < 0 || strings.ToLower(buy.Name) < strings.ToLower(p.plan.Rows[target].Name)) {
			target = i
		}
	}
	p.beginSeek(target)
	anchors, valid := p.canonicalAnchors(p.latest)
	p.seekNames = anchors
	if !valid {
		if p.seekReads >= 3 {
			p.fail("unreadable or nonalphabetical Ancient neighbors: " + p.seekStatus())
			return gameAction{}, false
		}
		p.retrySeek(now, false)
		return gameAction{}, false
	}
	// Read numbers only when a remaining name has a complete button and level crop.
	for _, point := range ancientButtons(frame.image) {
		region := ancientNameRegion(frame.image, point)
		for _, anchor := range anchors {
			if anchor.y < region.Min.Y || anchor.y >= region.Max.Y {
				continue
			}
			for i, buy := range p.plan.Rows {
				if !p.done[i] && anchor.name == buy.Name && (p.latest.namesOnly || p.seekReads < 3 && !ancientRowReadable(p.latest.rows, buy.Name)) {
					p.retrySeek(now, true)
					return gameAction{}, false
				}
			}
		}
	}
	for i, buy := range p.plan.Rows {
		if p.done[i] {
			continue
		}
		for _, row := range p.latest.rows {
			if !strings.EqualFold(row.name, buy.Name) {
				continue
			}
			if _, err := ancientcalc.Value(row.level); err != nil {
				continue
			}
			if !ancientDisplayMatches(row.level, buy.Current) {
				p.fail(fmt.Sprintf("exported level differs for %s: saved=%q, read=%q", buy.Name, buy.Current, row.level))
				return gameAction{}, false
			}
			remaining, e1 := p.remainingSouls()
			reserve, e2 := ancientBudgetValue(p.plan.Reserve)
			cost, e3 := ancientBudgetValue(buy.Cost)
			if e1 != nil || e2 != nil || e3 != nil {
				p.fail(fmt.Sprintf("invalid planned Hero Souls budget for %s: remaining=%v, reserve=%v, cost=%v", buy.Name, e1, e2, e3))
				return gameAction{}, false
			}
			if remaining.Cmp(new(big.Rat).Add(reserve, cost)) < 0 {
				p.fail(fmt.Sprintf("insufficient planned Hero Souls for %s: remaining=%s, cost=%q, reserve=%q", buy.Name, remaining.RatString(), buy.Cost, p.plan.Reserve))
				return gameAction{}, false
			}

			var err error
			p.quantity, err = ancientcalc.InputQuantity(buy.Quantity)
			if err != nil {
				p.fail(err.Error())
				return gameAction{}, false
			}
			p.beginSeek(i)
			p.selected = i
			p.quantityEntered = false
			return makeAction(openAncientQuantity, row.point)
		}
	}
	name := strings.ToLower(p.plan.Rows[target].Name)
	direction, fine := 1, false
	if name < strings.ToLower(anchors[0].name) {
		direction = -1
	} else if name <= strings.ToLower(anchors[len(anchors)-1].name) {
		fine = true
	}
	if p.seekArrows || p.seekTurns > 0 || p.seekDirection != 0 && p.seekDirection != direction {
		fine = true
	}
	if fine {
		direction = 1
		onScreen := false
		for _, anchor := range anchors {
			if strings.EqualFold(anchor.name, p.plan.Rows[target].Name) {
				onScreen = true
				if anchor.y < frame.context.bounds.Min.Y+frame.context.bounds.Dy()/2 {
					direction = -1
				} else if anchor.y > frame.context.bounds.Min.Y+frame.context.bounds.Dy()*3/4 {
					direction = 1
				} else if p.seekLocal%4 >= 2 {
					direction = -1
				}
				break
			}
		}
		if name < strings.ToLower(anchors[0].name) {
			direction = -1
		} else if name > strings.ToLower(anchors[len(anchors)-1].name) {
			direction = 1
		} else if !onScreen && p.seekLocal%4 >= 2 {
			direction = -1
		}
	}
	limit := 64
	if p.seekArrows {
		limit = 128
	}
	if p.seekClicks >= limit || p.seekLocal >= 24 || p.seekTurns >= 4 {
		p.fail("Ancient navigation limit reached: " + p.seekStatus())
		return gameAction{}, false
	}
	point := vision.Rect(frame.image, image.Rect(350, 510, 351, 511)).Min
	if fine {
		var found bool
		var err error
		point, found, err = ancientScrollArrow(frame.image, direction)
		if err != nil || !found {
			if p.seekReads >= 6 {
				p.fail("Ancient fine-scroll control unreadable: " + p.seekStatus())
				return gameAction{}, false
			}
			p.retrySeek(now, false)
			return gameAction{}, false
		}
	}
	a, _ := makeAction(scrollAncients, point)
	a.ancient.direction, a.ancient.fine, a.ancient.before = direction, fine, append([]ancientNameAnchor(nil), anchors...)
	return a, true
}
func (p *ancientPlanner) sent(a gameAction, now time.Time) {
	fmt.Printf("Ancient action: %s at (%d, %d), quantity=%q\n", a.ancient.step, a.point.X, a.point.Y, a.ancient.quantity)
	p.waiting = ""
	if a.ancient.step == scrollAncients {
		p.seekClicks++
		name := strings.ToLower(p.plan.Rows[p.seekTarget].Name)
		// Arrow transit toward a distant target has its own total-input bound;
		// reserve the fine limit for corrections within the readable name range.
		if a.ancient.fine && (!p.seekArrows || len(a.ancient.before) > 0 && name >= strings.ToLower(a.ancient.before[0].name) && name <= strings.ToLower(a.ancient.before[len(a.ancient.before)-1].name)) {
			p.seekLocal++
		}
		if p.seekDirection != 0 && p.seekDirection != a.ancient.direction {
			p.seekTurns++
		}
		p.seekDirection = a.ancient.direction
		p.seekReads = 0
		p.needFullRead = false
		fmt.Printf("Ancient seek: %s, fine=%t\n", p.seekStatus(), a.ancient.fine)
	}
	p.pending = &a
	p.latest = ancientObservation{}
	p.jobFrame = 0
	p.nextRead = now.Add(200 * time.Millisecond)
	if _, scrolling := listScrollAction(a); scrolling {
		p.nextRead = now.Add(listScrollSettle)
	}
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
		if !current.context.known || !current.context.ancients || current.context.ancientDialog || (a.ancient.direction != -1 && a.ancient.direction != 1) {
			return false
		}
		if !a.ancient.fine {
			return a.point == vision.Rect(current.image, image.Rect(350, 510, 351, 511)).Min
		}
		point, found, err := ancientScrollArrow(current.image, a.ancient.direction)
		return found && err == nil && point == a.point
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
	// Let the game process V before the mouse-down event, not just during the click.
	return withHeldKey(ctx, input, "v", 100*time.Millisecond, func() error { return input.click(p) })
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
