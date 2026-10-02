package main

import (
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
)

func TestAscensionRealControlsAndReward(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	modal := loadTestImage(t, "testdata/ascension-confirm.png")
	for _, width := range []int{2560, 1280} {
		var screen image.Image = modal
		if width != modal.Bounds().Dx() {
			resized := image.NewRGBA(image.Rect(0, 0, width, 720))
			xdraw.CatmullRom.Scale(resized, resized.Bounds(), modal, modal.Bounds(), draw.Src, nil)
			screen = resized
		}
		c, err := recognizedGame(screen)
		if err != nil || !c.known || !c.ascension || c.heroes || c.questDialog {
			t.Fatalf("width%d context=%+v err=%v", width, c, err)
		}
		out, err := readAscensionObservation(context.Background(), gameFrame{image: screen, context: c})
		if err != nil || !out.confirm || !out.no || math.Abs(out.souls-(65+math.Log10(1.588))) > .001 {
			t.Fatalf("width%d reward=%+v err=%v", width, out, err)
		}
		yes, found, err := ascensionControl(screen, 2)
		if err != nil || !found || yes.Y >= screen.Bounds().Dy()*75/100 {
			t.Fatal("Yes must be above the paid Quick Ascension control")
		}
		covered := image.NewRGBA(screen.Bounds())
		draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
		draw.Draw(covered, controlRect(screen, ascensionRewardRegion), image.NewUniform(color.Black), image.Point{}, draw.Src)
		if _, err := readAscensionObservation(context.Background(), gameFrame{image: covered, context: c}); err == nil {
			t.Fatal("covered reward accepted")
		}
		a := gameAction{kind: handleAscension, frame: gameFrame{image: screen, context: c}, ascension: confirmAscension, point: yes}
		if ascensionActionStable(a, gameFrame{image: covered, context: c}) {
			t.Fatal("changed reward retained a confirmation")
		}
	}
	for _, path := range []string{"testdata/hero-skogur-hire.png", "testdata/ascension-ancients.png"} {
		screen := loadTestImage(t, path)
		if _, found, err := ascensionControl(screen, 0); err != nil || !found {
			t.Fatalf("%s Ascension spiral missing: %v", path, err)
		}
		if dialog, err := ascensionDialog(screen); err != nil || dialog {
			t.Fatalf("ordinary HUD treated as Ascension dialog: %s %v", path, err)
		}
	}
	for _, path := range []string{"testdata/gild-chest.png", "testdata/gild-reward.png", "testdata/mercenary-quests.png"} {
		screen := loadTestImage(t, path)
		if dialog, err := ascensionDialog(screen); err != nil || dialog {
			t.Fatalf("unrelated dialog treated as Ascension: %s %v", path, err)
		}
	}
}

func TestAscensionStallRequiresObservedBossFallback(t *testing.T) {
	now := time.Now()
	stall := 15 * time.Minute
	p := ascensionPlanner{}
	// Starting on a farm zone, even with an old progression wall, cannot trigger a reset.
	for i := 0; i <= 200; i++ {
		at := now.Add(time.Duration(i) * 5 * time.Second)
		p.observeProgress(progressionState{Known: true, Zone: 14779}, 14780, at, false)
		if p.due(at, stall) {
			t.Fatal("ascended without observing a boss")
		}
	}
	p.interrupt()
	p.observeProgress(progressionState{Known: true, Enabled: true, Zone: 14780}, 0, now, false)
	for i := 1; i <= 180; i++ {
		at := now.Add(time.Duration(i) * 5 * time.Second)
		p.observeProgress(progressionState{Known: true, Zone: 14779}, 14780, at, false)
		if p.due(at, stall) != (i == 180) {
			t.Fatalf("stall threshold failed at %s", at.Sub(now))
		}
	}
	at := now.Add(stall)
	p.observeProgress(progressionState{Known: true, Enabled: true, Zone: 14780}, 14780, at, false)
	if p.due(at, stall) {
		t.Fatal("reset during a boss retry")
	}
	p.observeProgress(progressionState{Known: true, Enabled: true, Zone: 14781}, 0, at, false)
	if p.due(at, stall) {
		t.Fatal("reset while progressing")
	}
	// Observation gaps suspend decisions; fresh observations retain the wall history.
	for _, reason := range []string{"gap", "manual zone", "pause"} {
		q := ascensionPlanner{highestZone: 14780, wallZone: 14780, lastObservation: at, lastProgress: now}
		s := progressionState{Known: true, Zone: 14779}
		if reason == "manual zone" {
			s.Zone = 14000
		}
		if reason == "pause" {
			q.interrupt()
		}
		q.observeProgress(s, 14780, at.Add(11*time.Second), false)
		if q.due(at.Add(11*time.Second), stall) != (reason == "gap") {
			t.Fatalf("%s wall history wrong", reason)
		}
	}
}

