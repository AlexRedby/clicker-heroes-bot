package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
	"time"
)

type analysisKind uint8

const (
	fishAnalysis analysisKind = iota
	skillAnalysis
	progressionAnalysis
	heroAnalysis
	mercenaryAnalysis
	analysisCount
)

type actionKind uint8

const (
	collectFish actionKind = iota
	collectGilds
	castSkill
	enableProgression
	handleMercenary
	parkPointer
	selectQuantity
	scrollHeroes
	buyHero
	clickMonster
)

type gameContext struct {
	known, heroes, mercenaries, questDialog bool
	modal                                   gildModal
	window                                  string
	bounds                                  image.Rectangle
}
type gameFrame struct {
	id, generation, layout uint64
	at                     time.Time
	image                  image.Image
	context                gameContext
}
type analysisJob struct {
	frame      gameFrame
	heroBefore *heroObservation
	modeOnly   bool
	skills     [9]skillState
}
type observation struct {
	kind        analysisKind
	frame       gameFrame
	elapsed     time.Duration
	point       image.Point
	found       bool
	skills      [9]skillState
	progression progressionState
	hero        heroObservation
	mercenary   mercenaryObservation
	err         error
}
type gameAction struct {
	kind           actionKind
	frame          gameFrame
	point, target  image.Point
	key            int
	hero           heroObservation
	skills         [9]skillState
	progression    progressionState
	mercenary      mercenaryCommand
	mercenaryThumb image.Point
	queuedAt       time.Time
}
type actionResult struct {
	action  gameAction
	acted   bool
	elapsed time.Duration
	err     error
}

type pipelineReaders struct {
	context     func(image.Image) (gameContext, error)
	fish        func(image.Image) (image.Point, bool, error)
	skills      func(context.Context, image.Image) ([9]skillState, error)
	progression func(context.Context, image.Image, [9]skillState, bool) (progressionState, error)
	mercenaries func(context.Context, gameFrame) (mercenaryObservation, error)
	heroes      heroReaders
	window      func() string
}
type pipelineOptions struct {
	heroes, skills, progression, mercenaries, monster, gilds bool
	monsterPoint                                             image.Point
	fishInterval, clickInterval, gildInterval                time.Duration
}
type pipelineMetrics struct {
	captures                          uint64
	counts                            [analysisCount]uint64
	elapsed                           [analysisCount]time.Duration
	captureTime, inputTime, queueTime time.Duration
	actions, dropped                  uint64
}

type gamePipeline struct {
	readers                                             pipelineReaders
	options                                             pipelineOptions
	input                                               heroInput
	controls                                            *pauseControl
	frame                                               gameFrame
	state                                               [analysisCount]observation
	barriers                                            [analysisCount]uint64
	queue                                               map[actionKind]gameAction
	hero                                                heroRunner
	skill                                               skillPlanner
	progression                                         progressionPlanner
	mercenary                                           mercenaryPlanner
	fish                                                fishClickTracker
	gild                                                gildCollector
	metrics                                             pipelineMetrics
	generation, layout                                  uint64
	nextCapture, nextFish, nextProgression, nextMonster time.Time
	busy                                                bool
	settleUntil                                         time.Time
	diagnostics                                         chan heroAttempt
	progressionJobs                                     chan analysisJob
	focusFallback                                       bool
	heroJobFrame                                        uint64
	mercenaryJobFrame                                   uint64
	nextMercenary                                       time.Time
}

func newGamePipeline(controls *pauseControl, input heroInput, readers pipelineReaders, options pipelineOptions) *gamePipeline {
	p := &gamePipeline{controls: controls, input: input, readers: readers, options: options, hero: heroRunner{enabled: options.heroes}, queue: make(map[actionKind]gameAction), diagnostics: make(chan heroAttempt, 1)}
	p.hero.onFailure = func(a heroAttempt) {
		select {
		case p.diagnostics <- a:
		default:
			fmt.Println("hero diagnostic worker busy; skipped screenshots")
		}
	}
	return p
}

