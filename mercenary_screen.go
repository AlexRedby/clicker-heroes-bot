package main

import (
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gocv.io/x/gocv"
	xdraw "golang.org/x/image/draw"
)

// References come from the user's full-screen game frames.
//
//go:embed assets/mercenary-title.png
var mercenaryTitlePNG []byte

//go:embed assets/mercenary-tab.png
var mercenaryTabPNG []byte
var mercenaryTitleImage, mercenaryTabImage decodedPNG
var mercenaryTabScale struct {
	sync.Mutex
	size  image.Point
	image image.Image
}

func mercenaryTemplate(screen image.Image, region image.Rectangle, reference *decodedPNG, data []byte, originalSize image.Point) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	region = region.Intersect(screen.Bounds())
	crop := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
	draw.Draw(crop, crop.Bounds(), screen, region.Min, draw.Src)
	source, err := reference.get(data)
	if err != nil {
		return false
	}
	scene, err := gocv.ImageToMatRGB(crop)
	if err != nil {
		return false
	}
	defer scene.Close()
	icon, err := gocv.ImageToMatRGB(source)
	if err != nil {
		return false
	}
	defer icon.Close()
	size := image.Pt(max(1, screen.Bounds().Dx()*originalSize.X/2560), max(1, screen.Bounds().Dy()*originalSize.Y/1440))
	if region.Dx() < size.X || region.Dy() < size.Y {
		return false
	}
	score, err := templateScore(scene, icon, size)
	return err == nil && score >= .87
}

func mercenaryTabSelected(screen image.Image) bool {
	if screen == nil {
		return false
	}
	b := screen.Bounds()
	region := image.Rectangle{Min: mercenaryPoint(b, 370, 150), Max: mercenaryPoint(b, 385, 210)}
	gold, total := 0, 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			r, g, blue := rgb(screen.At(x, y))
			total++
			if r >= 160 && g >= 100 && g <= 145 && blue <= 60 {
				gold++
			}
		}
	}
	return total > 0 && gold*5 >= total
}

func mercenaryQuestDialog(screen image.Image) bool {
	if screen == nil {
		return false
	}
	b := screen.Bounds()
	// Match the specific title, not just a cream modal with purple text.
	return mercenaryTemplate(screen, image.Rectangle{Min: mercenaryPoint(b, 278, 91), Max: mercenaryPoint(b, 722, 131)}, &mercenaryTitleImage, mercenaryTitlePNG, image.Pt(1105, 43))
}

func mercenaryNotification(screen image.Image) bool {
	if screen == nil || screen.Bounds().Dx() < 640 || screen.Bounds().Dy() < 360 {
		return false
	}
	b := screen.Bounds()
	source, err := mercenaryTabImage.get(mercenaryTabPNG)
	if err != nil {
		return false
	}
	// Fixed opaque skull patch: scenery never enters the comparison. The left
	// strip must still match the unselected plate; a changed skull prompts a
	// roster check, whose Collect/Start Quest buttons decide what to do.
	region := image.Rect(b.Min.X+b.Dx()*936/2560, b.Min.Y+b.Dy()*272/1440,
		b.Min.X+b.Dx()*1024/2560, b.Min.Y+b.Dy()*320/1440)
	var reference image.Image = source
	if region.Size() != source.Bounds().Size() {
		mercenaryTabScale.Lock()
		if mercenaryTabScale.size != region.Size() {
			scaled := image.NewRGBA(image.Rect(0, 0, region.Dx(), region.Dy()))
			xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), source, source.Bounds(), draw.Src, nil)
			mercenaryTabScale.size, mercenaryTabScale.image = region.Size(), scaled
		}
		reference = mercenaryTabScale.image
		mercenaryTabScale.Unlock()
	}
	anchorChanged, anchors, changed, total := 0, 0, 0, 0
	for y := 3 * region.Dy() / 48; y < 44*region.Dy()/48; y++ {
		for x := 3 * region.Dx() / 88; x < 84*region.Dx()/88; x++ {
			r, g, blue := rgb(reference.At(x, y))
			cr, cg, cb := rgb(screen.At(region.Min.X+x, region.Min.Y+y))
			diff := absDiff(r, cr)+absDiff(g, cg)+absDiff(blue, cb) > 100
			if x < 19*region.Dx()/88 {
				anchors++
				if diff {
					anchorChanged++
				}
			} else {
				total++
				if diff {
					changed++
				}
			}
		}
	}
	return anchorChanged*5 <= anchors && changed*100 >= total*12
}

