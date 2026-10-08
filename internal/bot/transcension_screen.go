package bot

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"regexp"
	"strings"

	"clicker-heroes-bot/internal/vision"
)

const (
	transcensionTitle = iota
	transcensionYes
	transcensionNo
	transcensionUnchecked
	transcensionOpen
)

func transcensionControl(screen image.Image, which int) (image.Point, bool, error) {
	regions := [...]image.Rectangle{image.Rect(485, 99, 795, 124), image.Rect(413, 520, 518, 561), image.Rect(763, 520, 869, 561), image.Rect(529, 585, 568, 620), image.Rect(182, 328, 423, 379)}
	names := [...]string{"title", "yes", "no", "unchecked", "open"}
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 || which < 0 || which >= len(regions) {
		return image.Point{}, false, nil
	}
	found, err := vision.MatchControl(screen, regions[which], "transcension/"+names[which]+".png")
	r := vision.Rect(screen, regions[which])
	return r.Min.Add(r.Size().Div(2)), found, err
}

func transcensionDialog(screen image.Image) bool {
	_, found, err := transcensionControl(screen, transcensionTitle)
	return found && err == nil
}

type transcensionObservation struct {
	frame               gameFrame
	known, confirm, no  bool
	respecKnown, respec bool
	reward              int
}

var transcensionRewardLabel = regexp.MustCompile(`^([0-9][0-9,]*) Ancient Souls?$`)

func readTranscensionObservation(ctx context.Context, frame gameFrame) (transcensionObservation, error) {
	out := transcensionObservation{frame: frame}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if !transcensionDialog(frame.image) {
		return out, nil
	}
	_, out.confirm, _ = transcensionControl(frame.image, transcensionYes)
	_, out.no, _ = transcensionControl(frame.image, transcensionNo)
	// Only the supplied unchecked control is verified. A tick or an obscured
	// checkbox remains unknown and cannot authorize automatic respec/reset.
	_, out.respecKnown, _ = transcensionControl(frame.image, transcensionUnchecked)
	if !out.confirm || !out.no || !out.respecKnown {
		return out, fmt.Errorf("Transcension confirmation or unchecked respec is unreadable")
	}
	// Purple reward text is dark against the light panel. Keep that contrast,
	// and exclude the following line from the single-line OCR crop.
	width := frame.image.Bounds().Dx()
	region := vision.Rect(frame.image, image.Rect(729, 192, 929, 211))
	mask := image.NewGray(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(mask, mask.Bounds(), frame.image, region.Min, draw.Src)
	for i, value := range mask.Pix {
		if value < 160 {
			mask.Pix[i] = 255
		} else {
			mask.Pix[i] = 0
		}
	}
	raw, err := readGameText(ctx, mask, mask.Bounds(), max(1, (5120+width-1)/width), 7, 230, "0123456789,abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
	if err != nil {
		return out, err
	}
	match := transcensionRewardLabel.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return out, fmt.Errorf("unreadable Transcension reward %q", strings.TrimSpace(raw))
	}
	out.reward, err = parseOutsiderInteger(match[1])
	if err != nil || out.reward <= 0 {
		return out, fmt.Errorf("invalid Transcension reward")
	}
	out.known = true
	return out, nil
}

// Geometry is derived from the same complete cards consumed by the reader.
// A clipped card, MAX mode or unaffordable row never supplies a FEED target.
func outsiderFeedPoint(ui outsiderObservation, name string) (image.Point, bool) {
	if !ui.known || ui.quantity != "x1" && ui.quantity != "x10" && ui.quantity != "x100" && ui.quantity != "x1000" || !outsiderTabSelected(ui.frame.image) {
		return image.Point{}, false
	}
	tops := outsiderCardTops(ui.frame.image)
	if len(tops) != len(ui.rows) {
		return image.Point{}, false
	}
	for i, row := range ui.rows {
		if row.name == name && row.cost > 0 && ui.wallet >= row.cost {
			r := vision.Rect(ui.frame.image, image.Rect(430, tops[i]+61, 539, tops[i]+131))
			return r.Min.Add(r.Size().Div(2)), true
		}
	}
	return image.Point{}, false
}

func outsiderQuantityPoint(ui outsiderObservation, quantity int) (image.Point, bool) {
	if !ui.known || !outsiderTabSelected(ui.frame.image) {
		return image.Point{}, false
	}
	for i, value := range []int{1, 10, 100, 1000} {
		if quantity == value {
			x := [...]int{122, 220, 320, 420}[i]
			return vision.Rect(ui.frame.image, image.Rect(x, 271, x+1, 272)).Min, true
		}
	}
	return image.Point{}, false
}

func outsiderTabPoint(screen image.Image) image.Point {
	return vision.Rect(screen, image.Rect(555, 145, 556, 146)).Min
}
