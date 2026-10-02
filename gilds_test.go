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

func gildFixture(t *testing.T, name string) image.Image {
	t.Helper()
	f, err := os.Open("testdata/gild-" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestGildScreenshotWorkflow(t *testing.T) {
	cases := []struct {
		name  string
		modal gildModal
		point image.Point
	}{
		{"hud", noGildModal, image.Pt(1217, 581)},
		{"chest", gildChestModal, image.Pt(640, 349)},
		{"reward", gildRewardModal, image.Pt(937, 556)},
		{"roster", gildRosterModal, image.Pt(1149, 39)},
	}
	for _, tc := range cases {
		for _, scale := range []int{1, 2} {
			t.Run(tc.name+string(rune('0'+scale)), func(t *testing.T) {
				img := gildFixture(t, tc.name)
				if scale == 2 {
					enlarged := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
					xdraw.CatmullRom.Scale(enlarged, enlarged.Bounds(), img, img.Bounds(), draw.Src, nil)
					img = enlarged
				}
				c, err := recognizedGame(img)
				if err != nil {
					t.Fatal(err)
				}
				if c.modal != tc.modal || !c.known || (tc.modal != noGildModal && c.heroes) {
					t.Fatalf("context=%+v", c)
				}
				point, found, err := gildActionPoint(gameFrame{image: img, context: c})
				if err != nil || !found || (absDiff(point.X, tc.point.X*scale) > 1 || absDiff(point.Y, tc.point.Y*scale) > 1) {
					t.Fatalf("point=%v found=%v err=%v", point, found, err)
				}
			})
		}
	}
	f, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	negative, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := gildActionPoint(gameFrame{image: negative, context: gameContext{known: true}}); err != nil || found {
		t.Fatalf("real no-gift screen: %v %v", found, err)
	}
	// Unknown contents and a missing gift must not produce a blind click.
	hud := gildFixture(t, "hud")
	absent := image.NewRGBA(hud.Bounds())
	draw.Draw(absent, absent.Bounds(), hud, image.Point{}, draw.Src)
	draw.Draw(absent, image.Rect(1190, 550, 1280, 630), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if _, found, err := gildActionPoint(gameFrame{image: absent, context: gameContext{known: true}}); err != nil || found {
		t.Fatalf("absent gift: %v %v", found, err)
	}
	unknown := image.NewRGBA(hud.Bounds())
	draw.Draw(unknown, unknown.Bounds(), hud, image.Point{}, draw.Src)
	draw.Draw(unknown, image.Rect(280, 128, 1000, 591), image.NewUniform(color.RGBA{255, 252, 213, 255}), image.Point{}, draw.Src)
	c, err := recognizedGame(unknown)
	if err != nil || c.modal != unknownGildModal || c.known {
		t.Fatalf("unknown modal=%+v err=%v", c, err)
	}
	// If Open All is absent (a single reward), use only a recognized X.
	reward := gildFixture(t, "reward")
	single := image.NewRGBA(reward.Bounds())
	draw.Draw(single, single.Bounds(), reward, image.Point{}, draw.Src)
	draw.Draw(single, image.Rect(885, 528, 989, 583), image.NewUniform(color.RGBA{255, 252, 213, 255}), image.Point{}, draw.Src)
	point, found, err := gildActionPoint(gameFrame{image: single, context: gameContext{known: true, modal: gildRewardModal}})
	if err != nil || !found || point != image.Pt(994, 128) {
		t.Fatalf("single close=%v found=%v err=%v", point, found, err)
	}
}

func TestGildGiftIgnoresAnimatedNotification(t *testing.T) {
	hud := gildFixture(t, "hud")
	for _, scale := range []int{1, 2} {
		for _, state := range []string{"hidden", "changed", "lowered", "gift absent"} {
			t.Run(state+string(rune('0'+scale)), func(t *testing.T) {
				img := image.NewRGBA(hud.Bounds())
				draw.Draw(img, img.Bounds(), hud, hud.Bounds().Min, draw.Src)
				switch state {
				case "hidden":
					draw.Draw(img, image.Rect(1233, 530, 1275, 630), image.NewUniform(color.Black), image.Point{}, draw.Src)
				case "changed":
					draw.Draw(img, image.Rect(1233, 530, 1275, 630), image.NewUniform(color.RGBA{255, 220, 0, 255}), image.Point{}, draw.Src)
				case "lowered":
					// The notification bounces vertically into the lower gift body.
					draw.Draw(img, image.Rect(1233, 560, 1263, 620), hud, image.Pt(1233, 530), draw.Src)
				case "gift absent":
					// A notification alone must not identify a gift.
					draw.Draw(img, image.Rect(1190, 550, 1233, 630), image.NewUniform(color.Black), image.Point{}, draw.Src)
				}
				if scale == 2 {
					enlarged := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
					xdraw.CatmullRom.Scale(enlarged, enlarged.Bounds(), img, img.Bounds(), draw.Src, nil)
					img = enlarged
				}
				c, err := recognizedGame(img)
				if err != nil || !c.known || c.modal != noGildModal {
					t.Fatalf("context=%+v err=%v", c, err)
				}
				point, found, err := gildActionPoint(gameFrame{image: img, context: c})
				if err != nil || found != (state != "gift absent") {
					t.Fatalf("point=%v found=%v err=%v", point, found, err)
				}
				if found && (absDiff(point.X, 1217*scale) > 1 || absDiff(point.Y, 581*scale) > 1) {
					t.Fatalf("click outside gift body: %v", point)
				}
			})
		}
	}
}

func TestGildGiftOnBrightScenery(t *testing.T) {
	// Native 2560x1440 screenshot crop at (2360,1060)-(2560,1260).
	patch := gildFixture(t, "gift-beach")
	hud := gildFixture(t, "hud")
	for _, scale := range []int{1, 2} {
		t.Run(string(rune('0'+scale)), func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 1280*scale, 720*scale))
			xdraw.CatmullRom.Scale(img, img.Bounds(), hud, hud.Bounds(), draw.Src, nil)
			region := image.Rect(1180*scale, 530*scale, 1280*scale, 630*scale)
			xdraw.CatmullRom.Scale(img, region, patch, patch.Bounds(), draw.Src, nil)
			point, found, err := gildActionPoint(gameFrame{image: img})
			if err != nil || !found || absDiff(point.X, 1217*scale) > 1 || absDiff(point.Y, 581*scale) > 1 {
				t.Fatalf("bright scenery: point=%v found=%v err=%v", point, found, err)
			}
			// Bright scenery without a gift must not cause a click.
			draw.Draw(img, region, image.NewUniform(color.White), image.Point{}, draw.Src)
			if _, found, err := gildActionPoint(gameFrame{image: img}); err != nil || found {
				t.Fatalf("empty bright scenery: found=%v err=%v", found, err)
			}
		})
	}
}

