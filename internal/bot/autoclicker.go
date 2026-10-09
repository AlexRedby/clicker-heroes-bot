package bot

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"clicker-heroes-bot/internal/vision"
)

type autoClickerPool struct {
	known            bool
	available, total int
}

type autoClickerTarget uint8

const (
	autoClickerMonster autoClickerTarget = iota
	autoClickerUpgrades
)

type autoClickerCommand struct {
	frame  gameFrame
	pool   autoClickerPool
	target autoClickerTarget
	point  image.Point
}

// The fixture establishes this owned pool skin. Unknown skins remain unreadable.
func readAutoClickerPool(ctx context.Context, frame gameFrame) (autoClickerPool, error) {
	if frame.image == nil || !bootstrapHeroes(frame.context) {
		return autoClickerPool{}, nil
	}
	found, err := vision.MatchControl(frame.image, image.Rect(1197, 368, 1257, 409), "ui/autoclicker-pool.png")
	if err != nil || !found {
		return autoClickerPool{}, err
	}
	raw, err := readGameText(ctx, frame.image, autoClickerCountRegion(frame.image), max(2, 4096/frame.image.Bounds().Dx()), 7, -180, "0123456789/")
	if err != nil {
		return autoClickerPool{}, err
	}
	return parseAutoClickerPool(raw), nil
}

func autoClickerCountRegion(screen image.Image) image.Rectangle {
	return vision.Rect(screen, image.Rect(1200, 410, 1255, 433))
}

func parseAutoClickerPool(raw string) autoClickerPool {
	parts := strings.Split(strings.Join(strings.Fields(raw), ""), "/")
	if len(parts) != 2 {
		return autoClickerPool{}
	}
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return autoClickerPool{}
		}
	}
	available, aerr := strconv.Atoi(parts[0])
	total, terr := strconv.Atoi(parts[1])
	if aerr != nil || terr != nil || available > total {
		return autoClickerPool{}
	}
	return autoClickerPool{known: true, available: available, total: total}
}

// A submitted placement survives F8. An ambiguous result is never replayed.
// The caller records sent before dispatch and creates a new planner only after a confirmed reset.
type autoClickerPlanner struct {
	pending         *autoClickerCommand
	afterAt         time.Time
	deadline        time.Time
	upgrades        bool
	footerAttempted bool
	footerPasses    uint8
	blocked         bool // An uncertain monster placement blocks only monster replay.
	lastPool        autoClickerPool
	lastPoolFrame   gameFrame
}

func (p *autoClickerPlanner) command(frame gameFrame, pool autoClickerPool, target autoClickerTarget, point image.Point) (autoClickerCommand, bool) {
	a := autoClickerCommand{frame: frame, pool: pool, target: target, point: point}
	if p.pending != nil || p.blocked && target == autoClickerMonster || !pool.known || pool.available <= 0 || !autoClickerTargetValid(a) {
		return a, false
	}
	if target == autoClickerUpgrades && (p.footerAttempted || p.footerPasses > 0 || pool.total == 1) {
		return a, false
	}
	// Keep one free for upgrades when more than one clicker is owned.
	if target == autoClickerMonster && pool.total > 1 && !p.footerAttempted && p.footerPasses == 0 && pool.available == 1 {
		return a, false
	}
	return a, true
}

func (p *autoClickerPlanner) sent(a autoClickerCommand, now time.Time) {
	if p.pending != nil || p.blocked && a.target == autoClickerMonster {
		return
	}
	p.pending = &a
	p.afterAt = now.Add(200 * time.Millisecond)
	p.deadline = now.Add(5 * time.Second)
}

func (p *autoClickerPlanner) observe(frame gameFrame, pool autoClickerPool, now time.Time) {
	p.rememberPool(frame, pool)
	if p.pending == nil || frame.id <= p.pending.frame.id || frame.at.Before(p.afterAt) {
		return
	}
	before := p.pending
	if !bootstrapHeroes(frame.context) || frame.context.bounds != before.frame.context.bounds ||
		frame.context.window != before.frame.context.window || frame.context.geometry != before.frame.context.geometry {
		if now.After(p.deadline) {
			p.unconfirmed(frame, pool)
		}
		return
	}
	if pool.known && pool.total == before.pool.total && pool.available == before.pool.available-1 {
		p.upgrades = p.upgrades || before.target == autoClickerUpgrades
		p.footerAttempted = p.footerAttempted || before.target == autoClickerUpgrades
		p.pending = nil
		return
	}
	if now.After(p.deadline) || pool.known && (pool.total != before.pool.total || pool.available != before.pool.available) {
		p.unconfirmed(frame, pool)
	}
}

func (p *autoClickerPlanner) rememberPool(frame gameFrame, pool autoClickerPool) {
	if !pool.known || frame.image == nil || frame.id <= p.lastPoolFrame.id {
		return
	}
	p.lastPool, p.lastPoolFrame = pool, frame
}

