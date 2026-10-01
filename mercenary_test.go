package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestMercenaryPolicyAndConfirmedCycle(t *testing.T) {
	for _, tc := range []struct {
		quests []mercenaryQuest
		want   int
	}{
		{[]mercenaryQuest{{reward: "rubies", duration: 5 * time.Minute}, {reward: "rubies", duration: 4 * time.Hour}}, 1},
		{[]mercenaryQuest{{reward: "rubies", duration: 24 * time.Hour}, {reward: "gold", duration: 2 * time.Hour}}, 1},
		{[]mercenaryQuest{{reward: "gold", duration: 8 * time.Hour}, {reward: "rubies", duration: 24 * time.Hour}}, 1},
		{[]mercenaryQuest{{reward: "rubies", duration: 48 * time.Hour}, {reward: "skills", duration: 24 * time.Hour}}, 1},
		{[]mercenaryQuest{{reward: "rubies", duration: 4 * time.Hour}, {reward: "recruitment", duration: 8 * time.Hour}}, 1},
		{[]mercenaryQuest{{reward: "revive", duration: time.Hour}, {reward: "rubies", duration: 6 * time.Hour}}, -1},
	} {
		if got := chooseMercenaryQuest(tc.quests); got != tc.want {
			t.Fatalf("quests=%+v: selected=%d want=%d", tc.quests, got, tc.want)
		}
	}

	now := time.Now()
	bounds := image.Rect(0, 0, 2560, 1440)
	row := image.Pt(765, 620)
	thumb := image.Pt(1172, 890)
	o := mercenaryObservation{frame: gameFrame{id: 1, at: now, context: gameContext{known: true, heroes: true, bounds: bounds}}, notify: true, selected: -1, readable: true}
	p := mercenaryPlanner{}
	p.observe(o, now)
	o.frame.id++
	o.frame.at = now.Add(200 * time.Millisecond)
	p.observe(o, o.frame.at)
	step := func(want mercenaryStep, after mercenaryObservation) gameAction {
		t.Helper()
		a, ok := p.action(now)
		if !ok || a.mercenary.step != want {
			t.Fatalf("step=%d: got=%+v ok=%t planner=%+v", want, a.mercenary, ok, p)
		}
		p.sent(a, now)
		// An unchanged frame must never confirm the action or emit another one.
		p.observe(o, now.Add(time.Millisecond))
		if _, ok := p.action(now.Add(250 * time.Millisecond)); ok {
			t.Fatal("pending action repeated before confirmation")
		}
		now = now.Add(300 * time.Millisecond)
		after.frame.id, after.frame.at = o.frame.id+1, now
		p.observe(after, now)
		o = after
		if p.pending != nil {
			t.Fatalf("step %d was not confirmed", want)
		}
		return a
	}
	merc := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, bounds: bounds}}, collect: []image.Point{row}, thumb: thumb, thumbFound: true, top: true, selected: -1, readable: true}
	step(openMercenaries, merc)
	dialog := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, questDialog: true, bounds: bounds}}, selected: -1, readable: true,
		quests: []mercenaryQuest{{reward: "gold", duration: 2 * time.Hour, point: image.Pt(1000, 390)}, {reward: "gold", duration: 4 * time.Hour, point: image.Pt(1000, 600)}}}
	step(claimAndOpenMercenaryQuest, dialog)
	dialog.selected, dialog.okay = 0, image.Pt(1815, 720)
	a := step(selectMercenaryQuest, dialog)
	if a.mercenary.quest != 0 {
		t.Fatalf("wrong quest: %+v", a)
	}
	merc.collect, merc.start = nil, nil
	merc.running = []image.Point{row}
	step(confirmMercenaryQuest, merc)
	merc.top, merc.bottom = false, true
	step(scrollMercenariesBottom, merc)
	heroes := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, heroes: true, bounds: bounds}}, selected: -1, readable: true}
	step(returnToHeroes, heroes)
	if p.active || p.pending != nil {
		t.Fatal("mercenary visit survived confirmed return")
	}
}

