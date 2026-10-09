package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"runtime/debug"
	"sync"
	"time"
)

type heroReaders struct {
	gold  func(context.Context, image.Image) (float64, error)
	price func(context.Context, image.Image, image.Point) (float64, error)
	level func(context.Context, image.Image, image.Point) (int, error)
}
type heroObservation struct {
	frame                                        gameFrame
	button, thumb, footer                        image.Point
	thumbFound, bottom, x1, found, owned, stable bool
	startup, passiveReady, startupComplete       bool
	sweep                                        startupSweep
	startupScroll                                image.Point
	level                                        int
	gold, nextPrice                              float64
}
type heroAttempt struct {
	action   gameAction
	afterAt  time.Time
	attempts int
	last     heroObservation
}
type heroDiagnostic struct {
	before, after heroObservation
	readError     error
}

type heroRunner struct {
	sweep          startupSweep
	enabled        bool
	failures       int
	scrollFailures int
	nextScan       time.Time
	latest         heroObservation
	pending        *heroAttempt
	quantityTaps   int
	parked         bool
	lastReadError  string
	onDiagnostic   func(heroDiagnostic) bool
}

// A reset starts a new bounded sweep. F8 preserves its viewport cursor.
func (p *heroRunner) startStartup() {
	p.interrupt()
	p.sweep = startupSweep{}
	p.scrollFailures = 0
	p.lastReadError = ""
}

func (p *heroRunner) due(now time.Time) bool {
	return p.enabled && !now.Before(p.nextScan) && !(p.pending != nil && p.pending.action.kind == buyHero && p.pending.attempts >= 5)
}
func (p *heroRunner) before() *heroObservation {
	if p.pending != nil && p.pending.action.kind == buyHero {
		v := p.pending.action.hero
		return &v
	}
	return nil
}
func (p *heroRunner) interrupt() {
	p.pending = nil
	p.latest = heroObservation{}
	p.quantityTaps = 0
	p.parked = false
	p.nextScan = time.Time{}
}
func (p *heroRunner) obstructed(now time.Time) {
	p.pending = nil
	p.latest = heroObservation{}
	p.nextScan = now.Add(5 * time.Second)
}
func (p *heroRunner) readFailed(err error, out heroObservation, now time.Time) {
	if p.pending != nil && p.pending.action.kind == buyHero {
		return
	}
	if message := err.Error(); message != p.lastReadError && (p.onDiagnostic == nil || p.onDiagnostic(heroDiagnostic{after: out, readError: err})) {
		p.lastReadError = message
	}
	fmt.Printf("hero numbers unreadable: %v; retrying in 30s\n", err)
	p.nextScan = now.Add(30 * time.Second)
	p.latest = heroObservation{}
}
func readHeroObservation(ctx context.Context, frame gameFrame, read heroReaders, before *heroObservation) (heroObservation, error) {
	out := heroObservation{frame: frame}
	if !frame.context.heroes || !heroQuantityBarPresent(frame.image) {
		return out, nil
	}
	out.thumb, _, out.thumbFound = heroScrollbarThumb(frame.image)
	out.bottom = out.thumbFound && heroScrollbarAtBottom(frame.image)
	out.x1 = heroQuantitySelected(frame.image, 122)
	if before != nil {
		out.button, out.found, out.owned = before.button, true, true
		out.stable = (heroViewportStable(*before, frame) || !before.owned && before.bottom) && heroRowNameMatches(before.frame.image, frame.image, before.button, before.button)
		if !out.stable {
			return out, nil
		}
		level, err := read.level(ctx, frame.image, before.button)
		out.level = level
		if err != nil {
			err = fmt.Errorf("hero level at %v: %w", before.button, err)
		}
		return out, err
	}
	if !out.thumbFound {
		var known bool
		var err error
		out.footer, known, _, err = readHeroUpgradeFooter(ctx, frame.image)
		if err != nil {
			return out, err
		}
		out.bottom = known
	}
	if !out.bottom || !out.x1 {
		return out, nil
	}
	out.button, out.found = findHeroLevelButton(frame.image)
	if !out.found || !heroCandidateKnown(frame.image, out.button) {
		out.found = false
		return out, nil
	}
	out.owned = heroRowHasLevel(frame.image, out.button.Y)
	if !out.owned {
		return out, nil
	}
	next, found := findNextHeroButton(frame.image, out.button)
	if !found {
		return out, nil
	}
	// Cropped reads share one frame; the global OCR semaphore bounds process concurrency.
	var wg sync.WaitGroup
	var goldErr, priceErr, levelErr error
	wg.Add(3)
	go func() { defer wg.Done(); out.gold, goldErr = read.gold(ctx, frame.image) }()
	go func() { defer wg.Done(); out.nextPrice, priceErr = read.price(ctx, frame.image, next) }()
	go func() { defer wg.Done(); out.level, levelErr = read.level(ctx, frame.image, out.button) }()
	wg.Wait()
	if goldErr != nil {
		goldErr = fmt.Errorf("hero gold: %w", goldErr)
	}
	if priceErr != nil {
		priceErr = fmt.Errorf("next hero price at %v: %w", next, priceErr)
	}
	if levelErr != nil {
		levelErr = fmt.Errorf("hero level at %v: %w", out.button, levelErr)
	}
	return out, errors.Join(goldErr, priceErr, levelErr)
}

