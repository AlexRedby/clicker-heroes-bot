package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
)

var startupHeroNames, startupHeroNamesErr = loadStartupHeroNames()

func startupHeroLetters(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, name)
}

func loadStartupHeroNames() (map[string]string, error) {
	names, err := ancientcalc.HeroNames()
	if err != nil {
		return nil, err
	}
	keys := make(map[string]string, len(names))
	for _, name := range names {
		key := startupHeroLetters(name)
		if name == "Cid, the Helpful Adventurer" {
			key = "Cid,theHelpfulAdventurer"
		}
		keys[strings.ToLower(startupHeroLetters(name))] = key
	}
	return keys, nil
}

// A clipped field needs an overlapping view, not another OCR of the same crop.
func startupHeroNavigate(out *heroObservation, button image.Point, thumbHeight, direction int) bool {
	if !out.thumbFound {
		return false
	}
	viewport := heroListViewport(out.frame.image)
	if direction == 0 && button.Y-out.frame.image.Bounds().Dy()*7/100 < viewport.Min.Y {
		direction = -1
	}
	if direction == 0 && button.Y+out.frame.image.Bounds().Dy()*3/100 > viewport.Max.Y {
		direction = 1
	}
	if direction == 0 {
		return false
	}
	b := out.frame.image.Bounds()
	target := max(b.Min.Y+b.Dy()*4/10, min(out.thumb.Y+direction*max(3, thumbHeight/2), b.Min.Y+b.Dy()*965/1000-thumbHeight/2))
	if target == out.thumb.Y {
		return false
	}
	out.found, out.stable, out.startupComplete = false, false, false
	out.startupScroll = image.Pt(out.thumb.X, target)
	return true
}

