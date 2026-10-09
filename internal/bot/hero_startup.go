package bot

import (
	"context"
	"fmt"
	"image"
	"sort"
)

// A sweep owns only a viewport cursor, not a roster or OCR-derived identity.
// At most two purchase inputs per row keep missing clicks from blocking the sweep.
type startupSweep struct {
	top, needsLevels bool
	y, attempts      int
	retry            uint8 // 0: initial; 1: unaffordable locked row seen; 2: final pass.
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
		locked    bool
	}
	var rows []row
	for _, available := range []bool{true, false} {
		for _, band := range findHeroButtonBands(frame.image, available) {
			rows = append(rows, row{button: image.Pt(b.Min.X+b.Dx()*8/100, (band.Min.Y+band.Max.Y-1)/2), band: band, available: available})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].button.Y < rows[j].button.Y })
	if len(rows) == 0 {
		return out, nil
	}
	if !sweep.top {
		atTop := out.thumbFound && out.thumb.Y-height/2 <= b.Min.Y+b.Dy()*420/1000+max(3, b.Dy()/100)
		if !out.thumbFound {
			atTop = absDiff(rows[0].button.Y, b.Min.Y+b.Dy()*455/1000) <= b.Dy()/40
		}
		if !atTop {
			if out.thumbFound {
				out.startupScroll = image.Pt(out.thumb.X, b.Min.Y+b.Dy()*4/10)
			}
			return out, nil
		}
		out.sweep.top = true
	}
	// Hiring expands a card. Follow its nearest button on the next shared frame;
	// input is bounded even if the click did not register.
	if before != nil {
		// A small list scroll can put a different card at the old Y coordinate.
		// The attempt budget belongs to the visible list position, so restart it
		// whenever the viewport moved before applying the Y cursor.
		if !heroListStable(before.frame.image, frame.image) &&
			!(!before.thumbFound && !out.thumbFound && heroRowNameMatches(before.frame.image, frame.image, before.button, before.button)) {
			out.sweep.attempts = 0
		}
		nearest := rows[0].button.Y
		for _, r := range rows {
			if absDiff(r.button.Y, before.button.Y) < absDiff(nearest, before.button.Y) {
				nearest = r.button.Y
			}
		}
		if absDiff(nearest, before.button.Y) <= b.Dy()/12 {
			out.sweep.y = nearest
		}
	}
	for i := range rows {
		var err error
		rows[i].kind, err = readHeroButtonKind(frame.image, rows[i].button)
		if err != nil {
			return out, err
		}
		// At the top Cid is the first card; every subsequent owned row has DPS.
		out.passiveReady = out.passiveReady || rows[i].kind == heroButtonLevelUp && (i > 0 || out.thumbFound && out.thumb.Y-height/2 > b.Min.Y+b.Dy()*435/1000)
		if rows[i].kind == heroButtonLevelUp && rows[i].band.Min.Y > viewport.Min.Y+edgeGap && rows[i].button.Y+b.Dy()*85/1000 < viewport.Max.Y {
			rows[i].locked, err = heroHasLockedUpgrade(frame.image, rows[i].button)
			if err != nil {
				return out, err
			}
			if rows[i].locked && !rows[i].available {
				out.sweep.needsLevels = true
				if out.sweep.retry == 0 {
					out.sweep.retry = 1
				}
			}
		}
	}
	// A successful hire opens a larger card; revisit earlier owned rows once
	// affordable hires have stopped taking priority over their MAX purchases.
	if before != nil && !before.owned {
		for _, r := range rows {
			if absDiff(r.button.Y, out.sweep.y) <= b.Dy()/20 && r.kind == heroButtonLevelUp {
				out.sweep.y, out.sweep.attempts = 0, 0
				break
			}
		}
	}
	// Hire affordable successors before MAX can spend their gold on old rows.
	for _, r := range rows {
		y := r.button.Y
		if r.kind != heroButtonHire || !r.available || r.band.Min.Y <= viewport.Min.Y+edgeGap ||
			r.band.Max.Y >= viewport.Max.Y-edgeGap || !heroPriceRegion(frame.image, r.button).In(viewport) {
			continue
		}
		if y < out.sweep.y-b.Dy()/20 || out.sweep.attempts >= 2 && absDiff(y, out.sweep.y) <= b.Dy()/20 {
			out.sweep.needsLevels = true
			if out.sweep.retry == 0 {
				out.sweep.retry = 1
			}
			continue
		}
		if absDiff(y, out.sweep.y) > b.Dy()/20 {
			out.sweep.attempts = 0
		}
		out.sweep.y = y
		out.button, out.found = r.button, true
		return out, nil
	}
	clipped := false
	for _, r := range rows {
		y := r.button.Y
		if r.band.Min.Y <= viewport.Min.Y+edgeGap {
			continue
		}
		if y < out.sweep.y-b.Dy()/20 || out.sweep.attempts >= 2 && absDiff(y, out.sweep.y) <= b.Dy()/20 {
			if r.locked || r.kind == heroButtonHire && r.available {
				out.sweep.needsLevels = true
				if out.sweep.retry == 0 {
					out.sweep.retry = 1
				}
			}
			continue
		}
		if r.kind == heroButtonUnknown {
			if r.band.Max.Y >= viewport.Max.Y-edgeGap {
				clipped = true
				break
			}
			return out, fmt.Errorf("startup hero button at %v: caption is obscured", r.button)
		}
		owned := r.kind == heroButtonLevelUp
		if owned {
			if y+b.Dy()*85/1000 >= viewport.Max.Y {
				clipped = true
				break
			}
			if !r.locked {
				continue
			}
			if !r.available {
				if out.sweep.retry == 0 {
					out.sweep.retry = 1
				}
				continue
			}
		} else if r.band.Max.Y >= viewport.Max.Y-edgeGap || !heroPriceRegion(frame.image, r.button).In(viewport) {
			// HIRE can remain readable when its price extends below the screen.
			// Bring that price into view before buying or checking affordability.
			clipped = true
			break
		}
		if r.available {
			if absDiff(y, out.sweep.y) > b.Dy()/20 {
				out.sweep.attempts = 0
			}
			out.sweep.y = y
			out.button, out.found, out.owned = r.button, true, owned
			return out, nil
		}
		// The disabled HIRE button already proves this row cannot be bought.
		out.startupComplete = out.passiveReady
		return out, nil
	}
	if out.bottom && !clipped {
		out.startupComplete = out.passiveReady
		return out, nil
	}
	if out.thumbFound {
		out.startupScroll = image.Pt(out.thumb.X, min(out.thumb.Y+max(3, height/2), b.Min.Y+b.Dy()*965/1000-height/2))
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
