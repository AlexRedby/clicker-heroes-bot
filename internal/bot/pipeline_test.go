package bot

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"sync/atomic"
	"testing"
	"time"
)

func testPipelineFrame() gameFrame {
	screen := image.NewRGBA(image.Rect(0, 0, 100, 100))
	return gameFrame{id: 1, layout: 1, at: time.Now(), image: screen, context: gameContext{known: true, bounds: screen.Bounds(), window: "game"}}
}

func TestPipelineBoundedParallelAnalysis(t *testing.T) {
	controls := pauseControl{}
	frame := testPipelineFrame()
	release := make(chan struct{})
	var captures, activeFish, peakFish, scans, casts atomic.Int32
	var ready atomic.Bool
	ready.Store(true)
	skillSent := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readers := pipelineReaders{
		context: func(image.Image) (gameContext, error) { return frame.context, nil },
		fish: func(image.Image) (image.Point, bool, error) {
			active := activeFish.Add(1)
			if active > peakFish.Load() {
				peakFish.Store(active)
			}
			defer activeFish.Add(-1)
			if scans.Add(1) == 1 {
				<-release
			}
			return image.Point{}, false, nil
		},
		skills: func(context.Context, image.Image) ([9]skillState, error) {
			if ready.Load() {
				return skillsReady(1), nil
			}
			return skillsReady(), nil
		},
		window: func() string { return "game" },
	}
	p := newGamePipeline(&controls, heroInput{
		capture: func() (image.Image, error) { captures.Add(1); return frame.image, nil },
		keyToggle: func(key, state string) error {
			if state == "down" {
				casts.Add(1)
				ready.Store(false)
				select {
				case skillSent <- struct{}{}:
				default:
				}
			}
			return nil
		},
	}, readers, pipelineOptions{skills: true, fishInterval: 10 * time.Millisecond})
	done := make(chan error, 1)
	go func() { done <- p.run(ctx) }()
	select {
	case <-skillSent:
	case <-time.After(2 * time.Second):
		close(release)
		cancel()
		<-done
		t.Fatal("slow SIFT blocked skill input")
	}
	time.Sleep(350 * time.Millisecond)
	if captures.Load() < 3 || scans.Load() != 1 {
		t.Errorf("captures=%d fish=%d; expected captures while one fish job runs", captures.Load(), scans.Load())
	}
	close(release)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peakFish.Load() != 1 || casts.Load() != 1 {
		t.Fatalf("fish overlap=%d duplicate casts=%d", peakFish.Load(), casts.Load())
	}
	if p.skill.pending != nil {
		t.Fatal("shared post-action frame did not confirm skill")
	}
}

