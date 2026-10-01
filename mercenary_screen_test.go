package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func TestMercenaryAnimatedNotification(t *testing.T) {
	// Actual crops: 160644.227 before/after, Oct 1 16:13:05/12:34:38,
	// and Sep 30 14:59:27 from Downloads. They show bouncing and squashing
	// on blue, black and cream backgrounds, with the tab artwork intact.
	atlas := loadTestImage(t, "testdata/mercenary-alert-phases.png")
	bounds := image.Rect(0, 0, 2560, 1440)
	var before gameFrame
	for _, divisor := range []int{1, 2, 4} {
		for phase := 0; phase < 5; phase++ {
			screen := image.NewRGBA(bounds)
			region := image.Rect(936, 158, 1076, 320)
			draw.Draw(screen, region, atlas, image.Pt(phase*region.Dx(), 0), draw.Src)
			var im image.Image = screen
			if divisor != 1 {
				scaled := image.NewRGBA(image.Rect(0, 0, 2560/divisor, 1440/divisor))
				xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), screen, bounds, draw.Src, nil)
				im = scaled
			}
			if !mercenaryNotification(im) {
				t.Errorf("missed real phase %d at scale 1/%d", phase, divisor)
			}
			if divisor == 1 {
				frame := gameFrame{image: im, context: gameContext{known: true, heroes: true, bounds: bounds}}
				if phase == 0 {
					before = frame
				}
				a := gameAction{frame: before, point: mercenaryPoint(bounds, 383, 200), mercenary: mercenaryCommand{step: openMercenaries}}
				if !mercenaryActionStable(a, frame) {
					t.Errorf("animation invalidated opening the fixed tab, phase %d", phase)
				}
			}
		}
	}
	// Two actual unselected tabs without a notification. Change all scenery
	// outside the opaque patch to ensure it cannot influence the result.
	normal := loadTestImage(t, "testdata/mercenary-normal-tabs.png")
	for phase := 0; phase < 2; phase++ {
		for _, divisor := range []int{1, 2, 4} {
			for _, bg := range []color.Color{color.Black, color.White, color.RGBA{50, 180, 240, 255}} {
				screen := image.NewRGBA(bounds)
				draw.Draw(screen, bounds, image.NewUniform(bg), image.Point{}, draw.Src)
				draw.Draw(screen, image.Rect(936, 272, 1024, 320), normal, image.Pt(phase*140, 114), draw.Src)
				var im image.Image = screen
				if divisor != 1 {
					scaled := image.NewRGBA(image.Rect(0, 0, 2560/divisor, 1440/divisor))
					xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), screen, bounds, draw.Src, nil)
					im = scaled
				}
				if mercenaryNotification(im) {
					t.Fatalf("normal tab accepted on changed scenery: phase=%d scale=1/%d", phase, divisor)
				}
			}
		}
	}
	for _, bg := range []color.Color{color.Black, color.White, color.RGBA{255, 220, 40, 255}} {
		screen := image.NewRGBA(bounds)
		draw.Draw(screen, bounds, image.NewUniform(bg), image.Point{}, draw.Src)
		if mercenaryNotification(screen) {
			t.Fatal("plain background accepted as a notification")
		}
	}
	for _, path := range []string{"testdata/mercenary-collect.png", "testdata/mercenary-idle.png"} {
		im := loadTestImage(t, path)
		if mercenaryNotification(im) {
			t.Fatalf("absent notification accepted in %s", path)
		}
	}
	for _, path := range []string{"testdata/hero-economy-x1.png", "testdata/gild-hud.png", "testdata/hero-tsuchi-x1.png"} {
		for _, divisor := range []int{1, 2, 4} {
			im := mercenaryScaled(t, path, divisor)
			if im.Bounds().Dx() < 640 {
				continue
			}
			if !mercenaryNotification(im) {
				t.Fatalf("missed actual notification in %s at scale 1/%d", path, divisor)
			}
		}
	}
}

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
	if mercenaryNotification(loadTestImage(t, "testdata/mercenary-idle.png")) {
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
	}{{"testdata/mercenary-quests.png", -1}, {"testdata/mercenary-selected.png", 0}, {"testdata/mercenary-quests-24-hours.png", -1}, {"testdata/mercenary-quests-random-skill.png", -1}} {
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
				durations := []time.Duration{2 * time.Hour, 4 * time.Hour, 8 * time.Hour, 48 * time.Hour}
				if tc.path == "testdata/mercenary-quests-24-hours.png" {
					want = []string{"hero souls", "hero souls", "skills", "skills"}
					durations = []time.Duration{30 * time.Minute, 2 * time.Hour, 24 * time.Hour, 48 * time.Hour}
					if chooseMercenaryQuest(o.quests) != 0 {
						t.Fatal("the real 24-hour offer blocked choosing the 30-minute Hero Souls quest")
					}
				}
				if tc.path == "testdata/mercenary-quests-random-skill.png" {
					want = []string{"gold", "skills", "rubies", "gold"}
					durations = []time.Duration{5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 4 * time.Hour}
					if chooseMercenaryQuest(o.quests) != 2 {
						t.Fatal("random skill reward blocked selecting the two-hour ruby quest")
					}
				}
				for i, q := range o.quests {
					if q.reward != want[i] {
						t.Errorf("%s quest%d reward=%q", tc.path, i, q.reward)
					}
				}
				for i, d := range durations {
					if o.quests[i].duration != d {
						t.Errorf("%s quest%d duration=%v", tc.path, i, o.quests[i].duration)
					}
				}
			}
		}
	}
}