// A recognized footer proves a short list has no viewport to scroll.
func heroViewportStable(before heroObservation, current gameFrame) bool {
	_, _, thumbFound := heroScrollbarThumb(current.image)
	return heroListStable(before.frame.image, current.image) ||
		!before.thumbFound && !thumbFound && before.bottom && before.footer != (image.Point{}) &&
			heroUpgradeButtonStable(before.frame.image, current.image, before.footer)
}

func (p *heroRunner) observe(out heroObservation, fish observation, now time.Time) {
	if !p.enabled {
		return
	}
	p.lastReadError = ""
	if out.startup {
		p.sweep = out.sweep
	}
	if p.pending != nil {
		pending := p.pending
		if out.frame.id <= pending.action.frame.id || out.frame.at.Before(pending.afterAt) {
			return
		}
		pending.last = out
		pending.attempts++
		switch pending.action.kind {
		case buyHero:
			if pending.action.hero.startup {
				p.pending = nil
				p.latest = out
				p.nextScan = now
				return
			}
			if out.stable && out.level > pending.action.hero.level {
				fmt.Printf("leveled hero at (%d, %d)\n", out.button.X, out.button.Y)
				p.failures = 0
				p.pending = nil
				p.latest = heroObservation{}
				p.nextScan = now
				return
			}
			if fish.found {
				p.obstructed(now)
				return
			}
			if pending.attempts >= 5 {
				p.finishFailure(fish, now)
				if p.pending != nil {
					p.nextScan = now.Add(150 * time.Millisecond)
				}
				return
			}
		case selectQuantity:
			if !out.x1 && p.quantityTaps >= 5 {
				p.pending = nil
				p.latest = heroObservation{}
				p.nextScan = now.Add(30 * time.Second)
				fmt.Println("x1 hero quantity not confirmed; retrying in 30s")
				return
			}
			p.pending = nil
		case scrollHeroes:
			moved := out.thumbFound && absDiff(out.thumb.Y, pending.action.point.Y) >= max(2, out.frame.context.bounds.Dy()/1000)
			if !out.bottom && !moved {
				// Give a busy game time to apply input, independently of scan cadence.
				if pending.attempts < 3 || now.Before(pending.afterAt.Add(time.Second)) {
					p.nextScan = now.Add(150 * time.Millisecond)
					return
				}
				p.scrollFailures++
				wait := 250 * time.Millisecond
				if p.scrollFailures >= 3 {
					wait = 2 * time.Second
				}
				p.pending = nil
				p.latest = heroObservation{}
				p.nextScan = now.Add(wait)
				fmt.Printf("hero scroll made no progress; reacquiring scrollbar in %s (attempt %d)\n", wait, p.scrollFailures)
				return
			}
			p.scrollFailures = 0
			p.pending = nil
			p.nextScan = now
		case parkPointer:
			p.pending = nil
		}
		if p.pending != nil {
			p.nextScan = now.Add(150 * time.Millisecond)
			return
		}
	}
	p.latest = out
	if out.found && out.owned && out.nextPrice > 0 && saveForNextHero(out.gold, out.nextPrice) {
		fmt.Println("saving gold for next hero")
		p.latest = heroObservation{}
		p.nextScan = now.Add(5 * time.Second)
	}
}
func (p *heroRunner) finishFailure(fish observation, now time.Time) {
	pending := p.pending
	if pending == nil || pending.action.kind != buyHero || pending.attempts < 5 {
		return
	}
	if fish.found {
		p.obstructed(now)
		return
	}
	p.failures++
	p.enabled = p.failures < 3
	p.pending = nil
	p.latest = heroObservation{}
	p.nextScan = now.Add(30 * time.Second)
	if p.onDiagnostic != nil {
		p.onDiagnostic(heroDiagnostic{before: pending.action.hero, after: pending.last})
	}
	suffix := "retrying hero purchases in 30s"
	if !p.enabled {
		suffix = "hero purchases stopped after three failures"
	}
	fmt.Printf("hero level change not confirmed at (%d, %d) (list stable=%t, level increased=false); %s\n", pending.action.point.X, pending.action.point.Y, pending.last.stable, suffix)
}
func (p *heroRunner) action(now time.Time) (gameAction, bool) {
	if !p.due(now) || p.pending != nil || p.latest.frame.id == 0 {
		return gameAction{}, false
	}
	o := p.latest
	a := gameAction{frame: o.frame, hero: o}
	switch {
	case o.startup && !o.x1 && heroQuantityBarPresent(o.frame.image):
		a.kind = selectQuantity
	case o.startup && o.startupScroll != (image.Point{}):
		a.kind, a.point, a.target = scrollHeroes, o.thumb, o.startupScroll
	case o.startup && (o.startupComplete || !o.found):
		p.nextScan = now.Add(time.Second)
		p.latest = heroObservation{}
		return a, false
	case !o.startup && !o.thumbFound && !o.bottom:
		if p.parked {
			fmt.Println("hero scrollbar not recognized; retrying in 30s")
			if o.frame.image != nil {
				const path = "artifacts/hero-scrollbar-unrecognized.png"
				if err := saveImage(path, o.frame.image); err != nil {
					fmt.Printf("save hero scrollbar screenshot: %v\n", err)
				} else {
					fmt.Printf("saved hero scrollbar screenshot: %s\n", path)
				}
			}
			p.nextScan = now.Add(30 * time.Second)
			p.latest = heroObservation{}
			return a, false
		}
		a.kind = parkPointer
		a.point = parkPoint(o.frame.context.bounds)
	case !o.startup && !o.bottom:
		a.kind = scrollHeroes
		a.point = o.thumb
		a.target = image.Pt(o.thumb.X, o.frame.context.bounds.Max.Y-1)
	case !o.x1:
		a.kind = selectQuantity
	case o.found:
		a.kind = buyHero
		a.point = o.button
	default:
		p.nextScan = now.Add(30 * time.Second)
		p.latest = heroObservation{}
		return a, false
	}
	return a, true
}
func (p *heroRunner) sent(a gameAction, now time.Time) {
	if a.kind == scrollHeroes && a.hero.startup {
		// The next viewport must be selected with a fresh cursor. Resetting
		// after analysis can carry a skipped row or completion into the next step.
		p.sweep.y, p.sweep.attempts = 0, 0
	}
	if a.kind == buyHero && a.hero.startup {
		p.sweep = a.hero.sweep
		p.sweep.attempts++
	}
	p.latest = heroObservation{}
	p.pending = &heroAttempt{action: a, afterAt: now.Add(200 * time.Millisecond)}
	p.nextScan = now.Add(200 * time.Millisecond)
	if a.kind == scrollHeroes {
		p.pending.afterAt = now.Add(listScrollSettle)
		p.nextScan = p.pending.afterAt
	}
	if a.kind == selectQuantity {
		p.quantityTaps++
		p.pending.afterAt = now.Add(100 * time.Millisecond)
		p.nextScan = p.pending.afterAt
	}
	if a.kind == parkPointer {
		p.parked = true
	}
}