func TestPipelineGildBatchIsolationAndPause(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	controls := &pauseControl{}
	var img image.Image
	clicks := 0
	p := newGamePipeline(controls, heroInput{
		capture: func() (image.Image, error) { return img, nil },
		click:   func(image.Point) error { clicks++; return nil },
	}, pipelineReaders{context: recognizedGame}, pipelineOptions{gilds: true, gildInterval: 5 * time.Minute, heroes: true, skills: true, progression: true, monster: true, fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	drain := func() {
		for _, ch := range jobs {
			for len(ch) > 0 {
				<-ch
			}
		}
	}
	var stale gameFrame
	for i, name := range []string{"hud", "chest", "reward", "roster"} {
		img = gildFixture(t, name)
		now = now.Add(2 * time.Second)
		if err := p.capture(ctx, now, jobs); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			stale = p.frame
			drain()
		} else {
			for _, ch := range jobs {
				if len(ch) != 0 {
					t.Fatal("modal scheduled background analysis")
				}
			}
			p.enqueue(gameAction{kind: collectFish, frame: p.frame, point: image.Pt(10, 10)}, now)
			p.enqueue(gameAction{kind: castSkill, frame: p.frame, key: 1}, now)
			p.enqueue(gameAction{kind: buyHero, frame: p.frame}, now)
			p.enqueue(gameAction{kind: clickMonster, frame: p.frame}, now)
			if err := p.accept(ctx, observation{kind: fishAnalysis, frame: stale, found: true, point: image.Pt(10, 10)}, now); err != nil {
				t.Fatal(err)
			}
			if p.state[fishAnalysis].found {
				t.Fatal("old background result accepted in modal")
			}
		}
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != collectGilds {
			t.Fatalf("%s next action=%+v ok=%v", name, a, ok)
		}
		if name == "chest" {
			controls.toggle()
			if acted, err := p.execute(ctx, a); acted || err != nil {
				t.Fatal("paused gift clicked")
			}
			controls.toggle()
			p.reset(controls.snapshot())
			if acted, _ := p.execute(ctx, a); acted {
				t.Fatal("stale gift clicked after resume")
			}
			if err := p.capture(ctx, now, jobs); err != nil {
				t.Fatal(err)
			}
			p.plan(now)
			a, ok = p.nextAction(now)
			if !ok || a.kind != collectGilds {
				t.Fatal("fresh gift did not resume")
			}
		}
		acted, err := p.execute(ctx, a)
		if err != nil || !acted {
			t.Fatalf("gift input=%v %v", acted, err)
		}
		p.actionCompleted(actionResult{action: a, acted: true}, now)
		if name == "roster" {
			img = gildFixture(t, "hud")
			if err := p.capture(ctx, now.Add(200*time.Millisecond), jobs); err != nil {
				t.Fatal(err)
			}
			p.plan(now.Add(200 * time.Millisecond))
			if !p.gild.active {
				t.Fatal("transaction ended before the post-click settling interval")
			}
			for kind, ch := range jobs {
				if analysisKind(kind) != fishAnalysis && len(ch) != 0 {
					t.Fatal("background recognition ran during modal settling")
				}
			}
		}
		if _, ok := p.nextAction(now); ok {
			t.Fatal("another action survived modal click")
		}
	}
	img = gildFixture(t, "hud")
	now = now.Add(2 * time.Second)
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	if p.gild.active || clicks != 4 {
		t.Fatalf("active=%v clicks=%d", p.gild.active, clicks)
	}
	if _, ok := p.queue[collectGilds]; ok {
		t.Fatal("gift immediately reopened instead of waiting for batch interval")
	}
	drain()
	now = now.Add(300 * time.Millisecond)
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[skillAnalysis]) != 1 || len(jobs[heroAnalysis]) != 1 {
		t.Fatal("background analysis did not resume")
	}
	// Disabling collection still isolates a manually opened gild modal.
	p.options.gilds = false
	img = gildFixture(t, "reward")
	if err := p.capture(ctx, now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	p.plan(now.Add(time.Second))
	if _, ok := p.nextAction(now.Add(time.Second)); ok {
		t.Fatal("disabled gift collection sent input")
	}
}

func TestGildNoOpPausesAndResumeUsesFreshFrame(t *testing.T) {
	img := gildFixture(t, "chest")
	c, err := recognizedGame(img)
	if err != nil {
		t.Fatal(err)
	}
	controls := &pauseControl{}
	p := newGamePipeline(controls, heroInput{}, pipelineReaders{}, pipelineOptions{gilds: true, gildInterval: 5 * time.Minute, fishInterval: time.Second})
	now := time.Now()
	for i := 0; i < 3; i++ {
		p.frame = gameFrame{id: uint64(i + 1), layout: p.layout, at: now, image: img, context: c}
		p.plan(now)
		a, ok := p.nextAction(now)
		if !ok || a.kind != collectGilds {
			t.Fatal("missing retry")
		}
		p.actionCompleted(actionResult{action: a, acted: true}, now)
		now = now.Add(2 * time.Second)
	}
	p.frame = gameFrame{id: 4, layout: p.layout, at: now, image: img, context: c}
	p.plan(now)
	if !controls.isPaused() {
		t.Fatal("unchanged gift window clicked indefinitely")
	}
	controls.toggle()
	p.reset(controls.snapshot())
	if p.gild.attempts != 0 || !p.gild.active || len(p.queue) != 0 {
		t.Fatal("resume retained stale retries/input")
	}
}

func TestGildUnknownTransitionTimesOut(t *testing.T) {
	now := time.Now()
	control := &pauseControl{}
	p := newGamePipeline(control, heroInput{}, pipelineReaders{}, pipelineOptions{gilds: true})
	p.gild = gildCollector{active: true, deadline: now}
	p.frame.context = gameContext{modal: unknownGildModal}
	p.plan(now.Add(time.Second))
	if !control.isPaused() || len(p.queue) != 0 {
		t.Fatal("unknown active transaction did not pause")
	}
}