func TestAscensionTransactionOwnershipAndReset(t *testing.T) {
	now := time.Now()
	dialog := gameFrame{id: 2, at: now, context: gameContext{known: true, ascension: true}}
	p := ascensionPlanner{}
	out := ascensionObservation{frame: dialog, confirm: true, no: true, souls: 65}
	p.observe(out, nil, now)
	if p.active || p.step == confirmAscension {
		t.Fatal("manual dialog acquired automatic confirmation")
	}
	p.sent(openAscension, 1, now)
	p.observe(out, nil, now)
	if p.step != confirmAscension {
		t.Fatal("verified reward did not allow confirmation")
	}
	p.sent(confirmAscension, 2, now)
	if p.observe(out, nil, now) || p.step != waitAscensionReset {
		t.Fatal("old confirmation frame treated as reset")
	}
	out.frame.id = 3
	out.zone = 1
	if p.observe(out, nil, now) {
		t.Fatal("zone without closing dialog treated as reset")
	}
	out.frame.context = gameContext{known: true, heroes: true}
	out.zone = 14779
	if p.observe(out, nil, now) {
		t.Fatal("old run treated as reset")
	}
	out.zone = 1
	if !p.observe(out, nil, now) || p.active {
		t.Fatal("confirmed reset did not finish transaction")
	}
	for _, failure := range []error{errors.New("OCR failed"), nil} {
		p.sent(openAscension, 1, now)
		out = ascensionObservation{frame: dialog, souls: math.Inf(-1)}
		p.observe(out, failure, now)
		if p.step != cancelAscension {
			t.Fatal("zero or unreadable reward allowed Yes")
		}
	}
	p.interrupt()
	p.observe(ascensionObservation{frame: dialog, souls: 65}, nil, now)
	if p.active {
		t.Fatal("F8 lost ownership but automatic transaction resumed")
	}
}

