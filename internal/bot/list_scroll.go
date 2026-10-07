package bot

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

func listScrollbarThumb(screen image.Image, top int) (image.Point, int, bool) {
	if screen == nil {
		return image.Point{}, 0, false
	}
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	xStart, xEnd := bounds.Min.X+w*445/1000, bounds.Min.X+w*495/1000
	yStart, yEnd := bounds.Min.Y+h*top/1000, bounds.Min.Y+h*965/1000
	bestScore, bestY, bestHeight, bestX := 0, 0, 0, 0
	span := func(x, y int) (int, int) {
		left, right := x, x
		for left > xStart {
			r, g, b := rgb(screen.At(left-1, y))
			if r <= 190 || g <= 145 || b >= 160 {
				break
			}
			left--
		}
		for right+1 < xEnd {
			r, g, b := rgb(screen.At(right+1, y))
			if r <= 190 || g <= 145 || b >= 160 {
				break
			}
			right++
		}
		return left, right
	}
	for x := xStart; x < xEnd; x += max(1, w/1000) {
		type run struct{ start, end, count int }
		var runs []run
		gold := 0
		for y := yStart; y < yEnd; y++ {
			r, g, b := rgb(screen.At(x, y))
			if r <= 190 || g <= 145 || b >= 110 {
				continue
			}
			gold++
			if len(runs) == 0 || y-runs[len(runs)-1].end > max(2, h/60) {
				runs = append(runs, run{start: y, end: y, count: 1})
			} else {
				runs[len(runs)-1].end = y
				runs[len(runs)-1].count++
			}
		}
		for _, candidate := range runs {
			height := candidate.end - candidate.start + 1
			if height <= h*55/1000 || height >= h*18/100 {
				continue
			}
			score := 2*candidate.count - gold
			if score <= bestScore {
				continue
			}
			centerY := (candidate.start + candidate.end) / 2
			left, right := span(x, centerY)
			width := right - left + 1
			if width < w*8/1000 || width > w*25/1000 || left == xStart || right == xEnd-1 {
				continue
			}
			// A thumb has parallel sides; an orange fish can contain a tall gold run.
			rectangular := true
			for _, y := range []int{centerY - height*3/10, centerY + height*3/10} {
				l, r := span(x, y)
				if absDiff(l, left) > max(2, w/1000) || absDiff(r, right) > max(2, w/1000) {
					rectangular = false
					break
				}
			}
			if rectangular {
				bestScore, bestX, bestY, bestHeight = score, (left+right)/2, centerY, height
			}
		}
	}
	if bestScore <= 0 {
		return image.Point{}, 0, false
	}
	return image.Pt(bestX, bestY), bestHeight, true
}

func mercenaryScrollbar(screen image.Image) (point image.Point, found, top, bottom bool) {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	x := b.Min.X + w*458/1000
	trackTop, trackBottom := b.Min.Y+h*388/1000, b.Min.Y+h*965/1000
	first, last, bestFirst, bestLast := -1, -1, -1, -1
	for y := trackTop; y <= trackBottom+max(4, h/100); y++ {
		bright := false
		if y < trackBottom {
			r, g, blue := rgb(screen.At(x, y))
			bright = r > 225 && g > 190 && blue < 100
		}
		if bright {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 && y-last > max(4, h/100) {
			if last-first > bestLast-bestFirst {
				bestFirst, bestLast = first, last
			}
			first = -1
		}
	}
	if bestLast-bestFirst < h/30 {
		return image.Point{}, false, false, false
	}
	return image.Pt(x, (bestFirst+bestLast)/2), true, bestFirst-trackTop < h/100, trackBottom-bestLast < h/100
}
