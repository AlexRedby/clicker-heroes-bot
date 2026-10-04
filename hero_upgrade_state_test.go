package main

import (
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"testing"

	xdraw "golang.org/x/image/draw"
)

func TestReadHeroUpgradeState(t *testing.T) {
	for _, tc := range []struct {
		path              string
		y                 int
		visible, complete bool
	}{
		{"testdata/hero-economy-x1.png", 707, true, true},
		{"testdata/hero-economy-x1.png", 917, true, false},
		{"testdata/hero-startup-bottom-hire.png", 732, true, true},
		{"testdata/hero-startup-bottom-hire.png", 942, true, false},
		{"testdata/hero-startup-bottom-hire.png", 1152, true, false},
		{"testdata/hero-startup-bottom-hire.png", 1362, false, false},
	} {
		t.Run(tc.path+"/"+strconv.Itoa(tc.y), func(t *testing.T) {
			source := loadTestImage(t, tc.path)
			for _, width := range []int{1280, 1920, 2560} {
				s := source
				if width != 2560 {
					scaled := image.NewRGBA(image.Rect(0, 0, width, source.Bounds().Dy()*width/source.Bounds().Dx()))
					xdraw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), source, source.Bounds(), draw.Src, nil)
					s = scaled
				}
				point := image.Pt(s.Bounds().Dx()*8/100, tc.y*width/2560)
				complete, visible, err := readHeroUpgradeState(s, point)
				if err != nil || visible != tc.visible || complete != tc.complete {
					t.Fatalf("width=%d state=(%v,%v,%v), want=(%v,%v)", width, complete, visible, err, tc.complete, tc.visible)
				}
			}
		})
	}
}

func TestHeroUpgradeChecksNativeSmallFrameAndObstructions(t *testing.T) {
	s := loadTestImage(t, "testdata/hero-nongilded-successor.jpg")
	buttons := findHeroButtons(s, true)
	if len(buttons) != 2 {
		t.Fatalf("native buttons: %v", buttons)
	}
	for i, point := range buttons {
		complete, visible, err := readHeroUpgradeState(s, point)
		if err != nil || !visible || complete != (i == 0) {
			t.Fatalf("native1280 row%d at%v: complete=%t visible=%t err=%v", i, point, complete, visible, err)
		}
	}
	hire := findHeroButtons(s, false)
	if len(hire) != 1 {
		t.Fatalf("native hire: %v", hire)
	}
	complete, visible, err := readHeroUpgradeState(s, hire[0])
	if err != nil || complete || visible {
		t.Fatal("HIRE acquired upgrades", complete, visible, err)
	}
	source := loadTestImage(t, "testdata/hero-economy-x1.png")
	for _, kind := range []string{"missing check", "green artwork", "covered strip"} {
		t.Run(kind, func(t *testing.T) {
			modified := image.NewRGBA(source.Bounds())
			draw.Draw(modified, modified.Bounds(), source, source.Bounds().Min, draw.Src)
			region := image.Rect(392, 718, 434, 756)
			fill := color.RGBA{50, 50, 50, 255}
			if kind == "green artwork" {
				fill = color.RGBA{30, 220, 40, 255}
			}
			if kind == "covered strip" {
				region = image.Rect(380, 718, 710, 815)
			}
			draw.Draw(modified, region, image.NewUniform(fill), image.Point{}, draw.Src)
			complete, visible, err := readHeroUpgradeState(modified, image.Pt(204, 707))
			if err != nil || complete || kind == "covered strip" && visible {
				t.Fatalf("%s: complete=%t visible=%t %v", kind, complete, visible, err)
			}
		})
	}
}