func TestAscensionPipelineIsolationAndPause(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/ascension-confirm.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	controls := &pauseControl{}
	p := newGamePipeline(controls, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, skills: true, progression: true, monster: true, fishInterval: time.Second})
	p.layout = 1
	p.frame = gameFrame{id: 2, layout: 1, at: now, image: screen, context: c}
	// A manual dialog blocks all unrelated input, even when automatic Ascension is disabled.
	for kind := collectFish; kind <= clickMonster; kind++ {
		if kind != handleAscension {
			p.enqueue(gameAction{kind: kind, frame: p.frame, key: 1}, now)
		}
	}
	p.plan(now)
	if action, ok := p.nextAction(now); ok {
		t.Fatalf("input through manual dialog: %+v", action)
	}
	p.ascension.sent(openAscension, 1, now.Add(-time.Second))
	out, err := readAscensionObservation(context.Background(), p.frame)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.accept(context.Background(), observation{kind: ascensionAnalysis, frame: p.frame, ascension: out}, now); err != nil {
		t.Fatal(err)
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != handleAscension || a.ascension != confirmAscension {
		t.Fatalf("confirmation missing: %+v %t", a, ok)
	}
	clicks := 0
	p.input.click = func(point image.Point) error {
		clicks++
		if point != a.point {
			t.Fatal("wrong Ascension target")
		}
		return nil
	}
	acted, err := p.execute(context.Background(), a)
	if err != nil || !acted || clicks != 1 {
		t.Fatalf("confirmation clicked=%t count=%d err=%v", acted, clicks, err)
	}
	p.actionCompleted(actionResult{action: a, acted: true}, now)
	if len(p.queue) != 0 || !p.ascension.active || p.ascension.step != waitAscensionReset {
		t.Fatal("confirmation retained old input or lost reset ownership")
	}
	// Preserve display/window identity while transitioning; otherwise the next
	// shared capture would mistake the transaction for a changed display.
	p.readers.context = func(image.Image) (gameContext, error) { return c, nil }
	p.input.capture = func() (image.Image, error) { return screen, nil }
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if !p.ascension.active {
		t.Fatal("shared capture lost Ascension transaction")
	}
	for i, job := range jobs {
		if i != int(ascensionAnalysis) && len(job) != 0 {
			t.Fatalf("background analyzer%d scheduled through modal", i)
		}
	}
	if len(jobs[ascensionAnalysis]) != 1 {
		t.Fatal("reset observation not scheduled on shared frame")
	}
	if err := p.accept(context.Background(), observation{kind: fishAnalysis, frame: p.frame, found: true}, now); err != nil || len(p.queue) != 0 {
		t.Fatal("late fish observation escaped modal isolation")
	}
	reset := p.frame
	reset.id++
	reset.context = gameContext{known: true, heroes: true, bounds: screen.Bounds()}
	p.frame = reset
	if err := p.accept(context.Background(), observation{kind: ascensionAnalysis, frame: reset, ascension: ascensionObservation{frame: reset, zone: 1}}, now); err != nil {
		t.Fatal(err)
	}
	if !controls.isPaused() || p.ascension.active {
		t.Fatal("confirmed reset did not pause for the next stage")
	}
	if acted, err := p.execute(context.Background(), a); err != nil || acted || clicks != 1 {
		t.Fatal("old confirmation replayed after reset pause")
	}
	controls.toggle()
	p.reset(controls.snapshot())
	p.frame = gameFrame{id: 9, generation: p.generation, layout: p.layout, at: now, image: screen, context: c}
	p.plan(now)
	if action, ok := p.nextAction(now); ok {
		t.Fatalf("F8 authorized a leftover manual dialog: %+v", action)
	}
}

func TestAscensionStaleDecisionAndTimeout(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true, ascension: true, ascensionStall: time.Minute, fishInterval: time.Second})
	p.layout = 1
	p.frame = gameFrame{id: 1, layout: 1, at: now, image: screen, context: c}
	p.ascension = ascensionPlanner{highestZone: 14780, wallZone: 14780, lastProgress: now.Add(-2 * time.Minute), lastObservation: now}
	p.state[heroAnalysis] = observation{frame: p.frame, hero: heroObservation{found: true}}
	p.ascension.latest = ascensionObservation{frame: p.frame, economy: true, bank: 60, souls: 65}
	p.state[progressionAnalysis] = observation{frame: p.frame, progression: progressionState{Known: true, Zone: 14779}}
	p.state[fishAnalysis] = observation{frame: p.frame}
	p.plan(now)
	if _, ok := p.queue[handleAscension]; !ok {
		t.Fatal("eligible Ascension not queued")
	}
	// A new boss win invalidates the queued opening before any click.
	p.ascension.observeProgress(progressionState{Known: true, Enabled: true, Zone: 14781}, 0, now, false)
	if action, ok := p.nextAction(now); ok {
		t.Fatalf("stale stalled-run decision executed: %+v", action)
	}
	p.ascension.sent(openAscension, 1, now.Add(-time.Minute))
	p.plan(now)
	if !p.controls.isPaused() {
		t.Fatal("missed/blocking dialog did not time out to pause")
	}
}

