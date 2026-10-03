package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"

	"github.com/go-vgo/robotgo"
	hook "github.com/robotn/gohook"
)

func main() {
	robotgo.Scale = false
	mode := flag.String("mode", "help", "help, shot, click, run, ancients-plan, relics-plan, or transcension-plan")
	output := flag.String("out", "artifacts/screenshot.png", "screenshot file for shot or preview JSON for transcension-plan mode")
	save := flag.String("save", "", "exported save for preview modes, run Outsider roster, or Ascension invested-soul baseline (read only)")
	outsiderShot := flag.String("screenshot", "", "existing Outsiders screenshot to reconcile with -save in transcension-plan mode (read only)")
	ancientReserve := flag.String("ancient-reserve", "0", "Optional Hero Souls spending floor beyond the calculator soul bank")
	ancientSkillRate := flag.Float64("ancient-skill-rate", 1, "calculator allocation to skill Ancients, from 0 to 1")
	ancientBeyond8k := flag.Bool("ancient-beyond8k", false, "best hero is levelled beyond 8000; changes calculator gold allocation")
	ancientSave := flag.String("ancients-save", "", "exported save for one Ancient purchase batch before normal run actions")
	exportDir := flag.String("export-dir", "", "Save folder; suggest relics, buy Ancients at startup/after Ascension, and inspect relics before Ascension")
	ancientPlanOutput := flag.String("ancient-plan-out", "artifacts/ancients-plan.json", "Ancient purchase plan with gild and Transcension previews")
	x := flag.Int("x", 0, "screen X coordinate for click or optional monster clicks in run mode")
	y := flag.Int("y", 0, "screen Y coordinate for click or optional monster clicks in run mode")
	interval := flag.Duration("interval", 100*time.Millisecond, "time between optional monster clicks in run mode")
	fishInterval := flag.Duration("fish-interval", time.Second, "time between fish scans in run mode")
	skills := flag.Bool("skills", false, "activate unlocked skills with hotkeys 1-9 in run mode")
	progression := flag.Bool("progression", false, "manage progression mode and wait for damage improvements after failed bosses")
	heroLevels := flag.Bool("hero-levels", false, "hire and level heroes, buy their available upgrades in run mode")
	autoClickers := flag.Bool("auto-clickers", false, "place available owned Auto Clickers on the monster and hero upgrade footer without spending rubies")
	gilds := flag.Bool("gilds", false, "open earned gild gifts in batches in run mode")
	gildInterval := flag.Duration("gild-interval", 5*time.Minute, "time between earned gild gift checks")
	ascension := flag.Bool("ascension", false, "ascend after a full combat boss loss or fallback stall with meaningful Hero Souls gain")
	ascensionMinGain := flag.Float64("ascension-min-gain", 0.25, "minimum Ascension reward as a fraction of soul capital (0.25 means 25%)")
	ascensionStall := flag.Duration("ascension-stall", 3*time.Minute, "fallback stall before Ascension when a full combat failure was not observed")
	mercenaries := flag.Bool("mercenaries", false, "collect mercenary rewards and send new quests without spending rubies")
	duration := flag.Duration("duration", 0, "maximum run time (0 means unlimited)")
	stats := flag.Bool("stats", false, "print pipeline timing and analysis counters when run stops")
	windowed := flag.Bool("windowed", false, "detect a visible game viewport on Windows/macOS; -x/-y use viewport screenshot pixels")
	delay := flag.Duration("delay", 5*time.Second, "time to focus the game before shot or click (run waits for F8)")
	flag.DurationVar(&ocrTimeout, "ocr-timeout", ocrTimeout, "maximum time per Tesseract execution (excluding queue wait)")
	flag.Parse()

	if *mode == "help" {
		flag.Usage()
		return
	}
	if *outsiderShot != "" && *mode != "transcension-plan" {
		log.Fatal("-screenshot requires transcension-plan mode")
	}
	if *windowed {
		if *mode != "shot" && *mode != "click" && *mode != "run" {
			log.Fatal("-windowed requires shot, click or run mode")
		}
		if err := windowedPreflight(); err != nil {
			log.Fatal(err)
		}
	}

	var hasX, hasY bool
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x":
			hasX = true
		case "y":
			hasY = true
		}
	})
	if *mode == "click" && (!hasX || !hasY) {
		log.Fatal("click requires both -x and -y")
	}
	if *mode == "run" && hasX != hasY {
		log.Fatal("run requires both -x and -y when monster clicks are enabled")
	}
	if *mode == "shot" || *mode == "click" {
		if *delay < 0 {
			log.Fatal("-delay must be non-negative")
		}
		fmt.Printf("starting %s in %s; focus the game now\n", *mode, *delay)
		time.Sleep(*delay)
	}

	var err error
	switch *mode {
	case "ancients-plan":
		if *save == "" {
			err = errors.New("ancients-plan requires -save")
			break
		}
		var plan ancientPlan
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		plan, err = calculateAncients(ctx, *save, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
		if err == nil {
			err = writeAncientPlan(*ancientPlanOutput, plan)
		}
	case "transcension-plan":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		path := ""
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "out" {
				path = *output
			}
		})
		if *outsiderShot != "" {
			err = previewOutsiders(ctx, *save, *outsiderShot, path, os.Stdout)
		} else {
			err = previewTranscension(ctx, *save, path, os.Stdout)
		}
	case "relics-plan":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		err = previewRelics(ctx, *save, os.Stdout)
	case "shot":
		if *windowed {
			var capture windowCapture
			var img image.Image
			img, err = capture.capture()
			if err == nil {
				if shot := img.(*viewportImage); shot.reason != "" {
					err = errors.New(shot.reason)
				} else {
					err = saveImage(*output, shot.Image)
					if err == nil {
						fmt.Println("saved viewport pixels", *output)
					}
				}
			}
		} else {
			err = saveScreenshot(*output)
		}
	case "click":
		if *windowed {
			var capture windowCapture
			var img image.Image
			img, err = capture.capture()
			if err == nil {
				shot := img.(*viewportImage)
				if shot.reason != "" {
					err = errors.New(shot.reason)
				} else {
					err = bindViewportInput(context.Background(), heroInput{}, shot.geometry).click(image.Pt(*x, *y))
				}
			}
		} else {
			err = clickAt(context.Background(), *x, *y)
		}
	case "run":
		var export *saveExportOptions
		if *exportDir != "" {
			if *ancientSave != "" {
				err = errors.New("use either -export-dir or -ancients-save")
				break
			}
			if _, e := snapshotExports(*exportDir); e != nil {
				err = fmt.Errorf("export directory: %w", e)
				break
			}
			if _, e := ancientcalc.Value(strings.TrimSuffix(*ancientReserve, "%")); e != nil {
				err = fmt.Errorf("soul reserve: %w", e)
				break
			}
			if !(*ancientSkillRate >= 0 && *ancientSkillRate <= 1) {
				err = errors.New("-ancient-skill-rate must be between 0 and 1")
				break
			}
			export = &saveExportOptions{dir: *exportDir, reserve: *ancientReserve, skillRate: *ancientSkillRate, beyond8k: *ancientBeyond8k, planOutput: *ancientPlanOutput}
		}
		var plan *ancientPlan
		if *ancientSave != "" {
			value, e := calculateAncients(context.Background(), *ancientSave, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
			if e != nil {
				err = e
				break
			}
			plan = &value
			if err = writeAncientPlan(*ancientPlanOutput, value); err != nil {
				break
			}
		}
		capitalPlan := plan
		if *ascension && *save != "" {
			value, e := calculateAncients(context.Background(), *save, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
			if e != nil {
				err = e
				break
			}
			capitalPlan = &value
		}
		capital, e := ascensionSoulCapital(capitalPlan)
		if e != nil {
			err = e
			break
		}
		var outsiderBase *ancientcalc.TranscensionPreview
		if *save != "" {
			if *ascension {
				outsiderBase = capitalPlan.Transcension
				if capitalPlan.TranscensionError != "" {
					fmt.Println("Outsider save preview unavailable:", capitalPlan.TranscensionError)
				}
			} else {
				value, e := readTranscensionPreview(context.Background(), *save)
				if e != nil {
					fmt.Println("Outsider save preview unavailable:", e)
				} else {
					outsiderBase = &value
				}
			}
		}

		err = runBot(*x, *y, hasX, *interval, *fishInterval, *duration, *heroLevels, *skills, *progression, *mercenaries, *stats, *gilds, *gildInterval, *ascension, *ascensionStall, *ascensionMinGain, capital, plan, export, outsiderBase, *windowed, *autoClickers)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func saveScreenshot(path string) error {
	img, err := robotgo.CaptureImg()
	if err != nil {
		return fmt.Errorf("capture screen: %w", err)
	}
	if img == nil {
		return errors.New("capture screen returned no image")
	}
	if err := saveImage(path, img); err != nil {
		return err
	}
	fmt.Println("saved", path)
	return nil
}

func saveImage(path string, img image.Image) error {
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return fmt.Errorf("encode screenshot: %w", err)
	}
	return writeScreenshot(path, data.Bytes())
}

func writeScreenshot(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write screenshot: %w", err)
	}
	return nil
}

