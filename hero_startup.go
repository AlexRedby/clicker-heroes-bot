package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sort"
	"strings"
)

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
	level, err := read.level(ctx, screen, button)
	if err == nil || ctx.Err() != nil {
		return level, err
	}
	// Unhired cards show a bare zero. Empty/covered level areas do not prove it.
	b := screen.Bounds()
	glyph := startupBareZeroGlyph(screen, button)
	if glyph.Empty() {
		return 0, fmt.Errorf("startup hero level at %v: %w", button, err)
	}
	label := image.Rect(b.Min.X+b.Dx()*179/2560, glyph.Min.Y-b.Dy()*15/1440, b.Min.X+b.Dx()*294/2560, glyph.Max.Y+b.Dy()*6/1440).Intersect(b)
	caption := image.NewRGBA(image.Rect(0, 0, label.Dx(), label.Dy()))
	draw.Draw(caption, caption.Bounds(), screen, label.Min, draw.Src)
	// HIRE positively identifies an unhired row. Use a single-word retry for
	// dark disabled buttons that line segmentation cannot read.
	for _, psm := range []int{7, 8} {
		hire, hireErr := readTextImage(ctx, caption, psm, "HIRELVUP")
		if hireErr != nil {
			return 0, hireErr
		}
		if strings.TrimSpace(hire) == "HIRE" {
			return 0, nil
		}
	}
	return 0, fmt.Errorf("startup hero level at %v: %w", button, err)
}

func startupBareZeroRegion(screen image.Image, button image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*193/1280, button.Y-b.Dy()*80/1440,
		b.Min.X+b.Dx()*206/1280, button.Y+b.Dy()*30/1440).Intersect(b)
}

// The bare zero shifts within differently sized HIRE buttons. Locate its dark
// outline in the narrow text column, then normalize the OCR crop around it.
func startupBareZeroGlyph(screen image.Image, button image.Point) image.Rectangle {
	region := startupBareZeroRegion(screen, button)
	b := screen.Bounds()
	best, run := image.Rectangle{}, image.Rectangle{}
	for y := region.Min.Y; y <= region.Max.Y; y++ {
		row := image.Rectangle{}
		if y < region.Max.Y {
			for x := region.Min.X; x < region.Max.X; x++ {
				if color.GrayModel.Convert(screen.At(x, y)).(color.Gray).Y < 80 {
					row = row.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if !row.Empty() {
			run = run.Union(row)
			continue
		}
		if run.Dx() >= max(3, b.Dx()*8/2560) && run.Dy() >= max(5, b.Dy()*15/1440) && run.Dy() <= b.Dy()*40/1440 &&
			(best.Empty() || absDiff((run.Min.Y+run.Max.Y)/2, button.Y) < absDiff((best.Min.Y+best.Max.Y)/2, button.Y)) {
			best = run
		}
		run = image.Rectangle{}
	}
	if best.Empty() {
		return best
	}
	return best.Inset(-max(1, b.Dx()/2560)).Intersect(region)
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