func TestMercenaryInterruptedDialogAndPipelineGuards(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	screen := loadTestImage(t, "testdata/mercenary-selected.png")
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, mercenaries: true, questDialog: true, bounds: screen.Bounds()}}
	planner := mercenaryPlanner{}
	planner.observe(mercenaryObservation{frame: frame, readable: true, selected: 0, okay: image.Pt(1815, 720)}, now)
	a, ok := planner.action(now)
	if !ok || a.mercenary.step != closeMercenaryQuest {
		t.Fatal("an interrupted/user dialog must close without confirming a quest")
	}
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{click: func(image.Point) error { t.Fatal("paused action clicked"); return nil }}, pipelineReaders{}, pipelineOptions{mercenaries: true, gilds: true})
	p.frame, p.layout = frame, frame.layout
	p.gild.active = true // F8 preserves a gift batch for resumption.
	if p.planGilds(now) || p.gild.active {
		t.Fatal("an interrupted gift batch blocked the user's quest dialog")
	}
	p.queue[collectFish] = gameAction{kind: collectFish, frame: frame}
	p.queue[collectGilds] = gameAction{kind: collectGilds, frame: frame}
	p.queue[clickMonster] = gameAction{kind: clickMonster, frame: frame}
	p.queue[castSkill] = gameAction{kind: castSkill, frame: frame, key: 1}
	if _, ok := p.nextAction(now); ok || len(p.queue) != 0 {
		t.Fatal("input reached game controls underneath the quest dialog")
	}
	controls.toggle()
	if acted, err := p.execute(ctx, a); acted || err != nil {
		t.Fatalf("F8 did not invalidate mercenary action: acted=%t err=%v", acted, err)
	}
	p.mercenary.sent(a, now)
	p.reset(controls.snapshot())
	if p.mercenary.pending != nil || p.mercenary.active {
		t.Fatal("F8 kept a pending mercenary decision")
	}

	// A manual change at the click target invalidates old OCR coordinates.
	changed := image.NewRGBA(screen.Bounds())
	draw.Draw(changed, changed.Bounds(), screen, screen.Bounds().Min, draw.Src)
	region := image.Rect(a.point.X-80, a.point.Y-40, a.point.X+80, a.point.Y+40).Intersect(changed.Bounds())
	draw.Draw(changed, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
	current := frame
	current.image = changed
	if !mercenaryActionStable(a, frame) || mercenaryActionStable(a, current) {
		t.Fatal("stale mercenary target accepted")
	}
}

