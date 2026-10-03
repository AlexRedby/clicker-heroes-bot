package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
)

// This reset-only sweep levels each affordable row before allowing Ancient spending.
// passiveReady is a combat fact; only startupComplete ends the hero sweep.
func readStartupHeroObservation(ctx context.Context, frame gameFrame, read heroReaders, before *heroObservation, visited map[string]bool) (heroObservation, error) {
	out := heroObservation{frame: frame, startup: true}
	if !bootstrapHeroes(frame.context) || !heroQuantityBarPresent(frame.image) {
		return out, nil
	}
	out.x1 = heroQuantitySelected(frame.image, 122)
	if !out.x1 {
		return out, nil
	}
	var thumbHeight int
	out.thumb, thumbHeight, out.thumbFound = heroScrollbarThumb(frame.image)
	b := frame.image.Bounds()
	out.bottom = out.thumbFound && absDiff(out.thumb.Y+thumbHeight/2, b.Min.Y+b.Dy()*965/1000) <= max(3, b.Dy()/100)
	for name, done := range visited {
		if done && name != "Cid,theHelpfulAdventurer" {
			out.passiveReady = true
		}
	}
	if before != nil {
		out.button, out.found, out.startupName = before.button, before.found, before.startupName
		out.stable = startupHeroStable(*before, frame)
		if !out.stable {
			return out, nil
		}
		level, err := readStartupHeroLevel(ctx, frame.image, out.button, read)
		out.level, out.owned = level, level > 0
		out.passiveReady = out.passiveReady || out.owned && out.startupName != "Cid,theHelpfulAdventurer"
		return out, err
	}
	buttons := findHeroLevelButtons(frame.image)
	enabled := make(map[int]bool, len(buttons))
	for _, button := range buttons {
		enabled[button.Y] = true
	}
	buttons = append(buttons, findHeroButtons(frame.image, false)...)
	sort.Slice(buttons, func(i, j int) bool { return buttons[i].Y < buttons[j].Y })
	if len(buttons) == 0 {
		return out, nil
	}
	if len(visited) == 0 || !out.thumbFound {
		b := frame.image.Bounds()
		if absDiff(buttons[0].Y, b.Min.Y+b.Dy()*455/1000) > b.Dy()/40 {
			return out, nil
		}
		name, err := readStartupHeroName(ctx, frame.image, buttons[0])
		if err != nil {
			return out, err
		}
		if name != "Cid,theHelpfulAdventurer" {
			return out, nil
		}
	}
	for _, button := range buttons {
		name, err := readStartupHeroName(ctx, frame.image, button)
		if err != nil {
			return out, err
		}
		if name == "" {
			return out, fmt.Errorf("startup hero name missing at %v", button)
		}
		level, err := readStartupHeroLevel(ctx, frame.image, button, read)
		if err != nil {
			return out, err
		}
		owned := level > 0
		out.passiveReady = out.passiveReady || owned && name != "Cid,theHelpfulAdventurer"
		if visited[name] && !owned {
			return out, fmt.Errorf("confirmed startup hero %s lost its level", name)
		}
		if owned && (visited[name] || !enabled[button.Y]) {
			continue
		}
		if enabled[button.Y] {
			out.button, out.found, out.owned, out.level, out.startupName = button, true, owned, level, name
			return out, nil
		}
		// A dark button is not enough: unreadable/obscured prices cannot finish startup.
		price, err := read.price(ctx, frame.image, button)
		if err != nil {
			return out, fmt.Errorf("startup locked hero price: %w", err)
		}
		gold, err := read.gold(ctx, frame.image)
		if err != nil {
			return out, fmt.Errorf("startup gold: %w", err)
		}
		out.startupComplete = price > gold && out.passiveReady
		return out, nil
	}
	if out.thumbFound && !out.bottom {
		// Half a thumb advances half a viewport, preserving overlap between visits.
		out.startupScroll = image.Pt(out.thumb.X, min(out.thumb.Y+max(3, thumbHeight/2), b.Min.Y+b.Dy()*965/1000-thumbHeight/2))
	}
	return out, nil
}

func readStartupHeroLevel(ctx context.Context, screen image.Image, button image.Point, read heroReaders) (int, error) {
	level, err := read.level(ctx, screen, button)
	if err == nil || ctx.Err() != nil {
		return level, err
	}
	if heroRowUnowned(screen, button.Y) {
		return 0, nil
	}
	// Initial HIRE cards show a bare zero next to the button, not LVL 0.
	b := screen.Bounds()
	region := image.Rect(b.Min.X+b.Dx()*145/1000, button.Y-b.Dy()*4/100, b.Min.X+b.Dx()*20/100, button.Y+b.Dy()/100)
	raw, zeroErr := readGameText(ctx, screen, region, max(1, 2048/b.Dx()), 7, 180, "0123456789")
	if zeroErr == nil && strings.TrimSpace(raw) == "0" {
		return 0, nil
	}
	return 0, fmt.Errorf("startup hero level at %v: %w", button, err)
}

func startupHeroNameRegion(screen image.Image, button image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*17/100, button.Y-b.Dy()*7/100, b.Min.X+b.Dx()*365/1000, button.Y-b.Dy()*25/1000).Intersect(b)
}

func readStartupHeroName(ctx context.Context, screen image.Image, button image.Point) (string, error) {
	region := startupHeroNameRegion(screen, button)
	mask := image.NewGray(image.Rect(0, 0, region.Dx(), region.Dy()))
	purple := 0
	for y := 0; y < region.Dy(); y++ {
		for x := 0; x < region.Dx(); x++ {
			r, g, b := rgb(screen.At(region.Min.X+x, region.Min.Y+y))
			v := uint8(255)
			if b > 100 && b > r+30 && b > g+50 {
				v = 0
				purple++
			}
			mask.SetGray(x, y, color.Gray{Y: v})
		}
	}
	var raw string
	var err error
	if purple > max(20, region.Dy()) {
		// Gilded names use purple letters; their white outline alone loses glyphs.
		raw, err = readTextImage(ctx, mask, 7, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	} else {
		raw, err = readGameText(ctx, screen, region, max(1, 2048/screen.Bounds().Dx()), 7, 180, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz, ")
	}
	return strings.ReplaceAll(strings.TrimSpace(raw), " ", ""), err
}

// The positively identified name stays fixed even when the reset list grows.
// Unlike the ordinary list guard, this does not infer validity from a missing thumb.
func startupHeroStable(before heroObservation, current gameFrame) bool {
	a, z := before.frame.image, current.image
	if !before.startup || before.startupName == "" || !before.found || a == nil || z == nil || a.Bounds() != z.Bounds() || !current.context.heroes || !heroQuantityBarPresent(z) || !heroRowYellow(z, before.button.Y) {
		return false
	}
	return heroTextStable(a, z, startupHeroNameRegion(a, before.button))
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
	return c.known && c.heroes && c.modal == noGildModal && !c.ascension && !c.ancients && !c.ancientDialog && !c.saveMenu && !c.questDialog && !c.mercenaries
}
