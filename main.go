package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
	heroLevels := flag.Bool("hero-levels", false, "scroll the Heroes list and buy hero levels in run mode")
	duration := flag.Duration("duration", 0, "maximum run time (0 means unlimited)")
	delay := flag.Duration("delay", 5*time.Second, "time to focus the game before starting")
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
	if *mode == "shot" || *mode == "click" || *mode == "run" {
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
		err = runBot(*x, *y, hasX, *interval, *fishInterval, *duration, *heroLevels, *skills)
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

func clickHeroAt(ctx context.Context, point image.Point) error {
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
	last    image.Point
	clicked bool
	misses  int
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
	return !tracker.clicked || delta.X*delta.X+delta.Y*delta.Y > 150*150
}

func (tracker *fishClickTracker) recordClick(point image.Point) {
	tracker.last = point
	tracker.clicked = true
	tracker.misses = 0
}

func captureHeroScreen(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, bounds image.Rectangle) (image.Image, error) {
	// Leaving hero buttons clears tooltips that can obscure the scrollbar and level text.
	acted, err := controls.runClick(ctx, generation, func() error {
		return input.move(image.Pt(bounds.Min.X+bounds.Dx()*85/100, bounds.Min.Y+bounds.Dy()/2))
	})
	if err != nil || !acted {
		return nil, err
	}
	time.Sleep(200 * time.Millisecond)
	if !controls.valid(ctx, generation) {
		return nil, nil
	}
	capture, err := input.capture()
	if err != nil {
		return nil, fmt.Errorf("capture hero screen: %w", err)
	}
	if capture == nil {
		return nil, errors.New("capture hero screen returned no image")
	}
	return capture, nil
}

func findHeroButtonWithScroll(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, screen image.Image, scanFish func(image.Image) (bool, error)) (image.Image, image.Point, bool, error) {
	if !heroTabSelected(screen) {
		return nil, image.Point{}, false, nil
	}
	drag := func(thumb image.Point, targetY int) (image.Image, bool, error) {
		if ctx.Err() != nil {
			return nil, false, nil
		}
		acted, err := controls.runClick(ctx, generation, func() error {
			return input.drag(thumb, image.Pt(thumb.X, targetY))
		})
		if err != nil || !acted {
			return nil, false, err
		}
		time.Sleep(200 * time.Millisecond)
		if !controls.valid(ctx, generation) {
			return nil, false, nil
		}
		capture, err := input.capture()
		if err != nil {
			return nil, false, fmt.Errorf("capture hero list after drag: %w", err)
		}
		if capture == nil {
			return nil, false, errors.New("capture hero list after drag returned no image")
		}
		clicked, err := scanFish(capture)
		if err != nil || clicked {
			return nil, false, err
		}
		return capture, heroListMoved(screen, capture), nil
	}

	bounds := screen.Bounds()
	thumb, _, found := heroScrollbarThumb(screen)
	if !found {
		return nil, image.Point{}, false, nil
	}
	if !heroScrollbarAtBottom(screen) {
		capture, _, err := drag(thumb, bounds.Max.Y-1)
		if err != nil || capture == nil {
			return nil, image.Point{}, false, err
		}
		screen = capture
		if screen.Bounds() != bounds || !heroScrollbarAtBottom(screen) {
			return nil, image.Point{}, false, nil
		}
	}
	if !controls.valid(ctx, generation) || !heroTabSelected(screen) {
		return nil, image.Point{}, false, nil
	}
	capture, err := captureHeroScreen(ctx, controls, generation, input, bounds)
	if err != nil || capture == nil {
		return nil, image.Point{}, false, err
	}
	clicked, err := scanFish(capture)
	if err != nil || clicked {
		return nil, image.Point{}, false, err
	}
	if capture.Bounds() != bounds || !heroScrollbarAtBottom(capture) {
		return nil, image.Point{}, false, nil
	}
	button, found := findHeroLevelButton(capture)
	if !found {
		return nil, image.Point{}, false, nil
	}
	if !heroCandidateKnown(capture, button) {
		return nil, image.Point{}, false, nil
	}
	return capture, button, true, nil
}

func selectHeroX1(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, screen image.Image) (image.Image, bool, error) {
	if !controls.valid(ctx, generation) {
		return nil, false, nil
	}
	if screen == nil || !heroQuantityBarPresent(screen) {
		return nil, false, nil
	}
	b := screen.Bounds()
	// T cycles the five purchase quantities; verify each frame instead of assuming an order.
	for taps := 0; taps <= 5; taps++ {
		if !controls.valid(ctx, generation) || screen.Bounds() != b || !heroQuantityBarPresent(screen) {
			return screen, false, nil
		}
		if heroQuantitySelected(screen, 122) {
			return screen, true, nil
		}
		if taps == 5 {
			break
		}
		acted, err := controls.runClick(ctx, generation, func() error { return input.keyTap("t") })
		if err != nil || !acted {
			return nil, false, err
		}
		time.Sleep(100 * time.Millisecond)
		captured, err := input.capture()
		screen = captured
		if err != nil {
			return nil, false, fmt.Errorf("capture hero quantity selection: %w", err)
		}
		if screen == nil {
			return nil, false, errors.New("capture hero quantity selection returned no image")
		}
	}
	return screen, false, nil
}

func clickHeroMax(ctx context.Context, input heroInput, button image.Point) (err error) {
	// Release Q before confirmation captures, fish clicks or any error return.
	defer func() { err = errors.Join(err, input.keyToggle("q", "up")) }()
	if err = input.keyToggle("q", "down"); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return input.click(button)
}

func saveForNextHero(gold, nextPrice float64) bool {
	return nextPrice-gold <= 1
}

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration, heroLevels, skills bool) error {
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
	if heroLevels {
		if err := checkHeroOCR(ctx); err != nil {
			cancel()
			return err
		}
	}
	input := heroInput{
		capture:   func() (image.Image, error) { return robotgo.CaptureImg() },
		move:      moveAt,
		click:     func(p image.Point) error { return clickHeroAt(ctx, p) },
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
	scanFish := func(screen image.Image, generation uint64) (bool, error) {
		if !controls.valid(ctx, generation) || time.Since(lastFishScan) < fishInterval {
			return false, nil
		}
		lastFishScan = time.Now()
		point, found, err := sift.Find(screen)
		if err != nil {
			return false, fmt.Errorf("find fish with OpenCV: %w", err)
		}
		if !controls.valid(ctx, generation) || !fishClicks.shouldClick(point, found) {
			return false, nil
		}
		clicked, err := controls.runClick(ctx, generation, func() error { return clickAt(ctx, point.X, point.Y) })
		if err != nil {
			return false, fmt.Errorf("click fish: %w", err)
		}
		if clicked {
			fishClicks.recordClick(point)
			fmt.Printf("clicked fish at (%d, %d)\n", point.X, point.Y)
			clicks++
		}
		return clicked, nil
	}
	heroPurchasesEnabled := heroLevels
	heroFailures := 0
	var nextHeroScan time.Time
	skillPlan := skillPlanner{}
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
		fishClicked, err := scanFish(screenshot, generation)
		if err != nil {
			return err
		}
		if skills && !fishClicked && heroQuantityBarPresent(screenshot) {
			_, err := skillPlan.run(ctx, &controls, generation, input, screenshot, readSkillStates)
			if err != nil {
				return err
			}
		}
		if !heroPurchasesEnabled || fishClicked || ctx.Err() != nil || time.Now().Before(nextHeroScan) || controls.isPaused() {
			return nil
		}
		nextHeroScan = time.Now().Add(5 * time.Second)
		heroScreen, err := robotgo.CaptureImg()
		if err != nil {
			return fmt.Errorf("capture hero screen: %w", err)
		}
		if heroScreen == nil {
			return errors.New("capture hero screen returned no image")
		}
		hadHeroTab := heroLayoutValid(heroScreen)
		if !hadHeroTab {
			return nil
		}
		if !heroQuantityBarPresent(heroScreen) {
			nextHeroScan = time.Now().Add(30 * time.Second)
			return nil
		}
		x1Screen, selected, err := selectHeroX1(ctx, &controls, generation, input, heroScreen)
		if err != nil {
			return fmt.Errorf("select x1 hero levels: %w", err)
		}
		if !selected {
			return nil
		}
		x1Screen, button, found, findErr := findHeroButtonWithScroll(ctx, &controls, generation, input, x1Screen, func(screen image.Image) (bool, error) {
			clicked, err := scanFish(screen, generation)
			fishClicked = fishClicked || clicked
			return clicked, err
		})
		var gold, nextPrice float64
		var readErr error
		if found && heroRowHasLevel(x1Screen, button.Y) {
			if next, hasNext := findNextHeroButton(x1Screen, button); hasNext {
				gold, readErr = readHeroGold(ctx, x1Screen)
				if readErr != nil {
					readErr = fmt.Errorf("gold: %w", readErr)
				}
				if readErr == nil {
					nextPrice, readErr = readHeroPrice(ctx, x1Screen, next)
					if readErr != nil {
						readErr = fmt.Errorf("next hero price: %w", readErr)
					}
				}
			}
		}
		if findErr != nil {
			return findErr
		}
		if !found || ctx.Err() != nil {
			if !found && hadHeroTab && !fishClicked && !controls.isPaused() && ctx.Err() == nil {
				nextHeroScan = time.Now().Add(30 * time.Second)
			}
			return nil
		}
		if readErr != nil {
			fmt.Printf("hero numbers unreadable: %v; retrying in 30s\n", readErr)
			nextHeroScan = time.Now().Add(30 * time.Second)
			return nil
		}
		if nextPrice > 0 && saveForNextHero(gold, nextPrice) {
			fmt.Println("saving gold for next hero")
			return nil
		}
		heroScreen = x1Screen
		beforeLevel := 0
		if heroRowHasLevel(heroScreen, button.Y) {
			beforeLevel, err = readHeroLevel(ctx, heroScreen, button)
			if err != nil {
				fmt.Printf("hero level unreadable: %v; retrying in 30s\n", err)
				nextHeroScan = time.Now().Add(30 * time.Second)
				return nil
			}
		} else if !heroRowUnowned(heroScreen, button.Y) {
			return nil
		}

		clicked, err := controls.runClick(ctx, generation, func() error { return clickHeroMax(ctx, input, button) })
		if err != nil {
			return fmt.Errorf("click hero level: %w", err)
		}
		if !clicked {
			return nil
		}
		clicks++
		var listStable, levelChanged bool
		var lastAfter image.Image
		for range 5 {
			if !controls.valid(ctx, generation) {
				return nil
			}
			after, err := captureHeroScreen(ctx, &controls, generation, input, heroScreen.Bounds())
			if err != nil {
				return fmt.Errorf("capture hero screen after click: %w", err)
			}
			if after == nil {
				return nil
			}
			lastAfter = after
			listStable = heroListStable(heroScreen, after)
			levelChanged = false
			if listStable && sameHeroRow(heroScreen, after, button, button) {
				afterLevel, readErr := readHeroLevel(ctx, after, button)
				levelChanged = readErr == nil && afterLevel > beforeLevel
			}
			if listStable && levelChanged {
				heroFailures = 0
				nextHeroScan = time.Now().Add(5 * time.Second)
				fmt.Printf("leveled hero at (%d, %d)\n", button.X, button.Y)
				return nil
			}
			if _, err := scanFish(after, generation); err != nil {
				return err
			}

		}
		stamp := time.Now().Format("20060102-150405.000")
		beforePath := fmt.Sprintf("artifacts/hero-failure-%s-before.png", stamp)
		afterPath := fmt.Sprintf("artifacts/hero-failure-%s-after.png", stamp)
		marked := image.NewRGBA(heroScreen.Bounds())
		draw.Draw(marked, marked.Bounds(), heroScreen, heroScreen.Bounds().Min, draw.Src)
		for offset := -max(12, marked.Bounds().Dx()/100); offset <= max(12, marked.Bounds().Dx()/100); offset++ {
			marked.Set(button.X+offset, button.Y, color.RGBA{R: 255, A: 255})
			marked.Set(button.X, button.Y+offset, color.RGBA{R: 255, A: 255})
		}
		if err := saveImage(beforePath, marked); err != nil {
			fmt.Printf("failed to save hero screenshot: %v\n", err)
		} else if err := saveImage(afterPath, lastAfter); err != nil {
			fmt.Printf("failed to save hero screenshot: %v\n", err)
		} else {
			fmt.Printf("saved hero failure screenshots: %s, %s\n", beforePath, afterPath)
		}
		heroFailures++
		if heroFailures >= 3 {
			heroPurchasesEnabled = false
			fmt.Printf("hero level change not confirmed at (%d, %d) three times (list stable=%t, level increased=%t); hero purchases stopped\n", button.X, button.Y, listStable, levelChanged)
			return nil
		}
		nextHeroScan = time.Now().Add(30 * time.Second)
		fmt.Printf("hero level change not confirmed at (%d, %d) (list stable=%t, level increased=%t); retrying hero purchases in 30s\n", button.X, button.Y, listStable, levelChanged)
		return nil
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
