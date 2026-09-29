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

func heroLevelChanged(before, after image.Image, button image.Point) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	bounds := before.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if !heroRowYellow(after, button.Y) {
		return false
	}
	region := image.Rect(bounds.Min.X+w*27/100, button.Y-h*4/100, bounds.Min.X+w*36/100, button.Y+h*2/100).Intersect(bounds)
	changed, total := 0, 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			ar, ag, ab := rgb(before.At(x, y))
			br, bg, bb := rgb(after.At(x, y))
			if max(absDiff(ar, br), absDiff(ag, bg), absDiff(ab, bb)) > 20 {
				changed++
			}
			total++
		}
	}
	return total > 0 && changed*200 > total
}

func heroListMoved(before, after image.Image) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() {
		return false
	}
	bounds := before.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	changed, total := 0, 0
	for y := bounds.Min.Y + h*40/100; y < bounds.Min.Y+h*90/100; y += max(1, h/250) {
		for x := bounds.Min.X + w*17/100; x < bounds.Min.X+w*28/100; x += max(1, w/400) {
			ar, ag, ab := rgb(before.At(x, y))
			br, bg, bb := rgb(after.At(x, y))
			if max(absDiff(ar, br), absDiff(ag, bg), absDiff(ab, bb)) > 20 {
				changed++
			}
			total++
		}
	}
	return total > 0 && changed*10 > total
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
			if score > bestScore {
				bestScore = score
				bestX = x
				bestY = (candidate.start + candidate.end) / 2
				bestHeight = height
			}
		}
	}
	if bestScore <= 0 {
		return image.Point{}, 0, false
	}
	left, right := bestX, bestX
	for left > xStart {
		r, g, b := rgb(screen.At(left-1, bestY))
		if r <= 190 || g <= 145 || b >= 160 {
			break
		}
		left--
	}
	for right+1 < xEnd {
		r, g, b := rgb(screen.At(right+1, bestY))
		if r <= 190 || g <= 145 || b >= 160 {
			break
		}
		right++
	}
	return image.Pt((left+right)/2, bestY), bestHeight, true
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
