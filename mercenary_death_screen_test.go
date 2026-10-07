package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
)

func TestMercenaryDeadPortraitOnRealFrames(t *testing.T) {
	for _, divisor := range []int{1, 2, 4} {
		dead := mercenaryScaled(t, "testdata/mercenary-dead.png", divisor)
		rows := mercenaryRows(dead)
		if len(rows) != 4 {
			t.Fatalf("scale 1/%d rows=%v", divisor, rows)
		}
		for i, row := range rows {
			if got := mercenaryDead(dead, row); got != (i == 1) {
				t.Fatalf("scale 1/%d row %d dead=%t", divisor, i, got)
			}
		}
		for _, path := range []string{"testdata/mercenary-idle.png", "testdata/mercenary-mixed-roster.png", "testdata/mercenary-bottom-idle.png"} {
			living := mercenaryScaled(t, path, divisor)
			for _, row := range mercenaryRows(living) {
				if mercenaryDead(living, row) {
					t.Fatalf("living row marked dead: %s scale 1/%d row=%v", path, divisor, row)
				}
			}
		}
	}
}

func TestMercenaryAllDeadRosterWithUnreadableRecoveryTraits(t *testing.T) {
	im := loadTestImage(t, "testdata/mercenary-dead.png")
	rows := mercenaryRows(im)
	screen := image.NewRGBA(im.Bounds())
	draw.Draw(screen, screen.Bounds(), im, im.Bounds().Min, draw.Src)
	portrait := image.Rect(115, 755, 242, 906)
	for _, row := range rows {
		target := portrait.Add(image.Pt(0, row.Y-rows[1].Y))
		draw.Draw(screen, target, im, portrait.Min, draw.Src)
	}
	original := tesseractExecutable
	tesseractExecutable = filepath.Join(t.TempDir(), "missing-ocr")
	t.Cleanup(func() { tesseractExecutable = original })
	o, err := readMercenaryObservation(context.Background(), gameFrame{image: screen})
	if err != nil || !o.readable || len(o.dead) != len(rows) || len(o.collect)+len(o.start)+len(o.running) != 0 {
		t.Fatalf("unreadable recovery traits blocked all-dead roster or exposed a target: %+v err=%v", o, err)
	}
}

func TestMercenaryDeadRowSkipsTimerOCR(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	for _, divisor := range []int{1, 2, 4} {
		im := mercenaryScaled(t, "testdata/mercenary-dead.png", divisor)
		rows := mercenaryRows(im)
		// Obscure the dead timer and recovery controls. Its portrait is sufficient;
		// this row must not generate an OCR error or shift the living labels.
		obscured := image.NewRGBA(im.Bounds())
		draw.Draw(obscured, obscured.Bounds(), im, im.Bounds().Min, draw.Src)
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		region := image.Rect(w*220/1000, rows[1].Y-h*30/1000, w*390/1000, rows[1].Y+h*60/1000)
		draw.Draw(obscured, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
		for _, screen := range []image.Image{im, obscured} {
			o, err := readMercenaryObservation(context.Background(), gameFrame{image: screen})
			if err != nil || !o.readable {
				t.Fatalf("scale 1/%d: readable=%t err=%v", divisor, o.readable, err)
			}
			if len(o.dead) != 1 || o.dead[0] != rows[1] || len(o.collect) != 2 || o.collect[0] != rows[0] || o.collect[1] != rows[3] || len(o.running) != 1 || o.running[0] != rows[2] || len(o.start) != 0 {
				t.Fatalf("scale 1/%d wrong row association: dead=%v collect=%v running=%v start=%v", divisor, o.dead, o.collect, o.running, o.start)
			}
		}
	}
}
