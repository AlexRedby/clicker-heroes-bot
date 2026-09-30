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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
	hook "github.com/robotn/gohook"
)

func main() {
	robotgo.Scale = runtime.GOOS == "darwin"
	mode := flag.String("mode", "help", "help, shot, click, or run")
	output := flag.String("out", "artifacts/screenshot.png", "screenshot file for shot mode")
	x := flag.Int("x", 0, "screen X coordinate for click or optional monster clicks in run mode")
	y := flag.Int("y", 0, "screen Y coordinate for click or optional monster clicks in run mode")
	interval := flag.Duration("interval", 100*time.Millisecond, "time between optional monster clicks in run mode")
	fishInterval := flag.Duration("fish-interval", time.Second, "time between fish scans in run mode")
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
		err = clickAt(*x, *y)
	case "run":
		err = runBot(*x, *y, hasX, *interval, *fishInterval, *duration, *heroLevels)
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

func clickAt(x, y int) error {
	robotgo.Move(x, y)
	time.Sleep(50 * time.Millisecond)
	return robotgo.Click("left")
}

type pauseControl struct {
	mu     sync.Mutex
	paused bool
}

func (control *pauseControl) toggle() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.paused = !control.paused
	return control.paused
}

func (control *pauseControl) pause() {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.paused = true
}

func (control *pauseControl) isPaused() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.paused
}

