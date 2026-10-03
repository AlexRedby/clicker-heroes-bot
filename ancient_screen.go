package main

import (
	"context"
	"fmt"
	"image"
	"regexp"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
)

var ancientLevelLabel = regexp.MustCompile(`(?i)^[li]?v[li1][li]*\s*`)

func ancientControl(screen image.Image, which int) (image.Point, bool, error) {
	regions := [...]image.Rectangle{image.Rect(475, 265, 805, 295), image.Rect(581, 370, 695, 420)}
	names := [...]string{"ancients/quantity-title.png", "ancients/quantity-okay.png"}
	found, err := matchControl(screen, regions[which], names[which])
	r := controlRect(screen, regions[which])
	return r.Min.Add(r.Size().Div(2)), found, err
}
func ancientQuantityDialog(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	// The title belongs to this quantity window, not another cream game modal.
	r := controlRect(screen, image.Rect(410, 310, 411, 311))
	red, g, b := rgb(screen.At(r.Min.X, r.Min.Y))
	if red < 230 || g < 220 || b < 150 {
		return false
	}
	_, found, err := ancientControl(screen, 0)
	return found && err == nil
}
func ancientTabSelected(screen image.Image) bool {
	if screen == nil {
		return false
	}
	b := screen.Bounds()
	r := controlRect(screen, image.Rect(260, 129, 299, 156))
	gold, total := 0, 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, blue := rgb(screen.At(x, y))
			total++
			if red >= 160 && g >= 100 && g <= 145 && blue <= 60 {
				gold++
			}
		}
	}
	return b.Dx() >= 640 && total > 0 && gold*4 >= total
}
func ancientTabPoint(screen image.Image, heroes bool) image.Point {
	x := 286
	if heroes {
		x = 80
	}
	r := controlRect(screen, image.Rect(x, 145, x+1, 146))
	return r.Min
}
func ancientButtons(screen image.Image) []image.Point {
	if screen == nil {
		return nil
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	var rows []image.Point
	first, last := -1, -1
	gap := max(3, h/100)
	top, bottom := b.Min.Y+h*275/720, b.Min.Y+h*718/720
	for y := top; y < bottom+gap; y++ {
		blue, n := 0, 0
		if y < bottom {
			for x := b.Min.X + w*65/1000; x < b.Min.X+w*70/1000; x += max(1, w/500) {
				r, g, bl := rgb(screen.At(x, y))
				n++
				if bl > 150 && bl > r+40 && bl >= g-10 && g > 90 {
					blue++
				}
			}
		}
		if n > 0 && blue*10 > n*3 {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 && y-last > gap {
			if first > top+gap && last < bottom-gap && last-first > h*75/1000 && last-first < h/7 {
				point := image.Pt(b.Min.X+w*95/1000, (first+last)/2)
				// A complete button can still have its level clipped by the list header.
				if ancientLevelRegion(screen, point).Min.Y >= top {
					rows = append(rows, point)
				}
			}
			first = -1
		}
	}
	return rows
}
func ancientLevelRegion(screen image.Image, p image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*35/1280, p.Y-b.Dy()*100/1000, b.Min.X+b.Dx()*195/1280, p.Y-b.Dy()*58/1000)
}
func ancientNameRegion(screen image.Image, p image.Point) image.Rectangle {
	b := screen.Bounds()
	return image.Rect(b.Min.X+b.Dx()*170/1280, p.Y-b.Dy()*100/1000, b.Min.X+b.Dx()*570/1280, p.Y-b.Dy()*58/1000)
}

type ancientScreenRow struct {
	name, level string
	point       image.Point
}
type ancientNameAnchor struct {
	name string
	y    int
}
type ancientObservation struct {
	frame     gameFrame
	souls     string
	rows      []ancientScreenRow
	anchors   []ancientNameAnchor
	namesOnly bool
	okay      bool
}

func ancientScrollArrow(screen image.Image, direction int) (image.Point, bool, error) {
	r, name := image.Rect(576, 276, 596, 294), "ancients/scroll-up.png"
	if direction > 0 {
		r, name = image.Rect(576, 699, 596, 717), "ancients/scroll-down.png"
	}
	found, err := matchControl(screen, r, name)
	box := controlRect(screen, r)
	return box.Min.Add(box.Size().Div(2)), found, err
}

func readAncientNames(ctx context.Context, frame gameFrame) (ancientObservation, error) {
	out := ancientObservation{frame: frame, namesOnly: true}
	if !frame.context.ancients {
		return out, nil
	}
	b := frame.image.Bounds()
	region := controlRect(frame.image, image.Rect(170, 275, 570, 699))
	first, last := -1, -1
	gap := max(2, b.Dy()/200)
	for y := region.Min.Y; y < region.Max.Y+gap; y++ {
		pink := 0
		if y < region.Max.Y {
			for x := region.Min.X; x < region.Max.X; x++ {
				r, g, bl := rgb(frame.image.At(x, y))
				if r > 150 && bl > 150 && g < 140 {
					pink++
				}
			}
		}
		if pink >= max(3, b.Dx()/1000) {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first < 0 || y-last <= gap {
			continue
		}
		if last-first >= b.Dy()/100 && last-first < b.Dy()/25 {
			padding := max(gap, b.Dy()/100)
			crop := image.Rect(region.Min.X, first-padding, region.Max.X, last+padding+1).Intersect(b)
			raw, err := readGameText(ctx, frame.image, crop, max(1, 2560/b.Dx()), 7, -180, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ,'")
			if err != nil {
				return out, err
			}
			name := strings.TrimSpace(strings.Split(raw, ",")[0])
			if name != "" {
				out.anchors = append(out.anchors, ancientNameAnchor{name, (first + last) / 2})
			}
		}
		first = -1
	}
	return out, nil
}

func readAncientObservation(ctx context.Context, frame gameFrame) (ancientObservation, error) {
	out := ancientObservation{frame: frame}
	screen := frame.image
	if frame.context.ancientDialog {
		_, out.okay, _ = ancientControl(screen, 1)
		return out, nil
	}
	if !frame.context.ancients {
		return out, nil
	}
	names, err := readAncientNames(ctx, frame)
	if err != nil {
		return out, err
	}
	out.anchors = names.anchors
	raw, err := readGameText(ctx, screen, controlRect(screen, image.Rect(410, 172, 591, 199)), max(1, 2560/screen.Bounds().Dx()), 7, 180, "0123456789.eEabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
	if err != nil {
		return out, err
	}
	match := ascensionRewardLabel.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return out, fmt.Errorf("unreadable Ancient soul budget %q", strings.TrimSpace(raw))
	}
	out.souls = match[1]
	for _, point := range ancientButtons(screen) {
		var name string
		for _, anchor := range out.anchors {
			if anchor.y >= ancientNameRegion(screen, point).Min.Y && anchor.y < ancientNameRegion(screen, point).Max.Y {
				name = anchor.name
				break
			}
		}
		if name == "" {
			continue
		}
		raw, err := readGameText(ctx, screen, ancientLevelRegion(screen, point), max(2, 2560/screen.Bounds().Dx()), 7, 0, "0123456789.eElLvViI")
		if err != nil {
			return out, err
		}
		raw = ancientLevelLabel.ReplaceAllString(strings.TrimSpace(raw), "")
		if _, err := ancientcalc.Value(raw); err != nil {
			raw = ""
		}
		out.rows = append(out.rows, ancientScreenRow{name, raw, point})
	}
	return out, nil
}
