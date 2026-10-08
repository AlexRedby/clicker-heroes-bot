package bot

import (
	"context"
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/vision"
	xdraw "golang.org/x/image/draw"
)

type gildRedistributionObservation struct {
	frame                       gameFrame
	roster, targetFound, bottom bool
	entry, close, down, target  image.Point
	entryRegion, targetRegion   image.Rectangle
	targetID                    int
	rosterHash                  [32]byte
}

func readGildRedistribution(ctx context.Context, frame gameFrame, target ancientcalc.GildHero) (gildRedistributionObservation, error) {
	u := gildRedistributionObservation{frame: frame}
	if frame.image == nil || !frame.context.known {
		return u, nil
	}
	s := frame.image
	if bootstrapHeroes(frame.context) {
		thumb, height, known := heroScrollbarThumb(s)
		u.bottom = known && heroScrollbarAtEnd(s, thumb, height)
		r, found, err := vision.FindControl(s, image.Rect(65, 220, 168, 705), image.Pt(94, 35), "gilds/entry.png")
		if found {
			u.entryRegion = r
			u.entry = u.entryRegion.Min.Add(u.entryRegion.Size().Div(2))
		}
		return u, err
	}
	if frame.context.modal != gildRosterModal {
		return u, nil
	}
	u.roster = true
	point, found, err := gildActionPoint(frame)
	if err != nil || !found {
		return u, err
	}
	u.close = point
	found, err = vision.MatchControl(s, image.Rect(1120, 493, 1142, 516), "gilds/scroll-down.png")
	if err != nil {
		return u, err
	}
	if found {
		u.down = vision.Rect(s, image.Rect(1131, 504, 1132, 505)).Min
	}
	grid := vision.Rect(s, image.Rect(143, 129, 1098, 515))
	var pixels []byte
	for y := grid.Min.Y; y < grid.Max.Y; y += max(1, s.Bounds().Dy()/720) {
		for x := grid.Min.X; x < grid.Max.X; x += max(1, s.Bounds().Dx()/1280) {
			r, g, b := rgb(s.At(x, y))
			pixels = append(pixels, byte(r), byte(g), byte(b))
		}
	}
	u.rosterHash = sha256.Sum256(pixels)
	// Names are read inside the observed grid, never inferred from tile index.
	// Cid is absent from the roster; unlocked and locked heroes can share it.
	if target.ID < 1 || target.Name == "" {
		return u, nil
	}
	for col := 0; col < 4; col++ {
		r := vision.Rect(s, image.Rect(205+col*240, 130, 371+col*240, 492))
		mask := gildNameMask(s, r)
		scale := max(1, 3840/s.Bounds().Dx())
		up := image.NewGray(image.Rect(0, 0, mask.Bounds().Dx()*scale+20, mask.Bounds().Dy()*scale+20))
		draw.Draw(up, up.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
		xdraw.NearestNeighbor.Scale(up, up.Bounds().Inset(10), mask, mask.Bounds(), draw.Src, nil)
		raw, err := readTextImage(ctx, up, 6, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz,' ", "tsv")
		if err != nil {
			return u, err
		}
		region, found := gildTargetTextRegion(raw, r, scale, target.Name)
		if !found {
			continue
		}
		if u.targetFound {
			u.targetFound = false
			return u, nil
		}
		u.targetFound, u.targetID, u.targetRegion = true, target.ID, region
		// Click the name itself, which is part of the hero tile. This avoids
		// the changing gild counter and every ruby control below the viewport.
		u.target = region.Min.Add(region.Size().Div(2))
	}
	return u, nil
}

func gildPurple(c color.Color) bool {
	r, g, b := rgb(c)
	return b > 130 && r > 40 && r < 180 && g < 100 && b > r+40
}

func gildNameMask(s image.Image, r image.Rectangle) *image.Gray {
	m := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			v := uint8(255)
			if gildPurple(s.At(r.Min.X+x, r.Min.Y+y)) {
				v = 0
			}
			m.SetGray(x, y, color.Gray{Y: v})
		}
	}
	return m
}

func gildNameKey(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		if r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, name)
}

// TSV word coordinates bind an exact full name to its own text rectangle.
// Two-line names must remain together; no fuzzy spelling or prefix matches.
func gildTargetTextRegion(raw string, crop image.Rectangle, scale int, name string) (image.Rectangle, bool) {
	type word struct {
		text string
		r    image.Rectangle
	}
	var words []word
	for _, row := range strings.Split(raw, "\n") {
		f := strings.Split(row, "\t")
		if len(f) != 12 || f[0] != "5" || strings.TrimSpace(f[11]) == "" {
			continue
		}
		var n [4]int
		valid := true
		for i := range n {
			v, e := strconv.Atoi(f[6+i])
			n[i] = v
			valid = valid && e == nil
		}
		r := image.Rect(n[0], n[1], n[0]+n[2], n[1]+n[3])
		if !valid || r.Empty() || !r.In(image.Rect(10, 10, crop.Dx()*scale+10, crop.Dy()*scale+10)) {
			continue
		}
		words = append(words, word{gildNameKey(f[11]), r})
	}
	want := gildNameKey(name)
	result, found := image.Rectangle{}, false
	for i := range words {
		text, r := "", image.Rectangle{}
		for j := i; j < len(words) && j < i+8; j++ {
			text += words[j].text
			r = r.Union(words[j].r)
			if r.Dy() > 32*scale || !strings.HasPrefix(want, text) {
				break
			}
			if text != want {
				continue
			}
			if found {
				return image.Rectangle{}, false
			}
			result = image.Rect(crop.Min.X+(r.Min.X-10)/scale, crop.Min.Y+(r.Min.Y-10)/scale, crop.Min.X+(r.Max.X-10+scale-1)/scale, crop.Min.Y+(r.Max.Y-10+scale-1)/scale).Inset(-2)
			found = true
			break
		}
	}
	return result, found
}

func gildNameStable(before, after image.Image, region image.Rectangle) bool {
	if before == nil || after == nil || before.Bounds() != after.Bounds() || region.Empty() || !region.In(before.Bounds()) {
		return false
	}
	ink := 0
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			a, b := gildPurple(before.At(x, y)), gildPurple(after.At(x, y))
			if a != b {
				return false
			}
			if a {
				ink++
			}
		}
	}
	return ink > 8
}