func TestAscensionOCRFailureRetainsHistoryButNeedsFreshProgress(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []analysisKind{heroAnalysis, progressionAnalysis} {
		p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, progression: true, ascension: true, ascensionStall: time.Minute, fishInterval: time.Second})
		p.layout = 1
		p.frame = gameFrame{id: 2, layout: 1, at: now, image: screen, context: c}
		p.ascension = ascensionPlanner{highestZone: 14780, wallZone: 14780, lastProgress: now.Add(-2 * time.Minute), lastObservation: now}
		old := p.frame
		old.id--
		p.state[heroAnalysis] = observation{frame: old, hero: heroObservation{found: true}}
		p.state[fishAnalysis] = observation{frame: p.frame}
		p.plan(now)
		if err := p.accept(context.Background(), observation{kind: kind, frame: p.frame, err: errors.New("OCR failed")}, now); err != nil {
			t.Fatal(err)
		}
		if p.ascension.highestZone != 14780 || p.ascension.lastProgress != now.Add(-2*time.Minute) {
			t.Fatal("OCR failure erased wall history")
		}
		if kind == progressionAnalysis && p.ascension.due(now, time.Minute) {
			t.Fatal("failed progression remained fresh")
		}
		if action, ok := p.nextAction(now); ok {
			t.Fatalf("automation failure triggered input: %+v", action)
		}
	}
}

