package bot

import (
	"context"
	"fmt"
	"image"
	"sort"
)

// A sweep owns only a viewport cursor, not a roster or OCR-derived identity.
// Two attempts per caption bound a missed hire or MAX input.
type startupSweep struct {
	bottom, boosted, maxPending, needsLevels bool
	y, attempts                              int
	retry                                    uint8 // 0: initial; 1: unaffordable locked row seen; 2: final pass.
}

func readStartupHeroObservation(ctx context.Context, frame gameFrame, _ heroReaders, before *heroObservation, sweep startupSweep) (heroObservation, error) {
	out := heroObservation{frame: frame, startup: true, sweep: sweep}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if !bootstrapHeroes(frame.context) || !heroQuantityBarPresent(frame.image) {
		return out, nil
	}
	out.x1 = heroQuantitySelected(frame.image, 122)
	if !out.x1 {
		return out, nil
	}
	b := frame.image.Bounds()
	viewport := heroListViewport(frame.image)
	// The color-band detector bridges small gaps; its last colored pixel can
	// stop just short of the viewport even when the button is clipped.
	edgeGap := max(3, b.Dy()/150)
	var height int
	out.thumb, height, out.thumbFound = heroScrollbarThumb(frame.image)
	out.bottom = out.thumbFound && heroScrollbarAtBottom(frame.image)
	type row struct {
		button    image.Point
		band      image.Rectangle
		available bool
		kind      heroButtonKind
	}
	var rows []row
	for _, available := range []bool{true, false} {
		for _, band := range findHeroButtonBands(frame.image, available) {
			rows = append(rows, row{button: image.Pt(b.Min.X+b.Dx()*8/100, (band.Min.Y+band.Max.Y-1)/2), band: band, available: available})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].button.Y > rows[j].button.Y })
	if len(rows) == 0 {
		return out, nil
	}
	atTop := out.thumbFound && out.thumb.Y-height/2 <= b.Min.Y+b.Dy()*420/1000+max(3, b.Dy()/100)
	if !out.thumbFound {
		atTop = absDiff(rows[len(rows)-1].button.Y, b.Min.Y+b.Dy()*455/1000) <= b.Dy()/40
	}
	if !sweep.bottom {
		if out.thumbFound && !out.bottom {
			out.startupScroll = image.Pt(out.thumb.X, b.Max.Y-1)
			return out, nil
		}
		if !out.thumbFound && !atTop {
			return out, nil
		}
		out.sweep.bottom = true
	}
	// Hiring can expand the list. Follow the visible row and give its MAX input
	// a fresh budget once the caption changes from HIRE to LVL UP.
	if before != nil {
		if !heroListStable(before.frame.image, frame.image) &&
			!(!before.thumbFound && !out.thumbFound && heroRowNameMatches(before.frame.image, frame.image, before.button, before.button)) {
			out.sweep.attempts = 0
		}
		nearest := 0
		for i := range rows {
			if absDiff(rows[i].button.Y, before.button.Y) < absDiff(rows[nearest].button.Y, before.button.Y) {
				nearest = i
			}
		}
		if absDiff(rows[nearest].button.Y, before.button.Y) <= b.Dy()/12 {
			out.sweep.y = rows[nearest].button.Y
		}
	}
	for i := range rows {
		var err error
		rows[i].kind, err = readHeroButtonKind(frame.image, rows[i].button)
		if err != nil {
			return out, err
		}
		// Cid is the top card; subsequent owned rows provide passive DPS.
		out.passiveReady = out.passiveReady || rows[i].kind == heroButtonLevelUp &&
			(i < len(rows)-1 || out.thumbFound && out.thumb.Y-height/2 > b.Min.Y+b.Dy()*435/1000)
		if before != nil && !before.owned && rows[i].kind == heroButtonLevelUp && absDiff(rows[i].button.Y, out.sweep.y) <= b.Dy()/20 {
			out.sweep.attempts = 0
			out.sweep.maxPending = true
		}
	}
	clippedTop := false
	for _, r := range rows {
		y := r.button.Y
		if out.sweep.y != 0 && y > out.sweep.y+b.Dy()/20 {
			continue
		}
		if r.band.Min.Y <= viewport.Min.Y+edgeGap {
			clippedTop = true
			continue
		}
		if r.band.Max.Y >= viewport.Max.Y-edgeGap || r.kind == heroButtonHire && !heroPriceRegion(frame.image, r.button).In(viewport) {
			if (!out.sweep.boosted || out.sweep.maxPending) && out.thumbFound {
				out.sweep.bottom = false
				out.startupScroll = image.Pt(out.thumb.X, b.Max.Y-1)
				return out, nil
			}
			continue
		}
		if r.kind == heroButtonUnknown {
			return out, fmt.Errorf("startup hero button at %v: caption is obscured", r.button)
		}
		owned := r.kind == heroButtonLevelUp
		forceMax := owned && (!out.sweep.boosted || out.sweep.maxPending && (out.sweep.y == 0 || absDiff(y, out.sweep.y) <= b.Dy()/20))
		locked := false
		if owned && (!forceMax || !r.available) {
			if y+b.Dy()*85/1000 >= viewport.Max.Y {
				continue
			}
			var err error
			locked, err = heroHasLockedUpgrade(frame.image, r.button)
			if err != nil {
				return out, err
			}
			if !locked && !forceMax {
				continue
			}
		}
		if out.sweep.attempts >= 2 && absDiff(y, out.sweep.y) <= b.Dy()/20 || !r.available {
			if !owned {
				out.sweep.maxPending = false
			}
			if !owned || locked {
				out.sweep.needsLevels = true
				if out.sweep.retry == 0 {
					out.sweep.retry = 1
				}
			}
			if forceMax {
				out.sweep.boosted, out.sweep.maxPending = true, false
			}
			continue
		}
		if absDiff(y, out.sweep.y) > b.Dy()/20 {
			out.sweep.attempts = 0
		}
		out.sweep.y = y
		out.button, out.found, out.owned = r.button, true, owned
		out.sweep.maxPending = !owned
		if owned {
			out.sweep.boosted = true
		}
		return out, nil
	}
	if atTop && !clippedTop {
		out.startupComplete = out.passiveReady
		return out, nil
	}
	if out.thumbFound {
		out.startupScroll = image.Pt(out.thumb.X, max(out.thumb.Y-max(3, height/2), b.Min.Y+b.Dy()*420/1000+height/2))
	}
	return out, nil
}

