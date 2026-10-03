package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"
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
	found, err := matchControl(frame.image, image.Rect(1197, 368, 1257, 409), "ui/autoclicker-pool.png")
	if err != nil || !found {
		return autoClickerPool{}, err
	}
	raw, err := readGameText(ctx, frame.image, autoClickerCountRegion(frame.image), max(2, 4096/frame.image.Bounds().Dx()), 7, 180, "0123456789/")
	if err != nil {
		return autoClickerPool{}, err
	}
	return parseAutoClickerPool(raw), nil
}

func autoClickerCountRegion(screen image.Image) image.Rectangle {
	return controlRect(screen, image.Rect(1200, 410, 1255, 433))
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
	blocked         bool
}

func (p *autoClickerPlanner) command(frame gameFrame, pool autoClickerPool, target autoClickerTarget, point image.Point) (autoClickerCommand, bool) {
	a := autoClickerCommand{frame: frame, pool: pool, target: target, point: point}
	if p.pending != nil || p.blocked || !pool.known || pool.available <= 0 || !autoClickerTargetValid(a) {
		return a, false
	}
	if target == autoClickerUpgrades && (p.footerAttempted || pool.total == 1) {
		return a, false
	}
	// Keep one free for upgrades when more than one clicker is owned.
	if target == autoClickerMonster && pool.total > 1 && !p.footerAttempted && pool.available == 1 {
		return a, false
	}
	return a, true
}

func (p *autoClickerPlanner) sent(a autoClickerCommand, now time.Time) {
	if p.pending != nil || p.blocked {
		return
	}
	p.pending = &a
	p.afterAt = now.Add(200 * time.Millisecond)
	p.deadline = now.Add(5 * time.Second)
}

func (p *autoClickerPlanner) observe(frame gameFrame, pool autoClickerPool, now time.Time) {
	if p.pending == nil || p.blocked || frame.id <= p.pending.frame.id || frame.at.Before(p.afterAt) {
		return
	}
	before := p.pending
	if !bootstrapHeroes(frame.context) || frame.context.bounds != before.frame.context.bounds ||
		frame.context.window != before.frame.context.window || frame.context.geometry != before.frame.context.geometry {
		if now.After(p.deadline) {
			p.unconfirmed()
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
		p.unconfirmed()
	}
}

func (p *autoClickerPlanner) unconfirmed() {
	if p.pending.target == autoClickerUpgrades {
		// An occupied footer is a native no-op. Never retry it; plain clicks remain available.
		p.footerAttempted = true
		p.pending = nil
		fmt.Println("Auto Clicker footer placement unconfirmed; retaining ordinary upgrade clicks")
		return
	}
	p.blocked = true
	fmt.Println("Auto Clicker monster placement unconfirmed; further placements disabled")
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
	// Counts are static outlined text: require the entire white glyph mask unchanged.
	r := autoClickerCountRegion(before)
	count := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			white := func(s image.Image) bool {
				r, g, b := rgb(s.At(x, y))
				return min(r, g, b) > 180 && max(r, g, b)-min(r, g, b) < 55
			}
			if white(before) != white(after) {
				return false
			}
			if white(before) {
				count++
			}
		}
	}
	return count > 0
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
	defer func() { err = errors.Join(err, input.keyToggle("c", "up")) }()
	if err = input.keyToggle("c", "down"); err != nil {
		return err
	}
	// Like the native V transaction, let the client sample the held key before clicking.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return input.click(a.point)
	}
}
