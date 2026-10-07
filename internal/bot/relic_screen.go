package bot

import (
	"context"
	"errors"
	"image"
	"regexp"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/vision"
)

// relicUI is deliberately a small, visual-only observation. It does not assign
// UIDs: the cards expose no stable identifier in the client UI.
type relicUI struct {
	known     bool
	equipment [4]bool
	junk      []image.Point
}

var relicEquipmentRegions = [...]image.Rectangle{
	image.Rect(75, 264, 145, 333),
	image.Rect(214, 264, 285, 333),
	image.Rect(355, 264, 424, 333),
	image.Rect(495, 264, 564, 333),
}

var relicJunkRegions = [...]image.Rectangle{
	image.Rect(104, 391, 175, 461),
	image.Rect(176, 391, 247, 461),
	image.Rect(249, 391, 319, 461),
	image.Rect(321, 391, 391, 461),
	image.Rect(394, 391, 464, 461),
	image.Rect(466, 391, 537, 461),
}

func relicTabPoint(screen image.Image) image.Point {
	return vision.Rect(screen, image.Rect(353, 145, 354, 146)).Min
}

func relicCream(screen image.Image, points ...image.Point) bool {
	for _, point := range points {
		r := vision.Rect(screen, image.Rect(point.X, point.Y, point.X+1, point.Y+1)).Min
		red, green, blue := rgb(screen.At(r.X, r.Y))
		if red < 240 || green < 215 || blue < 160 {
			return false
		}
	}
	return true
}

func relicDarkFraction(screen image.Image, logical image.Rectangle) float64 {
	r := vision.Rect(screen, logical).Intersect(screen.Bounds())
	if r.Empty() {
		return 1
	}
	dark, total := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y += max(1, r.Dy()/24) {
		for x := r.Min.X; x < r.Max.X; x += max(1, r.Dx()/16) {
			red, green, blue := rgb(screen.At(x, y))
			total++
			if max(red, max(green, blue)) < 150 {
				dark++
			}
		}
	}
	if total == 0 {
		return 1
	}
	return float64(dark) / float64(total)
}

func relicCardLooksReal(screen image.Image, logical image.Rectangle) bool {
	r := vision.Rect(screen, logical).Intersect(screen.Bounds())
	if r.Empty() {
		return false
	}
	bright := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			a, b, c := rgb(screen.At(x, y))
			if max(a, max(b, c)) > 180 {
				bright++
			}
		}
	}
	// Rounded corners/artwork remain bright even on the native dark Common
	// card; antialiasing reduces the bright fraction at smaller scales.
	// Uniform dark patches are unknown.
	return bright*500 >= r.Dx()*r.Dy()
}

func relicTabSelected(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	found, err := vision.MatchControl(screen, image.Rect(333, 125, 373, 165), "relics/tab.png")
	return err == nil && found
}

// relicPanelPresent is intentionally looser than readRelicUI: a card tooltip
// may cover part of the inventory while the surrounding Relics panel remains
// safely identifiable. Modals are dimmed and fail the panel anchors.
func relicPanelPresent(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 || !relicTabSelected(screen) {
		return false
	}
	if !relicCream(screen, image.Pt(105, 385), image.Pt(520, 385), image.Pt(105, 690), image.Pt(520, 690)) {
		return false
	}
	found, err := vision.MatchControl(screen, image.Rect(45, 348, 180, 382), "relics/junk-title.png")
	return err == nil && found
}

func readRelicUI(screen image.Image) relicUI {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return relicUI{}
	}
	if !relicPanelPresent(screen) {
		return relicUI{}
	}

	ui := relicUI{known: true}
	for i, region := range relicEquipmentRegions {
		ui.equipment[i] = relicDarkFraction(screen, region) > 0.20
	}
	// A tooltip, clipped card, or unexplained pixel makes the inventory unknown.
	// Six first-row positions are supported by the observed 72-ish pixel
	// stride. No second row or scrolling geometry is inferred.
	for _, region := range relicJunkRegions {
		if relicDarkFraction(screen, region) > 0.18 && relicCardLooksReal(screen, region) {
			ui.junk = append(ui.junk, vision.Rect(screen, region).Min.Add(vision.Rect(screen, region).Size().Div(2)))
		}
	}
	salvageFound, err := vision.MatchControl(screen, image.Rect(225, 505, 415, 546), "relics/salvage.png")
	if err != nil {
		return relicUI{}
	}
	if (len(ui.junk) == 0) == salvageFound {
		return relicUI{}
	}
	// Only present, recognized cards and the present salvage button are
	// exempted from the full beige-panel scan.
	panel := vision.Rect(screen, image.Rect(105, 385, 538, 689)).Intersect(screen.Bounds())
	allowed := make([]image.Rectangle, 0, len(ui.junk)+1)
	for _, region := range relicJunkRegions {
		if relicDarkFraction(screen, region) > 0.18 && relicCardLooksReal(screen, region) {
			allowed = append(allowed, vision.Rect(screen, region).Inset(-3))
		}
	}
	if salvageFound {
		allowed = append(allowed, vision.Rect(screen, image.Rect(222, 502, 418, 549)))

	}
	for y := panel.Min.Y; y < panel.Max.Y; y++ {
		for x := panel.Min.X; x < panel.Max.X; x++ {
			point := image.Pt(x, y)
			permitted := false
			for _, region := range allowed {
				if point.In(region) {
					permitted = true
					break
				}
			}
			if permitted {
				continue
			}
			red, green, blue := rgb(screen.At(x, y))
			if red < 225 || green < 195 || blue < 145 || red-green < 10 {
				return relicUI{}
			}
		}
	}
	return ui
}