func (control *pauseControl) runClick(action func() error) (bool, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused {
		return false, nil
	}
	if err := action(); err != nil {
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
}

func (tracker *fishClickTracker) shouldClick(point image.Point, found bool) bool {
	if !found {
		tracker.clicked = false
		return false
	}
	delta := point.Sub(tracker.last)
	return !tracker.clicked || delta.X*delta.X+delta.Y*delta.Y > 20*20
}

func (tracker *fishClickTracker) recordClick(point image.Point) {
	tracker.last = point
	tracker.clicked = true
}

func findHeroButtonWithScroll(ctx context.Context, controls *pauseControl, screen image.Image, scanFish func(image.Image) (bool, error)) (image.Image, image.Point, bool, error) {
	if !heroTabSelected(screen) {
		return nil, image.Point{}, false, nil
	}
	drag := func(thumb image.Point, targetY int) (image.Image, bool, error) {
		if ctx.Err() != nil {
			return nil, false, nil
		}
		acted, err := controls.runClick(func() error {
			robotgo.Move(thumb.X, thumb.Y)
			time.Sleep(50 * time.Millisecond)
			robotgo.DragSmooth(thumb.X, targetY, 0.1, 0.2, 0)
			return nil
		})
		if err != nil || !acted {
			return nil, false, err
		}
		time.Sleep(50 * time.Millisecond)
		capture, err := robotgo.CaptureImg()
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
	thumb, height, found := heroScrollbarThumb(screen)
	if !found {
		return nil, image.Point{}, false, nil
	}
	bottom := bounds.Min.Y + bounds.Dy()*965/1000 - height/2
	if thumb.Y < bottom-bounds.Dy()/100 {
		capture, moved, err := drag(thumb, bottom)
		if err != nil || capture == nil {
			return nil, image.Point{}, false, err
		}
		screen = capture
		afterThumb, _, found := heroScrollbarThumb(screen)
		if !found || (!moved && bottom-thumb.Y > bounds.Dy()/20 && afterThumb.Y <= thumb.Y+bounds.Dy()/100) {
			return nil, image.Point{}, false, nil
		}
	}
	if ctx.Err() != nil || controls.isPaused() || !heroTabSelected(screen) {
		return nil, image.Point{}, false, nil
	}
	acted, err := controls.runClick(func() error {
		robotgo.Move(bounds.Min.X+bounds.Dx()*60/100, bounds.Min.Y+bounds.Dy()/2)
		return nil
	})
	if err != nil || !acted {
		return nil, image.Point{}, false, err
	}
	time.Sleep(100 * time.Millisecond)
	capture, err := robotgo.CaptureImg()
	if err != nil {
		return nil, image.Point{}, false, fmt.Errorf("capture hero screen before click: %w", err)
	}
	if capture == nil {
		return nil, image.Point{}, false, errors.New("capture hero screen before click returned no image")
	}
	clicked, err := scanFish(capture)
	if err != nil || clicked {
		return nil, image.Point{}, false, err
	}
	button, found := findHeroLevelButton(capture)
	if !found {
		return nil, image.Point{}, false, nil
	}
	if next, found := findNextHeroButton(capture, button); found && heroRowHasLevel(capture, next.Y) {
		return nil, image.Point{}, false, nil
	}
	return capture, button, true, nil
}

func selectHeroQuantity(controls *pauseControl, xPercent int) (image.Image, bool, error) {
	screen, err := robotgo.CaptureImg()
	if err != nil {
		return nil, false, fmt.Errorf("capture hero quantity bar: %w", err)
	}
	if screen == nil {
		return nil, false, errors.New("capture hero quantity bar returned no image")
	}
	if !heroQuantityBarPresent(screen) {
		return nil, false, nil
	}
	b := screen.Bounds()
	clicked, err := controls.runClick(func() error {
		return clickAt(b.Min.X+b.Dx()*xPercent/1000, b.Min.Y+b.Dy()*345/1000)
	})
	if err != nil || !clicked {
		return nil, clicked, err
	}
	time.Sleep(100 * time.Millisecond)
	updated, err := robotgo.CaptureImg()
	if err != nil {
		return nil, true, fmt.Errorf("capture hero quantity selection: %w", err)
	}
	if updated == nil {
		return nil, true, errors.New("capture hero quantity selection returned no image")
	}
	return updated, true, nil
}

func saveForNextHero(gold, nextPrice float64) bool {
	return nextPrice-gold <= 1
}

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration, heroLevels bool) error {
	if fishInterval <= 0 || duration < 0 || (monsterClicks && interval <= 0) {
		return errors.New("-fish-interval must be positive; -duration must be non-negative; -interval must be positive when monster clicks are enabled")
	}
	if heroLevels {
		if _, err := exec.LookPath("tesseract"); err != nil {
			return errors.New("hero leveling requires Tesseract OCR in PATH")
		}
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
	scanFish := func(screen image.Image) (bool, error) {
		if ctx.Err() != nil || controls.isPaused() || time.Since(lastFishScan) < fishInterval {
			return false, nil
		}
		lastFishScan = time.Now()
		point, found, err := sift.Find(screen)
		if err != nil {
			return false, fmt.Errorf("find fish with OpenCV: %w", err)
		}
		if !fishClicks.shouldClick(point, found) {
			return false, nil
		}
		clicked, err := controls.runClick(func() error { return clickAt(point.X, point.Y) })
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
	scan := func() error {
		if controls.isPaused() {
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
		fishClicked, err := scanFish(screenshot)
		if err != nil {
			return err
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
		hadHeroTab := heroTabSelected(heroScreen)
		if !hadHeroTab {
			return nil
		}
		if !heroQuantityBarPresent(heroScreen) {
			nextHeroScan = time.Now().Add(30 * time.Second)
			return nil
		}
		x1Screen, selected, err := selectHeroQuantity(&controls, 122)
		if err != nil {
			return fmt.Errorf("select x1 hero levels: %w", err)
		}
		if !selected {
			return nil
		}
		x1Screen, button, found, findErr := findHeroButtonWithScroll(ctx, &controls, x1Screen, func(screen image.Image) (bool, error) {
			clicked, err := scanFish(screen)
			fishClicked = fishClicked || clicked
			return clicked, err
		})
		var gold, nextPrice float64
		var readErr error
		if found && heroRowHasLevel(x1Screen, button.Y) {
			if next, hasNext := findNextHeroButton(x1Screen, button); hasNext {
				gold, readErr = readHeroGold(x1Screen)
				if readErr == nil {
					nextPrice, readErr = readHeroPrice(x1Screen, next)
				}
			}
		}
		maxScreen, restored, restoreErr := selectHeroQuantity(&controls, 435)
		if findErr != nil {
			return findErr
		}
		if restoreErr != nil {
			return fmt.Errorf("restore MAX hero levels: %w", restoreErr)
		}
		if !restored {
			return nil
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
		if current, enabled := findHeroLevelButton(maxScreen); !enabled || absDiff(current.Y, button.Y) > maxScreen.Bounds().Dy()/50 {
			return nil
		}
		heroScreen = maxScreen
		clicked, err := controls.runClick(func() error { return clickAt(button.X, button.Y) })
		if err != nil {
			return fmt.Errorf("click hero level: %w", err)
		}
		if !clicked {
			return nil
		}
		clicks++
		var listMoved, levelChanged bool
		var lastAfter image.Image
		for range 5 {
			if ctx.Err() != nil || controls.isPaused() {
				return nil
			}
			time.Sleep(200 * time.Millisecond)
			after, err := robotgo.CaptureImg()
			if err != nil {
				return fmt.Errorf("capture hero screen after click: %w", err)
			}
			if after == nil {
				return errors.New("capture hero screen after click returned no image")
			}
			lastAfter = after
			listMoved = heroListMoved(heroScreen, after)
			levelChanged = heroLevelChanged(heroScreen, after, button)
			if !listMoved && levelChanged {
				heroFailures = 0
				nextHeroScan = time.Now().Add(5 * time.Second)
				fmt.Printf("leveled hero at (%d, %d)\n", button.X, button.Y)
				return nil
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
			fmt.Printf("hero level change not confirmed at (%d, %d) three times (list moved=%t, level pixels changed=%t); hero purchases stopped\n", button.X, button.Y, listMoved, levelChanged)
			return nil
		}
		nextHeroScan = time.Now().Add(30 * time.Second)
		fmt.Printf("hero level change not confirmed at (%d, %d) (list moved=%t, level pixels changed=%t); retrying hero purchases in 30s\n", button.X, button.Y, listMoved, levelChanged)
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
			clicked, err := controls.runClick(func() error { return clickAt(x, y) })
			if err != nil {
				return fmt.Errorf("click monster: %w", err)
			}
			if clicked {
				clicks++
			}
		}
	}
}
