package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
	"sync"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

type analysisKind uint8

const (
	fishAnalysis analysisKind = iota
	skillAnalysis
	progressionAnalysis
	heroAnalysis
	mercenaryAnalysis
	ascensionAnalysis
	ancientAnalysis
	exportAnalysis
	outsiderAnalysis
	autoClickerAnalysis
	analysisCount
)

type actionKind uint8

const (
	collectFish actionKind = iota
	collectGilds
	castSkill
	enableProgression
	placeOwnedClicker
	handleMercenary
	handleAscension
	handleAncient
	handleExport
	parkPointer
	selectQuantity
	scrollHeroes
	buyHero
	buyHeroUpgrades
	visitHeroes
	clickMonster
)

type gameContext struct {
	known, heroes, mercenaries, questDialog, ascension, ancients, ancientDialog, saveMenu, outsiders bool
	modal                                                                                            gildModal
	window                                                                                           string
	bounds                                                                                           image.Rectangle
	geometry                                                                                         viewportGeometry
}
type gameFrame struct {
	id, generation, layout uint64
	at                     time.Time
	image                  image.Image
	context                gameContext
}
type analysisJob struct {
	frame        gameFrame
	export       *exportJob
	heroBefore   *heroObservation
	startup      startupPhase
	sweep        startupSweep
	upgrades     bool
	modeOnly     bool
	ancientNames bool
	economy      bool
	skills       [9]skillState
	outsiderBase *ancientcalc.TranscensionPreview
}
type observation struct {
	kind          analysisKind
	startup       startupPhase
	upgrades      bool
	upgradesKnown bool
	frame         gameFrame
	elapsed       time.Duration
	point         image.Point
	found         bool
	skills        [9]skillState
	progression   progressionState
	hero          heroObservation
	mercenary     mercenaryObservation
	ascension     ascensionObservation
	ancient       ancientObservation
	export        exportResult
	outsider      outsiderObservation
	outsiderPlan  outsiderAdvice
	clickerPool   autoClickerPool
	err           error
}
type gameAction struct {
	kind          actionKind
	frame         gameFrame
	point, target image.Point
	key           int
	hero          heroObservation
	skills        [9]skillState
	progression   progressionState
	mercenary     mercenaryCommand
	ascension     ascensionStep
	ancient       ancientCommand
	export        *exportCommand
	clicker       autoClickerCommand
	queuedAt      time.Time
}
type actionResult struct {
	action  gameAction
	acted   bool
	elapsed time.Duration
	err     error
}

type pipelineReaders struct {
	context          func(image.Image) (gameContext, error)
	fish             func(image.Image) (image.Point, bool, error)
	skills           func(context.Context, image.Image) ([9]skillState, error)
	progression      func(context.Context, image.Image, [9]skillState, bool) (progressionState, error)
	mercenaries      func(context.Context, gameFrame) (mercenaryObservation, error)
	ascension        func(context.Context, gameFrame) (ascensionObservation, error)
	ascensionEconomy func(context.Context, gameFrame) (ascensionObservation, error)
	ancients         func(context.Context, gameFrame) (ancientObservation, error)
	ancientNames     func(context.Context, gameFrame) (ancientObservation, error)
	outsiders        func(context.Context, gameFrame) (outsiderObservation, error)
	heroes           heroReaders
	autoClickers     func(context.Context, gameFrame) (autoClickerPool, error)
	window           func() string
}
type pipelineOptions struct {
	windowed, autoClickers                                              bool
	heroes, skills, progression, mercenaries, monster, gilds, ascension bool
	ascensionStall                                                      time.Duration
	ascensionMinGain, ascensionCapital                                  float64
	ancientPlan                                                         *ancientPlan
	outsiderBase                                                        *ancientcalc.TranscensionPreview
	export                                                              *saveExportOptions
	monsterPoint                                                        image.Point
	fishInterval, clickInterval, gildInterval                           time.Duration
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
	clickers                                            autoClickerPlanner
	clickerJobFrame                                     uint64
	nextClickerRead                                     time.Time
	input                                               heroInput
	controls                                            *pauseControl
	frame                                               gameFrame
	state                                               [analysisCount]observation
	fishTarget                                          *image.Point
	fishAfter                                           uint64
	barriers                                            [analysisCount]uint64
	queue                                               map[actionKind]gameAction
	hero                                                heroRunner
	startupCheck                                        bool
	startup                                             startupPhase
	startupPassive                                      bool
	startupDeadline                                     time.Time
	nextUpgrades                                        time.Time
	skill                                               skillPlanner
	progression                                         progressionPlanner
	ascension                                           ascensionPlanner
	ancient                                             ancientPlanner
	export                                              saveExporter
	relicMessage                                        string
	outsiderMessage                                     string
	outsiderBase                                        *ancientcalc.TranscensionPreview
	outsiderJobFrame                                    uint64
	nextOutsider                                        time.Time
	mercenary                                           mercenaryPlanner
	fish                                                fishClickTracker
	gild                                                gildCollector
	metrics                                             pipelineMetrics
	generation, layout                                  uint64
	nextCapture, nextFish, nextProgression, nextMonster time.Time
	busy                                                bool
	settleUntil                                         time.Time
	diagnostics                                         chan heroDiagnostic
	progressionJobs                                     chan analysisJob
	focusFallback                                       bool
	heroJobFrame                                        uint64
	mercenaryJobFrame                                   uint64
	nextMercenary                                       time.Time
	monsterSize                                         image.Point
}