func TestMercenaryVisitDefersGildBatch(t *testing.T) {
	now := time.Now()
	screen := gildFixture(t, "hud")
	c, err := recognizedGame(screen)
	if err != nil || !c.known || c.questDialog || c.modal != noGildModal {
		t.Fatalf("gift HUD context=%+v err=%v", c, err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{gilds: true, mercenaries: true, gildInterval: 5 * time.Minute})
	p.frame = gameFrame{id: 1, at: now, image: screen, context: c}
	p.mercenary.active = true
	p.queue[collectGilds] = gameAction{kind: collectGilds, frame: p.frame}
	if p.planGilds(now) || len(p.queue) != 0 {
		t.Fatal("gift batch interrupted an active mercenary visit")
	}
	p.mercenary.active = false
	p.planGilds(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != collectGilds {
		t.Fatal("gift polling did not resume after the mercenary visit")
	}
	// Starting the next transaction must invalidate all earlier mercenary work.
	p.mercenary.pending = &mercenaryAttempt{action: gameAction{frame: p.frame}}
	p.mercenaryJobFrame = 1
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if p.mercenary.pending != nil || p.mercenaryJobFrame != 0 || !p.gild.active {
		t.Fatal("gild transaction retained a stale mercenary decision")
	}
}

func TestMercenaryCaptureSchedulesActionAndPreservesExpectedTabChange(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	c := gameContext{known: true, heroes: true, bounds: screen.Bounds()}
	controls := pauseControl{}
	p := newGamePipeline(&controls, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return c, nil },
		mercenaries: func(_ context.Context, f gameFrame) (mercenaryObservation, error) {
			return mercenaryObservation{frame: f, notify: f.context.heroes, readable: true, selected: -1, top: true, collect: []image.Point{image.Pt(765, 620)}}, nil
		},
	}, pipelineOptions{mercenaries: true, fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	out := p.analyze(ctx, mercenaryAnalysis, <-jobs[mercenaryAnalysis])
	if err := p.accept(ctx, out, now.Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	p.plan(now.Add(10 * time.Millisecond))
	if _, ok := p.mercenary.action(now.Add(10 * time.Millisecond)); ok {
		t.Fatal("one notification observation opened the tab")
	}
	now = now.Add(2 * time.Second)
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	out = p.analyze(ctx, mercenaryAnalysis, <-jobs[mercenaryAnalysis])
	if err := p.accept(ctx, out, now.Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	p.state[fishAnalysis] = observation{frame: p.frame}
	p.plan(now.Add(10 * time.Millisecond))
	a, ok := p.nextAction(now.Add(10 * time.Millisecond))
	if !ok || a.mercenary.step != openMercenaries {
		t.Fatal("capture cadence delayed or prevented a ready mercenary action")
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now.Add(20*time.Millisecond))
	p.queue[clickMonster] = gameAction{kind: clickMonster, frame: p.frame}
	if _, ok := p.nextAction(now.Add(30 * time.Millisecond)); ok {
		t.Fatal("queued input ran between tab click and fresh layout capture")
	}
	screen = loadTestImage(t, "testdata/mercenary-collect.png")
	c.heroes, c.mercenaries = false, true
	if err := p.capture(ctx, now.Add(300*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	if !p.mercenary.active || !p.mercenary.returnHeroes || p.mercenary.pending == nil {
		t.Fatal("the expected tab transition erased the visit/return state")
	}
	out = p.analyze(ctx, mercenaryAnalysis, <-jobs[mercenaryAnalysis])
	if err := p.accept(ctx, out, now.Add(310*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if p.mercenary.pending != nil {
		t.Fatal("new roster frame did not confirm tab transition")
	}
	p.state[fishAnalysis] = observation{frame: p.frame}
	p.plan(now.Add(310 * time.Millisecond))
	if a, ok := p.nextAction(now.Add(310 * time.Millisecond)); !ok || a.mercenary.step != claimAndOpenMercenaryQuest {
		t.Fatal("confirmed roster did not lead to reward collection")
	}
}

func TestMercenaryNotificationNeedsFreshConsistentFrames(t *testing.T) {
	now := time.Now()
	o := mercenaryObservation{notify: true, frame: gameFrame{id: 1, at: now, context: gameContext{known: true, heroes: true}}}
	p := mercenaryPlanner{}
	check := func(want bool) {
		t.Helper()
		if _, got := p.action(now); got != want {
			t.Fatalf("notification action=%t want=%t", got, want)
		}
	}
	p.observe(o, now)
	check(false)
	p.observe(o, now) // Duplicate work is not another frame.
	check(false)
	o.frame.id++
	o.notify = false
	p.observe(o, now)
	check(false)
	o.frame.id++
	o.notify = true
	p.observe(o, now)
	check(false)
	o.frame.id++
	o.frame.layout++
	p.observe(o, now)
	check(false)
	o.frame.id++
	p.observe(o, now)
	check(true)
	p.interrupt()
	p.observe(o, now)
	check(false)
	o.frame.id++
	o.frame.at = now.Add(6 * time.Second)
	p.observe(o, o.frame.at)
	check(false)
	o.frame.id++
	o.frame.at = now
	p.observe(o, now)
	check(false)
}

func TestMercenaryTransientOCRDoesNotAbandonVisit(t *testing.T) {
	now := time.Now()
	c := gameContext{known: true, mercenaries: true, bounds: image.Rect(0, 0, 2560, 1440)}
	row := image.Pt(768, 1245)
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true}
	o := mercenaryObservation{frame: gameFrame{id: 1, at: now, context: c}, readable: true, bottom: true, collect: []image.Point{row}}
	p.observe(o, now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != claimAndOpenMercenaryQuest {
		t.Fatal("reward was not collected")
	}
	p.sent(a, now)
	o.frame.id++
	o.collect = nil
	o.frame.context.questDialog = true
	o.readable = false
	p.observe(o, now.Add(300*time.Millisecond))
	if p.pending != nil {
		t.Fatal("claim/open was not confirmed by the quest dialog")
	}
	// A transient empty OCR result after the reward must not cause a return.
	o.frame.id++
	o.readable = false
	p.observe(o, now.Add(400*time.Millisecond))
	if _, ok := p.action(now.Add(500 * time.Millisecond)); ok || p.aborting {
		t.Fatal("one unreadable frame abandoned the visit")
	}
	o.frame.id++
	o.readable = true
	o.selected = -1
	o.quests = []mercenaryQuest{{reward: "gold", duration: time.Hour, point: image.Pt(1000, 390)}}
	p.observe(o, now.Add(600*time.Millisecond))
	a, ok = p.action(now.Add(600 * time.Millisecond))
	if !ok || a.mercenary.step != selectMercenaryQuest || p.questRow != row {
		t.Fatal("recovered frame did not select a quest for the fifth mercenary")
	}
	// Persistent unreadability has a bounded timeout and no guessed clicks.
	p.sent(a, now.Add(600*time.Millisecond))
	o.frame.id++
	o.selected, o.okay = 0, image.Pt(1815, 720)
	p.observe(o, now.Add(650*time.Millisecond))
	o.frame.id++
	o.frame.context.questDialog = true
	o.readable = false
	p.observe(o, now.Add(700*time.Millisecond))
	if _, ok := p.action(now.Add(800 * time.Millisecond)); ok {
		t.Fatal("unreadable quest transition clicked")
	}
	o.frame.id++
	p.observe(o, now.Add(2*time.Second))
	if _, ok := p.action(now.Add(5 * time.Second)); ok {
		t.Fatal("OCR retry deadline moved on another unreadable frame")
	}
	a, ok = p.action(now.Add(6 * time.Second))
	if !ok || a.mercenary.step != closeMercenaryQuest || !p.aborting {
		t.Fatal("persistent unreadability did not safely close the dialog")
	}
}

func TestMercenaryFastClaimAndSingleSweep(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	bounds := image.Rect(0, 0, 2560, 1440)
	roster := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, bounds: bounds}}, readable: true, top: true, thumbFound: true, thumb: image.Pt(1172, 890)}
	p := mercenaryPlanner{}
	id := uint64(0)
	observe := func(o mercenaryObservation) {
		id++
		now = now.Add(300 * time.Millisecond)
		o.frame.id, o.frame.at = id, now
		p.observe(o, now)
	}
	send := func(step mercenaryStep) gameAction {
		t.Helper()
		a, ok := p.action(now)
		if !ok || a.mercenary.step != step {
			t.Fatalf("wanted step %d got %+v ok=%t", step, a.mercenary, ok)
		}
		p.sent(a, now)
		return a
	}
	rows := []image.Point{image.Pt(768, 597), image.Pt(768, 810), image.Pt(768, 1027), image.Pt(768, 1240)}
	roster.collect = append([]image.Point(nil), rows...)
	observe(roster)
	dialog := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, questDialog: true, bounds: bounds}}, readable: true,
		quests: []mercenaryQuest{{reward: "rubies", duration: time.Hour, point: image.Pt(1000, 390)}}}
	for i := 0; i < 5; i++ {
		row := roster.collect[0]
		a := send(claimAndOpenMercenaryQuest)
		var clicks []image.Point
		input := heroInput{click: func(point image.Point) error { clicks = append(clicks, point); return nil }, move: func(image.Point) error { return nil }}
		pipeline := newGamePipeline(&pauseControl{}, input, pipelineReaders{}, pipelineOptions{})
		acted, err := pipeline.execute(ctx, a)
		if !acted || err != nil || len(clicks) != 2 || clicks[0] != row || clicks[1] != row {
			t.Fatalf("claim/start clicks=%v acted=%t err=%v", clicks, acted, err)
		}
		pipeline.controls.toggle()
		if acted, err := pipeline.execute(ctx, a); acted || err != nil || len(clicks) != 2 {
			t.Fatal("F8 allowed a queued claim/start pair")
		}
		dialog.selected, dialog.okay = -1, image.Point{}
		observe(dialog)
		send(selectMercenaryQuest)
		dialog.selected, dialog.okay = 0, image.Pt(1815, 720)
		observe(dialog)
		send(confirmMercenaryQuest)
		roster.collect = roster.collect[1:]
		roster.running = append(roster.running, row)
		observe(roster)
		if i == 3 {
			send(scrollMercenariesBottom)
			roster.top, roster.bottom = false, true
			roster.thumb = image.Pt(1172, 1055)
			roster.collect = []image.Point{image.Pt(768, 1245)}
			observe(roster)
		}
	}
	// Even if a later readable frame reports a different thumb position, the
	// completed sweep must not start another down/up cycle to wait for timers.
	roster.bottom = false
	observe(roster)
	send(returnToHeroes)
	observe(mercenaryObservation{frame: gameFrame{context: gameContext{known: true, heroes: true, bounds: bounds}}})
	if p.active || p.pending != nil {
		t.Fatal("five running quests did not finish the visit")
	}
	// A missed second click leaves Start Quest visible and must not cause a
	// rapid repeat of Collect/Start or a guessed selection.
	p = mercenaryPlanner{active: true, returnHeroes: true, topVisited: true}
	roster.collect, roster.running, roster.start = []image.Point{rows[0]}, nil, nil
	observe(roster)
	send(claimAndOpenMercenaryQuest)
	roster.collect, roster.start = nil, []image.Point{rows[0]}
	observe(roster)
	if _, ok := p.action(now); ok {
		t.Fatal("unconfirmed quest open emitted another click")
	}
	now = now.Add(6 * time.Second)
	observe(roster)
	send(returnToHeroes)
}

func TestMercenaryScrollContextFlickerDoesNotRestartVisit(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	im := loadTestImage(t, "testdata/mercenary-bottom-idle.png")
	bounds := im.Bounds()
	c := gameContext{known: true, mercenaries: true, bounds: bounds}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return im, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return c, nil }}, pipelineOptions{})
	p.frame = gameFrame{id: 1, image: im, context: c}
	p.mercenary = mercenaryPlanner{active: true, returnHeroes: true, topVisited: true}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	// Capture can see an intermediate HUD before the native drag returns and
	// actionCompleted installs its pending confirmation.
	c.known, c.mercenaries = false, false
	if err := p.capture(ctx, now.Add(100*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	if !p.mercenary.active || !p.mercenary.topVisited {
		t.Fatal("unknown context before input completion restarted the visit")
	}
	probe := p.mercenary
	if probe.expects(c, now.Add(6*time.Second)) {
		t.Fatal("unknown context retained a visit indefinitely")
	}
	c.known, c.mercenaries = true, true
	if err := p.capture(ctx, now.Add(200*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	p.mercenary.sent(gameAction{kind: handleMercenary, frame: p.frame, mercenary: mercenaryCommand{step: scrollMercenariesBottom}}, now)
	c.known, c.mercenaries = false, false
	if err := p.capture(ctx, now.Add(300*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	c.known, c.mercenaries = true, true
	if err := p.capture(ctx, now.Add(600*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	if !p.mercenary.topVisited || !p.mercenary.active || p.mercenary.pending == nil {
		t.Fatal("temporary unknown context restarted the sweep")
	}
	p.mercenary.observe(mercenaryObservation{frame: p.frame, readable: true, bottom: true, thumbFound: true, running: []image.Point{image.Pt(768, 1245)}}, now.Add(600*time.Millisecond))
	a, ok := p.mercenary.action(now.Add(600 * time.Millisecond))
	if !ok || a.mercenary.step != returnToHeroes {
		t.Fatal("confirmed bottom did not return to Heroes")
	}
	// A deliberate tab change still invalidates all mercenary work.
	c.mercenaries, c.heroes = false, true
	if err := p.capture(ctx, now.Add(900*time.Millisecond), jobs); err != nil {
		t.Fatal(err)
	}
	if p.mercenary.active {
		t.Fatal("manual tab change retained mercenary work")
	}
}

func TestMercenaryManualFailedQuestStillCollectsRemainingRewards(t *testing.T) {
	now := time.Now()
	bounds := image.Rect(0, 0, 2560, 1440)
	roster := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, bounds: bounds}}, readable: true, top: true, thumbFound: true, thumb: image.Pt(1172, 890), start: []image.Point{image.Pt(768, 1027)}}
	p := mercenaryPlanner{}
	id := uint64(0)
	observe := func(o mercenaryObservation) {
		now = now.Add(300 * time.Millisecond)
		id++
		o.frame.id = id
		o.frame.at = now
		p.observe(o, now)
	}
	send := func(step mercenaryStep) gameAction {
		t.Helper()
		a, ok := p.action(now)
		if !ok || a.mercenary.step != step {
			t.Fatalf("wanted step%d got%+v ok%t planner%+v", step, a.mercenary, ok, p)
		}
		p.sent(a, now)
		return a
	}
	observe(roster)
	send(openMercenaryQuest)
	dialog := mercenaryObservation{frame: gameFrame{context: gameContext{known: true, mercenaries: true, questDialog: true, bounds: bounds}}, selected: -1}
	observe(dialog)
	if _, ok := p.action(now); ok {
		t.Fatal("unreadable offers were clicked")
	}
	now = now.Add(6 * time.Second)
	send(closeMercenaryQuest)
	roster.start = nil
	roster.collect = []image.Point{image.Pt(768, 597), image.Pt(768, 810)}
	observe(roster)
	if !p.collectOnly || !p.returnHeroes {
		t.Fatal("manual failed selection did not recover into reward collection")
	}
	for len(roster.collect) > 0 {
		a := send(claimMercenaryReward)
		if a.point != roster.collect[0] {
			t.Fatal("wrong remaining reward clicked")
		}
		roster.start = append(roster.start, a.point)
		roster.collect = roster.collect[1:]
		observe(roster)
	}
	send(scrollMercenariesBottom)
	roster.top, roster.bottom = false, true
	roster.thumb = image.Pt(1172, 1055)
	roster.start = []image.Point{image.Pt(768, 810)}
	roster.collect = []image.Point{image.Pt(768, 1245)}
	observe(roster)
	a := send(claimMercenaryReward)
	if a.point != roster.collect[0] {
		t.Fatal("fifth mercenary reward was skipped")
	}
	roster.start = append(roster.start, a.point)
	roster.collect = nil
	observe(roster)
	send(returnToHeroes)
	observe(mercenaryObservation{frame: gameFrame{context: gameContext{known: true, heroes: true, bounds: bounds}}})
	if p.active || p.pending != nil {
		t.Fatal("manual error left the bot stuck in the roster")
	}
	// A missed reward click must stop recovery instead of retrying forever.
	p = mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, aborting: true, collectOnly: true}
	roster.collect = []image.Point{image.Pt(768, 1245)}
	observe(roster)
	send(claimMercenaryReward)
	now = now.Add(6 * time.Second)
	observe(roster)
	if p.collectOnly || !p.aborting {
		t.Fatal("unconfirmed reward click kept recovery active")
	}
	send(returnToHeroes)
}
