package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"math"
	"regexp"
	"strings"
	"time"
)

//go:embed assets/ascension-controls.png
var ascensionControlsPNG []byte
var ascensionControlsImage decodedPNG
var ascensionRewardLabel = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?(?:[eE][0-9]+)?)\s+Hero Souls?$`)

const (
	ascensionSpiral = iota
	ascensionTitle
	ascensionYes
	ascensionNo
)

var ascensionRewardRegion = image.Rect(675, 307, 887, 329)

func ascensionControl(screen image.Image, control int) (image.Point, bool, error) {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return image.Point{}, false, nil
	}
	regions := [...]image.Rectangle{
		image.Rect(1232, 240, 1255, 268), // Right-hand World Ascension spiral.
		image.Rect(502, 108, 778, 130),   // Specific Ascension dialog title.
		image.Rect(485, 468, 625, 530),   // Yes; Quick Ascension is a different button below it.
		image.Rect(655, 468, 796, 530),   // No.
	}
	references := [...]image.Rectangle{
		image.Rect(0, 0, 46, 56), image.Rect(0, 56, 552, 100),
		image.Rect(0, 100, 280, 224), image.Rect(0, 224, 282, 348),
	}
	atlas, err := ascensionControlsImage.get(ascensionControlsPNG)
	if err != nil {
		return image.Point{}, false, err
	}
	found, err := matchControl(screen, regions[control], atlas, references[control])
	r := controlRect(screen, regions[control])
	return r.Min.Add(r.Size().Div(2)), found, err
}

func ascensionDialog(screen image.Image) (bool, error) {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false, nil
	}
	// Avoid template work on the ordinary HUD; these points are blank dialog corners.
	for _, point := range []image.Point{{370, 140}, {910, 140}} {
		p := controlRect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
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
)

type ascensionObservation struct {
	frame       gameFrame
	confirm, no bool
	souls       float64
	zone        int
}

func readAscensionObservation(ctx context.Context, frame gameFrame) (ascensionObservation, error) {
	out := ascensionObservation{frame: frame}
	if frame.context.ascension {
		_, out.confirm, _ = ascensionControl(frame.image, ascensionYes)
		_, out.no, _ = ascensionControl(frame.image, ascensionNo)
		if !out.confirm || !out.no {
			return out, fmt.Errorf("Ascension confirmation controls not recognized")
		}
		raw, err := readGameText(ctx, frame.image, controlRect(frame.image, ascensionRewardRegion), max(1, 2048/frame.image.Bounds().Dx()), 7, 0, "0123456789.eEabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
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

type ascensionPlanner struct {
	highestZone, wallZone          int
	lastProgress, lastObservation  time.Time
	nextCheck                      time.Time
	active                         bool
	step                           ascensionStep
	lastInputFrame, jobFrame       uint64
	nextRead, nextAction, deadline time.Time
	latest                         ascensionObservation
}

func (p *ascensionPlanner) interrupt() { *p = ascensionPlanner{} }

func (p *ascensionPlanner) observeProgress(s progressionState, wall int, now time.Time) {
	if p.active || !s.Known || s.Zone <= 0 {
		return
	}
	if p.lastObservation.IsZero() || now.Sub(p.lastObservation) > 10*time.Second || s.Zone < p.highestZone-1 {
		p.highestZone, p.wallZone = s.Zone, 0
		p.lastProgress = now
	}
	p.lastObservation = now
	if s.Zone > p.highestZone {
		p.highestZone = s.Zone
		p.lastProgress = now
	}
	p.wallZone = 0
	// Require an observed boss and its recognized fallback, not a disabled toggle alone.
	if !s.Enabled && wall > 0 && p.highestZone >= wall && s.Zone >= wall-1 && s.Zone <= wall {
		p.wallZone = wall
	}
}

func (p *ascensionPlanner) due(now time.Time, stall time.Duration) bool {
	return !p.active && p.wallZone > 0 && now.Sub(p.lastObservation) <= 10*time.Second &&
		!now.Before(p.lastProgress.Add(stall)) && !now.Before(p.nextCheck)
}

func (p *ascensionPlanner) observe(out ascensionObservation, err error, now time.Time) (reset bool) {
	if !p.active || out.frame.id <= p.lastInputFrame {
		return false
	}
	p.latest = out
	if (p.step == openAscension || p.step == confirmAscension) && out.frame.context.ascension {
		if err != nil || math.IsInf(out.souls, -1) {
			reason := "no Hero Souls gained"
			if err != nil {
				reason = err.Error()
			}
			fmt.Printf("Ascension skipped: %s\n", reason)
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
		*p = ascensionPlanner{nextCheck: now.Add(5 * time.Minute)}
	}
	return false
}

func (p *ascensionPlanner) sent(step ascensionStep, frameID uint64, now time.Time) {
	p.active = true
	p.lastInputFrame = frameID
	p.latest = ascensionObservation{}
	p.nextAction, p.nextRead = now.Add(500*time.Millisecond), now.Add(500*time.Millisecond)
	if p.deadline.IsZero() {
		p.deadline = now.Add(20 * time.Second)
	}
	if step == confirmAscension {
		p.step = waitAscensionReset
	} else {
		p.step = step
	}
}

func ascensionActionStable(a gameAction, current gameFrame) bool {
	if a.frame.image == nil || current.image == nil || a.frame.context != current.context {
		return false
	}
	control := ascensionSpiral
	if a.ascension == confirmAscension {
		control = ascensionYes
	} else if a.ascension == cancelAscension {
		control = ascensionNo
	}
	point, found, err := ascensionControl(current.image, control)
	if err != nil || !found || point != a.point {
		return false
	}
	if a.ascension != confirmAscension {
		return true
	}
	// The pending reward must still be visible, unchanged since its single OCR read.
	r := controlRect(current.image, ascensionRewardRegion)
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
