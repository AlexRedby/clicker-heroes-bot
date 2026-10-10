package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func ascensionUnlockFixture(t *testing.T, path string, y int, available bool) gameFrame {
	t.Helper()
	f := startupFrame(t, path)
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, f.image.Bounds().Min, draw.Src)
	point := image.Pt(s.Bounds().Dx()*8/100, y)
	region := ascensionUnlockNameRegion(s, point)
	draw.Draw(s, region, image.NewUniform(color.RGBA{255, 218, 80, 255}), image.Point{}, draw.Src)
	// Synthetic target text reuses native button/list geometry.
	name := image.NewRGBA(image.Rect(0, 0, 9*7, 15))
	draw.Draw(name, name.Bounds(), image.NewUniform(color.RGBA{255, 218, 80, 255}), image.Point{}, draw.Src)
	d := font.Drawer{Dst: name, Src: image.NewUniform(color.RGBA{128, 32, 200, 255}), Face: basicfont.Face7x13, Dot: fixed.P(0, 12)}
	d.DrawString("Amenhotep")
	dst := image.Rect(region.Max.X-name.Bounds().Dx()*2-8, region.Min.Y+8, region.Max.X-8, region.Min.Y+8+name.Bounds().Dy()*2)
	xdraw.NearestNeighbor.Scale(s, dst, name, name.Bounds(), draw.Src, nil)
	if !available {
		for _, band := range findHeroButtonBands(s, true) {
			if point.Y < band.Min.Y || point.Y >= band.Max.Y {
				continue
			}
			for row := band.Min.Y; row < band.Max.Y; row++ {
				for x := band.Min.X; x < band.Max.X; x++ {
					r, g, b := rgb(s.At(x, row))
					if b > 150 && b > r+40 && b >= g-10 && g > 90 {
						s.Set(x, row, color.RGBA{45, 60, 70, 255})
					}
				}
			}
		}
	}
	f.image = s
	return f
}

func TestAscensionUnlockTargetsOnlyAmenhotep(t *testing.T) {
	requireAncientOCR(t)
	for _, tc := range []struct {
		name, path              string
		y, level                int
		available, owned, ready bool
	}{
		{"level missing", "../../testdata/hero-startup-fisherman-after.png", 943, 149, true, true, false},
		{"ready", "../../testdata/hero-startup-fisherman-after.png", 943, 150, true, true, true},
		{"unaffordable", "../../testdata/hero-startup-fisherman-after.png", 943, 149, false, true, false},
		{"unowned", "../../testdata/hero-startup-fisherman-before.png", 966, 0, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := ascensionUnlockFixture(t, tc.path, tc.y, tc.available)
			out, err := readAscensionUnlockObservation(context.Background(), f, heroReaders{level: func(_ context.Context, _ image.Image, p image.Point) (int, error) {
				if absDiff(p.Y, tc.y) > 3 {
					t.Fatalf("read unrelated hero at %v", p)
				}
				return tc.level, nil
			}}, false)
			if err != nil || !out.hero.found || out.hero.owned != tc.owned || out.ready != tc.ready || out.unavailable != !tc.available {
				t.Fatalf("out=%+v error=%v", out, err)
			}
			a, ok := out.action()
			if ok != (tc.available && !tc.ready) || ok && (a.kind != buyHero || absDiff(a.point.Y, tc.y) > 3 || !ascensionUnlockHeroStable(a.hero, f)) {
				t.Fatalf("action=%+v allowed=%t", a, ok)
			}
		})
	}
}

func TestAscensionUnlockNativeNamesAndSearch(t *testing.T) {
	requireAncientOCR(t)
	white := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	whiteName, whiteErr := readAscensionUnlockName(context.Background(), white.image, ascensionUnlockNameRegion(white.image, image.Pt(204, 865)))
	if whiteErr != nil || whiteName != "Treebeast" {
		t.Fatalf("native white name=%q error=%v", whiteName, whiteErr)
	}
	f := startupFrame(t, "../../testdata/hero-startup-fisherman-after.png")
	name, err := readAscensionUnlockName(context.Background(), f.image, ascensionUnlockNameRegion(f.image, image.Pt(204, 943)))
	if err != nil || name != "The Wandering Fisherman" {
		t.Fatalf("native name=%q error=%v", name, err)
	}
	for _, fromTop := range []bool{false, true} {
		out, err := readAscensionUnlockObservation(context.Background(), f, heroReaders{}, fromTop)
		a, ok := out.action()
		if err != nil || out.hero.found || out.ready || !ok || a.kind != scrollHeroes || (a.target.Y > a.point.Y) != fromTop {
			t.Fatalf("fromTop=%t out=%+v action=%+v ok=%t error=%v", fromTop, out, a, ok, err)
		}
	}
}

func TestAscensionUnlockRejectsChangedRowAndGeneration(t *testing.T) {
	requireAncientOCR(t)
	f := ascensionUnlockFixture(t, "../../testdata/hero-startup-fisherman-after.png", 943, true)
	out, err := readAscensionUnlockObservation(context.Background(), f, heroReaders{level: func(context.Context, image.Image, image.Point) (int, error) { return 149, nil }}, false)
	if err != nil || !out.hero.found || !ascensionUnlockHeroStable(out.hero, f) {
		t.Fatalf("target not established: %+v %v", out, err)
	}
	changed := f
	changed.generation++
	if ascensionUnlockHeroStable(out.hero, changed) {
		t.Fatal("old generation authorized target purchase")
	}
	changed = f
	s := image.NewRGBA(f.image.Bounds())
	draw.Draw(s, s.Bounds(), f.image, f.image.Bounds().Min, draw.Src)
	draw.Draw(s, ascensionUnlockNameRegion(s, out.hero.button), image.NewUniform(color.Black), image.Point{}, draw.Src)
	changed.image = s
	if ascensionUnlockHeroStable(out.hero, changed) {
		t.Fatal("changed/covered target name authorized purchase")
	}
	if a, ok := (ascensionUnlockObservation{hero: heroObservation{frame: f, x1: true}, unavailable: true}).action(); ok {
		t.Fatalf("unavailable target authorized %+v", a)
	}
}