func startupHeroStable(before heroObservation, current gameFrame) bool {
	return before.startup && before.found && bootstrapHeroes(current.context) &&
		heroQuantityBarPresent(current.image) &&
		(heroListStable(before.frame.image, current.image) || !before.thumbFound && heroRowNameMatches(before.frame.image, current.image, before.button, before.button)) &&
		heroRowYellow(current.image, before.button.Y)
}

func heroTextStable(a, z image.Image, region image.Rectangle) bool {
	if a == nil || z == nil || a.Bounds() != z.Bounds() {
		return false
	}
	changed, white, total := 0, 0, 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			ar, ag, ab := rgb(a.At(x, y))
			zr, zg, zb := rgb(z.At(x, y))
			aw := min(ar, ag, ab) > 180 && max(ar, ag, ab)-min(ar, ag, ab) < 55
			zw := min(zr, zg, zb) > 180 && max(zr, zg, zb)-min(zr, zg, zb) < 55
			if aw {
				white++
			}
			if aw != zw {
				changed++
			}
			total++
		}
	}
	return white > total/100 && changed*100 <= total
}

func bootstrapHeroes(c gameContext) bool {
	return c.known && c.heroes && c.modal == noGildModal && !c.transcension && !c.ascension && !c.ancients && !c.ancientDialog && !c.saveMenu && !c.questDialog && !c.mercenaries
}