func mercenaryScrollbar(screen image.Image) (point image.Point, found, top, bottom bool) {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	x := b.Min.X + w*458/1000
	trackTop, trackBottom := b.Min.Y+h*388/1000, b.Min.Y+h*965/1000
	first, last, bestFirst, bestLast := -1, -1, -1, -1
	for y := trackTop; y <= trackBottom+max(4, h/100); y++ {
		bright := false
		if y < trackBottom {
			r, g, blue := rgb(screen.At(x, y))
			bright = r > 225 && g > 190 && blue < 100
		}
		if bright {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 && y-last > max(4, h/100) {
			if last-first > bestLast-bestFirst {
				bestFirst, bestLast = first, last
			}
			first = -1
		}
	}
	if bestLast-bestFirst < h/30 {
		return image.Point{}, false, false, false
	}
	return image.Pt(x, (bestFirst+bestLast)/2), true, bestFirst-trackTop < h/100, trackBottom-bestLast < h/100
}

// Cards can move when the roster scrolls. Only complete cards produce targets.
func mercenaryRows(screen image.Image) []image.Point {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	first, last := -1, -1
	var points []image.Point
	for y := b.Min.Y + h*355/1000; y < b.Max.Y; y++ {
		// The right card edge stays clear of reward icons and timer bars.
		r, g, blue := rgb(screen.At(b.Min.X+w*425/1000, y))
		brown := r >= 60 && r <= 160 && g >= 35 && g <= 115 && blue >= 25 && blue <= 95 && r-g >= 20 && g-blue >= 5
		if brown {
			if first < 0 {
				first = y
			}
			last = y
			continue
		}
		if first >= 0 && y-last > max(2, h/400) {
			if last-first >= h*115/1000 && last-first <= h*155/1000 {
				points = append(points, image.Pt(b.Min.X+w*300/1000, (first+last)/2))
			}
			first = -1
		}
	}
	return points
}

var mercenaryDurationText = regexp.MustCompile(`(?i)\btime:\s*(5|15|30)\s+minutes?\s*$|\btime:\s*(1|2|4|8)\s+hours?\s*$|\btime:\s*(1|2)\s+days?\s*$`)
var mercenaryRewardText = regexp.MustCompile(`(?i)^reward:\s*(.+)$`)
var mercenaryTimer = regexp.MustCompile(`^[0-9]{1,2}:[0-5][0-9](?::[0-5][0-9])?$`)

func parseMercenaryDuration(raw string) (time.Duration, bool) {
	raw = strings.Join(strings.Fields(raw), " ")
	match := mercenaryDurationText.FindStringSubmatch(raw)
	if match == nil {
		return 0, false
	}
	for i, unit := range []time.Duration{time.Minute, time.Hour, 24 * time.Hour} {
		if match[i+1] != "" {
			n, _ := strconv.Atoi(match[i+1])
			return time.Duration(n) * unit, true
		}
	}
	return 0, false
}

func parseMercenaryReward(raw string) (string, bool) {
	match := mercenaryRewardText.FindStringSubmatch(strings.ToLower(strings.Join(strings.Fields(raw), " ")))
	if match == nil {
		return "", false
	}
	words := strings.FieldsFunc(match[1], func(r rune) bool { return r < 'a' || r > 'z' })
	for _, word := range words {
		switch word {
		case "gold":
			return "gold", true
		case "ruby", "rubies":
			return "rubies", true
		case "relic", "relics":
			return "relics", true
		case "souls":
			return "hero souls", true
		case "skills":
			return "skills", true
		case "mercenary":
			return "recruitment", true
		}
	}
	return "", false
}

func readMercenaryObservation(ctx context.Context, frame gameFrame) (mercenaryObservation, error) {
	out := mercenaryObservation{frame: frame, selected: -1}
	if frame.image == nil {
		return out, fmt.Errorf("nil mercenary screen")
	}
	screen := frame.image
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	if !mercenaryQuestDialog(screen) {
		if !mercenaryTabSelected(screen) {
			out.notify = mercenaryNotification(screen)
			return out, nil
		}
		out.thumb, out.thumbFound, out.top, out.bottom = mercenaryScrollbar(screen)
		rows := mercenaryRows(screen)
		if len(rows) == 0 {
			return out, fmt.Errorf("mercenary roster cards not recognized")
		}
		for _, point := range rows {
			region := image.Rect(b.Min.X+w*237/1000, point.Y-h*27/1000, b.Min.X+w*371/1000, point.Y+h*27/1000)
			raw, err := readGameText(ctx, screen, region, max(2, 4096/w), 7, -170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789: ")
			if err != nil {
				return out, err
			}
			label := strings.TrimSpace(raw)
			compact := strings.ToLower(strings.Join(strings.Fields(label), ""))
			switch {
			case compact == "collect":
				out.collect = append(out.collect, point)
			case compact == "startquest":
				out.start = append(out.start, point)
			case mercenaryTimer.MatchString(label):
				out.running = append(out.running, point)
			case strings.Contains(compact, "revive") || strings.Contains(compact, "bury"): // Never spend rubies or remove a mercenary.
			default:
				return out, fmt.Errorf("unreadable mercenary row button %q", label)
			}
		}
		out.readable = true
		return out, nil
	}
	out.quests = make([]mercenaryQuest, 4)
	bright := 0
	for i, start := range []int{202, 353, 505, 656} {
		// The selected card stays brown; all other cards turn almost black.
		r, g, blue := rgb(screen.At(b.Min.X+w*58/100, b.Min.Y+h*(start+100)/1000))
		if r < 60 || g < 35 || blue < 25 {
			continue
		}
		bright++
		out.selected = i
		region := image.Rectangle{Min: mercenaryPoint(b, 258, start+7), Max: mercenaryPoint(b, 595, start+130)}
		raw, err := readGameText(ctx, screen, region, max(2, 4096/w), 6, 170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+% .:")
		if err != nil {
			return out, err
		}
		lines := strings.Split(strings.TrimSpace(raw), "\n")
		timeLine := -1
		for j, line := range lines {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "time:") {
				timeLine = j
			}
		}
		if timeLine < 1 {
			return out, fmt.Errorf("unreadable mercenary quest %d: %q", i, raw)
		}
		reward, ok := parseMercenaryReward(strings.Join(lines[:timeLine], " "))
		if !ok {
			return out, fmt.Errorf("unrecognized mercenary reward %q", raw)
		}
		duration, ok := parseMercenaryDuration(lines[timeLine])
		if !ok {
			return out, fmt.Errorf("unrecognized mercenary duration %q", raw)
		}
		out.quests[i] = mercenaryQuest{reward: reward, duration: duration, point: mercenaryPoint(b, 400, start+70)}
	}
	switch bright {
	case 4:
		out.selected = -1
	case 1:
		raw, err := readGameText(ctx, screen, image.Rectangle{Min: mercenaryPoint(b, 668, 468), Max: mercenaryPoint(b, 754, 527)}, max(2, 4096/w), 7, 170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ")
		if err != nil {
			return out, err
		}
		if strings.TrimSpace(raw) != "Okay" {
			return out, fmt.Errorf("unreadable mercenary Okay button %q", raw)
		}
		out.okay = mercenaryPoint(b, 710, 500)
	default:
		return out, fmt.Errorf("ambiguous mercenary selection: %d bright cards", bright)
	}
	out.readable = true
	return out, nil
}