func TestAscensionCombatWallAndRewardPolicy(t *testing.T) {
	now := time.Unix(1000, 0)
	farm := progressionState{Known: true, Zone: 109, DamageKnown: true, Damage: 100}
	boss := farm
	boss.Zone = 110
	boss.Enabled = true
	boss.FullCombat = true
	for _, tc := range []struct {
		name     string
		interval time.Duration
		full     bool
	}{
		{"overlapping combat", 2 * time.Second, true}, {"single observation", 0, false}, {"observation gap", 11 * time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			progress := progressionPlanner{}
			ascend := ascensionPlanner{}
			progress.observe(boss, now)
			ascend.observeProgress(boss, 0, now, false)
			progress.observe(boss, now.Add(tc.interval))
			failedAt := now.Add(tc.interval + time.Second)
			progress.observe(farm, failedAt)
			ascend.observeProgress(farm, progress.wallZone, failedAt, progress.wallFullCombat)
			if progress.wallFullCombat != tc.full || ascend.due(failedAt, 3*time.Minute) != tc.full {
				t.Fatalf("combat proof=%t due=%t", progress.wallFullCombat, ascend.due(failedAt, 3*time.Minute))
			}
			fullFarm := farm
			fullFarm.FullCombat = true
			if progress.observe(fullFarm, failedAt.Add(time.Second)) == tc.full {
				t.Fatal("fresh full window was lost, or previously failed full window retried")
			}
			if tc.full {
				improved := farm
				improved.Damage += 1
				if !progress.observe(improved, failedAt.Add(2*time.Second)) {
					t.Fatal("new damage after full combat loss did not allow an immediate retry")
				}
			}
		})
	}
	// An activation's farm baseline must never count as an actual full boss attempt.
	progress := progressionPlanner{seen: true, wallZone: 110, lastZone: 109}
	fullFarm := farm
	fullFarm.FullCombat = true
	progress.sent(fullFarm, 1, now)
	progress.observeFrame(progressionState{Known: true, Enabled: true}, 2, now.Add(time.Second))
	progress.observe(farm, now.Add(3*time.Second))
	if progress.wallFullCombat {
		t.Fatal("farm retry baseline fabricated a full boss fight")
	}
	states := [9]skillState{}
	for _, key := range []int{1, 2, 3, 7} {
		states[key-1] = skillState{Known: true, Active: true}
	}
	if !fullCombatActive(states) {
		t.Fatal("full combat not recognized")
	}
	states[1].Known = false
	if fullCombatActive(states) {
		t.Fatal("unknown combat skill counted as active")
	}

	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	frame := gameFrame{id: 2, at: now, image: screen, context: c}
	for _, tc := range []struct {
		name                  string
		bank, capital, reward float64
		eligible              bool
	}{
		{"large gain", 60, 62, 65, true}, {"small gain", 60, 62, 60, false}, {"empty bank", math.Inf(-1), math.Inf(-1), 0, true}, {"zero reward", math.Inf(-1), math.Inf(-1), math.Inf(-1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{ascension: true, ascensionStall: 3 * time.Minute, ascensionMinGain: .25, ascensionCapital: tc.capital, fishInterval: time.Second})
			p.frame = frame
			p.ascension = ascensionPlanner{highestZone: 110, wallZone: 110, fullCombatFailed: true, lastObservation: now, lastProgress: now}
			p.ascension.latest = ascensionObservation{frame: frame, economy: true, bank: tc.bank, souls: tc.reward}
			p.state[fishAnalysis] = observation{frame: frame}
			p.state[progressionAnalysis] = observation{frame: frame, progression: farm}
			if p.ascensionReady(now) != tc.eligible {
				t.Fatal("wrong reward decision")
			}
			if tc.eligible {
				p.plan(now)
				if _, ok := p.queue[handleAscension]; !ok {
					t.Fatal("no hero row should not block Ascension")
				}
				p.barriers[progressionAnalysis] = frame.id + 1
				if p.ascensionReady(now) {
					t.Fatal("pre-purchase damage allowed a reset")
				}
				p.barriers[progressionAnalysis] = 0
				p.ascension.sent(openAscension, frame.id, now)
				dialog := gameFrame{id: 3, at: now, context: gameContext{known: true, ascension: true}}
				p.ascension.observe(ascensionObservation{frame: dialog, souls: math.Inf(-1)}, nil, now)
				if p.ascension.step != cancelAscension {
					t.Fatal("lower dialog reward still confirmed")
				}
			}
		})
	}
	if math.Abs(soulCapital(60, 60)-(60+math.Log10(2))) > .0001 || !math.IsInf(soulCapital(math.Inf(-1), math.Inf(-1)), -1) {
		t.Fatal("large/zero soul sum wrong")
	}
}

func TestAscensionHUDRealBudget(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract not installed")
	}
	original := loadTestImage(t, "testdata/ascension-ancients.png")
	for _, width := range []int{2560, 1280} {
		screen := image.NewRGBA(image.Rect(0, 0, width, width*9/16))
		xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
		frame := gameFrame{image: screen}
		out, err := readAscensionEconomy(context.Background(), frame)
		if err != nil || !out.economy || math.Abs(out.bank-(58+math.Log10(1.755))) > .001 || math.Abs(out.souls-(65+math.Log10(1.719))) > .001 {
			t.Fatalf("%d HUD=%+v err=%v", width, out, err)
		}
		covered := image.NewRGBA(screen.Bounds())
		draw.Draw(covered, covered.Bounds(), screen, image.Point{}, draw.Src)
		draw.Draw(covered, controlRect(screen, ascensionSoulRegions[1]), image.NewUniform(color.Black), image.Point{}, draw.Src)
		if _, err := readAscensionEconomy(context.Background(), gameFrame{image: covered}); err == nil {
			t.Fatal("covered reward accepted")
		}
		if ascensionBudgetStable(screen, covered) {
			t.Fatal("changed budget accepted")
		}
	}
}

