package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-vgo/robotgo"
	"image"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type ancientLevel struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Level string `json:"level"`
}
type ancientPurchase struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Current  string `json:"current"`
	Target   string `json:"target"`
	Quantity string `json:"quantity"`
	Cost     string `json:"cost"`
}
type ancientPlan struct {
	Souls      string            `json:"souls"`
	Invested   string            `json:"invested,omitempty"`
	Reserve    string            `json:"reserve"`
	Spent      string            `json:"spent"`
	Remaining  string            `json:"remaining"`
	Ascensions int               `json:"ascensions"`
	Owned      []ancientLevel    `json:"owned"`
	Rows       []ancientPurchase `json:"rows"`
	SaveHash   string            `json:"saveHash"`
	CreatedAt  time.Time         `json:"createdAt"`
	savePath   string
}

var ancientDecimal = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,5})?$`)

// Plans retain Decimal's original strings. Big floats only validate the budget;
// the hero OCR log10 representation cannot retain integer purchase quantities.
func ancientValue(s string) (*big.Float, error) {
	if len(s) > 10000 || !ancientDecimal.MatchString(s) {
		return nil, errors.New("invalid Ancient quantity")
	}
	n, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil || n.IsInf() || n.Sign() < 0 {
		return nil, errors.New("invalid Ancient quantity")
	}
	return n, nil
}
func (p ancientPlan) validate() error {
	if p.Ascensions < 0 || len(p.Owned) == 0 || len(p.Owned) > 100 || len(p.Rows) > len(p.Owned) {
		return errors.New("invalid Ancient plan roster")
	}
	totals := make([]*big.Float, 4)
	for i, s := range []string{p.Souls, p.Reserve, p.Spent, p.Remaining} {
		v, err := ancientValue(s)
		if err != nil {
			return err
		}
		totals[i] = v
	}
	if p.Invested != "" {
		if _, err := ancientValue(p.Invested); err != nil {
			return err
		}
	}
	if totals[2].Cmp(totals[0]) > 0 || totals[3].Cmp(totals[1]) < 0 {
		return errors.New("Ancient plan exceeds soul budget or reserve")
	}
	owned := map[int]ancientLevel{}
	for _, a := range p.Owned {
		if a.ID <= 0 || a.Name == "" || len(a.Name) > 60 || strings.ContainsAny(a.Name, "\r\n") {
			return errors.New("invalid Ancient identity")
		}
		level, err := ancientValue(a.Level)
		if err != nil || level.Sign() <= 0 {
			return errors.New("invalid owned Ancient level")
		}
		if _, exists := owned[a.ID]; exists {
			return errors.New("duplicate Ancient identity")
		}
		owned[a.ID] = a
	}
	seen := map[int]bool{}
	for _, a := range p.Rows {
		old, ok := owned[a.ID]
		if !ok || old.Name != a.Name || old.Level != a.Current || seen[a.ID] {
			return errors.New("Ancient plan targets an unknown or duplicate Ancient")
		}
		seen[a.ID] = true
		current, err := ancientValue(a.Current)
		if err != nil {
			return err
		}
		target, err := ancientValue(a.Target)
		if err != nil {
			return err
		}
		quantity, err := ancientValue(a.Quantity)
		if err != nil {
			return err
		}
		cost, err := ancientValue(a.Cost)
		if err != nil {
			return err
		}
		if quantity.Sign() <= 0 || cost.Sign() < 0 || target.Cmp(current) <= 0 {
			return errors.New("Ancient purchase must increase an owned level")
		}
	}
	return nil
}

func calculateAncients(ctx context.Context, savePath, reserve string, skillRate float64, beyond8k bool) (ancientPlan, error) {
	var plan ancientPlan
	if _, err := ancientValue(strings.TrimSuffix(reserve, "%")); err != nil {
		return plan, fmt.Errorf("soul reserve: %w", err)
	}
	if !(skillRate >= 0 && skillRate <= 1) {
		return plan, errors.New("-ancient-skill-rate must be between 0 and 1")
	}
	info, err := os.Stat(savePath)
	if err != nil {
		return plan, fmt.Errorf("read exported save: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 4<<20 {
		return plan, errors.New("exported save must be a nonempty regular file no larger than 4 MiB")
	}
	save, err := os.ReadFile(savePath)
	if err != nil {
		return plan, fmt.Errorf("read exported save: %w", err)
	}
	request, err := json.Marshal(struct {
		Save      string  `json:"save"`
		Reserve   string  `json:"reserve"`
		SkillRate float64 `json:"skillRate"`
		Beyond8k  bool    `json:"beyond8k"`
	}{string(save), reserve, skillRate, beyond8k})
	if err != nil {
		return plan, err
	}
	script := filepath.Join("tools", "ancients", "plan.cjs")
	if _, err := os.Stat(script); err != nil {
		return plan, errors.New("run from the project directory containing tools/ancients/plan.cjs")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Stdin = bytes.NewReader(request)
	var stderr cappedBuffer
	stderr.n = 2048
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return plan, fmt.Errorf("Ancient calculator: %w", ctx.Err())
		}
		if strings.Contains(stderr.b.String(), "Cannot find module") {
			return plan, errors.New("Ancient calculator dependencies missing; run npm ci --prefix tools/ancients")
		}
		return plan, fmt.Errorf("Ancient calculator: %w: %s", err, strings.TrimSpace(stderr.b.String()))
	}
	if len(output) > 1<<20 {
		return plan, errors.New("Ancient calculator output exceeds 1 MiB")
	}
	if err := json.Unmarshal(output, &plan); err != nil {
		return plan, fmt.Errorf("decode Ancient plan: %w", err)
	}
	if err := plan.validate(); err != nil {
		return plan, err
	}
	sum := sha256.Sum256(save)
	plan.SaveHash = hex.EncodeToString(sum[:])
	plan.CreatedAt = time.Now().UTC()
	plan.savePath, err = filepath.Abs(savePath)
	if err != nil {
		return plan, err
	}
	return plan, nil
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

// A scientific UI value denotes a rounding interval, not an exact saved level.
func ancientDisplayMatches(display, exact string) bool {
	shown, err := ancientValue(display)
	if err != nil {
		return false
	}
	wanted, err := ancientValue(exact)
	if err != nil {
		return false
	}
	if !strings.ContainsAny(display, "eE.") {
		return shown.Cmp(wanted) == 0
	}
	parts := strings.Split(strings.ToLower(display), "e")
	exponent := 0
	if len(parts) == 2 {
		if _, err := fmt.Sscan(parts[1], &exponent); err != nil {
			return false
		}
	}
	decimals := 0
	if i := strings.IndexByte(parts[0], '.'); i >= 0 {
		decimals = len(parts[0]) - i - 1
	}
	tolerance, err := ancientValue(fmt.Sprintf("5e%d", exponent-decimals-1))
	if err != nil {
		return false
	}
	diff := new(big.Float).SetPrec(256).Sub(shown, wanted)
	diff.Abs(diff)
	return diff.Cmp(tolerance) <= 0
}

// A short, rounded-down quantity fits the visible text field and never increases
// calculator spending. The omitted digits are below the game's displayed precision.
func ancientInputQuantity(quantity string) (string, error) {
	if _, err := ancientValue(quantity); err != nil {
		return "", err
	}
	rat, ok := new(big.Rat).SetString(quantity)
	if !ok {
		return "", errors.New("invalid Ancient purchase quantity")
	}
	integer := new(big.Int).Quo(rat.Num(), rat.Denom())
	if integer.Sign() <= 0 {
		return "", errors.New("Ancient quantity is below one level")
	}
	digits := integer.String()
	if len(digits) <= 15 {
		return digits, nil
	}
	return digits[:1] + "." + digits[1:15] + fmt.Sprint("e", len(digits)-1), nil
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
	target                             string
	deadline, nextRead, nextAction     time.Time
	jobFrame                           uint64
}

func (p *ancientPlanner) interrupt() {
	if p.started && !p.finished {
		p.blocked = true
	}
	p.active = false
	p.pending = nil
	p.latest = ancientObservation{}
	p.jobFrame = 0
}
func (p *ancientPlanner) fail(reason string) {
	if !p.blocked {
		fmt.Printf("Ancient purchases paused: %s; close any dialog, export a fresh save and restart before another batch\n", reason)
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
			p.fail("exported Hero Souls do not match the current game")
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
			souls, e1 := ancientValue(p.latest.souls)
			reserve, e2 := ancientValue(p.plan.Reserve)
			cost, e3 := ancientValue(buy.Cost)
			if e1 != nil || e2 != nil || e3 != nil || souls.Cmp(new(big.Float).SetPrec(256).Add(reserve, cost)) < 0 {
				p.fail("insufficient observed Hero Souls for " + buy.Name)
				return gameAction{}, false
			}
			var err error
			p.quantity, err = ancientInputQuantity(buy.Quantity)
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
			r := controlRect(current.image, image.Rect(508, 325, 772, 350))
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					text := func(img image.Image) bool { red, g, b := rgb(img.At(x, y)); return max(red, g, b) < 140 }
					if text(a.frame.image) != text(current.image) {
						return false
					}
				}
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
	if _, err := ancientValue(got); err != nil {
		return false
	}
	a, ok := new(big.Rat).SetString(got)
	if !ok {
		return false
	}
	b, ok := new(big.Rat).SetString(want)
	return ok && a.Cmp(b) == 0
}