func clickHeroMax(ctx context.Context, input heroInput, button image.Point) error {
	return clickHeroModified(ctx, input, button, "q")
}

func clickHeroModified(ctx context.Context, input heroInput, button image.Point, key string) (err error) {
	if key == "" {
		if err = ctx.Err(); err != nil {
			return err
		}
		return input.click(button)
	}
	return withHeldKey(ctx, input, key, 0, func() error { return input.click(button) })
}
func saveForNextHero(gold, nextPrice float64) bool { return nextPrice-gold <= 1 }
func saveHeroFailure(before, after heroObservation) {
	if before.frame.image == nil || after.frame.image == nil {
		return
	}
	stamp := time.Now().Format("20060102-150405.000")
	beforePath := fmt.Sprintf("artifacts/hero-failure-%s-before.png", stamp)
	afterPath := fmt.Sprintf("artifacts/hero-failure-%s-after.png", stamp)
	marked := image.NewRGBA(before.frame.image.Bounds())
	draw.Draw(marked, marked.Bounds(), before.frame.image, marked.Bounds().Min, draw.Src)
	for offset := -max(12, marked.Bounds().Dx()/100); offset <= max(12, marked.Bounds().Dx()/100); offset++ {
		marked.Set(before.button.X+offset, before.button.Y, color.RGBA{R: 255, A: 255})
		marked.Set(before.button.X, before.button.Y+offset, color.RGBA{R: 255, A: 255})
	}
	if err := saveImage(beforePath, marked); err != nil {
		fmt.Printf("failed to save hero screenshot: %v\n", err)
	} else if err := saveImage(afterPath, after.frame.image); err != nil {
		fmt.Printf("failed to save hero screenshot: %v\n", err)
	} else {
		fmt.Printf("saved hero failure screenshots: %s, %s\n", beforePath, afterPath)
	}
}

func saveHeroReadFailure(frame gameFrame, err error) {
	if frame.image == nil {
		return
	}
	path := fmt.Sprintf("artifacts/hero-unreadable-%s.png", time.Now().Format("20060102-150405.000"))
	if saveErr := saveImage(path, frame.image); saveErr != nil {
		fmt.Printf("failed to save hero unreadable screenshot: %v\n", saveErr)
	} else {
		fmt.Printf("saved hero unreadable screenshot: %s (frame=%d, build=%s, error=%v)\n", path, frame.id, botBuildRevision(), err)
	}
}

func botBuildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	revision, modified := "unknown", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if modified {
		revision += " (modified)"
	}
	return revision
}