// This initial/reset sweep levels each affordable row before allowing Ancient spending.
// passiveReady is a combat fact; only startupComplete ends the hero sweep.
func readStartupHeroObservation(ctx context.Context, frame gameFrame, read heroReaders, before *heroObservation, visited map[string]bool, started bool) (heroObservation, error) {
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
		out.button, out.startupName = before.button, before.startupName
		button, found, err := findStartupHeroAfter(ctx, *before, frame)
		out.found, out.stable = found, found
		if err != nil || !found {
			return out, err
		}
		out.button = button
		level, err := readStartupHeroLevel(ctx, frame.image, out.button, read)
		out.level, out.owned = level, level > 0
		out.passiveReady = out.passiveReady || out.owned && out.startupName != "Cid,theHelpfulAdventurer"
		return out, err
	}
	var buttons []image.Point
	enabled, edges := make(map[int]bool), make(map[int]int)
	viewport := heroListViewport(frame.image)
	for _, available := range []bool{true, false} {
		for _, band := range findHeroButtonBands(frame.image, available) {
			button := image.Pt(b.Min.X+b.Dx()*8/100, (band.Min.Y+band.Max.Y-1)/2)
			buttons = append(buttons, button)
			enabled[button.Y] = available
			if band.Min.Y == viewport.Min.Y {
				edges[button.Y] = -1
			}
			if band.Max.Y == viewport.Max.Y {
				edges[button.Y] = 1
			}
		}
	}
	sort.Slice(buttons, func(i, j int) bool { return buttons[i].Y < buttons[j].Y })
	if len(buttons) == 0 {
		return out, nil
	}
	if !started || !out.thumbFound {
		b := frame.image.Bounds()
		if absDiff(buttons[0].Y, b.Min.Y+b.Dy()*455/1000) > b.Dy()/40 {
			if !started && out.thumbFound {
				out.startupScroll = image.Pt(out.thumb.X, b.Min.Y+b.Dy()*4/10)
			}
			return out, nil
		}
		name, err := readStartupHeroName(ctx, frame.image, buttons[0])
		if err != nil {
			if !started && out.thumbFound {
				out.startupScroll = image.Pt(out.thumb.X, b.Min.Y+b.Dy()*4/10)
				return out, nil
			}
			return out, err
		}
		if name != "Cid,theHelpfulAdventurer" {
			if !started && out.thumbFound {
				out.startupScroll = image.Pt(out.thumb.X, b.Min.Y+b.Dy()*4/10)
			}
			return out, nil
		}
		out.startupTop = true
	}
	for _, button := range buttons {
		// Overlap scrolling exposes an already handled card at the top. Keep the sweep monotonic.
		if started && edges[button.Y] < 0 {
			continue
		}
		name, err := readStartupHeroName(ctx, frame.image, button)
		if err != nil {
			if started && button.Y-b.Dy()*7/100 < heroListViewport(frame.image).Min.Y {
				continue
			}
			if startupHeroNavigate(&out, button, thumbHeight, edges[button.Y]) {
				return out, nil
			}
			return out, err
		}
		if name == "" {
			return out, fmt.Errorf("startup hero name missing at %v", button)
		}
		kind, err := startupHeroButtonKind(frame.image, button)
		if err != nil {
			if visited[name] && button.Y-b.Dy()*7/100 < viewport.Min.Y {
				continue
			}
			if startupHeroNavigate(&out, button, thumbHeight, edges[button.Y]) {
				return out, nil
			}
			return out, err
		}
		owned := kind == heroButtonLevelUp
		out.passiveReady = out.passiveReady || owned && name != "Cid,theHelpfulAdventurer"
		if visited[name] && !owned {
			return out, fmt.Errorf("confirmed startup hero %s lost its level", name)
		}
		if owned && (visited[name] || !enabled[button.Y]) {
			continue
		}
		if enabled[button.Y] {
			level := 0
			if owned {
				level, err = readStartupOwnedHeroLevel(ctx, frame.image, button, read)
				if err != nil {
					if startupHeroNavigate(&out, button, thumbHeight, edges[button.Y]) {
						return out, nil
					}
					return out, err
				}
			}
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

// A hire changes the button height and may expand the card. Confirm the same
// OCR-identified hero at its current button, independently of that layout change.
func findStartupHeroAfter(ctx context.Context, before heroObservation, current gameFrame) (image.Point, bool, error) {
	if !before.startup || !before.found || before.startupName == "" || before.frame.image == nil ||
		current.image == nil || before.frame.image.Bounds() != current.image.Bounds() {
		return image.Point{}, false, nil
	}
	buttons := append(findHeroButtons(current.image, true), findHeroButtons(current.image, false)...)
	sort.Slice(buttons, func(i, j int) bool {
		return absDiff(buttons[i].Y, before.button.Y) < absDiff(buttons[j].Y, before.button.Y)
	})
	for _, button := range buttons {
		name, err := readStartupHeroName(ctx, current.image, button)
		if err != nil {
			return image.Point{}, false, err
		}
		if name == before.startupName {
			return button, true, nil
		}
	}
	return image.Point{}, false, nil
}

func readStartupHeroLevel(ctx context.Context, screen image.Image, button image.Point, read heroReaders) (int, error) {
	kind, err := startupHeroButtonKind(screen, button)
	if err != nil {
		return 0, err
	}
	if kind == heroButtonHire {
		return 0, nil
	}
	return readStartupOwnedHeroLevel(ctx, screen, button, read)
}

func startupHeroButtonKind(screen image.Image, button image.Point) (heroButtonKind, error) {
	kind, err := readHeroButtonKind(screen, button)
	if err != nil {
		return heroButtonUnknown, fmt.Errorf("startup hero button at %v: %w", button, err)
	}
	if kind == heroButtonUnknown {
		return kind, fmt.Errorf("startup hero button at %v crop %v: caption is missing or obscured", button, heroButtonCaptionRegion(screen, button))
	}
	return kind, nil
}

func readStartupOwnedHeroLevel(ctx context.Context, screen image.Image, button image.Point, read heroReaders) (int, error) {
	level, err := read.level(ctx, screen, button)
	if err != nil {
		return 0, fmt.Errorf("startup owned hero level at %v: %w", button, err)
	}
	if level <= 0 {
		return 0, fmt.Errorf("startup owned hero level at %v: invalid level %d", button, level)
	}
	return level, nil
}

func startupHeroNameRegion(screen image.Image, button image.Point) image.Rectangle {
	b := screen.Bounds()
	search := image.Rect(b.Min.X+b.Dx()*17/100, button.Y-b.Dy()/10, b.Min.X+b.Dx()*365/1000, button.Y-b.Dy()/100).Intersect(heroListViewport(screen))
	purple := 0
	for y := search.Min.Y; y < search.Max.Y; y++ {
		for x := search.Min.X; x < search.Max.X; x++ {
			r, g, blue := rgb(screen.At(x, y))
			if blue > 100 && blue > r+30 && blue > g+50 {
				purple++
			}
		}
	}
	isPurple := purple > max(20, search.Dy())
	start, last, bestStart, bestEnd := -1, -1, -1, -1
	for y := search.Min.Y; y <= search.Max.Y+2; y++ {
		ink := 0
		if y < search.Max.Y {
			for x := search.Min.X; x < search.Max.X; x++ {
				r, g, blue := rgb(screen.At(x, y))
				if isPurple && blue > 100 && blue > r+30 && blue > g+50 || !isPurple && min(r, g, blue) > 180 && max(r, g, blue)-min(r, g, blue) < 55 {
					ink++
				}
			}
		}
		if ink >= max(2, b.Dx()/500) {
			if start < 0 {
				start = y
			}
			last = y
			continue
		}
		if start >= 0 && y-last > 2 {
			center := button.Y - b.Dy()/20
			if last-start >= b.Dy()/100 && (bestStart < 0 || absDiff((start+last)/2, center) < absDiff((bestStart+bestEnd)/2, center)) {
				bestStart, bestEnd = start, last
			}
			start = -1
		}
	}
	if bestStart >= 0 {
		search.Min.Y = max(search.Min.Y, bestStart-3)
		search.Max.Y = min(search.Max.Y, bestEnd+4)
	}
	return search
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
	if err != nil {
		return "", err
	}
	if startupHeroNamesErr != nil {
		return "", startupHeroNamesErr
	}
	if name, ok := startupHeroNames[strings.ToLower(startupHeroLetters(raw))]; ok {
		return name, nil
	}
	return "", fmt.Errorf("startup hero name at %v crop %v: unrecognized %q", button, region, strings.TrimSpace(raw))
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