func recognizedGame(screen image.Image) (gameContext, error) {
	c := gameContext{}
	if screen == nil {
		return c, errors.New("capture returned no image")
	}
	c.bounds = screen.Bounds()
	// The quest dialog dims the HUD, including the normal game-context anchor.
	if mercenaryQuestDialog(screen) {
		c.known, c.mercenaries, c.questDialog = true, true, true
		return c, nil
	}
	modal, err := readGildModal(screen)
	c.modal = modal
	if err != nil || modal != noGildModal {
		c.known = modal != unknownGildModal
		return c, err
	}
	known, _, err := progressionMode(screen)
	c.known = known
	c.heroes = known && heroTabSelected(screen)
	c.mercenaries = known && mercenaryTabSelected(screen)
	return c, err
}

func (p *gamePipeline) analyze(ctx context.Context, kind analysisKind, job analysisJob) observation {
	out := observation{kind: kind, frame: job.frame}
	start := time.Now()
	switch kind {
	case fishAnalysis:
		out.point, out.found, out.err = p.readers.fish(job.frame.image)
	case skillAnalysis:
		out.skills, out.err = p.readers.skills(ctx, job.frame.image)
	case progressionAnalysis:
		out.progression, out.err = p.readers.progression(ctx, job.frame.image, job.skills, job.modeOnly)
	case heroAnalysis:
		out.hero, out.err = readHeroObservation(ctx, job.frame, p.readers.heroes, job.heroBefore)
	case mercenaryAnalysis:
		out.mercenary, out.err = p.readers.mercenaries(ctx, job.frame)
	}
	out.elapsed = time.Since(start)
	return out
}

// Each worker has one running job and one replaceable waiting frame.
func replaceJob(ch chan analysisJob, job analysisJob) {
	select {
	case ch <- job:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- job:
	default:
	}
}

func (p *gamePipeline) reset(generation uint64) {
	p.generation = generation
	p.layout++
	p.queue = make(map[actionKind]gameAction)
	p.state = [analysisCount]observation{}
	p.barriers = [analysisCount]uint64{}
	p.hero.interrupt()
	p.gild.interrupt()
	p.heroJobFrame = 0
	p.skill.interrupt()
	p.progression.pending = nil
	p.progression.wantAction = false
	p.mercenary.interrupt()
	p.mercenaryJobFrame = 0
	p.nextMercenary = time.Time{}
	p.nextCapture, p.nextFish, p.nextProgression = time.Time{}, time.Time{}, time.Time{}
	p.settleUntil = time.Time{}
}