func mousePoint(point image.Point) (image.Point, error) {
	w, h := robotgo.GetScreenSize()
	pixels := image.Rect(0, 0, w, h)
	desktop := pixels
	if runtime.GOOS == "darwin" {
		r := robotgo.GetScreenRect(0)
		desktop = image.Rect(r.X, r.Y, r.X+r.W, r.Y+r.H)
	}
	return desktopPoint(point, pixels, desktop)
}

func moveAt(point image.Point) error {
	target, err := mousePoint(point)
	if err != nil {
		return err
	}
	robotgo.Move(target.X, target.Y)
	return nil
}

func moveForClick(ctx context.Context, point image.Point) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := moveAt(point); err != nil {
		return err
	}
	time.Sleep(50 * time.Millisecond)
	return ctx.Err()
}

func clickAt(ctx context.Context, x, y int) error {
	if err := moveForClick(ctx, image.Pt(x, y)); err != nil {
		return err
	}
	return robotgo.Click("left")
}

func clickGameAt(ctx context.Context, point image.Point) error {
	if err := moveForClick(ctx, point); err != nil {
		return err
	}
	return clickLeft(ctx, robotgo.Toggle)
}

// RobotGo Click holds for only 5 ms; keep the press and release visible to game frames.
func clickLeft(ctx context.Context, toggle func(...interface{}) error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	wait := func() error {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	defer func() {
		err = errors.Join(err, toggle("left", "up"))
		if err == nil {
			// Leave the pointer and Q unchanged while the game consumes mouse-up.
			err = wait()
		}
	}()
	if err = toggle("left", "down"); err != nil {
		return err
	}
	return wait()
}

type heroInput struct {
	bind         func(viewportGeometry) heroInput
	capture      func() (image.Image, error)
	monsterClick func(image.Point) error
	move         func(image.Point) error
	drag         func(image.Point, image.Point) error
	click        func(image.Point) error
	scroll       func(image.Point, int) error
	keyTap       func(string) error
	keyToggle    func(string, string) error
	typeText     func(string) error
	focus        func(string) error
}

type pauseControl struct {
	mu            sync.Mutex
	paused        bool
	resumeBlocked bool
	pauseReason   string
	generation    uint64
}

func (control *pauseControl) toggle() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused && control.resumeBlocked {
		return true
	}
	control.paused = !control.paused
	control.pauseReason = ""
	if control.paused {
		control.pauseReason = "F8"
	}
	control.generation++
	return control.paused
}

