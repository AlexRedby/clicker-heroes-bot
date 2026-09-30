package main

import (
	"image"
	"image/color"
)

func findHeroLevelButton(screen image.Image) (image.Point, bool) {
	if screen == nil {
		return image.Point{}, false
	}
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 640 || h < 360 {
		return image.Point{}, false
	}
	if !heroTabSelected(screen) {
		return image.Point{}, false
	}
	xStart, xEnd := bounds.Min.X+w*55/1000, bounds.Min.X+w*130/1000
	yStart, yEnd := bounds.Min.Y+h/5, bounds.Min.Y+h*94/100
	xStep, gap := max(1, w/500), max(3, h/150)
	var best image.Point
	found := false
	start, last := -1, -1
	for y := yStart; y <= yEnd+gap; y++ {
		blue, samples := 0, 0
		if y < yEnd {
			for x := xStart; x < xEnd; x += xStep {
				r, g, b := rgb(screen.At(x, y))
				if b > 150 && b > r+40 && b >= g-10 && g > 90 {
					blue++
				}
				samples++
			}
		}
		if samples > 0 && blue*10 >= samples*3 {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start >= 0 && y-last > gap {
			center := (start + last) / 2
			if last-start > h/40 && heroRowYellow(screen, center) {
				best = image.Pt(bounds.Min.X+w*8/100, center)
				found = true
			}
			start = -1
		}
	}
	return best, found
}

func findNextHeroButton(screen image.Image, current image.Point) (image.Point, bool) {
	if screen == nil || !heroTabSelected(screen) {
		return image.Point{}, false
	}
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	xStart, xEnd := bounds.Min.X+w*55/1000, bounds.Min.X+w*130/1000
	yStart, yEnd := current.Y+h/15, bounds.Min.Y+h*94/100
	xStep, gap := max(1, w/500), max(3, h/150)
	start, last := -1, -1
	for y := yStart; y <= yEnd+gap; y++ {
		dark, samples := 0, 0
		if y < yEnd {
			for x := xStart; x < xEnd; x += xStep {
				r, g, b := rgb(screen.At(x, y))
				if r < 150 && g < 150 && b < 150 {
					dark++
				}
				samples++
			}
		}
		if samples > 0 && dark*10 >= samples*7 {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start >= 0 && y-last > gap {
			center := (start + last) / 2
			if last-start > h/40 && heroRowYellow(screen, center) {
				return image.Pt(bounds.Min.X+w*8/100, center), true
			}
			start = -1
		}
	}
	return image.Point{}, false
}

func heroQuantityBarPresent(screen image.Image) bool {
	if screen == nil || !heroTabSelected(screen) {
		return false
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	for _, x := range []int{100, 410} {
		r, g, blue := rgb(screen.At(b.Min.X+w*x/1000, b.Min.Y+h*345/1000))
		if r < 180 || g < 100 || blue > 100 {
			return false
		}
	}
	return true
}

func heroRowHasLevel(screen image.Image, y int) bool {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*26/100, y-h*3/100, b.Min.X+w*36/100, y+h*3/100).Intersect(b)
	white, total := 0, 0
	for row := region.Min.Y; row < region.Max.Y; row += max(1, h/600) {
		for x := region.Min.X; x < region.Max.X; x += max(1, w/600) {
			r, g, blue := rgb(screen.At(x, row))
			if min(r, g, blue) > 180 && max(r, g, blue)-min(r, g, blue) < 55 {
				white++
			}
			total++
		}
	}
	return total > 0 && white*100 > total*3
}

func heroTabSelected(screen image.Image) bool {
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	tab := image.Rect(bounds.Min.X+w*45/1000, bounds.Min.Y+h*15/100, bounds.Min.X+w*70/1000, bounds.Min.Y+h*21/100)
	gold, samples := 0, 0
	for y := tab.Min.Y; y < tab.Max.Y; y++ {
		for x := tab.Min.X; x < tab.Max.X; x++ {
			r, g, b := rgb(screen.At(x, y))
			if r >= 160 && g >= 100 && g <= 145 && b <= 60 {
				gold++
			}
			samples++
		}
	}
	return samples > 0 && gold*4 >= samples
}

func heroRowYellow(screen image.Image, y int) bool {
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	yStart := max(bounds.Min.Y, y-h*15/1000)
	yEnd := min(bounds.Max.Y, y+h*15/1000)
	yellow, samples := 0, 0
	for x := bounds.Min.X + w*17/100; x < bounds.Min.X+w*35/100; x += max(1, w/200) {
		for row := yStart; row < yEnd; row += max(1, h/100) {
			r, g, b := rgb(screen.At(x, row))
			if r > 150 && g > 110 && r > b+40 && g > b+20 {
				yellow++
			}
			samples++
		}
	}
	return samples > 0 && yellow*10 >= samples*4
}

func heroListMoved(before, after image.Image) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	beforeThumb, beforeHeight, beforeFound := heroScrollbarThumb(before)
	afterThumb, afterHeight, afterFound := heroScrollbarThumb(after)
	return beforeFound && afterFound && absDiff(beforeThumb.Y-beforeHeight/2, afterThumb.Y-afterHeight/2) > max(3, before.Bounds().Dy()/100)
}

func heroScrollbarThumb(screen image.Image) (image.Point, int, bool) {
	if screen == nil || !heroTabSelected(screen) {
		return image.Point{}, 0, false
	}
	bounds := screen.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	xStart, xEnd := bounds.Min.X+w*445/1000, bounds.Min.X+w*495/1000
	yStart, yEnd := bounds.Min.Y+h*32/100, bounds.Min.Y+h*965/1000
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

func rgb(c color.Color) (int, int, int) {
	r, g, b, _ := c.RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

func heroQuantitySelected(screen image.Image, xPercent int) bool {
	if !heroQuantityBarPresent(screen) {
		return false
	}
	b := screen.Bounds()
	r, g, blue := rgb(screen.At(b.Min.X+b.Dx()*(xPercent-25)/1000, b.Min.Y+b.Dy()*345/1000))
	return r > 180 && g > 90 && g < 190 && blue < 100 && r-g > 50
}

func heroScrollbarAtBottom(screen image.Image) bool {
	thumb, height, found := heroScrollbarThumb(screen)
	if !found {
		return false
	}
	b := screen.Bounds()
	return absDiff(thumb.Y+height/2, b.Min.Y+b.Dy()*965/1000) <= max(3, b.Dy()/100)
}

func heroListStable(before, after image.Image) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	_, _, a := heroScrollbarThumb(before)
	_, _, b := heroScrollbarThumb(after)
	return a && b && !heroListMoved(before, after)
}

func heroRowUnowned(screen image.Image, y int) bool {
	if screen == nil || heroRowHasLevel(screen, y) {
		return false
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	region := image.Rect(b.Min.X+w*28/100, y-h/100, b.Min.X+w*36/100, y+h/100).Intersect(b)
	yellow, total := 0, 0
	for row := region.Min.Y; row < region.Max.Y; row++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, blue := rgb(screen.At(x, row))
			if r > 200 && g > 160 && blue < 140 {
				yellow++
			}
			total++
		}
	}
	return total > 0 && yellow*100 >= total*98
}

func heroCandidateKnown(screen image.Image, button image.Point) bool {
	if !heroScrollbarAtBottom(screen) {
		return false
	}
	if heroRowUnowned(screen, button.Y) {
		return true
	}
	if !heroRowHasLevel(screen, button.Y) {
		return false
	}
	next, found := findNextHeroButton(screen, button)
	// A missing successor can be a clipped or obscured row. Wait for a clear frame.
	return found && heroRowUnowned(screen, next.Y)
}

func sameHeroRow(before, after image.Image, old, current image.Point) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() || !heroListStable(before, after) {
		return false
	}
	b := before.Bounds()
	w, h := b.Dx(), b.Dy()
	if absDiff(old.X, current.X) > max(2, w/200) || absDiff(old.Y, current.Y) > max(2, h/200) {
		return false
	}
	// The name is static across quantity changes and leveling; animated artwork is excluded.
	region := image.Rect(b.Min.X+w*28/100, old.Y-h*5/100, b.Min.X+w*365/1000, old.Y-h*3/100).Intersect(b)
	changed, total := 0, 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			ar, ag, ab := rgb(before.At(x, y))
			br, bg, bb := rgb(after.At(x, y+current.Y-old.Y))
			a := min(ar, ag, ab) > 180
			z := min(br, bg, bb) > 180
			if a != z {
				changed++
			}
			total++
		}
	}
	return total > 0 && changed*100 <= total
}

func heroLayoutValid(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 || !heroQuantityBarPresent(screen) {
		return false
	}
	_, _, found := heroScrollbarThumb(screen)
	return found && (heroQuantitySelected(screen, 122) || heroQuantitySelected(screen, 435) || heroQuantitySelected(screen, 200) || heroQuantitySelected(screen, 278) || heroQuantitySelected(screen, 356))
}