func TestMercenaryBottomRosterOCRAndDispatch(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	for _, scale := range []int{1, 2} {
		im := mercenaryScaled(t, "testdata/mercenary-bottom-idle.png", scale)
		c, err := recognizedGame(im)
		if err != nil || !c.known || !c.mercenaries || c.questDialog {
			t.Fatalf("bottom roster context=%+v err=%v", c, err)
		}
		now := time.Now()
		o, err := readMercenaryObservation(context.Background(), gameFrame{id: 1, at: now, image: im, context: c})
		if err != nil || !o.readable || len(o.running) != 3 || len(o.start) != 1 || len(o.collect) != 0 || !o.bottom || o.top {
			t.Fatalf("bottom roster at scale1/%d: observation=%+v err=%v", scale, o, err)
		}
		p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true}
		p.observe(o, now)
		a, ok := p.action(now)
		if !ok || a.mercenary.step != openMercenaryQuest || a.point != o.start[0] || absDiff(a.point.Y, 1245/scale) > 2 {
			t.Fatalf("fifth mercenary not dispatched: action=%+v ok=%t", a, ok)
		}
	}
}

func TestMercenaryParsingRejectsAmbiguity(t *testing.T) {
	for _, raw := range []string{"Time: 5 minutes", "Time: 15 minutes", "Time: 30 minutes", "Time: 1 hour", "Time: 2 hours", "Time: 4 hours", "Time: 8 hours", "Time: 24 hours\r\n", "Time: 48 hours", "Time: 1 day", "Time: 2 days"} {
		if _, ok := parseMercenaryDuration(raw); !ok {
			t.Fatalf("rejected duration %q", raw)
		}
	}
	for _, raw := range []string{"Time: 15 minutes extra", "Time: 45 minutes", "Reward: 5 minutes", "15 minutes", "Time: 12 hours", "Time: 240 hours"} {
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

func TestMercenaryManuallyOpenedMixedRoster(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	for _, scale := range []int{1, 2} {
		im := mercenaryScaled(t, "testdata/mercenary-mixed-roster.png", scale)
		c, err := recognizedGame(im)
		if err != nil || !c.known || !c.mercenaries || c.questDialog {
			t.Fatalf("context=%+v err=%v", c, err)
		}
		now := time.Now()
		o, err := readMercenaryObservation(context.Background(), gameFrame{id: 1, at: now, image: im, context: c})
		if err != nil || !o.readable || len(o.collect) != 2 || len(o.start) != 1 || len(o.running) != 1 || !o.top || o.bottom {
			t.Fatalf("mixed roster=%+v err=%v", o, err)
		}
		p := mercenaryPlanner{}
		p.observe(o, now)
		a, ok := p.action(now)
		if !ok || a.mercenary.step != claimMercenaryReward || a.point != o.collect[0] || !p.returnHeroes {
			t.Fatal("manual visit did not collect the first reward before unrelated idle quests")
		}
	}
}
