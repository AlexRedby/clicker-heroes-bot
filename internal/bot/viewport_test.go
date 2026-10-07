package bot

import (
	"image"
	"image/color"
	"testing"
)

func TestViewportGeometry(t *testing.T) {
	for _, scale := range []struct{ pixels, points int }{{1, 1}, {2, 1}, {5, 4}, {3, 2}} {
		for _, origin := range []image.Point{{0, 0}, {-1000, -600}, {400, -600}} {
			capture := image.Rect(7, 9, 7+1000*scale.pixels, 9+600*scale.pixels)
			desktop := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(1000*scale.points, 600*scale.points))}
			g := viewportGeometry{Scene: nativeScene{Window: "game", Desktop: desktop}, Capture: capture,
				ROI: image.Rect(7+100*scale.pixels, 9+50*scale.pixels, 7+900*scale.pixels, 9+550*scale.pixels)}
			point := image.Pt(300*scale.pixels, 200*scale.pixels)
			got, err := g.point(point)
			want := origin.Add(image.Pt(400*scale.points, 250*scale.points))
			if err != nil || got != want {
				t.Fatalf("scale=%v got=%v want=%v err=%v", scale, got, want, err)
			}
			for _, p := range []image.Point{{-1, 0}, {g.ROI.Dx(), 0}, {0, g.ROI.Dy()}} {
				if _, err := g.point(p); err == nil {
					t.Fatalf("accepted outside %v", p)
				}
			}
			g.Scene.Bounds = image.Rectangle{Min: origin.Add(image.Pt(100*scale.points, 50*scale.points)), Max: origin.Add(image.Pt(900*scale.points, 550*scale.points))}
			roi, err := windowPixels(g.Scene, capture)
			if err != nil || roi != g.ROI {
				t.Fatalf("ROI=%v want=%v err=%v", roi, g.ROI, err)
			}
			g.Scene.Bounds = g.Scene.Bounds.Add(image.Pt(-2000, 0))
			if _, err := windowPixels(g.Scene, capture); err == nil {
				t.Fatal("offscreen window accepted")
			}
		}
	}
	if _, err := (viewportGeometry{}).point(image.Point{}); err == nil {
		t.Fatal("missing geometry accepted")
	}
	if _, err := windowPixels(nativeScene{}, image.Rect(0, 0, 10, 10)); err == nil {
		t.Fatal("empty desktop accepted")
	}
}

func TestViewportCompactCrop(t *testing.T) {
	source := image.NewRGBA(image.Rect(11, 7, 31, 22))
	roi := image.Rect(14, 9, 21, 15)
	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			source.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 99, 255})
		}
	}
	crop := compactCrop(source.SubImage(image.Rect(12, 8, 30, 20)), roi)
	if crop.Bounds() != (image.Rectangle{Max: roi.Size()}) || crop.Stride != crop.Bounds().Dx()*4 {
		t.Fatal("crop is not compact and zero-origin")
	}
	for y := 0; y < crop.Bounds().Dy(); y++ {
		for x := 0; x < crop.Bounds().Dx(); x++ {
			if crop.At(x, y) != source.At(x+roi.Min.X, y+roi.Min.Y) {
				t.Fatal("crop pixels shifted")
			}
		}
	}
}

func TestViewportCandidates(t *testing.T) {
	full := image.Rect(20, 30, 1300, 750)
	if got := viewportCandidates(full); len(got) != 1 || got[0] != full {
		t.Fatalf("full-screen candidates=%v", got)
	}
	decorated := image.Rect(20, 8, 1300, 750)
	got := viewportCandidates(decorated)
	if got[len(got)-1] != full {
		t.Fatalf("title bar candidate=%v", got)
	}
	if len(viewportCandidates(image.Rectangle{})) != 0 {
		t.Fatal("empty window candidates")
	}
}