func (p *gamePipeline) run(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); close(p.diagnostics); wg.Wait() }()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for failure := range p.diagnostics {
			saveHeroFailure(failure.action.hero, failure.last)
		}
	}()
	results := make(chan observation, 8)
	jobs := make([]chan analysisJob, analysisCount)
	for kind := analysisKind(0); kind < analysisCount; kind++ {
		jobs[kind] = make(chan analysisJob, 1)
		wg.Add(1)
		go func(kind analysisKind) {
			defer wg.Done()
			for {
				select {
				case <-workerCtx.Done():
					return
				case job := <-jobs[kind]:
					if !p.controls.valid(workerCtx, job.frame.generation) {
						continue
					}
					out := p.analyze(workerCtx, kind, job)
					select {
					case results <- out:
					case <-workerCtx.Done():
						return
					}
				}
			}
		}(kind)
	}
	actions := make(chan gameAction)
	actionDone := make(chan actionResult, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-workerCtx.Done():
				return
			case action := <-actions:
				start := time.Now()
				acted, err := p.execute(workerCtx, action)
				select {
				case actionDone <- actionResult{action, acted, time.Since(start), err}:
				case <-workerCtx.Done():
					return
				}
			}
		}
	}()
	timer := time.NewTicker(25 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case out := <-results:
			if err := p.accept(ctx, out, time.Now()); err != nil {
				return err
			}
		case done := <-actionDone:
			p.busy = false
			if done.err != nil {
				return done.err
			}
			if done.acted {
				p.controls.runClick(ctx, done.action.frame.generation, func() error {
					if done.action.frame.layout == p.layout {
						p.actionCompleted(done, time.Now())
					}
					return nil
				})
			}
		case <-timer.C:
			generation := p.controls.snapshot()
			if generation != p.generation {
				p.reset(generation)
			}
			if !p.controls.valid(ctx, generation) {
				continue
			}
			now := time.Now()
			if !p.busy && !now.Before(p.nextCapture) && !now.Before(p.settleUntil) {
				if err := p.capture(ctx, now, jobs); err != nil {
					return err
				}
			}
		}
		if !p.busy && p.controls.valid(ctx, p.generation) {
			now := time.Now()
			p.plan(now)
			if action, ok := p.nextAction(now); ok {
				p.busy = true
				p.metrics.queueTime += now.Sub(action.queuedAt)
				select {
				case actions <- action:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}
}

func (p *gamePipeline) capture(ctx context.Context, now time.Time, jobs []chan analysisJob) error {
	start := time.Now()
	image, err := p.input.capture()
	p.metrics.captureTime += time.Since(start)
	p.metrics.captures++
	if err != nil {
		return fmt.Errorf("capture game: %w", err)
	}
	if !p.controls.valid(ctx, p.generation) {
		return nil
	}
	c, err := p.readers.context(image)
	if err != nil {
		return err
	}
	if p.readers.window != nil {
		c.window = p.readers.window()
		if c.window == "!outside-game" {
			c.known, c.heroes, c.mercenaries, c.questDialog, c.modal = false, false, false, false, noGildModal
		}
		if c.window == "" && !p.focusFallback {
			fmt.Println("foreground window identity unavailable; using visual context and F8")
			p.focusFallback = true
		}
	}
	_, err = p.controls.runClick(ctx, p.generation, func() error {
		old := p.frame.context
		if c.known != old.known || c.modal != old.modal || c.heroes != old.heroes || c.mercenaries != old.mercenaries || c.questDialog != old.questDialog || c.bounds != old.bounds || c.window != old.window {
			if c.bounds != old.bounds || c.window != old.window || !p.mercenary.expects(c) {
				p.mercenary.interrupt()
			}
			p.mercenaryJobFrame = 0
			p.nextMercenary = time.Time{}
			p.layout++
			for _, ch := range jobs {
				select {
				case <-ch:
				default:
				}
			}
			p.queue = make(map[actionKind]gameAction)
			p.state = [analysisCount]observation{}
			p.hero.interrupt()
			p.heroJobFrame = 0
			p.skill.interrupt()
			p.progression.pending = nil
			p.progression.wantAction = false
			p.nextFish, p.nextProgression = time.Time{}, time.Time{}
		}
		p.frame = gameFrame{p.frame.id + 1, p.generation, p.layout, now, image, c}
		// Slow capture must not consume its own interval and immediately repeat.
		p.nextCapture = now.Add(time.Since(start) + min(250*time.Millisecond, p.options.fishInterval))
		if !c.known || c.modal != noGildModal || p.gild.active {
			return nil
		}
		if p.options.mercenaries && (c.heroes || c.mercenaries) && p.mercenaryJobFrame == 0 && !now.Before(p.mercenary.nextScan) && !now.Before(p.nextMercenary) {
			p.mercenaryJobFrame = p.frame.id
			replaceJob(jobs[mercenaryAnalysis], analysisJob{frame: p.frame})
			p.nextMercenary = now.Add(2 * time.Second)
			if c.mercenaries && p.mercenary.active {
				p.nextMercenary = now.Add(200 * time.Millisecond)
			}
		}
		// No clicks or hotkeys may reach controls hidden underneath a dialog.
		if c.questDialog {
			return nil
		}
		if !now.Before(p.nextFish) {
			replaceJob(jobs[fishAnalysis], analysisJob{frame: p.frame})
			p.nextFish = now.Add(p.options.fishInterval)
		}
		if p.options.progression && p.progression.pending != nil && !now.Before(p.nextProgression) {
			replaceJob(jobs[progressionAnalysis], analysisJob{frame: p.frame, modeOnly: true})
			p.nextProgression = now.Add(150 * time.Millisecond)
		}
		if p.options.skills || p.options.progression {
			replaceJob(jobs[skillAnalysis], analysisJob{frame: p.frame})
		}
		if p.options.heroes && c.heroes && p.hero.due(now) && p.heroJobFrame == 0 && (p.hero.latest.frame.id == 0 || p.hero.pending != nil) {
			p.heroJobFrame = p.frame.id
			replaceJob(jobs[heroAnalysis], analysisJob{frame: p.frame, heroBefore: p.hero.before()})
		}
		// Progression receives the skill observation for this exact frame, in accept().
		p.progressionJobs = jobs[progressionAnalysis]
		return nil
	})
	return err
}

func (p *gamePipeline) accept(ctx context.Context, out observation, now time.Time) error {
	if out.kind == mercenaryAnalysis && out.frame.id == p.mercenaryJobFrame {
		p.mercenaryJobFrame = 0
	}
	if out.kind == heroAnalysis && out.frame.id == p.heroJobFrame {
		p.heroJobFrame = 0
	}
	p.metrics.counts[out.kind]++
	p.metrics.elapsed[out.kind] += out.elapsed
	accepted, err := p.controls.runClick(ctx, out.frame.generation, func() error { return p.applyObservation(ctx, out, now) })
	if !accepted {
		p.metrics.dropped++
	}
	return err
}

func (p *gamePipeline) applyObservation(ctx context.Context, out observation, now time.Time) error {
	if out.frame.layout != p.layout || p.frame.context.modal != noGildModal || p.gild.active || out.frame.id < p.barriers[out.kind] || out.frame.id <= p.state[out.kind].frame.id {
		p.metrics.dropped++
		return nil
	}
	if out.err != nil {
		if ctx.Err() != nil {
			return nil
		}
		switch out.kind {
		case mercenaryAnalysis:
			out.mercenary = mercenaryObservation{frame: out.frame, selected: -1}
			fmt.Printf("mercenary panel unreadable: %v\n", out.err)
		case heroAnalysis:
			if p.hero.pending == nil {
				p.hero.readFailed(out.err, now)
				return nil
			}
		case progressionAnalysis:
			p.nextProgression = now.Add(30 * time.Second)
			fmt.Printf("progression numbers unreadable: %v; retrying in 30s\n", out.err)
			return nil
		default:
			return out.err
		}
	}
	p.state[out.kind] = out
	switch out.kind {
	case fishAnalysis:
		if p.fish.shouldClick(out.point, out.found) {
			p.enqueue(gameAction{kind: collectFish, frame: out.frame, point: out.point}, now)
		} else {
			delete(p.queue, collectFish)
		}
		if out.found {
			p.hero.obstructed(now)
			delete(p.queue, buyHero)
			delete(p.queue, scrollHeroes)
		}
		p.hero.finishFailure(out, now)
	case skillAnalysis:
		p.skill.observeFrame(out.skills, out.frame.id, out.frame.generation, now)
		if p.options.progression && p.progression.pending == nil && !now.Before(p.nextProgression) && p.progressionJobs != nil {
			replaceJob(p.progressionJobs, analysisJob{frame: out.frame, skills: out.skills, modeOnly: p.progression.pending != nil})
			delay := 2 * time.Second
			if p.progression.pending != nil {
				delay = 150 * time.Millisecond
			}
			p.nextProgression = now.Add(delay)
		}
	case heroAnalysis:
		p.hero.observe(out.hero, p.state[fishAnalysis], now)
	case progressionAnalysis:
		p.progression.observeFrame(out.progression, out.frame.id, now)
		if !p.progression.wantAction {
			delete(p.queue, enableProgression)
		}
	case mercenaryAnalysis:
		p.mercenary.observe(out.mercenary, now)
		delete(p.queue, handleMercenary)
	}
	return nil
}

func (p *gamePipeline) enqueue(action gameAction, now time.Time) {
	if old, ok := p.queue[action.kind]; ok && old.frame.id > action.frame.id {
		return
	}
	action.queuedAt = now
	if old, ok := p.queue[action.kind]; ok {
		action.queuedAt = old.queuedAt
	}
	p.queue[action.kind] = action
}
func (p *gamePipeline) fishFresh(now time.Time) bool {
	fish := p.state[fishAnalysis]
	return fish.frame.id > 0 && now.Sub(fish.frame.at) <= max(3*time.Second, fish.elapsed+p.options.fishInterval*2)
}
func (p *gamePipeline) plan(now time.Time) {
	if now.Before(p.settleUntil) {
		return
	}
	if p.planGilds(now) || !p.frame.context.known {
		return
	}
	if p.options.mercenaries {
		if action, ok := p.mercenary.action(now); ok {
			p.enqueue(action, now)
		}
	}
	if p.frame.context.questDialog {
		return
	}
	if p.options.skills {
		if key := p.skill.nextKey(p.state[skillAnalysis].skills, p.state[skillAnalysis].frame.id, now); key > 0 {
			p.enqueue(gameAction{kind: castSkill, frame: p.state[skillAnalysis].frame, key: key, skills: p.state[skillAnalysis].skills}, now)
		}
	}
	if p.options.progression && p.progression.wantAction && p.progression.pending == nil && now.After(p.progression.nextAttempt) {
		p.enqueue(gameAction{kind: enableProgression, frame: p.state[progressionAnalysis].frame, progression: p.state[progressionAnalysis].progression}, now)
	}
	if p.options.heroes && p.frame.context.heroes && !p.mercenary.active && p.mercenary.pending == nil {
		if action, ok := p.hero.action(now); ok {
			p.enqueue(action, now)
		}
	}
	if p.options.monster && !now.Before(p.nextMonster) {
		p.enqueue(gameAction{kind: clickMonster, frame: p.frame, point: p.options.monsterPoint}, now)
		p.nextMonster = now.Add(p.options.clickInterval)
	}
}

func (p *gamePipeline) nextAction(now time.Time) (gameAction, bool) {
	// A tab click can open a modal before capture observes it. Do not send a
	// previously queued hotkey/fish/monster click during that transition.
	if p.mercenary.pending != nil && p.frame.id <= p.mercenary.pending.action.frame.id {
		return gameAction{}, false
	}
	for kind := collectFish; kind <= clickMonster; kind++ {
		action, ok := p.queue[kind]
		if !ok {
			continue
		}
		if action.frame.layout != p.layout || action.frame.generation != p.generation || !p.frame.context.known || ((p.frame.context.modal != noGildModal || p.gild.active) && kind != collectGilds) {
			delete(p.queue, kind)
			continue
		}
		if p.frame.context.questDialog && kind != handleMercenary {
			delete(p.queue, kind)
			continue
		}
		if now.Sub(action.frame.at) > max(3*time.Second, p.state[fishAnalysis].elapsed+p.options.fishInterval*2) {
			delete(p.queue, kind)
			if kind == handleMercenary {
				p.mercenary.latest = mercenaryObservation{}
				p.nextMercenary = time.Time{}
			}
			if kind == buyHero || kind == scrollHeroes || kind == selectQuantity || kind == parkPointer {
				p.hero.latest = heroObservation{}
				p.hero.nextScan = now
			}
			continue
		}
		if kind == collectGilds && action.frame.id != p.frame.id {
			point, found, err := gildActionPoint(p.frame)
			if err != nil || !found || point != action.point {
				delete(p.queue, kind)
				continue
			}
		}
		if kind == enableProgression {
			known, enabled, err := progressionMode(p.frame.image)
			if err != nil || !known || enabled {
				delete(p.queue, kind)
				p.progression.wantAction = false
				continue
			}
		}
		if kind == castSkill && (p.skill.pending != nil || !p.state[skillAnalysis].skills[action.key-1].Known || !p.state[skillAnalysis].skills[action.key-1].Ready) {
			delete(p.queue, kind)
			continue
		}
		if kind == collectFish {
			if p.state[fishAnalysis].frame.id != action.frame.id || !p.state[fishAnalysis].found || !p.fishFresh(now) {
				delete(p.queue, kind)
				continue
			}
		}
		if kind == handleMercenary {
			if p.mercenary.pending != nil || p.mercenary.latest.frame.id != action.frame.id || !mercenaryActionStable(action, p.frame) {
				delete(p.queue, kind)
				p.mercenary.latest = mercenaryObservation{}
				p.mercenary.nextScan = now
				p.nextMercenary = time.Time{}
				continue
			}
			if !p.frame.context.questDialog && (!p.fishFresh(now) || p.state[fishAnalysis].found) {
				continue
			}
		}
		if kind == buyHero || kind == scrollHeroes || kind == selectQuantity {
			if !p.frame.context.heroes {
				delete(p.queue, kind)
				continue
			}
			if kind != selectQuantity && (!p.fishFresh(now) || p.state[fishAnalysis].found) {
				continue
			}
			if kind == buyHero && (!heroListStable(action.frame.image, p.frame.image) || !heroRowNameMatches(action.frame.image, p.frame.image, action.point, action.point) || !heroQuantitySelected(p.frame.image, 122)) {
				delete(p.queue, kind)
				p.hero.interrupt()
				continue
			}
			if kind == scrollHeroes {
				thumb, _, found := heroScrollbarThumb(p.frame.image)
				if !found || absDiff(thumb.Y, action.point.Y) > max(3, p.frame.context.bounds.Dy()/100) {
					delete(p.queue, kind)
					p.hero.interrupt()
					continue
				}
			}
		}
		delete(p.queue, kind)
		return action, true
	}
	return gameAction{}, false
}

var errInputContext = errors.New("input context changed")

func (p *gamePipeline) execute(ctx context.Context, a gameAction) (bool, error) {
	acted, err := p.controls.runClick(ctx, a.frame.generation, func() error {
		if p.readers.window != nil && p.readers.window() != a.frame.context.window {
			return errInputContext
		}
		switch a.kind {
		case handleMercenary:
			if a.mercenary.step == scrollMercenariesTop || a.mercenary.step == scrollMercenariesBottom {
				return p.input.drag(a.point, a.target)
			}
			if err := p.input.click(a.point); err != nil {
				return err
			}
			return p.input.move(parkPoint(a.frame.context.bounds))
		case clickMonster:
			return p.input.monsterClick(a.point)
		case collectFish, collectGilds:
			return p.input.click(a.point)
		case castSkill:
			return holdGameKey(ctx, p.input, fmt.Sprint(a.key))
		case enableProgression:
			return holdGameKey(ctx, p.input, "a")
		case selectQuantity:
			return p.input.keyTap("t")
		case scrollHeroes:
			return p.input.drag(a.point, a.target)
		case parkPointer:
			return p.input.move(a.point)
		case buyHero:
			if err := clickHeroMax(ctx, p.input, a.point); err != nil {
				return err
			}
			return p.input.move(parkPoint(a.frame.context.bounds))
		}
		return fmt.Errorf("unknown game action %d", a.kind)
	})
	if errors.Is(err, errInputContext) {
		return false, nil
	}
	return acted, err
}
func parkPoint(bounds image.Rectangle) image.Point {
	return bounds.Min.Add(image.Pt(bounds.Dx()*85/100, bounds.Dy()/2))
}

func (p *gamePipeline) actionCompleted(done actionResult, now time.Time) {
	a := done.action
	p.metrics.actions++
	p.metrics.inputTime += done.elapsed
	if a.kind == clickMonster {
		return
	}
	invalidate := func(kind analysisKind) { p.barriers[kind] = p.frame.id + 1; p.state[kind] = observation{} }
	switch a.kind {
	case collectGilds:
		p.gild.active = true
		p.gild.opening = a.frame.context.modal == noGildModal
		if a.point == p.gild.lastPoint {
			p.gild.attempts++
		} else {
			p.gild.attempts = 1
		}
		p.gild.lastPoint = a.point
		p.gild.nextAction = now.Add(time.Second)
		if p.gild.deadline.IsZero() {
			p.gild.deadline = now.Add(20 * time.Second)
		}
		gild := p.gild
		p.reset(p.generation)
		p.gild = gild
		p.frame.context = gameContext{}
		fmt.Printf("clicked gild gift control at (%d, %d)\n", a.point.X, a.point.Y)

	case handleMercenary:
		p.mercenary.sent(a, now)
		p.nextMercenary = time.Time{}
		invalidate(mercenaryAnalysis)
		invalidate(fishAnalysis)
		p.nextFish = time.Time{}
		delete(p.queue, collectFish)
		delete(p.queue, buyHero)
		delete(p.queue, scrollHeroes)
	case collectFish:
		p.fish.recordClick(a.point)
		invalidate(fishAnalysis)
		invalidate(heroAnalysis)
		invalidate(mercenaryAnalysis)
		p.mercenary.latest = mercenaryObservation{}
		p.mercenary.nextScan = now
		p.nextMercenary = time.Time{}
		delete(p.queue, handleMercenary)
		p.hero.obstructed(now)
		fmt.Printf("clicked fish at (%d, %d)\n", a.point.X, a.point.Y)
	case castSkill:
		p.skill.sent(a.key, a.skills, a.frame.id, now)
		invalidate(skillAnalysis)
		invalidate(heroAnalysis)
		invalidate(progressionAnalysis)
		delete(p.queue, buyHero)
		if p.hero.pending == nil {
			p.hero.latest = heroObservation{}
			p.hero.nextScan = now
		}
		delete(p.queue, enableProgression)
		p.progression.wantAction = false
		p.nextProgression = time.Time{}
	case enableProgression:
		p.progression.sent(a.progression, a.frame.id, now)
		invalidate(progressionAnalysis)
		p.nextProgression = time.Time{}
	case buyHero, scrollHeroes, selectQuantity, parkPointer:
		p.hero.sent(a, now)
		if a.kind == scrollHeroes {
			fmt.Printf("dragged hero scrollbar from (%d, %d); waiting for bottom confirmation\n", a.point.X, a.point.Y)
		}
		invalidate(heroAnalysis)
		if a.kind == buyHero {
			invalidate(progressionAnalysis)
			delete(p.queue, enableProgression)
			p.progression.wantAction = false
			p.nextProgression = time.Time{}
		}
	}
	p.settleUntil = now.Add(150 * time.Millisecond)
	if a.kind == buyHero || a.kind == scrollHeroes || a.kind == parkPointer || a.kind == handleMercenary {
		p.settleUntil = now.Add(200 * time.Millisecond)
	}
	p.nextCapture = time.Time{}
}

func (m pipelineMetrics) String() string {
	parts := []string{fmt.Sprintf("captures=%d capture=%s actions=%d input=%s queue=%s stale=%d", m.captures, m.captureTime, m.actions, m.inputTime, m.queueTime, m.dropped)}
	for i, name := range []string{"fish", "skills", "progression", "heroes", "mercenaries"} {
		parts = append(parts, fmt.Sprintf("%s=%d/%s", name, m.counts[i], m.elapsed[i]))
	}
	return strings.Join(parts, " ")
}

// Gift polling reuses the current shared image; modal steps never schedule background analyzers.
func (p *gamePipeline) planGilds(now time.Time) bool {
	modal := p.frame.context.modal
	if p.frame.context.questDialog {
		// A manually opened quest dialog supersedes an interrupted gift batch.
		p.gild = gildCollector{}
		delete(p.queue, collectGilds)
		return false
	}
	// Finish a mercenary visit before opening another transaction's window.
	if p.mercenary.active && modal == noGildModal && !p.gild.active {
		delete(p.queue, collectGilds)
		return false
	}
	if !p.options.gilds {
		return modal != noGildModal
	}
	if p.gild.active && p.frame.context.known && modal == noGildModal && !p.gild.opening && !now.Before(p.gild.nextAction) {
		p.gild.active = false
		p.gild.deadline = time.Time{}
		p.gild.nextCheck = now.Add(p.options.gildInterval)
		p.gild.attempts = 0
		p.hero.nextScan = now
		fmt.Println("finished opening earned gild gifts")
	}
	exclusive := p.gild.active || modal != noGildModal
	if !p.gild.active && modal != noGildModal && modal != gildChestModal && modal != gildRewardModal {
		return true
	}
	if exclusive && p.gild.deadline.IsZero() {
		p.gild.deadline = now.Add(20 * time.Second)
	}
	if exclusive && (now.After(p.gild.deadline) || p.gild.attempts >= 3) {
		fmt.Println("gild gift window did not advance; paused, check it and press F8 to resume")
		p.controls.pause()
		return true
	}
	if !p.frame.context.known {
		return exclusive
	}
	if now.Before(p.gild.nextAction) || (!exclusive && now.Before(p.gild.nextCheck)) {
		return exclusive
	}
	if !exclusive {
		p.gild.nextCheck = now.Add(p.options.gildInterval)
	}
	p.gild.nextAction = now.Add(250 * time.Millisecond)
	point, found, err := gildActionPoint(p.frame)
	if err != nil {
		fmt.Printf("gild controls unreadable: %v\n", err)
		return exclusive
	}
	if found {
		p.enqueue(gameAction{kind: collectGilds, frame: p.frame, point: point}, now)
	}
	if modal != noGildModal {
		p.gild.opening = false
	}
	return exclusive
}