func TestAscensionCapitalAndPipelineRecovery(t *testing.T) {
	for _, tc := range []struct {
		plan *ancientPlan
		want float64
	}{
		{nil, math.Inf(-1)}, {&ancientPlan{Plan: ancientcalc.Plan{Souls: "1e60"}}, math.Inf(-1)}, {&ancientPlan{Plan: ancientcalc.Plan{Souls: "0", Invested: "0"}}, math.Inf(-1)},
		{&ancientPlan{Plan: ancientcalc.Plan{Souls: "1e1000", Invested: "1e1000"}}, 1000 + math.Log10(2)},
	} {
		got, err := ascensionSoulCapital(tc.plan)
		if err != nil || (got != tc.want && math.Abs(got-tc.want) > .0001) {
			t.Fatalf("capital=%v want=%v err=%v", got, tc.want, err)
		}
	}
	if _, err := ascensionSoulCapital(&ancientPlan{Plan: ancientcalc.Plan{Souls: "NaN", Invested: "1"}}); err == nil {
		t.Fatal("invalid capital accepted")
	}
	now := time.Now()
	screen := loadTestImage(t, "testdata/hero-skogur-hire.png")
	c, err := recognizedGame(screen)
	if err != nil {
		t.Fatal(err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return screen, nil }}, pipelineReaders{}, pipelineOptions{ascension: true, progression: true, ascensionStall: 3 * time.Minute, ascensionMinGain: .25, ascensionCapital: math.Inf(-1), fishInterval: time.Second})
	p.frame = gameFrame{id: 1, at: now, image: screen, context: c}
	p.ascension = ascensionPlanner{highestZone: 14780, wallZone: 14780, lastProgress: now.Add(-4 * time.Minute), lastObservation: now}
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	// A bot tab visit suspends decisions without erasing the wall.
	merc := c
	merc.heroes = false
	merc.mercenaries = true
	p.readers.context = func(image.Image) (gameContext, error) { return merc, nil }
	if err := p.capture(context.Background(), now, jobs); err != nil {
		t.Fatal(err)
	}
	if p.ascension.highestZone != 14780 || p.ascension.due(now, 3*time.Minute) {
		t.Fatal("tab visit lost history or retained a fresh decision")
	}
	p.readers.context = func(image.Image) (gameContext, error) { return c, nil }
	if err := p.capture(context.Background(), now.Add(time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	p.ascension.observeProgress(progressionState{Known: true, Zone: 14779}, 14780, now.Add(time.Second), false)
	if !p.ascension.due(now.Add(time.Second), 3*time.Minute) {
		t.Fatal("returned tab restarted the stall timer")
	}
	if err := p.capture(context.Background(), now.Add(2*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-jobs[ascensionAnalysis]:
		if !job.economy || job.frame.id != p.frame.id {
			t.Fatal("budget did not reuse the shared capture")
		}
	default:
		t.Fatal("wall budget not scheduled")
	}
	// Repeated successful purchases/skill casts may require fresh damage, but
	// cannot indefinitely push the wall decision ten seconds into the future.
	for i := 0; i < 3; i++ {
		for _, kind := range []actionKind{castSkill, buyHero} {
			p.actionCompleted(actionResult{action: gameAction{kind: kind, key: 1, frame: p.frame}, acted: true}, now.Add(time.Duration(i)*time.Second))
			if !p.ascension.nextCheck.IsZero() {
				t.Fatal("ordinary actions postponed the reset")
			}
		}
	}
	// Gild collection uses reset() internally; preserve the same wall there too.
	p.actionCompleted(actionResult{action: gameAction{kind: collectGilds, frame: p.frame}, acted: true}, now)
	if err := p.capture(context.Background(), now.Add(3*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if p.ascension.highestZone != 14780 {
		t.Fatal("gild reset or subsequent capture erased wall history")
	}
	p.progression.wallZone = 14780
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if p.ascension.highestZone != 0 || p.progression.wallZone != 0 {
		t.Fatal("F8 retained an old reset decision or blocked a fresh boss assessment")
	}
}