func newGamePipeline(controls *pauseControl, input heroInput, readers pipelineReaders, options pipelineOptions) *gamePipeline {
	p := &gamePipeline{controls: controls, input: input, readers: readers, options: options, hero: heroRunner{enabled: options.heroes}, queue: make(map[actionKind]gameAction), diagnostics: make(chan heroDiagnostic, 1)}
	p.clickers.footerAttempted = !options.heroes
	p.ancient.plan = options.ancientPlan
	p.outsiderBase = options.outsiderBase
	if p.outsiderBase == nil && options.ancientPlan != nil {
		p.outsiderBase = options.ancientPlan.Transcension
	}
	p.export.requested = options.export != nil
	p.startupCheck = options.heroes
	p.hero.onDiagnostic = func(a heroDiagnostic) bool {
		select {
		case p.diagnostics <- a:
			return true
		default:
			fmt.Println("hero diagnostic worker busy; skipped screenshots")
			return false
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
	if saveMenu(screen) {
		c.known, c.saveMenu = true, true
		return c, nil
	}
	if ancientQuantityDialog(screen) {
		c.known, c.ancientDialog = true, true
		return c, nil
	}
	ascension, err := ascensionDialog(screen)
	if err != nil || ascension {
		c.known, c.ascension = ascension, ascension
		return c, err
	}
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
	// Immediately after Ascension the progression control can still be locked.
	// The selected Heroes tab and quantity bar remain valid HUD anchors.
	c.known = known || heroQuantityBarPresent(screen)
	c.heroes = c.known && heroTabSelected(screen)
	c.mercenaries = known && mercenaryTabSelected(screen)
	c.ancients = known && ancientTabSelected(screen)
	c.outsiders = c.known && outsiderTabSelected(screen)
	return c, err
}

func (p *gamePipeline) analyze(ctx context.Context, kind analysisKind, job analysisJob) observation {
	out := observation{kind: kind, startup: job.startup, upgrades: job.upgrades, frame: job.frame}
	start := time.Now()
	switch kind {
	case autoClickerAnalysis:
		out.clickerPool, out.err = p.readers.autoClickers(ctx, job.frame)
	case fishAnalysis:
		out.point, out.found, out.err = p.readers.fish(job.frame.image)
	case skillAnalysis:
		out.skills, out.err = p.readers.skills(ctx, job.frame.image)
	case progressionAnalysis:
		out.progression, out.err = p.readers.progression(ctx, job.frame.image, job.skills, job.modeOnly)
	case heroAnalysis:
		if job.startup == startupPrepare || job.startup == startupUpgrades || job.upgrades {
			out.hero = heroObservation{frame: job.frame, startup: true, x1: heroQuantitySelected(job.frame.image, 122)}
			var height int
			out.hero.thumb, height, out.hero.thumbFound = heroScrollbarThumb(job.frame.image)
			b := job.frame.image.Bounds()
			out.hero.bottom = out.hero.thumbFound && absDiff(out.hero.thumb.Y+height/2, b.Min.Y+b.Dy()*965/1000) <= max(3, b.Dy()/100)
			if !out.hero.thumbFound || out.hero.bottom {
				out.point, out.upgradesKnown, out.found, out.err = readHeroUpgradeFooter(ctx, job.frame.image)
			}
		} else if job.startup == startupHeroes {
			out.hero, out.err = readStartupHeroObservation(ctx, job.frame, p.readers.heroes, job.heroBefore, job.sweep)
		} else {
			out.hero, out.err = readHeroObservation(ctx, job.frame, p.readers.heroes, job.heroBefore)
		}
	case mercenaryAnalysis:
		out.mercenary, out.err = p.readers.mercenaries(ctx, job.frame)
	case ascensionAnalysis:
		if job.economy {
			out.ascension, out.err = p.readers.ascensionEconomy(ctx, job.frame)
		} else {
			out.ascension, out.err = p.readers.ascension(ctx, job.frame)
		}
	case ancientAnalysis:
		if job.modeOnly {
			out.ancient = ancientObservation{frame: job.frame}
		} else if job.ancientNames && p.readers.ancientNames != nil {
			out.ancient, out.err = p.readers.ancientNames(ctx, job.frame)
		} else {
			out.ancient, out.err = p.readers.ancients(ctx, job.frame)
		}
	case exportAnalysis:
		out.export, out.err = readExport(*job.export)
	case outsiderAnalysis:
		out.outsider, out.err = p.readers.outsiders(ctx, job.frame)
		if out.err == nil {
			out.outsiderPlan = reconcileOutsiders(ctx, out.outsider, job.outsiderBase)
		}
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
	// F8 revokes the previous reset assessment. A fresh boss attempt must be
	// possible even when the old farming planner had already exhausted retries.
	if generation != p.generation && p.options.ascension {
		p.progression = progressionPlanner{}
	}
	p.ancient.interrupt()
	p.export.interrupt()
	p.ascension.interrupt()
	p.generation = generation
	p.layout++
	p.queue = make(map[actionKind]gameAction)
	p.state = [analysisCount]observation{}
	p.barriers = [analysisCount]uint64{fishAnalysis: p.barriers[fishAnalysis]}
	p.hero.interrupt()
	p.startupDeadline = time.Time{}
	if p.startup != noStartup {
		p.hero.enabled, p.hero.failures = p.options.heroes, 0
	}
	p.gild.interrupt()
	p.heroJobFrame = 0
	p.clickerJobFrame = 0
	p.nextClickerRead = time.Time{}
	p.clickers.interrupt()
	p.outsiderJobFrame = 0
	p.nextOutsider = time.Time{}
	p.outsiderMessage = ""
	p.skill.interrupt()
	p.progression.invalidateCombat()
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
	defer func() { cancel(); p.export.interrupt(); close(p.diagnostics); wg.Wait() }()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for failure := range p.diagnostics {
			if failure.readError != nil {
				saveHeroReadFailure(failure.after.frame, failure.readError)
			} else {
				saveHeroFailure(failure.before, failure.after)
			}
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
				if done.action.kind == handleExport {
					p.exportFailed(done.err)
					continue
				}
				if p.options.windowed && errors.Is(done.err, errInputContext) {
					if done.action.kind == handleAncient {
						p.ancient.fail("native window context changed during input")
						p.ancient.interrupt()
						p.controls.block(p.ancient.pauseReason())
					} else {
						p.controls.pause("native window context changed; focus the game and press F8")
					}
					continue
				}
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
				if p.ancient.blocked {
					p.controls.block(p.ancient.pauseReason())
				}
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
				if action.kind == placeOwnedClicker {
					p.clickers.sent(action.clicker, now)
				}
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
	var geometry viewportGeometry
	if shot, ok := image.(*viewportImage); ok {
		image, geometry = shot.Image, shot.geometry
		if shot.reason != "" && !(p.export.active && (p.export.step == exportRestoreGame || p.export.step == exportCloseMenu)) {
			p.controls.pause(shot.reason + "; focus an unobscured game HUD and press F8")
			return nil
		}
	}
	c, err := p.readers.context(image)
	if err != nil {
		return err
	}
	if p.readers.window != nil {
		c.window = p.readers.window()
		if c.window == "!outside-game" {
			c.outsiders = false
			c.ancients, c.ancientDialog, c.saveMenu = false, false, false
			c.known, c.heroes, c.mercenaries, c.questDialog, c.ascension, c.modal = false, false, false, false, false, noGildModal
		}
		if c.window == "" && !p.focusFallback {
			fmt.Println("foreground window identity unavailable; using visual context and F8")
			p.focusFallback = true
		}
	}
	c.geometry = geometry
	if p.options.windowed && c.window != geometry.Scene.Window {
		p.controls.pause("native window changed after capture; focus the game and press F8")
		return nil
	}
	if p.options.windowed && p.options.monster && c.known {
		if p.monsterSize.X == 0 {
			p.monsterSize = c.bounds.Size()
		}
		if c.bounds.Size() != p.monsterSize || !p.options.monsterPoint.In(c.bounds) {
			p.controls.pause("windowed monster coordinates need a new viewport screenshot and restart")
			return nil
		}
	}
	_, err = p.controls.runClick(ctx, p.generation, func() error {
		old := p.frame.context
		geometryChanged := c.bounds != old.bounds || c.window != old.window || c.geometry != old.geometry
		if geometryChanged && p.ancient.active {
			p.ancient.fail("game window or display changed")
			p.ancient.interrupt()
			p.controls.pauseLocked(p.ancient.pauseReason(), true)
		}
		if c.outsiders != old.outsiders || c.saveMenu != old.saveMenu || c.ancients != old.ancients || c.ancientDialog != old.ancientDialog || c.ascension != old.ascension || c.known != old.known || c.modal != old.modal || c.heroes != old.heroes || c.mercenaries != old.mercenaries || c.questDialog != old.questDialog || geometryChanged {
			if geometryChanged || !p.mercenary.expects(c, now) {
				p.mercenary.interrupt()
			}
			// Save owns the temporary Explorer focus handoff. Keep the historical
			// boss wall, but discard current reward/combat observations as usual.
			exportFocus := p.export.active && c.bounds == old.bounds && c.geometry == old.geometry &&
				((p.export.step == exportRestoreGame && old.window == p.export.window && c.window == "!outside-game") ||
					(p.export.step == exportCloseMenu && old.window == "!outside-game" && c.window == p.export.window))
			if geometryChanged {
				p.skill.reset()
			}
			if geometryChanged && !exportFocus {
				p.fishTarget = nil
				p.ascension.interrupt()
			} else if !p.ascension.active {
				p.ascension.invalidate()
			}
			p.ascension.jobFrame = 0
			p.ancient.jobFrame = 0
			p.outsiderJobFrame = 0
			p.nextOutsider = time.Time{}
			p.outsiderMessage = ""
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
			p.progression.invalidateCombat()
			p.progression.pending = nil
			p.progression.wantAction = false
			p.nextFish, p.nextProgression = time.Time{}, time.Time{}
		}
		p.frame = gameFrame{p.frame.id + 1, p.generation, p.layout, now, image, c}
		p.mercenary.captured(p.frame, now)
		// Slow capture must not consume its own interval and immediately repeat.
		p.nextCapture = now.Add(time.Since(start) + min(250*time.Millisecond, p.options.fishInterval))
		// Fish can cover controls on any tab; modal windows cover the fish.
		if p.fishTarget == nil && p.fishContext(c) && !now.Before(p.nextFish) {
			replaceJob(jobs[fishAnalysis], analysisJob{frame: p.frame})
			p.nextFish = now.Add(p.options.fishInterval)
		}
		if p.options.autoClickers && p.readers.autoClickers != nil && bootstrapHeroes(c) && p.clickerJobFrame == 0 && !now.Before(p.nextClickerRead) {
			p.clickerJobFrame = p.frame.id
			replaceJob(jobs[autoClickerAnalysis], analysisJob{frame: p.frame})
			p.nextClickerRead = now.Add(5 * time.Second)
			if p.clickers.pending != nil && !p.clickers.blocked {
				p.nextClickerRead = now.Add(300 * time.Millisecond)
			}
		}
		if p.startupCheck {
			if bootstrapHeroes(c) {
				p.beginStartup()
			} else {
				return nil
			}
		}
		if p.startup != noStartup {
			if bootstrapHeroes(c) {
				if p.startup != startupProgression && p.hero.due(now) && p.heroJobFrame == 0 && (p.hero.latest.frame.id == 0 || p.hero.pending != nil) {
					p.heroJobFrame = p.frame.id
					replaceJob(jobs[heroAnalysis], analysisJob{frame: p.frame, startup: p.startup, heroBefore: p.hero.before(), sweep: p.hero.sweep})
				}
				if p.options.progression && p.startupPassive && !now.Before(p.nextProgression) {
					replaceJob(jobs[progressionAnalysis], analysisJob{frame: p.frame, modeOnly: true})
					p.nextProgression = now.Add(300 * time.Millisecond)
				}
			}
			return nil
		}
		if p.options.export != nil && p.export.requested {
			if p.export.active && p.export.step == exportReadFile && p.export.jobFrame == 0 && c.window == p.export.window && c.known && !c.saveMenu {
				jobCtx, cancel := context.WithDeadline(ctx, p.export.deadline)
				p.export.cancel, p.export.jobFrame = cancel, p.frame.id
				replaceJob(jobs[exportAnalysis], analysisJob{frame: p.frame, export: &exportJob{ctx: jobCtx, options: *p.options.export, before: p.export.before, relicsOnly: p.export.relicsOnly}})
			}
			return nil
		}
		if c.saveMenu {
			return nil
		}
		if p.ascension.active && (p.ascension.step == openAscension || p.ascension.step == waitAscensionReset || p.ascension.latest.frame.id == 0) && c.window != "!outside-game" && p.ascension.jobFrame == 0 && p.frame.id > p.ascension.lastInputFrame && !now.Before(p.ascension.nextRead) {
			p.ascension.jobFrame = p.frame.id
			replaceJob(jobs[ascensionAnalysis], analysisJob{frame: p.frame})
			p.ascension.nextRead = now.Add(time.Second)
		}
		if p.ancient.active && p.ancient.jobFrame == 0 && !now.Before(p.ancient.nextRead) && c.window != "!outside-game" {
			p.ancient.jobFrame = p.frame.id
			replaceJob(jobs[ancientAnalysis], analysisJob{frame: p.frame, modeOnly: p.ancient.pending != nil && p.ancient.pending.ancient.step == confirmAncientQuantity, ancientNames: p.ancient.budgetChecked && p.ancient.selected < 0 && !p.ancient.needFullRead})
			p.ancient.nextRead = now.Add(300 * time.Millisecond)
		}
		if c.ancientDialog || p.ancient.active || (p.ancient.plan != nil && !p.ancient.finished) {
			return nil
		}
		if c.ascension || p.ascension.active || !c.known || c.modal != noGildModal || p.gild.active {
			return nil
		}
		if c.outsiders && p.readers.outsiders != nil && p.outsiderJobFrame == 0 && !now.Before(p.nextOutsider) {
			p.outsiderJobFrame = p.frame.id
			replaceJob(jobs[outsiderAnalysis], analysisJob{frame: p.frame, outsiderBase: p.outsiderBase})
			p.nextOutsider = now.Add(5 * time.Second)
		}
		if p.options.ascension && c.heroes && p.ascension.due(now, p.options.ascensionStall) && p.ascension.jobFrame == 0 && !now.Before(p.ascension.nextRead) {
			p.ascension.jobFrame = p.frame.id
			replaceJob(jobs[ascensionAnalysis], analysisJob{frame: p.frame, economy: true})
			p.ascension.nextRead = now.Add(5 * time.Second)
		}
		if p.options.mercenaries && (c.heroes || c.mercenaries) && p.mercenary.needsRead(c) && p.mercenaryJobFrame == 0 && !now.Before(p.mercenary.nextScan) && !now.Before(p.nextMercenary) {
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
		if p.options.progression && p.progression.pending != nil && !now.Before(p.nextProgression) {
			replaceJob(jobs[progressionAnalysis], analysisJob{frame: p.frame, modeOnly: true})
			p.nextProgression = now.Add(150 * time.Millisecond)
		}
		if p.options.skills || p.options.progression {
			replaceJob(jobs[skillAnalysis], analysisJob{frame: p.frame})
		}
		if p.options.heroes && c.heroes && p.hero.due(now) && p.heroJobFrame == 0 && (p.hero.latest.frame.id == 0 || p.hero.pending != nil) {
			p.heroJobFrame = p.frame.id
			upgrades := p.hero.pending == nil && !p.clickers.upgrades && !now.Before(p.nextUpgrades)
			if upgrades {
				p.nextUpgrades = now.Add(30 * time.Second)
			}
			replaceJob(jobs[heroAnalysis], analysisJob{frame: p.frame, heroBefore: p.hero.before(), upgrades: upgrades})
		}
		// Progression receives the skill observation for this exact frame, in accept().
		p.progressionJobs = jobs[progressionAnalysis]
		return nil
	})
	return err
}

func (p *gamePipeline) accept(ctx context.Context, out observation, now time.Time) error {
	if out.kind == outsiderAnalysis && out.frame.id == p.outsiderJobFrame {
		p.outsiderJobFrame = 0
	}
	if out.kind == ancientAnalysis && out.frame.id == p.ancient.jobFrame {
		p.ancient.jobFrame = 0
	}
	if out.kind == ascensionAnalysis && out.frame.id == p.ascension.jobFrame {
		p.ascension.jobFrame = 0
	}
	if out.kind == mercenaryAnalysis && out.frame.id == p.mercenaryJobFrame {
		p.mercenaryJobFrame = 0
	}
	if out.kind == autoClickerAnalysis && out.frame.id == p.clickerJobFrame {
		p.clickerJobFrame = 0
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
	if out.kind == exportAnalysis {
		if !p.export.active || p.export.step != exportReadFile || out.frame.id != p.export.jobFrame {
			p.metrics.dropped++
			return nil
		}
		p.export.jobFrame = 0
		if p.export.relicsOnly && out.err == nil && (out.export.relics == nil || out.export.relicErr != nil || out.export.plan != nil) {
			out.err = errors.New("read-only export returned no valid relic preview")
		}
		if out.err != nil || out.frame.layout != p.layout || out.frame.context != p.frame.context || p.frame.context.window != p.export.window || !p.frame.context.known || p.frame.context.saveMenu {
			reason := out.err
			if reason == nil {
				reason = errors.New("game context changed while reading export")
			}
			// applyObservation owns the pause mutex.
			if p.export.relicsOnly {
				p.reportRelics(nil, reason)
			}
			p.export.interrupt()
			p.controls.pauseLocked(fmt.Sprintf("save export failed: %v; focus the game and press F8 to retry", reason), false)
			return nil
		}
		p.reportRelics(out.export.relics, out.export.relicErr)
		// A newer export with missing prestige metadata revokes the older roster
		// for advice only; ordinary Ancient/relic stages remain independent.
		p.outsiderBase = out.export.prestige
		if p.outsiderBase == nil && out.export.plan != nil {
			p.outsiderBase = out.export.plan.Transcension
		}
		if p.export.relicsOnly {
			p.export.interrupt()
			p.export.requested = false
			p.queue = make(map[actionKind]gameAction)
			p.state = [analysisCount]observation{}
			p.ascension.invalidate()
			// This records an advisory check, never live relic readiness.
			p.ascension.relicsChecked = true
			p.ascension.nextRead, p.nextFish, p.nextProgression = time.Time{}, time.Time{}, time.Time{}
			return nil
		}
		if out.export.plan == nil {
			return errors.New("save export returned no Ancient plan")
		}
		if err := writeAncientPlan(p.options.export.planOutput, *out.export.plan); err != nil {
			p.export.interrupt()
			p.controls.pauseLocked("save export plan output failed: "+err.Error(), false)
			return nil
		}
		capital, err := ascensionSoulCapital(out.export.plan)
		if err != nil {
			return err
		}
		p.options.ascensionCapital = capital
		p.ancient = ancientPlanner{plan: out.export.plan}
		p.export.interrupt()
		p.export.requested = false
		p.queue = make(map[actionKind]gameAction)
		p.state = [analysisCount]observation{}
		fmt.Println("save export: fresh plan ready")
		return nil
	}
	if out.kind == fishAnalysis && !p.fishContext(out.frame.context) {
		return nil
	}
	if out.kind == autoClickerAnalysis {
		if !p.options.autoClickers || out.frame.generation != p.generation || out.frame.layout != p.layout || !bootstrapHeroes(p.frame.context) || out.frame.id <= p.state[autoClickerAnalysis].frame.id {
			return nil
		}
		if out.err != nil {
			fmt.Printf("Auto Clicker pool unreadable: %v; retrying later\n", out.err)
			return nil
		}
		wasBlocked, wasPending := p.clickers.blocked, p.clickers.pending != nil
		p.clickers.observe(out.frame, out.clickerPool, now)
		if !wasBlocked && p.clickers.blocked {
			fmt.Println("Auto Clicker placement not confirmed; leaving assignments unchanged and continuing automation")
		}
		if wasPending && p.clickers.pending == nil {
			p.nextClickerRead = time.Time{}
		}
		p.state[autoClickerAnalysis] = out
		return nil
	}
	if out.kind != fishAnalysis && (p.export.requested && p.startup == noStartup && !p.startupCheck || p.frame.context.saveMenu) {
		p.metrics.dropped++
		return nil
	}
	if (out.kind != fishAnalysis && (out.frame.layout != p.layout || p.frame.context.modal != noGildModal || p.gild.active)) || (out.kind == fishAnalysis && (out.frame.context.bounds != p.frame.context.bounds || out.frame.context.geometry != p.frame.context.geometry || (out.frame.context.window != p.frame.context.window && !(p.export.active && out.frame.context.window == p.export.window && p.frame.context.window == "!outside-game")))) || out.frame.id < p.barriers[out.kind] || out.frame.id <= p.state[out.kind].frame.id {
		p.metrics.dropped++
		return nil
	}
	if (p.frame.context.ancientDialog || p.ancient.active) && out.kind != ancientAnalysis && out.kind != fishAnalysis {
		p.metrics.dropped++
		return nil
	}
	if out.kind == ancientAnalysis {
		p.ancient.observe(out.ancient, out.err, now)
		if p.ancient.finished {
			fmt.Println("Ancient batch finished on Heroes; continuing automation")
		} else if p.ancient.blocked {
			p.controls.pauseLocked(p.ancient.pauseReason(), true)
		}
		return nil
	}
	if (p.frame.context.ascension || p.ascension.active) && out.kind != ascensionAnalysis && out.kind != fishAnalysis {
		p.metrics.dropped++
		return nil
	}
	if out.kind == ascensionAnalysis {
		if p.ascension.observe(out.ascension, out.err, now) {
			if out.frame.image != nil {
				path := fmt.Sprintf("artifacts/ascension-start-%s.png", now.Format("20060102-150405.000"))
				if err := saveImage(path, out.frame.image); err != nil {
					fmt.Printf("failed to save Ascension startup screenshot: %v\n", err)
				} else {
					fmt.Println("saved Ascension startup screenshot:", path)
				}
			}
			p.progression = progressionPlanner{}
			p.skill.reset()
			p.clickers = autoClickerPlanner{footerAttempted: !p.options.heroes}
			p.nextClickerRead = time.Time{}
			p.hero.failures, p.hero.enabled = 0, p.options.heroes
			// accept() already owns the pause-control mutex.
			if p.options.export != nil {
				p.ancient = ancientPlanner{}
				p.export.relicsOnly = false
				p.queue = make(map[actionKind]gameAction)
				if p.options.heroes && p.options.progression {
					p.beginStartup()
				} else {
					p.export.requested = true
					fmt.Println("Ascension confirmed at zone 1; requesting fresh save export")
				}
			} else {
				p.controls.pauseLocked("Ascension confirmed at zone 1; export a fresh save for Hero Souls spending and restart setup; press F8 when ready", false)
			}
		}
		return nil
	}
	if out.kind == outsiderAnalysis {
		if !out.frame.context.outsiders || !p.frame.context.outsiders {
			p.metrics.dropped++
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}
		sourceHash := ""
		if p.outsiderBase != nil {
			sourceHash = p.outsiderBase.SaveHash
		}
		if out.err == nil && out.outsiderPlan.SaveHash != sourceHash {
			p.metrics.dropped++
			p.nextOutsider = time.Time{}
			return nil
		}
		message := out.outsider.String()
		if out.err != nil {
			out.outsider = outsiderObservation{frame: out.frame}
			out.outsiderPlan = outsiderAdvice{}
			message = fmt.Sprintf("Outsiders: unreadable: %v", out.err)
		} else {
			message += "\n" + out.outsiderPlan.String()
		}
		p.state[out.kind] = out
		if message != p.outsiderMessage {
			fmt.Println(message)
			p.outsiderMessage = message
		}
		return nil
	}
	if out.kind == heroAnalysis && out.startup != p.startup {
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
			p.state[heroAnalysis] = observation{}
			if p.hero.pending == nil {
				hero := out.hero
				hero.frame = out.frame
				p.hero.readFailed(out.err, hero, now)
				return nil
			}
		case progressionAnalysis:
			p.ascension.invalidate()
			p.progression.invalidateCombat()
			p.nextProgression = now.Add(30 * time.Second)
			fmt.Printf("progression numbers unreadable: %v; retrying in 30s\n", out.err)
			return nil
		default:
			return out.err
		}
	}
	if out.kind == progressionAnalysis {
		out.progression.observedAt = out.frame.at
		if out.progression.Known && out.progression.Zone == 1 && p.progression.lastZone > 2 {
			p.skill.reset()
		}
	}
	if p.startup != noStartup {
		if out.kind == progressionAnalysis {
			// Startup only confirms the mode; ordinary combat observations start afresh after the handoff.
			out.progression.Zone = 1
		}
	}
	p.state[out.kind] = out
	switch out.kind {
	case fishAnalysis:
		if out.found && p.fishTarget == nil {
			point := out.point
			p.fishTarget = &point
		}
		if !out.found {
			p.fish.shouldClick(out.point, false)
		}
	case skillAnalysis:
		pending := p.skill.pending
		p.skill.observeFrame(out.skills, out.frame.id, out.frame.generation, now)
		if pending != nil && p.skill.pending == nil && p.skill.retryAt[pending.key-1].After(now) && out.frame.image != nil {
			path := fmt.Sprintf("artifacts/skill-%d-unconfirmed.png", pending.key)
			if err := saveImage(path, out.frame.image); err != nil {
				fmt.Printf("save skill failure screenshot: %v\n", err)
			} else {
				fmt.Printf("saved skill failure screenshot: %s\n", path)
			}
		}
		if p.options.progression && p.progression.pending == nil && !now.Before(p.nextProgression) && p.progressionJobs != nil {
			replaceJob(p.progressionJobs, analysisJob{frame: out.frame, skills: out.skills, modeOnly: p.progression.pending != nil})
			delay := 2 * time.Second
			if p.progression.pending != nil {
				delay = 150 * time.Millisecond
			}
			p.nextProgression = now.Add(delay)
		}
	case heroAnalysis:
		if out.upgrades {
			if out.found {
				p.enqueue(gameAction{kind: buyHeroUpgrades, frame: out.frame, point: out.point}, now)
			}
			return nil
		}
		p.hero.observe(out.hero, p.state[fishAnalysis], now)
		if p.startup == startupHeroes && out.err == nil {
			p.startupPassive = p.startupPassive || out.hero.passiveReady
			if out.hero.startupComplete {
				p.startup = startupUpgrades
				p.hero.interrupt()
				p.startupDeadline = time.Time{}
				p.state[heroAnalysis] = observation{}
				p.barriers[heroAnalysis] = p.frame.id + 1
				p.heroJobFrame = 0
				p.queue = make(map[actionKind]gameAction)
				fmt.Println("startup: skill setup sweep complete; seeking Buy Available Upgrades")
			}
		}
	case progressionAnalysis:
		p.progression.observeFrame(out.progression, out.frame.id, now)
		if p.options.ascension && p.startup == noStartup && out.frame.context.heroes {
			p.ascension.observeProgress(out.progression, p.progression.wallZone, now, p.progression.wallFullCombat)
		}
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
func (p *gamePipeline) fishContext(c gameContext) bool {
	return c.known && c.window != "!outside-game" && !c.ancientDialog && !c.ascension && !c.saveMenu && !c.questDialog && c.modal == noGildModal
}

func (p *gamePipeline) plan(now time.Time) {
	if now.Before(p.settleUntil) {
		return
	}
	p.planAutoClickers(now)
	if p.planStartup(now) || p.planExport(now) || p.planAncients(now) || p.planAscension(now) || p.planGilds(now) || !p.frame.context.known {
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
	if now.Before(p.settleUntil) {
		return gameAction{}, false
	}
	if p.fishTarget != nil && p.frame.id > p.fishAfter && p.fishContext(p.frame.context) && p.fish.shouldClick(*p.fishTarget, true) {
		p.enqueue(gameAction{kind: collectFish, frame: p.frame, point: *p.fishTarget}, now)
	} else {
		delete(p.queue, collectFish)
	}
	for kind := collectFish; kind <= clickMonster; kind++ {
		action, ok := p.queue[kind]
		if !ok {
			continue
		}
		if kind != collectFish && ((p.mercenary.pending != nil && p.frame.id <= p.mercenary.pending.action.frame.id) || (p.ascension.active && p.frame.id <= p.ascension.lastInputFrame) || p.ancient.pending != nil) {
			continue
		}
		if action.frame.layout != p.layout || action.frame.generation != p.generation || (!p.frame.context.known && !(kind == handleExport && action.export.step == exportRestoreGame)) || ((p.frame.context.modal != noGildModal || p.gild.active) && kind != collectGilds && kind != collectFish) {
			delete(p.queue, kind)
			continue
		}
		if p.startup != noStartup && kind != collectFish && kind != enableProgression && kind != buyHero && kind != buyHeroUpgrades && kind != scrollHeroes && kind != selectQuantity && kind != visitHeroes && kind != placeOwnedClicker && kind != clickMonster {
			delete(p.queue, kind)
			continue
		}
		if kind == clickMonster && p.startup != noStartup && !p.startupNeedsSeedClicks() {
			delete(p.queue, kind)
			continue
		}
		if kind == buyHeroUpgrades && ((p.startup != startupPrepare && p.startup != startupUpgrades && p.startup != noStartup) || !bootstrapHeroes(p.frame.context) || !heroUpgradeButtonStable(action.frame.image, p.frame.image, action.point)) {
			delete(p.queue, kind)
			p.hero.latest = heroObservation{}
			p.hero.nextScan = now
			continue
		}
		if kind == placeOwnedClicker && (p.clickers.pending != nil || p.clickers.blocked || !autoClickerCommandStable(action.clicker, p.frame)) {
			delete(p.queue, kind)
			continue
		}
		if kind == visitHeroes && (!p.startupCheck || !p.fishContext(p.frame.context)) {
			delete(p.queue, kind)
			continue
		}
		if (p.export.requested && p.startup == noStartup && !p.startupCheck || p.frame.context.saveMenu) && kind != handleExport && kind != collectFish {
			delete(p.queue, kind)
			continue
		}
		if kind == handleExport && (!p.export.active || action.export.step != p.export.step || (action.export.step != exportRestoreGame && action.frame.context != p.frame.context)) {
			delete(p.queue, kind)
			continue
		}
		if (p.ancient.active || p.frame.context.ancientDialog || (p.startup == noStartup && !p.startupCheck && p.ancient.plan != nil && !p.ancient.finished)) && kind != handleAncient && kind != collectFish {
			delete(p.queue, kind)
			continue
		}
		if kind == collectFish && !p.fishContext(p.frame.context) {
			delete(p.queue, kind)
			continue
		}
		if kind == handleAncient && (p.ancient.blocked || !ancientActionStable(action, p.frame)) {
			delete(p.queue, kind)
			p.ancient.latest = ancientObservation{}
			p.ancient.nextRead = now
			continue
		}
		if (p.ascension.active || p.frame.context.ascension) && kind != handleAscension && kind != collectFish {
			delete(p.queue, kind)
			continue
		}
		if p.frame.context.questDialog && kind != handleMercenary && kind != collectFish {
			delete(p.queue, kind)
			continue
		}
		if now.Sub(action.frame.at) > max(3*time.Second, p.state[fishAnalysis].elapsed+p.options.fishInterval*2) {
			delete(p.queue, kind)
			if kind == handleAncient {
				p.ancient.latest = ancientObservation{}
				p.ancient.nextRead = now
			}
			if kind == handleMercenary {
				p.mercenary.latest = mercenaryObservation{}
				p.nextMercenary = time.Time{}
			}
			if kind == buyHero || kind == buyHeroUpgrades || kind == scrollHeroes || kind == selectQuantity || kind == parkPointer {
				p.hero.latest = heroObservation{}
				p.hero.nextScan = now
			}
			continue
		}
		if kind == handleAscension && ((action.ascension == openAscension && !p.ascensionReady(now)) || !ascensionActionStable(action, p.frame)) {
			delete(p.queue, kind)
			p.ascension.latest = ascensionObservation{}
			p.ascension.nextRead = now
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
			if p.fishTarget == nil || *p.fishTarget != action.point {
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
		}
		if kind == buyHero || kind == scrollHeroes || kind == selectQuantity {
			if !p.frame.context.heroes {
				delete(p.queue, kind)
				continue
			}
			stable := true
			if kind == buyHero {
				if action.hero.startup {
					stable = p.startup == startupHeroes && startupHeroStable(action.hero, p.frame)
				} else {
					stable = p.startup == noStartup && heroListStable(action.frame.image, p.frame.image) && heroRowNameMatches(action.frame.image, p.frame.image, action.point, action.point)
				}
			}
			if kind == buyHero && (!stable || !heroQuantitySelected(p.frame.image, 122)) {
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
	input := p.input
	if input.bind != nil && !(a.kind == handleExport && a.export.step == exportRestoreGame) {
		input = input.bind(a.frame.context.geometry)
	}
	acted, err := p.controls.runClick(ctx, a.frame.generation, func() error {
		if !(a.kind == handleExport && a.export.step == exportRestoreGame) && p.readers.window != nil && p.readers.window() != a.frame.context.window {
			return errInputContext
		}
		if a.kind == collectFish && (!p.fishContext(p.frame.context) || a.frame.context != p.frame.context) {
			return errInputContext
		}
		switch a.kind {
		case handleExport:
			if a.export.step == exportRestoreGame {
				return input.focus(a.export.window)
			}
			if a.export.step == exportSave {
				before, err := snapshotExports(p.options.export.dir)
				if err != nil {
					return err
				}
				a.export.before = before
			}
			return input.click(a.point)
		case handleAncient:
			switch a.ancient.step {
			case scrollAncients:
				if a.ancient.fine {
					if err := input.click(a.point); err != nil {
						return err
					}
				} else if err := input.scroll(a.point, a.ancient.direction); err != nil {
					return err
				}
				return input.move(parkPoint(a.frame.context.bounds))
			case openAncientQuantity:
				return clickAncientCustom(ctx, input, a.point)
			case fillAncientQuantity:
				if err := input.click(a.point); err != nil {
					return err
				}
				return fillAncientCustom(ctx, input, a.ancient.quantity)
			default:
				if err := input.click(a.point); err != nil {
					return err
				}
				return input.move(parkPoint(a.frame.context.bounds))
			}
		case handleMercenary:
			if a.mercenary.step == scrollMercenariesTop || a.mercenary.step == scrollMercenariesBottom {
				return input.drag(a.point, a.target)
			}
			if err := input.click(a.point); err != nil {
				return err
			}
			if a.mercenary.step == claimAndOpenMercenaryQuest {
				// Collect becomes Start Quest in place; each click includes release settling.
				if err := input.click(a.point); err != nil {
					return err
				}
			}
			if a.mercenary.step == selectMercenaryQuest {
				return nil
			}
			return input.move(parkPoint(a.frame.context.bounds))
		case placeOwnedClicker:
			if err := placeAutoClicker(ctx, input, a.clicker); err != nil {
				return err
			}
			return input.move(parkPoint(a.frame.context.bounds))
		case clickMonster:
			return input.monsterClick(a.point)
		case collectFish, collectGilds, handleAscension, visitHeroes:
			return input.click(a.point)
		case castSkill:
			return holdGameKey(ctx, input, fmt.Sprint(a.key))
		case enableProgression:
			return holdGameKey(ctx, input, "a")
		case selectQuantity:
			return input.keyTap("t")
		case scrollHeroes:
			return input.drag(a.point, a.target)
		case parkPointer:
			return input.move(a.point)
		case buyHeroUpgrades:
			if err := clickHeroUpgrades(ctx, input, a.point); err != nil {
				return err
			}
			return input.move(parkPoint(a.frame.context.bounds))
		case buyHero:
			key := "q"
			if a.hero.startup {
				key = "ctrl"
			}
			if err := clickHeroModified(ctx, input, a.point, key); err != nil {
				return err
			}
			return input.move(parkPoint(a.frame.context.bounds))
		}
		return fmt.Errorf("unknown game action %d", a.kind)
	})
	if acted && err == nil && a.kind == handleMercenary && a.mercenary.step == selectMercenaryQuest {
		// Okay is fixed. Wait outside the input lock so F8 can cancel the second click.
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false, nil
		case <-timer.C:
		}
		acted, err = p.controls.runClick(ctx, a.frame.generation, func() error {
			if p.frame.context != a.frame.context {
				return errInputContext
			}
			if p.readers.window != nil && p.readers.window() != a.frame.context.window {
				return errInputContext
			}
			if err := input.click(mercenaryPoint(a.frame.context.bounds, 710, 500)); err != nil {
				return err
			}
			return input.move(parkPoint(a.frame.context.bounds))
		})
	}
	if errors.Is(err, errInputContext) {
		if p.options.windowed {
			return false, err
		}
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
	p.fishAfter = p.frame.id
	invalidate := func(kind analysisKind) { p.barriers[kind] = p.frame.id + 1; p.state[kind] = observation{} }
	switch a.kind {
	case handleExport:
		p.export.sent(a, now)
		p.queue = make(map[actionKind]gameAction)
		p.state = [analysisCount]observation{}
	case handleAncient:
		p.ancient.sent(a, now)
		p.barriers[ancientAnalysis] = p.frame.id + 1
		p.queue = make(map[actionKind]gameAction)
		p.state = [analysisCount]observation{fishAnalysis: p.state[fishAnalysis]}
		p.hero.interrupt()
		p.skill.interrupt()
		p.mercenary.interrupt()
		p.progression.pending = nil
		p.progression.wantAction = false
	case handleAscension:
		p.ascension.sent(a.ascension, a.frame.id, now)
		ascension := p.ascension
		p.reset(p.generation)
		p.ascension = ascension
		p.frame.context = gameContext{bounds: a.frame.context.bounds, window: a.frame.context.window, geometry: a.frame.context.geometry}
		fmt.Printf("Ascension %s at (%d, %d)\n", [...]string{"dialog opened", "confirmed", "cancelled"}[a.ascension], a.point.X, a.point.Y)
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
		gild, ascension := p.gild, p.ascension
		p.reset(p.generation)
		p.gild, p.ascension = gild, ascension
		p.ascension.invalidate()
		p.frame.context = gameContext{bounds: a.frame.context.bounds, window: a.frame.context.window, geometry: a.frame.context.geometry}
		fmt.Printf("clicked gild gift control at (%d, %d)\n", a.point.X, a.point.Y)

	case handleMercenary:
		p.mercenary.sent(a, now)
		p.nextMercenary = time.Time{}
		invalidate(mercenaryAnalysis)
		delete(p.queue, collectFish)
		delete(p.queue, buyHero)
		delete(p.queue, scrollHeroes)
	case collectFish:
		p.fishTarget = nil
		p.queue = make(map[actionKind]gameAction)
		invalidate(ancientAnalysis)
		invalidate(ascensionAnalysis)
		p.ancient.latest = ancientObservation{}
		p.ascension.latest = ascensionObservation{}
		p.ancient.nextRead, p.ascension.nextRead, p.nextFish = time.Time{}, time.Time{}, time.Time{}
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
	case placeOwnedClicker:
		p.nextClickerRead = time.Time{}
		invalidate(autoClickerAnalysis)
		fmt.Printf("assigned owned Auto Clicker at (%d, %d)\n", a.point.X, a.point.Y)
	case visitHeroes:
		p.queue = make(map[actionKind]gameAction)
		invalidate(heroAnalysis)
	case buyHeroUpgrades:
		if p.startup == noStartup {
			p.hero.interrupt()
			invalidate(heroAnalysis)
			fmt.Println("bought available hero upgrades")
			break
		}
		p.finishStartupUpgradePass()
		invalidate(heroAnalysis)
		invalidate(progressionAnalysis)
		p.nextProgression = time.Time{}

	case buyHero, scrollHeroes, selectQuantity, parkPointer:
		p.hero.sent(a, now)
		if a.kind == buyHero && a.hero.startup {
			fmt.Printf("startup: submitted Ctrl+100 at (%d, %d), attempt %d/2\n", a.point.X, a.point.Y, p.hero.sweep.attempts)
		}
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
	if a.kind == handleExport {
		p.settleUntil = now.Add(500 * time.Millisecond)
	}
	if a.kind == buyHero || a.kind == buyHeroUpgrades || a.kind == scrollHeroes || a.kind == parkPointer || a.kind == handleMercenary {
		p.settleUntil = now.Add(200 * time.Millisecond)
	}
	p.nextCapture = time.Time{}
}

func (m pipelineMetrics) String() string {
	parts := []string{fmt.Sprintf("captures=%d capture=%s actions=%d input=%s queue=%s stale=%d", m.captures, m.captureTime, m.actions, m.inputTime, m.queueTime, m.dropped)}
	for i, name := range []string{"fish", "skills", "progression", "heroes", "mercenaries", "ascension", "ancients", "export", "outsiders", "clickers"} {
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
		p.controls.pause("gild gift window did not advance; check it and press F8 to resume")
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

// Ascension is exclusive from opening the dialog through reset confirmation.
func (p *gamePipeline) planAscension(now time.Time) bool {
	if !p.ascension.active {
		if p.frame.context.ascension {
			return true // A manually opened dialog is never permission to confirm it.
		}
		if !p.ascensionReady(now) {
			// Let an in-flight reward read finish without another purchase/skill
			// changing the damage baseline. Fish collection can still run.
			if p.ascension.jobFrame != 0 && p.ascension.due(now, p.options.ascensionStall) && !p.progression.wantAction && p.progression.pending == nil && p.hero.pending == nil && p.skill.pending == nil && !p.mercenary.active && p.mercenary.pending == nil {
				for _, kind := range []actionKind{castSkill, buyHero, scrollHeroes, selectQuantity, handleMercenary, clickMonster} {
					delete(p.queue, kind)
				}
				return true
			}
			return false
		}
		if p.options.export != nil && !p.ascension.relicsChecked {
			p.export.requested, p.export.relicsOnly = true, true
			return p.planExport(now)
		}
		point, found, err := ascensionControl(p.frame.image, ascensionSpiral)
		if found && err == nil {
			p.queue = make(map[actionKind]gameAction)
			p.enqueue(gameAction{kind: handleAscension, frame: p.frame, point: point, ascension: openAscension}, now)
			return true
		}
		return false
	}
	if now.After(p.ascension.deadline) {
		p.controls.pause("Ascension did not advance; check the dialog or relic junk pile and press F8 to resume")
		return true
	}
	if now.Before(p.ascension.nextAction) || p.ascension.latest.frame.id == 0 || !p.frame.context.ascension {
		return true
	}
	control := ascensionYes
	if p.ascension.step == cancelAscension {
		control = ascensionNo
	} else if p.ascension.step != confirmAscension {
		return true
	}
	point, found, err := ascensionControl(p.frame.image, control)
	if found && err == nil {
		p.enqueue(gameAction{kind: handleAscension, frame: p.ascension.latest.frame, point: point, ascension: p.ascension.step}, now)
	}
	return true
}

func (p *gamePipeline) ascensionReady(now time.Time) bool {
	if !p.options.ascension || !p.frame.context.heroes || p.gild.active || p.mercenary.active || p.mercenary.pending != nil || p.hero.pending != nil || p.skill.pending != nil || p.progression.pending != nil || p.progression.wantAction || !p.ascension.due(now, p.options.ascensionStall) {
		return false
	}
	progress := p.state[progressionAnalysis]
	if progress.frame.id == 0 || progress.frame.id < p.barriers[progressionAnalysis] || now.Sub(progress.frame.at) > 10*time.Second {
		return false
	}
	out := p.ascension.latest
	if !out.economy || out.frame.id == 0 || out.frame.context != p.frame.context || now.Sub(out.frame.at) > 10*time.Second {
		return false
	}
	p.ascension.minimumReward = max(out.bank, p.options.ascensionCapital) + math.Log10(p.options.ascensionMinGain)
	if math.IsInf(out.souls, -1) || out.souls < p.ascension.minimumReward {
		fmt.Println("Ascension deferred: small Hero Souls gain; review Ancient allocation and Transcension/Ancient Souls before another reset")
		p.ascension.nextCheck = now.Add(time.Minute)
		return false
	}
	return ascensionBudgetStable(out.frame.image, p.frame.image)
}

func (p *gamePipeline) planAncients(now time.Time) bool {
	if p.frame.context.ancientDialog && !p.ancient.active {
		return true
	}
	if p.ancient.plan == nil || p.ancient.finished {
		return false
	}
	if p.ancient.blocked {
		p.controls.block(p.ancient.pauseReason())
		return true
	}
	if !p.ancient.deadline.IsZero() && now.After(p.ancient.deadline) {
		p.ancient.fail("confirmation timed out after 20s")
		p.controls.block(p.ancient.pauseReason())
		return true
	}
	if action, ok := p.ancient.action(p.frame, now); ok {
		p.enqueue(action, now)
	}
	if p.ancient.blocked {
		p.controls.block(p.ancient.pauseReason())
	}
	return true
}
