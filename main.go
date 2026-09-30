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
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
	hook "github.com/robotn/gohook"
)

func main() {
	robotgo.Scale = false
	mode := flag.String("mode", "help", "help, shot, click, or run")
	output := flag.String("out", "artifacts/screenshot.png", "screenshot file for shot mode")
	x := flag.Int("x", 0, "screen X coordinate for click or optional monster clicks in run mode")
	y := flag.Int("y", 0, "screen Y coordinate for click or optional monster clicks in run mode")
	interval := flag.Duration("interval", 100*time.Millisecond, "time between optional monster clicks in run mode")
	fishInterval := flag.Duration("fish-interval", time.Second, "time between fish scans in run mode")
	skills := flag.Bool("skills", false, "activate unlocked skills with hotkeys 1-9 in run mode")
	progression := flag.Bool("progression", false, "manage progression mode and wait for damage improvements after failed bosses")
	heroLevels := flag.Bool("hero-levels", false, "scroll the Heroes list and buy hero levels in run mode")
	duration := flag.Duration("duration", 0, "maximum run time (0 means unlimited)")
	delay := flag.Duration("delay", 5*time.Second, "time to focus the game before shot or click (run waits for F8)")
	flag.Parse()

	if *mode == "help" {
		flag.Usage()
		return
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
	case "shot":
		err = saveScreenshot(*output)
	case "click":
		err = clickAt(context.Background(), *x, *y)
	case "run":
		err = runBot(*x, *y, hasX, *interval, *fishInterval, *duration, *heroLevels, *skills, *progression)
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

func desktopPoint(point image.Point, pixels, desktop image.Rectangle) (image.Point, error) {
	if !point.In(pixels) || pixels.Empty() || desktop.Empty() {
		return image.Point{}, fmt.Errorf("click coordinate %v is outside captured display %v", point, pixels)
	}
	return image.Pt(desktop.Min.X+(point.X-pixels.Min.X)*desktop.Dx()/pixels.Dx(), desktop.Min.Y+(point.Y-pixels.Min.Y)*desktop.Dy()/pixels.Dy()), nil
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
	capture   func() (image.Image, error)
	move      func(image.Point) error
	drag      func(image.Point, image.Point) error
	click     func(image.Point) error
	keyTap    func(string) error
	keyToggle func(string, string) error
}

type pauseControl struct {
	mu         sync.Mutex
	paused     bool
	generation uint64
}

func (control *pauseControl) toggle() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.paused = !control.paused
	control.generation++
	return control.paused
}

func (control *pauseControl) pause() {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.paused = true
	control.generation++
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

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration, heroLevels, skills, progression bool) error {
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
	if heroLevels || progression {
		if err := checkHeroOCR(ctx); err != nil {
			cancel()
			return err
		}
	}
	input := heroInput{
		capture:   func() (image.Image, error) { return robotgo.CaptureImg() },
		move:      moveAt,
		click:     func(p image.Point) error { return clickGameAt(ctx, p) },
		keyTap:    func(key string) error { return robotgo.KeyTap(key) },
		keyToggle: func(key, state string) error { return robotgo.KeyToggle(key, state) },
		drag: func(from, to image.Point) error {
			if err := moveAt(from); err != nil {
				return err
			}
			target, err := mousePoint(to)
			if err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond)
			robotgo.DragSmooth(target.X, target.Y, 0.5, 1.5)
			return nil
		},
	}
	controls := pauseControl{paused: true}
	// GoHook's End crashes on macOS when Accessibility is denied; this CLI releases the hook on exit.
	events := hook.Start()
	hookDone := make(chan struct{})
	go func() {
		defer close(hookDone)
		for {
			select {
			case event, ok := <-events:
				if !ok {
					controls.pause()
					cancel()
					return
				}
				if isPauseKey(event) {
					if controls.toggle() {
						fmt.Println("paused")
					} else {
						fmt.Println("resumed")
					}
				}
			case <-ctx.Done():
				controls.pause()
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-hookDone
	}()

	fishTicker := time.NewTicker(fishInterval)
	defer fishTicker.Stop()
	var clickTicks <-chan time.Time
	if monsterClicks {
		clickTicker := time.NewTicker(interval)
		defer clickTicker.Stop()
		clickTicks = clickTicker.C
	}

	fmt.Println("paused; press F8 to start or pause, Ctrl+C to stop")
	clicks := 0
	fishClicks := fishClickTracker{}
	var lastFishScan time.Time
	scanFish := func(screen image.Image, generation uint64, force bool) (bool, error) {
		// Hero interaction checks must inspect their fresh frame, even between periodic ticks.
		if !controls.valid(ctx, generation) || (!force && time.Since(lastFishScan) < fishInterval) {
			return false, nil
		}
		lastFishScan = time.Now()
		point, found, err := sift.Find(screen)
		if err != nil {
			return false, fmt.Errorf("find fish with OpenCV: %w", err)
		}
		if !controls.valid(ctx, generation) {
			return false, nil
		}
		if !fishClicks.shouldClick(point, found) {
			return found, nil
		}
		clicked, err := controls.runClick(ctx, generation, func() error { return clickGameAt(ctx, point) })
		if err != nil {
			return false, fmt.Errorf("click fish: %w", err)
		}
		if clicked {
			fishClicks.recordClick(point)
			fmt.Printf("clicked fish at (%d, %d)\n", point.X, point.Y)
			clicks++
		}
		return found, nil
	}
	heroPlan := heroRunner{enabled: heroLevels}
	skillPlan := skillPlanner{}
	progressionPlan := progressionPlanner{}
	scan := func() error {
		generation := controls.snapshot()
		if !controls.valid(ctx, generation) {
			return nil
		}
		screenshot, err := robotgo.CaptureImg()
		if err != nil {
			return fmt.Errorf("capture screen: %w", err)
		}
		if screenshot == nil {
			return errors.New("capture screen returned no image")
		}
		if ctx.Err() != nil {
			return nil
		}
		fishPresent, err := scanFish(screenshot, generation, false)
		if err != nil {
			return err
		}
		scanFreshFish := func(frame image.Image) (bool, error) {
			present, err := scanFish(frame, generation, true)
			fishPresent = fishPresent || present
			return present, err
		}
		if skills && !fishPresent && heroQuantityBarPresent(screenshot) {
			_, err := skillPlan.run(ctx, &controls, generation, input, screenshot, readSkillStates)
			if err != nil {
				return err
			}
		}
		if progression && !fishPresent && heroQuantityBarPresent(screenshot) {
			if _, err := progressionPlan.run(ctx, &controls, generation, input, readProgressionState, scanFreshFish); err != nil {
				return err
			}
		}
		if fishPresent {
			return nil
		}
		acted, err := heroPlan.run(ctx, &controls, generation, input, heroReaders{readHeroGold, readHeroPrice, readHeroLevel}, scanFreshFish)
		if acted {
			clicks++
		}
		return err
	}
	if err := scan(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("stopped after %d clicks\n", clicks)
			return nil
		case <-fishTicker.C:
			if err := scan(); err != nil {
				return err
			}
		case <-clickTicks:
			if ctx.Err() != nil {
				continue
			}
			generation := controls.snapshot()
			clicked, err := controls.runClick(ctx, generation, func() error { return clickAt(ctx, x, y) })
			if err != nil {
				return fmt.Errorf("click monster: %w", err)
			}
			if clicked {
				clicks++
			}
		}
	}
}
