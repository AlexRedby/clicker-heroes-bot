package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/draw"

	"gocv.io/x/gocv"
)

type skillState struct {
	Known, Ready, Active, Energized bool
}

func templateScore(scene, source gocv.Mat, size image.Point) (float32, error) {
	reference, result, mask := gocv.NewMat(), gocv.NewMat(), gocv.NewMat()
	defer reference.Close()
	defer result.Close()
	defer mask.Close()
	if err := gocv.Resize(source, &reference, size, 0, 0, gocv.InterpolationArea); err != nil {
		return 0, err
	}
	if err := gocv.MatchTemplate(scene, reference, &result, gocv.TmCcoeffNormed, mask); err != nil {
		return 0, err
	}
	_, score, _, _ := gocv.MinMaxLoc(result)
	return score, nil
}

// Upper icon strips exclude changing cooldown text and outer glow.
//
//go:embed assets/skill-icons.png
var skillIconsPNG []byte

func skillButton(screen image.Image, index int) image.Point {
	b := screen.Bounds()
	return b.Min.Add(image.Pt(b.Dx()*520/1000, b.Dy()*(283+81*index)/1000))
}

func skillCooldownVisible(screen image.Image, center image.Point) bool {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	// The white clock crosses the middle of the icon on a red strip.
	region := image.Rect(center.X-w*21/1000, center.Y-h*20/1000, center.X+w*21/1000, center.Y+h*20/1000).Intersect(b)
	for y := region.Min.Y; y < region.Max.Y; y++ {
		white, red, redRun, maxRedRun := 0, 0, 0, 0
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, blue := rgb(screen.At(x, y))
			if min(r, g, blue) > 170 && max(r, g, blue)-min(r, g, blue) < 100 {
				white++
			}
			if r > 140 && g < 100 && blue < 100 && r > g*3/2 {
				red++
				redRun++
				maxRedRun = max(maxRedRun, redRun)
			} else {
				redRun = 0
			}
		}
		if white >= max(5, w*25/2560) && red >= max(5, w*25/2560) && maxRedRun >= max(2, w*5/2560) {
			return true
		}
	}
	return false
}

func skillGlow(screen image.Image, center image.Point) (active, energized bool) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	gold, pink, samples := 0, 0, 0
	// Compare the outer halo with the nearby background, excluding the orange border.
	for y := center.Y - h*25/1000; y <= center.Y+h*25/1000; y += max(1, h/500) {
		for _, offsets := range [][2]int{{-20, -26}, {-21, -26}, {21, 27}, {22, 27}} {
			r, g, blue := rgb(screen.At(center.X+w*offsets[0]/1000, y))
			br, bg, bb := rgb(screen.At(center.X+w*offsets[1]/1000, y))
			samples++
			if min(r, g) > 200 && min(r, g)-blue > 80 && min(r, g)-blue-(min(br, bg)-bb) > 35 {
				gold++
			}
			if r > 180 && blue > 125 && r > g*6/5 && r-g-(br-bg) > 35 {
				pink++
			}
		}
	}
	energized = pink*5 > samples
	return energized || gold*5 > samples, energized
}

func readSkillStates(ctx context.Context, screen image.Image) ([9]skillState, error) {
	var states [9]skillState
	if screen == nil {
		return states, fmt.Errorf("nil skill screen")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 500 || h < 500 {
		return states, nil
	}
	atlas, err := gocv.IMDecode(skillIconsPNG, gocv.IMReadGrayScale)
	if err != nil {
		return states, fmt.Errorf("decode skill icons: %w", err)
	}
	defer atlas.Close()
	for i := range states {
		if err := ctx.Err(); err != nil {
			return states, err
		}
		center := skillButton(screen, i)
		// A visible clock positively identifies an unlocked skill on cooldown.
		// Its dimmed artwork changes as cooldown progresses, so do not match it.
		if skillCooldownVisible(screen, center) {
			states[i].Known = true
			if i != 5 && i < 7 {
				states[i].Active, states[i].Energized = skillGlow(screen, center)
			}
			continue
		}
		region := image.Rect(center.X-w*17/1000, center.Y-h*29/1000, center.X+w*17/1000, center.Y-h*9/1000).Intersect(b)
		crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
		draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
		color, err := gocv.ImageToMatRGB(crop)
		if err != nil {
			return states, err
		}
		gray := gocv.NewMat()
		strip := atlas.Region(image.Rect(0, i*11, 64, (i+1)*11))
		err = gocv.CvtColor(color, &gray, gocv.ColorBGRToGray)
		if err == nil {
			var score float32
			score, err = templateScore(gray, strip, image.Pt(max(1, w*64/2560), max(1, h*11/1440)))
			if err == nil {
				states[i].Known = score >= 0.85
			}
		}
		strip.Close()
		color.Close()
		gray.Close()
		if err != nil {
			return states, fmt.Errorf("recognize skill %d: %w", i+1, err)
		}
		if states[i].Known {
			states[i].Ready = true
			// Ritual and utility buttons have no ongoing timed buff.
			if i != 5 && i < 7 {
				states[i].Active, states[i].Energized = skillGlow(screen, center)
			}
		}
	}
	return states, nil
}