func (control *pauseControl) pause(reason string) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.pauseLocked(reason, false)
}
func (control *pauseControl) block(reason string) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.pauseLocked(reason, true)
}

// Call only while holding the input/pause mutex.
func (control *pauseControl) pauseLocked(reason string, blocked bool) {
	if control.paused && control.pauseReason == reason && control.resumeBlocked == blocked {
		return
	}
	control.paused = true
	control.pauseReason = reason
	control.resumeBlocked = blocked
	control.generation++
	fmt.Printf("paused: %s\n", reason)
}
func (control *pauseControl) message() string {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused {
		return "paused: " + control.pauseReason
	}
	return "resumed"
}

func (control *pauseControl) snapshot() uint64 {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.generation
}

func (control *pauseControl) valid(ctx context.Context, generation uint64) bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	return !control.paused && ctx.Err() == nil && control.generation == generation
}

func (control *pauseControl) isPaused() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.paused
}

func (control *pauseControl) runClick(ctx context.Context, generation uint64, action func() error) (bool, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused || ctx.Err() != nil || control.generation != generation {
		return false, nil
	}
	if err := action(); err != nil {
		if ctx.Err() != nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isPauseKey(event hook.Event) bool {
	return event.Kind == hook.KeyUp && event.Keycode == hook.Keycode["f8"]
}

type fishClickTracker struct {
	last        image.Point
	clicked     bool
	misses      int
	lastClickAt time.Time
}

func (tracker *fishClickTracker) shouldClick(point image.Point, found bool) bool {
	if !found {
		tracker.misses++
		if tracker.misses >= 3 {
			tracker.clicked = false
		}
		return false
	}
	tracker.misses = 0
	// A fish rotates and detection jitters; a nearby match is still the same fish.
	delta := point.Sub(tracker.last)
	// Retry only a fish still recognized on a fresh frame; the game may miss input.
	return !tracker.clicked || delta.X*delta.X+delta.Y*delta.Y > 150*150 || time.Since(tracker.lastClickAt) >= 5*time.Second
}

func (tracker *fishClickTracker) recordClick(point image.Point) {
	tracker.last = point
	tracker.lastClickAt = time.Now()
	tracker.clicked = true
	tracker.misses = 0
}

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration, heroLevels, skills, progression, mercenaries, stats, gilds bool, gildInterval time.Duration, ascension bool, ascensionStall time.Duration, ascensionMinGain, ascensionCapital float64, ancientPlan *ancientPlan, export *saveExportOptions, outsiderBase *ancientcalc.TranscensionPreview, windowed, autoClickers bool) error {
	if ascension && (!progression || ascensionStall <= 0 || ascensionMinGain <= 0 || math.IsNaN(ascensionMinGain) || math.IsInf(ascensionMinGain, 0)) {
		return errors.New("-ascension requires -progression, positive -ascension-stall and finite positive -ascension-min-gain")
	}
	if gilds && gildInterval <= 0 {
		return errors.New("-gild-interval must be positive")
	}
	if ocrTimeout <= 0 {
		return errors.New("-ocr-timeout must be positive")
	}
	if fishInterval <= 0 || duration < 0 || (monsterClicks && interval <= 0) {
		return errors.New("-fish-interval must be positive; -duration must be non-negative; -interval must be positive when monster clicks are enabled")
	}

	sift, err := newSIFTFishDetector()
	if err != nil {
		return fmt.Errorf("initialize OpenCV fish detector: %w", err)
	}
	defer sift.Close()

	interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx := interrupt
	cancel := stop
	if duration > 0 {
		ctx, cancel = context.WithTimeout(interrupt, duration)
	}
	if heroLevels || progression || mercenaries || ancientPlan != nil || export != nil {
		if err := checkHeroOCR(ctx); err != nil {
			cancel()
			return err
		}
	}
	input := heroInput{
		focus:        focusGameWindow,
		capture:      func() (image.Image, error) { return robotgo.CaptureImg() },
		monsterClick: func(p image.Point) error { return clickAt(ctx, p.X, p.Y) },
		move:         moveAt,
		click:        func(p image.Point) error { return clickGameAt(ctx, p) },
		scroll: func(p image.Point, direction int) error {
			if err := moveForClick(ctx, p); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			robotgo.Scroll(0, -direction, 150)
			return ctx.Err()
		},
		keyTap:    func(key string) error { return robotgo.KeyTap(key) },
		keyToggle: func(key, state string) error { return robotgo.KeyToggle(key, state) },
		typeText:  func(text string) error { robotgo.TypeStr(text); return nil },
		drag: func(from, to image.Point) error {
			if err := moveAt(from); err != nil {
				return err
			}
			target, err := mousePoint(to)
			if err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond)
			highDelay := 0.75
			if runtime.GOOS == "windows" {
				// RobotGo truncates Windows Sleep to integer milliseconds: keep
				// 1 ms on a quarter of steps instead of removing the delay entirely.
				highDelay = 1.25
			}
			robotgo.DragSmooth(target.X, target.Y, 0.25, highDelay)
			return nil
		},
	}
	controls := pauseControl{paused: true}
	windowReader := foregroundGameWindow
	if windowed {
		if err := windowedHookPreflight(); err != nil {
			cancel()
			return err
		}
		capture := &windowCapture{}
		input.capture = capture.capture
		input.focus = focusNativeWindow
		base := input
		input.bind = func(g viewportGeometry) heroInput { return bindViewportInput(ctx, base, g) }
		windowReader = func() string {
			scene, err := readNativeScene()
			if err != nil {
				return "!outside-game"
			}
			return scene.Window
		}
	}
	// GoHook's End crashes on macOS when Accessibility is denied; this CLI releases the hook on exit.
	events := hook.Start()
	hookDone := make(chan struct{})
	go func() {
		defer close(hookDone)
		for {
			select {
			case event, ok := <-events:
				if !ok {
					controls.pause("global keyboard hook stopped")
					cancel()
					return
				}
				if isPauseKey(event) {
					controls.toggle()
					fmt.Println(controls.message())
				}
			case <-ctx.Done():
				controls.pause("stopping")
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-hookDone
	}()

	fmt.Println("paused; press F8 to start or pause, Ctrl+C to stop")
	pipeline := newGamePipeline(&controls, input, pipelineReaders{
		context: recognizedGame, fish: sift.Find, skills: readSkillStates, progression: readProgressionState, mercenaries: readMercenaryObservation, ascension: readAscensionObservation, ascensionEconomy: readAscensionEconomy, ancients: readAncientObservation, ancientNames: readAncientNames, outsiders: readOutsiderObservation,
		heroes: heroReaders{readHeroGold, readHeroPrice, readHeroLevel}, window: windowReader, autoClickers: readAutoClickerPool,
	}, pipelineOptions{heroes: heroLevels, skills: skills, progression: progression, mercenaries: mercenaries, monster: monsterClicks, gilds: gilds, gildInterval: gildInterval, ascension: ascension, ascensionStall: ascensionStall, ascensionMinGain: ascensionMinGain, ascensionCapital: ascensionCapital, ancientPlan: ancientPlan, export: export, outsiderBase: outsiderBase,
		monsterPoint: image.Pt(x, y), fishInterval: fishInterval, clickInterval: interval, windowed: windowed, autoClickers: autoClickers})
	err = pipeline.run(ctx)
	fmt.Printf("stopped after %d actions\n", pipeline.metrics.actions)
	if stats {
		fmt.Println("pipeline:", pipeline.metrics.String())
	}
	return err
}

func foregroundGameWindow() string {
	title, pid := robotgo.GetTitle(), robotgo.GetPid()
	if title == "" || pid <= 0 {
		return ""
	}
	name := strings.ToLower(title)
	if !strings.Contains(name, "clicker heroes") && !strings.Contains(name, "clickerheroes") {
		return "!outside-game"
	}
	return fmt.Sprintf("%d:%s", pid, title)
}

func focusGameWindow(window string) error {
	pidText, _, ok := strings.Cut(window, ":")
	pid, err := strconv.Atoi(pidText)
	if !ok || err != nil || pid <= 0 {
		return errors.New("game process identity unavailable; focus the game and retry")
	}
	return robotgo.ActivePid(pid)
}