func TestPipelineFrameOwnershipAndContext(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, skills: true, progression: true, fishInterval: time.Second})
	p.startupCheck = false // This test begins after initial hero setup.
	p.nextUpgrades = time.Now().Add(time.Hour)
	frame := testPipelineFrame()
	frame.context.heroes = true
	current := frame.context
	p.input.capture = func() (image.Image, error) { return frame.image, nil }
	p.readers.context = func(image.Image) (gameContext, error) { return current, nil }
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	fish, skills, hero := <-jobs[fishAnalysis], <-jobs[skillAnalysis], <-jobs[heroAnalysis]
	if err := p.capture(context.Background(), now.Add(300*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[heroAnalysis]) != 0 {
		t.Fatal("duplicate hero OCR while its job is running")
	}
	if fish.frame.id != skills.frame.id || hero.frame.id != skills.frame.id || fish.frame.image != hero.frame.image {
		t.Fatal("analyzers did not share one frame")
	}
	if err := p.accept(context.Background(), observation{kind: skillAnalysis, frame: skills.frame, skills: skillsReady(3)}, now); err != nil {
		t.Fatal(err)
	}
	progression := <-jobs[progressionAnalysis]
	if progression.frame.id != skills.frame.id || progression.skills != skillsReady(3) {
		t.Fatal("progression did not reuse this frame's skills")
	}
	p.enqueue(gameAction{kind: buyHero, frame: p.frame}, now)
	current.heroes = false
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs[heroAnalysis]) != 0 || len(p.queue) != 0 {
		t.Fatal("hero work survived tab change")
	}
	if err := p.accept(context.Background(), observation{kind: heroAnalysis, frame: hero.frame, hero: heroObservation{frame: hero.frame}}, now); err != nil {
		t.Fatal(err)
	}
	if p.hero.latest.frame.id != 0 {
		t.Fatal("old panel result accepted")
	}
	// Mode confirmation requires no skill recognition or OCR.
	p.progression.pending = &progressionAttempt{frameID: p.frame.id}
	p.nextProgression = time.Time{}
	if err := p.capture(context.Background(), now.Add(2*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if !((<-jobs[progressionAnalysis]).modeOnly) {
		t.Fatal("mode confirmation waited for skill worker")
	}
	current.known = false
	if err := p.capture(context.Background(), now.Add(3*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	// Empty old waiting jobs before testing the next unknown frame.
	for _, ch := range jobs {
		select {
		case <-ch:
		default:
		}
	}
	if err := p.capture(context.Background(), now.Add(4*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	for _, ch := range jobs {
		if len(ch) != 0 {
			t.Fatal("unknown context scheduled recognition")
		}
	}
}

func TestPipelineQueueAndInputGuards(t *testing.T) {
	controls := pauseControl{}
	frame := testPipelineFrame()
	p := newGamePipeline(&controls, heroInput{}, pipelineReaders{}, pipelineOptions{fishInterval: time.Second})
	p.layout = 1
	frame.context.heroes = true
	p.frame = frame
	now := frame.at
	p.enqueue(gameAction{kind: scrollHeroes, frame: frame}, now)
	if _, ok := p.nextAction(now); ok {
		t.Fatal("unknown fish allowed mouse-related action")
	}
	p.enqueue(gameAction{kind: selectQuantity, frame: frame}, now)
	if a, ok := p.nextAction(now); !ok || a.kind != selectQuantity {
		t.Fatal("T hotkey waited for fish analysis")
	}
	p.enqueue(gameAction{kind: castSkill, frame: frame, key: 1}, now)
	p.state[skillAnalysis] = observation{frame: frame, skills: skillsReady(1)}
	p.enqueue(gameAction{kind: collectFish, frame: frame, point: image.Pt(20, 20)}, now)
	point := image.Pt(20, 20)
	p.fishTarget = &point
	p.state[fishAnalysis] = observation{frame: frame, found: true, point: point}
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectFish {
		t.Fatal("fish not prioritized")
	}
	p.fishTarget = nil
	a, ok = p.nextAction(now.Add(time.Second))
	if !ok || a.kind != castSkill {
		t.Fatal("independent skill did not follow fish")
	}
	for i := uint64(2); i < 100; i++ {
		f := frame
		f.id = i
		p.enqueue(gameAction{kind: castSkill, frame: f, key: 1}, now)
	}
	if len(p.queue) != 1 || p.queue[castSkill].frame.id != 99 {
		t.Fatal("queue not bounded/coalesced")
	}
	events := []string{}
	p.input.keyToggle = func(k, s string) error { events = append(events, k+":"+s); return nil }
	failure := errors.New("click failed")
	p.input.click = func(image.Point) error { return failure }
	p.input.move = func(image.Point) error { return nil }
	p.readers.window = func() string { return "other" }
	a = gameAction{kind: buyHero, frame: frame, hero: heroObservation{owned: true}}
	if acted, err := p.execute(context.Background(), a); acted || err != nil || len(events) != 0 {
		t.Fatal("focus change allowed input")
	}
	p.readers.window = func() string { return "game" }
	if acted, err := p.execute(context.Background(), a); acted || !errors.Is(err, failure) {
		t.Fatalf("lost click error: %v", err)
	}
	if len(events) != 2 || events[0] != "q:down" || events[1] != "q:up" {
		t.Fatalf("Q cleanup: %v", events)
	}
	controls.toggle()
	controls.toggle()
	events = nil
	if acted, err := p.execute(context.Background(), a); acted || err != nil || len(events) != 0 {
		t.Fatal("stale generation allowed input")
	}
}

func TestPipelineHeroPurchaseSharedConfirmation(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-tsuchi-x1.png")
	var clicked atomic.Bool
	var captures, fish, levels, prices, gold, purchases, assists atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := pipelineReaders{
		context: func(image.Image) (gameContext, error) {
			return gameContext{known: true, heroes: true, bounds: screen.Bounds()}, nil
		},
		fish: func(image.Image) (image.Point, bool, error) { fish.Add(1); return image.Point{}, false, nil },
		heroes: heroReaders{
			gold: func(context.Context, image.Image) (float64, error) {
				gold.Add(1)
				if clicked.Load() {
					return 101.5, nil
				}
				return 100, nil
			},
			price: func(context.Context, image.Image, image.Point) (float64, error) { prices.Add(1); return 102, nil },
			level: func(context.Context, image.Image, image.Point) (int, error) {
				levels.Add(1)
				if clicked.Load() {
					return 101, nil
				}
				return 100, nil
			},
		},
	}
	p := newGamePipeline(&pauseControl{}, heroInput{
		capture:      func() (image.Image, error) { captures.Add(1); return screen, nil },
		click:        func(image.Point) error { purchases.Add(1); clicked.Store(true); return nil },
		monsterClick: func(image.Point) error { assists.Add(1); return nil },
		move:         func(image.Point) error { return nil }, keyToggle: func(string, string) error { return nil },
	}, read, pipelineOptions{heroes: true, fishInterval: time.Hour})
	p.startupCheck = false // This test begins after initial hero setup.
	p.nextUpgrades = time.Now().Add(time.Hour)
	done := make(chan error, 1)
	go func() { done <- p.run(ctx) }()
	// After confirmation, a new economy read must choose saving rather than reuse the purchase.
	time.Sleep(1200 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !clicked.Load() || p.hero.pending != nil || p.hero.failures != 0 || purchases.Load() != 1 || assists.Load() == 0 {
		t.Fatalf("purchase/confirmation: %+v %s", p.hero, p.metrics.String())
	}
	if fish.Load() != 1 || gold.Load() != 2 || prices.Load() != 2 || levels.Load() < 2 || captures.Load() < 2 {
		t.Fatalf("duplicate full recognition: fish=%d gold=%d prices=%d level=%d captures=%d", fish.Load(), gold.Load(), prices.Load(), levels.Load(), captures.Load())
	}
}

func TestPipelineUnreadableConfirmationIsBounded(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.startupCheck = false // This test begins after initial hero setup.
	p.nextUpgrades = time.Now().Add(time.Hour)
	frame := testPipelineFrame()
	p.frame = frame
	p.layout = 1
	before := heroObservation{frame: frame, level: 100}
	p.hero.sent(gameAction{kind: buyHero, frame: frame, hero: before}, frame.at)
	for i := uint64(2); i <= 6; i++ {
		f := frame
		f.id = i
		f.at = frame.at.Add(time.Duration(i) * time.Second)
		if err := p.accept(context.Background(), observation{kind: heroAnalysis, frame: f, hero: heroObservation{frame: f, stable: true}, err: errors.New("unreadable")}, f.at); err != nil {
			t.Fatal(err)
		}
	}
	if p.hero.pending != nil || p.hero.failures != 1 {
		t.Fatal("unreadable confirmation was not bounded")
	}
	f := frame
	f.id = 7
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: f}, f.at); err != nil {
		t.Fatal(err)
	}
	if p.hero.pending != nil || p.hero.failures != 1 {
		t.Fatal("fish result changed an already finalized failure")
	}
}

func TestUtilityTargetsRemainRecognized(t *testing.T) {
	p := skillPlanner{keys: []int{8, 9}, confirmed: map[int]bool{3: true, 5: true}}
	states := skillsReady(8, 9)
	states[2].Known = false
	if key := p.nextKey(states, 1, time.Now()); key != 0 {
		t.Fatal("utility used after confirmed target became unknown")
	}
}

func TestPipelineObservationFreshness(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{fishInterval: time.Second})
	frame := testPipelineFrame()
	p.layout = 1
	p.frame = frame
	p.frame.id = 20
	out := observation{kind: fishAnalysis, frame: frame, found: true, point: image.Pt(20, 20), elapsed: 1700 * time.Millisecond}
	now := frame.at.Add(2 * time.Second)
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	// Slow SIFT must remain useful when faster HUD workers have captured newer frames.
	if a, ok := p.nextAction(now); !ok || a.kind != collectFish {
		t.Fatal("new captures discarded an otherwise fresh SIFT result")
	}
	a, _ := p.nextAction(now)
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	out.frame.id = 19
	if err := p.accept(context.Background(), out, now); err != nil {
		t.Fatal(err)
	}
	if p.fishTarget != nil || len(p.queue) != 0 {
		t.Fatal("pre-action observation passed invalidation barrier")
	}
	p.enqueue(gameAction{kind: collectFish, frame: frame}, now)
	if _, ok := p.nextAction(frame.at.Add(10 * time.Second)); ok {
		t.Fatal("expired fish coordinates were replayed")
	}
}

func TestMonsterClickDoesNotForceCaptureOrSettling(t *testing.T) {
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{})
	deadline := time.Now().Add(250 * time.Millisecond)
	p.nextCapture = deadline
	p.actionCompleted(actionResult{action: gameAction{kind: clickMonster}, acted: true}, time.Now())
	if !p.nextCapture.Equal(deadline) || !p.settleUntil.IsZero() {
		t.Fatal("monster click added unnecessary capture or settling delay")
	}
}

func TestSlowCapturePreservesInterval(t *testing.T) {
	frame := testPipelineFrame()
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { time.Sleep(350 * time.Millisecond); return frame.image, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return frame.context, nil }}, pipelineOptions{fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	start := time.Now()
	if err := p.capture(context.Background(), start, jobs); err != nil {
		t.Fatal(err)
	}
	if time.Until(p.nextCapture) < 200*time.Millisecond {
		t.Fatal("slow capture consumed the entire next-frame interval")
	}
	if !p.frame.at.Equal(start) {
		t.Fatal("capture-start timestamp no longer conservatively identifies the source frame")
	}
}

func TestPipelineScrollDoesNotWaitForFishAndConfirmsBottom(t *testing.T) {
	bottom := loadTestImage(t, "../../testdata/hero-panel-max.png")
	thumb, height, found := heroScrollbarThumb(bottom)
	if !found {
		t.Fatal("fixture thumb missing")
	}
	top := image.NewRGBA(bottom.Bounds())
	draw.Draw(top, top.Bounds(), bottom, bottom.Bounds().Min, draw.Src)
	region := image.Rect(thumb.X-30, thumb.Y-height/2-3, thumb.X+31, thumb.Y+height/2+4)
	draw.Draw(top, region, image.NewUniform(color.RGBA{R: 30, G: 25, B: 10, A: 255}), image.Point{}, draw.Src)
	moved := region.Add(image.Pt(0, bottom.Bounds().Dy()/2-thumb.Y))
	draw.Draw(top, moved, bottom, region.Min, draw.Src)
	if heroScrollbarAtBottom(top) {
		t.Fatal("test thumb did not move")
	}
	frame := gameFrame{id: 1, layout: 1, at: time.Now(), image: top, context: gameContext{known: true, heroes: true, bounds: top.Bounds()}}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, fishInterval: time.Second})
	p.startupCheck = false // This test begins after initial hero setup.
	p.nextUpgrades = time.Now().Add(time.Hour)
	p.frame, p.layout = frame, 1
	out, err := readHeroObservation(context.Background(), frame, heroReaders{}, nil)
	if err != nil || !out.thumbFound {
		t.Fatalf("top geometry: %+v %v", out, err)
	}
	p.hero.observe(out, observation{}, frame.at)
	p.plan(frame.at)
	if a, ok := p.nextAction(frame.at); !ok || a.kind != scrollHeroes {
		t.Fatal("scroll waited for fish analysis")
	}
	now := frame.at.Add(4 * time.Second)
	// Slow first SIFT expires the original intent; fresh geometry must recreate it.
	p.nextAction(now)
	frame.id++
	frame.at = now
	p.frame = frame
	out, err = readHeroObservation(context.Background(), frame, heroReaders{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.hero.observe(out, observation{}, now)
	fishFrame := frame
	fishFrame.id = 1
	fishFrame.at = now.Add(-4 * time.Second)
	p.state[fishAnalysis] = observation{frame: fishFrame, elapsed: 4 * time.Second}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != scrollHeroes {
		t.Fatalf("fresh scroll missing after slow SIFT: %+v %+v", a, p.hero)
	}
	p.input.drag = func(from, to image.Point) error {
		if from != out.thumb || to.Y != bottom.Bounds().Max.Y-1 {
			t.Fatalf("drag: %v -> %v", from, to)
		}
		return nil
	}
	if acted, err := p.execute(context.Background(), a); !acted || err != nil {
		t.Fatalf("drag=%t %v", acted, err)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	frame.id++
	frame.at = now.Add(time.Second)
	frame.image = bottom
	confirmed, err := readHeroObservation(context.Background(), frame, heroReaders{
		gold:  func(context.Context, image.Image) (float64, error) { return 100, nil },
		price: func(context.Context, image.Image, image.Point) (float64, error) { return 102, nil },
		level: func(context.Context, image.Image, image.Point) (int, error) { return 100, nil },
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.hero.observe(confirmed, p.state[fishAnalysis], frame.at)
	if p.hero.pending != nil || !p.hero.latest.bottom {
		t.Fatalf("bottom not confirmed: %+v", p.hero)
	}
}
