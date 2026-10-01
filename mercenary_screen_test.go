package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestMercenaryBatchOCRReusesOnlyUnchangedButtons(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	original := tesseractExecutable
	real, err := exec.LookPath(original)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tesseractExecutable = original })
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	t.Setenv("CH_MERCENARY_OCR_CALLS", calls)
	t.Setenv("CH_MERCENARY_OCR_REAL", real)
	tesseractExecutable = filepath.Join(dir, "tesseract")
	if err := os.WriteFile(tesseractExecutable, []byte("#!/bin/sh\necho call >> \"$CH_MERCENARY_OCR_CALLS\"\nexec \"$CH_MERCENARY_OCR_REAL\" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	im := loadTestImage(t, "testdata/mercenary-collect.png")
	frame := gameFrame{image: im, context: gameContext{known: true, mercenaries: true, bounds: im.Bounds()}}
	before, err := readMercenaryObservation(ctx, frame)
	if err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(calls)
	if err != nil || strings.Count(string(count), "call") != 1 {
		t.Fatalf("four visible buttons needed more than one OCR process: %q err=%v", count, err)
	}
	// An unavailable OCR engine proves the next identical roster uses only pixels.
	tesseractExecutable = filepath.Join(dir, "unavailable")
	after, err := readMercenaryObservationAfter(ctx, frame, before)
	if err != nil || !after.readable || len(after.collect) != len(before.collect) || len(after.running) != len(before.running) {
		t.Fatalf("unchanged buttons were not reused: %+v err=%v", after, err)
	}
	frame.generation++
	if _, err := readMercenaryObservationAfter(ctx, frame, before); err == nil {
		t.Fatal("F8 generation reused old button labels")
	}
	frame.generation--
	frame.image = loadTestImage(t, "testdata/mercenary-idle.png")
	if _, err := readMercenaryObservationAfter(ctx, frame, before); err == nil {
		t.Fatal("Collect becoming Start Quest reused an old label")
	}
	// Missing first-row text must not shift the later Collect labels upward.
	tesseractExecutable = original
	blank := image.NewRGBA(im.Bounds())
	draw.Draw(blank, blank.Bounds(), im, im.Bounds().Min, draw.Src)
	row := before.collect[0]
	region := image.Rect(im.Bounds().Dx()*237/1000, row.Y-im.Bounds().Dy()*27/1000, im.Bounds().Dx()*371/1000, row.Y+im.Bounds().Dy()*27/1000)
	draw.Draw(blank, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
	frame.image = blank
	if o, err := readMercenaryObservation(ctx, frame); err == nil || o.readable {
		t.Fatal("a blank first button was given another row's label")
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
	for _, raw := range []string{"Time: 15 minutes extra", "Time: 45 minutes", "Reward: 5 minutes", "15 minutes", "Time: 12 hours", "Time: 240 hours", "Time: 0 minutes", "Time: -60 minutes", "Time: 1.5 hours", "Time: 9999 days", "Time: 999999999999999999999999 hours"} {
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

func TestMercenaryDurationUnitAliases(t *testing.T) {
	for _, tc := range []struct {
		text string
		want time.Duration
	}{
		{"Time: 60 MINUTES\r\n", time.Hour},
		{"Time: 120 minutes", 2 * time.Hour},
		{"Time: 240 minutes", 4 * time.Hour},
		{"Time: 480 minutes", 8 * time.Hour},
		{"Time: 1440 minutes", 24 * time.Hour},
		{"Time: 2880 minutes", 48 * time.Hour},
		{"Time: 24 hours", 24 * time.Hour},
		{"Time: 48 hours", 48 * time.Hour},
	} {
		got, ok := parseMercenaryDuration(tc.text)
		if !ok || got != tc.want {
			t.Fatalf("%q: got %v ok=%t want %v", tc.text, got, ok, tc.want)
		}
	}
	// Exact multiline Windows OCR from the reported failed quest.
	lines := strings.Split(strings.TrimSpace("Reward: +1 skill activations and 8% chance\r\nof another\r\nTime: 60 minutes\r\n"), "\n")
	reward, rewardOK := parseMercenaryReward(strings.Join(lines[:2], " "))
	duration, durationOK := parseMercenaryDuration(lines[2])
	if !rewardOK || reward != "skills" || !durationOK || duration != time.Hour || mercenaryQuestRank(mercenaryQuest{reward: reward, duration: duration}) < 0 {
		t.Fatalf("reported quest not recognized: reward=%q duration=%v", reward, duration)
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
		if !ok || a.mercenary.step != claimAndOpenMercenaryQuest || a.point != o.collect[0] || !p.returnHeroes {
			t.Fatal("manual visit did not collect the first reward before unrelated idle quests")
		}
	}
}
