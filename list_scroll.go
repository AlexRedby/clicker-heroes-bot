package main

import (
	"context"
	"image"
	"time"
)

// Modes describe list navigation, without adding feature-specific input paths.
type listScrollMode uint8

const (
	listScrollEdge listScrollMode = iota
	listScrollPage
	listScrollRecord
	listScrollFine
)
const listScrollSettle = 350 * time.Millisecond

type listScrollCommand struct {
	mode           listScrollMode
	from, to, park image.Point
	direction      int
}

func listScrollAction(a gameAction) (listScrollCommand, bool) {
	s := listScrollCommand{from: a.point, to: a.target, park: parkPoint(a.frame.context.bounds)}
	switch {
	case a.kind == scrollHeroes:
		if a.hero.startup && a.hero.sweep.top && a.hero.startupScroll != (image.Point{}) {
			s.mode = listScrollPage
		}
	case a.kind == handleMercenary && (a.mercenary.step == scrollMercenariesTop || a.mercenary.step == scrollMercenariesBottom):
	case a.kind == handleAncient && a.ancient.step == scrollAncients:
		s.mode, s.direction = listScrollRecord, a.ancient.direction
		if a.ancient.fine {
			s.mode = listScrollFine
		}
	default:
		return s, false
	}
	return s, true
}

func scrollList(ctx context.Context, input heroInput, s listScrollCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch s.mode {
	case listScrollEdge, listScrollPage:
		return input.drag(s.from, s.to)
	case listScrollRecord:
		if err := input.scroll(s.from, s.direction); err != nil {
			return err
		}
	case listScrollFine:
		if err := input.click(s.from); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return input.move(s.park)
}
