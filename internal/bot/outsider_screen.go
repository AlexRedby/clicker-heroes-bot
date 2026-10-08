package bot

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"regexp"
	"strconv"
	"strings"

	"clicker-heroes-bot/internal/vision"

	xdraw "golang.org/x/image/draw"
)

// These observations describe the visible UI only; they never authorize a reset
// or a purchase. FEED costs belong to the selected quantity, not necessarily x1.
type outsiderScreenRow struct {
	name        string
	level, cost int
	feed        bool
}

type outsiderObservation struct {
	frame              gameFrame
	known              bool
	wallet, gain       int
	power              float64
	sacrificed, nextAS string
	quantity           string
	rows               []outsiderScreenRow
}

func outsiderTabSelected(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	// A dimmed background may still match the header; require the lit tab too.
	r := vision.Rect(screen, image.Rect(530, 135, 537, 155))
	gold, total := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, b := rgb(screen.At(x, y))
			total++
			if red >= 160 && g >= 100 && g <= 145 && b <= 60 {
				gold++
			}
		}
	}
	if gold*4 < total*3 {
		return false
	}
	found, err := vision.MatchControl(screen, image.Rect(42, 175, 295, 199), "outsiders/header.png")
	return found && err == nil
}

var outsiderInteger = regexp.MustCompile(`^(?:0|[1-9][0-9]*|[1-9][0-9]{0,2}(?:,[0-9]{3})+)$`)
var outsiderPower = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)%$`)
var outsiderNames = []string{"Xyliqil", "Chor'gorloth", "Phandoryss", "Ponyboy", "Borb", "Rhageist", "K'Ariqua", "Orphalas", "Sen-Akhan"}

func parseOutsiderInteger(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if !outsiderInteger.MatchString(raw) {
		return 0, fmt.Errorf("unreadable Outsider integer %q", raw)
	}
	v, err := strconv.ParseUint(strings.ReplaceAll(raw, ",", ""), 10, 53)
	if err != nil || v > uint64(^uint(0)>>1) {
		return 0, fmt.Errorf("Outsider integer out of range %q", raw)
	}
	return int(v), nil
}

func outsiderQuantity(screen image.Image) string {
	selected := ""
	for i, x := range []int{122, 220, 320, 420, 525} {
		r := vision.Rect(screen, image.Rect(x, 262, x+8, 280))
		orange, total := 0, 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				red, g, b := rgb(screen.At(x, y))
				total++
				if red > 180 && g >= 90 && g < 190 && b < 100 && red-g > 50 {
					orange++
				}
			}
		}
		if total > 0 && orange*4 >= total*3 {
			if selected != "" {
				return ""
			}
			selected = [...]string{"x1", "x10", "x100", "x1000", "MAX"}[i]
		}
	}
	return selected
}

func outsiderCardTops(screen image.Image) []int {
	// The supported skin has a continuous black left border. Require
	// a full card; additional skins need their own calibrated border anchor.
	b := screen.Bounds()
	r := vision.Rect(screen, image.Rect(50, 300, 51, 718))
	first := -1
	var tops []int
	for y := r.Min.Y; y <= r.Max.Y; y++ {
		dark := false
		if y < r.Max.Y {
			red, g, blue := rgb(screen.At(r.Min.X, y))
			dark = max(red, g, blue) < 12
		}
		if dark {
			if first < 0 {
				first = y
			}
		} else if first >= 0 {
			height := (y - first) * 720 / b.Dy()
			if height >= 140 && height <= 151 && first > r.Min.Y {
				tops = append(tops, (first-b.Min.Y)*720/b.Dy()-2)
			}
			first = -1
		}
	}
	return tops
}

func readOutsiderCost(ctx context.Context, screen image.Image, region image.Rectangle) (string, error) {
	// Disabled FEED text is cream with a gray outline. A luminance mask keeps
	// its thin digits, which the shared white/yellow masks remove at small sizes.
	crop := image.NewGray(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	for i, v := range crop.Pix {
		if v > 120 {
			crop.Pix[i] = 255
		} else {
			crop.Pix[i] = 0
		}
	}
	w := screen.Bounds().Dx()
	scale := max(1, (5120+w-1)/w)
	up := image.NewGray(image.Rect(0, 0, crop.Bounds().Dx()*scale+20, crop.Bounds().Dy()*scale+20))
	xdraw.ApproxBiLinear.Scale(up, image.Rect(10, 10, up.Bounds().Max.X-10, up.Bounds().Max.Y-10), crop, crop.Bounds(), draw.Src, nil)
	raw, err := readTextImage(ctx, up, 7, "0123456789,xX")
	return strings.TrimSpace(raw), err
}

func readOutsiderObservation(ctx context.Context, frame gameFrame) (outsiderObservation, error) {
	out := outsiderObservation{frame: frame}
	screen := frame.image
	if !frame.context.outsiders || !outsiderTabSelected(screen) {
		return out, nil
	}
	read := func(r image.Rectangle, threshold int, chars string) (string, error) {
		w := screen.Bounds().Dx()
		raw, err := readGameText(ctx, screen, vision.Rect(screen, r), max(1, (5120+w-1)/w), 7, threshold, chars)
		return strings.TrimSpace(raw), err
	}
	chars := "0123456789.,eE%+'-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ :"
	raw, err := readGameText(ctx, screen, vision.Rect(screen, image.Rect(292, 180, 382, 198)), max(1, 2560/screen.Bounds().Dx()), 7, 230, "0123456789.%")
	raw = strings.TrimSpace(raw)
	if err != nil {
		return out, err
	}
	match := outsiderPower.FindStringSubmatch(raw)
	if match == nil {
		return out, fmt.Errorf("unreadable Transcendent Power %q", raw)
	}
	out.power, err = strconv.ParseFloat(match[1], 64)
	if err != nil || out.power < 0 || out.power > 25 {
		return out, fmt.Errorf("invalid Transcendent Power %q", raw)
	}
	for _, field := range []struct {
		r      image.Rectangle
		prefix string
		suffix string
		target *int
	}{
		{image.Rect(420, 180, 605, 198), "", " Ancient Souls", &out.wallet},
		{image.Rect(445, 200, 605, 220), "Transcend for +", "", &out.gain},
	} {
		scale := max(1, 2560/screen.Bounds().Dx())
		if field.target == &out.gain {
			scale = max(1, (5120+screen.Bounds().Dx()-1)/screen.Bounds().Dx())
		}
		raw, err := readGameText(ctx, screen, vision.Rect(screen, field.r), scale, 7, 230, chars)
		raw = strings.TrimSpace(raw)
		if err != nil {
			return out, err
		}
		label := strings.ReplaceAll(raw, " ", "")
		prefix, suffix := strings.ReplaceAll(field.prefix, " ", ""), strings.ReplaceAll(field.suffix, " ", "")
		if !strings.HasPrefix(label, prefix) || !strings.HasSuffix(label, suffix) {
			return out, fmt.Errorf("unreadable Ancient Souls label %q", raw)
		}
		*field.target, err = parseOutsiderInteger(strings.TrimSuffix(strings.TrimPrefix(label, prefix), suffix))
		if err != nil {
			return out, err
		}
	}
	for _, field := range []struct {
		r      image.Rectangle
		target *string
	}{
		{image.Rect(268, 216, 355, 235), &out.sacrificed},
		{image.Rect(516, 225, 605, 244), &out.nextAS},
	} {
		raw, err := read(field.r, 230, "0123456789.eE")
		if err != nil {
			return out, err
		}
		// The UI displays at most three decimal places. Reject a missed 'e'
		// becoming a digit, rather than accepting a plausible decimal value.
		if _, known := parseGameNumber(raw); !known {
			return out, fmt.Errorf("unreadable Outsider soul value %q", raw)
		}
		*field.target = raw
	}
	out.quantity = outsiderQuantity(screen)
	if out.quantity == "" {
		return out, fmt.Errorf("unreadable Outsider quantity")
	}
	for _, top := range outsiderCardTops(screen) {
		nameRegion := image.Rect(238, top+10, 390, top+36)
		name := ""
		for _, candidate := range []struct{ name, asset string }{{"Xyliqil", "xyliqil"}, {"Chor'gorloth", "chorgorloth"}, {"Orphalas", "orphalas"}, {"Sen-Akhan", "senakhan"}} {
			found, err := vision.MatchControl(screen, nameRegion, "outsiders/"+candidate.asset+".png")
			if err != nil {
				return out, err
			}
			if found {
				name = candidate.name
				break
			}
		}
		if name == "" {
			name, err = read(nameRegion, 180, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ '-")
			if err != nil {
				return out, err
			}
		}
		matched := false
		for _, known := range outsiderNames {
			if strings.EqualFold(name, known) {
				name, matched = known, true
				break
			}
		}
		if !matched {
			return out, fmt.Errorf("unreadable Outsider name %q", name)
		}
		row := outsiderScreenRow{name: name}
		for i, r := range []image.Rectangle{image.Rect(442, top+12, 520, top+45), image.Rect(480, top+94, 530, top+117)} {
			var raw string
			var err error
			if i == 0 {
				raw, err = read(r, 0, "0123456789,lLvViIO ")
			} else {
				raw, err = readOutsiderCost(ctx, screen, vision.Rect(screen, r))
			}
			if err != nil {
				return out, err
			}
			if i == 0 {
				if !ancientLevelLabel.MatchString(raw) {
					return out, fmt.Errorf("unreadable Outsider level %q", raw)
				}
				raw = ancientLevelLabel.ReplaceAllString(raw, "")
				raw = strings.ReplaceAll(raw, "O", "0")
			} else {
				if !strings.HasPrefix(strings.ToLower(raw), "x") {
					return out, fmt.Errorf("unreadable Outsider cost %q", raw)
				}
				raw = raw[1:]
			}
			value, err := parseOutsiderInteger(raw)
			if err != nil || (i == 1 && value == 0) {
				return out, fmt.Errorf("unreadable Outsider level/cost %q", raw)
			}
			if i == 0 {
				row.level = value
			} else {
				row.cost = value
			}
		}
		if out.wallet >= row.cost {
			// A readable cost also exists on disabled buttons. Require the
			// bright FEED caption itself; unknown/dim controls remain inert.
			r := vision.Rect(screen, image.Rect(449, top+73, 513, top+94))
			bright := 0
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					red, green, blue := rgb(screen.At(x, y))
					if min(red, green) >= 180 && (blue < 120 || min(red, green, blue) >= 200) {
						bright++
					}
				}
			}
			if bright*20 >= r.Dx()*r.Dy() {
				// FEED is fixed artwork. Match its complete caption rather
				// than accepting a price at an assumed button coordinate.
				found, err := vision.MatchControl(screen, image.Rect(449, top+73, 513, top+94), "outsiders/feed.png")
				if err != nil {
					return out, err
				}
				row.feed = found
			}
		}
		out.rows = append(out.rows, row)
	}
	if len(out.rows) == 0 {
		return out, fmt.Errorf("no readable complete Outsider cards")
	}
	out.known = true
	return out, nil
}

func (o outsiderObservation) String() string {
	if !o.known {
		return "Outsiders: unreadable"
	}
	parts := []string{fmt.Sprintf("Outsiders: AS=%d reward=+%d TP=%.2f%% sacrificed=%s nextAS=%s quantity=%s", o.wallet, o.gain, o.power, o.sacrificed, o.nextAS, o.quantity)}
	for _, row := range o.rows {
		parts = append(parts, fmt.Sprintf("%s level=%d FEED=%d", row.name, row.level, row.cost))
	}
	return strings.Join(parts, "; ")
}