// recover clears placement failures after a periodic fresh OCR pass. The pool
// must be recognized in a newer valid Heroes frame. A submitted placement
// always wins over recovery, so an F8 interruption cannot replay or discard
// input.
func (p *autoClickerPlanner) recover(frame gameFrame, pool autoClickerPool) bool {
	if p.pending != nil || !pool.known || frame.image == nil || frame.id <= p.lastPoolFrame.id ||
		!bootstrapHeroes(frame.context) {
		return false
	}
	p.rememberPool(frame, pool)
	changed := p.blocked || p.footerAttempted || p.footerPasses != 0
	p.blocked = false
	p.footerAttempted = false
	p.footerPasses = 0
	return changed
}

// noteFooterUnavailable bounds the startup footer probe. It deliberately does
// not invent a footer target when recognition is absent; after one pass the
// final free clicker may be used on the monster.
func (p *autoClickerPlanner) noteFooterUnavailable() {
	if p.footerPasses == 0 {
		p.footerPasses = 1
	}
}

func (p *autoClickerPlanner) unconfirmed(frame gameFrame, pool autoClickerPool) {
	if p.pending == nil {
		return
	}
	target := p.pending.target
	name, read := "monster", "unknown"
	if target == autoClickerUpgrades {
		name = "upgrade footer"
	}
	if pool.known {
		read = fmt.Sprintf("%d/%d", pool.available, pool.total)
	}
	fmt.Printf("Auto Clicker placement unconfirmed: target=%s, frame=%d, before=%d/%d, expected=%d/%d, read=%s\n", name, frame.id,
		p.pending.pool.available, p.pending.pool.total, p.pending.pool.available-1, p.pending.pool.total, read)
	before := p.pending.frame.context
	if frame.image != nil && bootstrapHeroes(frame.context) && frame.context.window == before.window &&
		frame.context.bounds == before.bounds && frame.context.geometry == before.geometry {
		path := fmt.Sprintf("artifacts/auto-clicker-unconfirmed-%s.png", frame.at.Format("20060102-150405.000"))
		if err := saveImage(path, frame.image); err != nil {
			fmt.Printf("failed to save Auto Clicker failure frame: %v\n", err)
		} else {
			fmt.Printf("saved Auto Clicker failure frame: %s\n", path)
		}
	}
	p.pending = nil
	if target == autoClickerUpgrades {
		// An occupied footer is a native no-op. Retry only during a later recovery cycle.
		p.footerAttempted = true
		p.pending = nil
		fmt.Println("Auto Clicker footer placement unconfirmed; retaining ordinary upgrade clicks")
		return
	}
	p.blocked = true
	fmt.Println("Auto Clicker monster placement unconfirmed; retrying at the next periodic check")
}

func (p *autoClickerPlanner) interrupt() {
	// No submitted input is forgotten, including one interrupted before acknowledgement.
}

func autoClickerTargetValid(a autoClickerCommand) bool {
	if a.frame.image == nil || !bootstrapHeroes(a.frame.context) || !a.point.In(a.frame.context.bounds) {
		return false
	}
	switch a.target {
	case autoClickerMonster:
		b := a.frame.context.bounds
		return a.point == b.Min.Add(image.Pt(b.Dx()*3/4, b.Dy()/2))
	case autoClickerUpgrades:
		// Only a caller's positively OCR-identified footer may enter the queue.
		b := a.frame.context.bounds
		return a.point.X == b.Min.X+b.Dx()*34/100 && a.point.Y >= b.Min.Y+b.Dy()*4/10 && a.point.Y <= b.Min.Y+b.Dy()*98/100 && heroQuantityBarPresent(a.frame.image)
	}
	return false
}

func autoClickerCommandStable(a autoClickerCommand, current gameFrame) bool {
	if !a.pool.known || a.pool.available <= 0 || !autoClickerTargetValid(a) || !bootstrapHeroes(current.context) ||
		current.generation != a.frame.generation || current.layout != a.frame.layout || current.id < a.frame.id ||
		current.context != a.frame.context || current.image == nil || current.image.Bounds() != a.frame.image.Bounds() {
		return false
	}
	return autoClickerPoolStable(a.frame.image, current.image) && (a.target != autoClickerUpgrades || heroUpgradeButtonStable(a.frame.image, current.image, a.point))
}

// Reconcile pool OCR with a newer shared footer/capture frame without another OCR call.
func autoClickerPoolStable(before, after image.Image) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	// Use the OCR mask here too; animated light scenery is not part of the count.
	r := autoClickerCountRegion(before)
	a, err := gameTextMask(before, r, -180)
	if err != nil {
		return false
	}
	b, err := gameTextMask(after, r, -180)
	return err == nil && bytes.IndexByte(a.Pix, 255) >= 0 && bytes.Equal(a.Pix, b.Pix)
}

// Official 6144: footer mouse-up (39010) uses C to call Add...Button (28424),
// which is a no-op when occupied; monster C calls Add...Monster (28426).
// C on the pool instead calls DetachAll (28407), so that point is never a target.
func placeAutoClicker(ctx context.Context, input heroInput, a autoClickerCommand) (err error) {
	if !autoClickerTargetValid(a) || !a.pool.known || a.pool.available <= 0 {
		return fmt.Errorf("invalid owned Auto Clicker placement")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Like the native V transaction, let the client sample the held key before clicking.
	return withHeldKey(ctx, input, "c", 200*time.Millisecond, func() error { return input.click(a.point) })
}