var relicTooltipLevel = regexp.MustCompile(`(?i)\bLevel\s+([0-9]+(?:\.[0-9]+)?)\b`)
var relicTooltipBonus = regexp.MustCompile(`(?i)\+([0-9]+(?:\.[0-9]+)?)\s+Levels?\s+to\s+([A-Za-z']+),`)

// relicTooltipRegion returns the dark text block produced by a card hover.
// It is deliberately bounded to the Relics panel so unrelated modal text is
// never treated as an item identity.
func relicTooltipRegion(screen image.Image) image.Rectangle {
	area := vision.Rect(screen, image.Rect(40, 250, 600, 700))
	box := image.Rectangle{}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		start := -1
		for x := area.Min.X; x <= area.Max.X; x++ {
			dark := false
			if x < area.Max.X {
				a, b, c := rgb(screen.At(x, y))
				dark = max(a, max(b, c)) < 70
			}
			if dark && start < 0 {
				start = x
			}
			if !dark && start >= 0 {
				if x-start > screen.Bounds().Dx()*190/1280 {
					line := image.Rect(start, y, x, y+1)
					if box.Empty() {
						box = line
					} else {
						box = box.Union(line)
					}
				}
				start = -1
			}
		}
	}
	if box.Dy() < screen.Bounds().Dy()/10 {
		return image.Rectangle{}
	}
	return box
}

func relicTooltipMatches(raw string, item ancientcalc.Relic) bool {
	raw = strings.Join(strings.Fields(raw), " ")
	level := relicTooltipLevel.FindStringSubmatch(raw)
	rarity := map[int]string{1: "Common", 2: "Uncommon", 3: "Rare", 4: "Epic", 5: "Legendary", 6: "Mythical"}[item.Rarity]
	if level == nil || rarity == "" || !strings.Contains(raw, " "+rarity+" Level ") || !ancientDisplayMatches(level[1], item.Level) {
		return false
	}
	bonuses := relicTooltipBonus.FindAllStringSubmatch(raw, -1)
	if len(bonuses) != len(item.Bonuses) {
		return false
	}
	seen := map[int]bool{}
	for _, shown := range bonuses {
		found := false
		for _, bonus := range item.Bonuses {
			name := ancientcalc.RelicAncientName(bonus.Type)
			if !seen[bonus.Type] && name != "" && strings.EqualFold(shown[2], name) && ancientDisplayMatches(shown[1], bonus.Level) {
				seen[bonus.Type], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// readRelicObservation binds one visual frame to at most one exported item.
// A tooltip that matches multiple records is deliberately rejected: an
// ambiguous card must never become a drag source.
func readRelicObservation(ctx context.Context, frame gameFrame, snapshot *ancientcalc.RelicSnapshot, hover bool) (relicObservation, error) {
	out := relicObservation{frame: frame, ui: readRelicUI(frame.image)}
	if hover {
		if !relicPanelPresent(frame.image) {
			return out, errors.New("relic panel is not present")
		}
		region := relicTooltipRegion(frame.image)
		if region.Empty() {
			return out, errors.New("relic tooltip not visible")
		}
		raw, err := readGameText(ctx, frame.image, region, max(1, 2560/frame.image.Bounds().Dx()), 6, 120, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+.,'% ")
		if err != nil {
			return out, err
		}
		for _, item := range snapshot.Items {
			if relicTooltipMatches(raw, item) {
				if out.uid != 0 {
					return out, errors.New("ambiguous duplicate relic tooltip")
				}
				out.uid = item.UID
			}
		}
		if out.uid == 0 {
			return out, errors.New("tooltip does not match fresh export")
		}
		return out, nil
	}
	if !out.ui.known {
		return out, errors.New("obscured or unsupported relic inventory")
	}
	return out, nil
}

// relicNotification recognizes the animated yellow exclamation above the
// selected Relics/Chest tab. The narrow HUD window excludes the tab artwork
// and all other yellow controls; a low-saturation background cannot pass it.
func relicNotification(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	r := vision.Rect(screen, image.Rect(350, 90, 390, 138)).Intersect(screen.Bounds())
	if r.Empty() {
		return false
	}
	yellow := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			rr, gg, bb := rgb(screen.At(x, y))
			if rr > 210 && gg > 150 && bb < 120 && rr*100 > gg*105 {
				yellow++
			}
		}
	}
	return yellow*10 >= r.Dx()*r.Dy()
}
