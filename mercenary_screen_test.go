package main

import (
	"context"
	"image"
	"image/draw"
	"os"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func mercenaryScaled(t *testing.T, path string, scale int) image.Image {
	t.Helper()
	src := loadTestImage(t, path)
	if scale == 1 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx()/scale, src.Bounds().Dy()/scale))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func TestMercenaryScreenRecognizers(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	if mercenaryTabSelected(loadTestImage(t, "testdata/hero-tsuchi-x1.png")) {
		t.Fatal("Heroes tab accepted")
	}
	if mercenaryNotification(loadTestImage(t, "testdata/hero-economy-x1.png")) {
		t.Fatal("notification false positive")
	}
	if !mercenaryNotification(loadTestImage(t, "testdata/hero-tsuchi-x1.png")) {
		t.Fatal("notification missed")
	}
	for _, path := range []string{"testdata/mercenary-collect.png", "testdata/mercenary-idle.png"} {
		for _, scale := range []int{1, 2} {
			im := mercenaryScaled(t, path, scale)
			if !mercenaryTabSelected(im) {
				t.Fatalf("%s scale%d tab not recognized", path, scale)
			}
			c, contextErr := recognizedGame(im)
			if contextErr != nil || !c.known || !c.mercenaries || c.heroes || c.questDialog {
				t.Fatalf("roster context=%+v err=%v", c, contextErr)
			}
			o, err := readMercenaryObservation(context.Background(), gameFrame{image: im})
			if err != nil || !o.readable {
				t.Fatalf("%s scale%d observation=%+v err=%v", path, scale, o, err)
			}
			wantCollect, wantStart := 3, 0
			if path == "testdata/mercenary-idle.png" {
				wantCollect, wantStart = 0, 3
			}
			if len(o.collect) != wantCollect || len(o.start) != wantStart || len(o.running) != 1 || !o.top || o.bottom || !o.thumbFound {
				t.Fatalf("%s scale%d row/scroll recognition=%+v", path, scale, o)
			}
			if absDiff(o.running[0].Y, im.Bounds().Dy()*733/1000) > 5 {
				t.Fatal("timer position cannot confirm dispatch at the original row")
			}
		}
	}
}

func TestMercenaryQuestOCR(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	for _, tc := range []struct {
		path     string
		selected int
	}{{"testdata/mercenary-quests.png", -1}, {"testdata/mercenary-selected.png", 0}} {
		for _, scale := range []int{1, 2} {
			im := mercenaryScaled(t, tc.path, scale)
			c, contextErr := recognizedGame(im)
			if contextErr != nil || !c.known || !c.questDialog || c.heroes {
				t.Fatalf("dimmed dialog context=%+v err=%v", c, contextErr)
			}
			o, err := readMercenaryObservation(context.Background(), gameFrame{image: im, context: c})
			if err != nil {
				t.Fatalf("%s scale%d: %v", tc.path, scale, err)
			}
			wantOkay := tc.selected >= 0
			if !o.readable || o.selected != tc.selected || (o.okay != (image.Point{})) != wantOkay {
				t.Fatalf("%s scale%d observation=%+v", tc.path, scale, o)
			}
			if wantOkay && o.okay != mercenaryPoint(o.frame.image.Bounds(), 710, 500) {
				t.Fatal("Okay target does not match the actual button")
			}
			if tc.selected < 0 {
				want := []string{"gold", "gold", "relics", "skills"}
				for i, q := range o.quests {
					if q.reward != want[i] {
						t.Errorf("%s quest%d reward=%q", tc.path, i, q.reward)
					}
				}
				for i, d := range []time.Duration{2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 48 * time.Hour} {
					if o.quests[i].duration != d {
						t.Errorf("%s quest%d duration=%v", tc.path, i, o.quests[i].duration)
					}
				}
			}
		}
	}
}

func TestMercenaryParsingRejectsAmbiguity(t *testing.T) {
	for _, raw := range []string{"Time: 5 minutes", "Time: 15 minutes", "Time: 30 minutes", "Time: 1 hour", "Time: 2 hours", "Time: 4 hours", "Time: 8 hours", "Time: 1 day", "Time: 2 days"} {
		if _, ok := parseMercenaryDuration(raw); !ok {
			t.Fatalf("rejected duration %q", raw)
		}
	}
	for _, raw := range []string{"Time: 15 minutes extra", "Time: 45 minutes", "Reward: 5 minutes", "15 minutes"} {
		if _, ok := parseMercenaryDuration(raw); ok {
			t.Fatalf("accepted ambiguous duration %q", raw)
		}
	}
	for _, raw := range []string{"Revive: 30 rubies", "Reroll 30 rubies", "Reward: unknown"} {
		if _, ok := parseMercenaryReward(raw); ok {
			t.Fatalf("accepted unsafe reward %q", raw)
		}
	}
}
