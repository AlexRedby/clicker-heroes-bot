package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"math"
	"regexp"
	"strconv"
	"strings"

	"gocv.io/x/gocv"
	xdraw "golang.org/x/image/draw"
)

//go:embed assets/progression-icons.png
var progressionIconsPNG []byte
var zoneLabel = regexp.MustCompile(`(?i)[li]v[li]\s*([0-9]+)\s*$`)

func progressionMode(screen image.Image) (known, enabled bool, err error) {
	if screen == nil || screen.Bounds().Dx() < 500 || screen.Bounds().Dy() < 500 {
		return false, false, nil
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	center := b.Min.Add(image.Pt(w*972/1000, h*290/1000))
	region := image.Rect(center.X-w*18/1000, center.Y-h*34/1000, center.X+w*18/1000, center.Y+h*34/1000).Intersect(b)
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false, false, err
	}
	defer scene.Close()
	atlasImage, err := progressionIconsImage.get(progressionIconsPNG)
	if err != nil {
		return false, false, err
	}
	atlas, err := gocv.ImageToMatRGB(atlasImage)
	if err != nil {
		return false, false, fmt.Errorf("convert progression icons: %w", err)
	}
	defer atlas.Close()
	var scores [2]float32
	for i := range scores {
		icon := atlas.Region(image.Rect(i*48, 0, (i+1)*48, 48))
		scores[i], err = templateScore(scene, icon, image.Pt(max(1, w*48/2560), max(1, h*48/1440)))
		icon.Close()
		if err != nil {
			return false, false, err
		}
	}
	best := max(scores[0], scores[1])
	margin := math.Abs(float64(scores[0] - scores[1]))
	return best >= 0.85 && margin >= 0.1, scores[0] > scores[1], nil
}

var progressionIconsImage decodedPNG

func progressionBuffs(states [9]skillState) uint8 {
	var buffs uint8
	for bit, key := range []int{3, 7} {
		state := states[key-1]
		if state.Known && state.Active {
			buffs |= 1 << bit
			if state.Energized {
				buffs |= 1 << (bit + 2)
			}
		}
	}
	return buffs
}

func fullCombatActive(states [9]skillState) bool {
	for _, key := range []int{1, 2, 3, 7} {
		if !states[key-1].Known || !states[key-1].Active {
			return false
		}
	}
	return true
}

func readProgressionState(ctx context.Context, screen image.Image, states [9]skillState, modeOnly bool) (progressionState, error) {
	known, enabled, err := progressionMode(screen)
	s := progressionState{Known: known, Enabled: enabled}
	if err != nil || !known {
		return s, err
	}
	if modeOnly {
		return s, nil
	}
	s.Zone, err = readProgressionZone(ctx, screen)
	if err != nil {
		return s, err
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	s.Buffs = progressionBuffs(states)
	s.FullCombat = fullCombatActive(states)
	// Only farm decisions and boss baselines need damage OCR.
	if !enabled || s.Zone%5 == 0 {
		// Preserve antialiasing: a binary mask can turn small-font e into a digit.
		region := image.Rect(b.Min.X+w*35/1000, b.Min.Y+h*243/1000, b.Min.X+w*150/1000, b.Min.Y+h*307/1000).Intersect(b)
		damage := image.NewRGBA(image.Rect(0, 0, region.Dx()*2, region.Dy()*2))
		xdraw.CatmullRom.Scale(damage, damage.Bounds(), screen, region, draw.Src, nil)
		raw, err := readTextImage(ctx, damage, 6, "0123456789.eE ")
		if err != nil {
			return s, err
		}
		lines := strings.Split(strings.TrimSpace(raw), "\n")
		if len(lines) != 2 || len(strings.Fields(lines[0])) == 0 || len(strings.Fields(lines[1])) == 0 {
			return s, fmt.Errorf("unreadable damage %q", strings.TrimSpace(raw))
		}
		dps, ok := parseGameNumber(strings.Fields(lines[0])[0])
		click, clickOK := parseGameNumber(strings.Fields(lines[1])[0])
		if !ok || !clickOK {
			return s, fmt.Errorf("unreadable damage %q", strings.TrimSpace(raw))
		}
		s.Damage, s.DamageKnown = max(dps, click), true
	}
	return s, nil
}

func readProgressionZone(ctx context.Context, screen image.Image) (int, error) {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := max(3, 8192/w)
	raw, err := readGameText(ctx, screen, image.Rect(b.Min.X+w*62/100, b.Min.Y+h*118/1000, b.Min.X+w*89/100, b.Min.Y+h*154/1000), scale, 7, -150, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
	if err != nil {
		return 0, err
	}
	match := zoneLabel.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return 0, fmt.Errorf("unreadable zone %q", strings.TrimSpace(raw))
	}
	zone, err := strconv.Atoi(match[1])
	if err != nil || zone <= 0 {
		return 0, fmt.Errorf("invalid zone %q", match[1])
	}
	return zone, nil
}
