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
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
	hook "github.com/robotn/gohook"
)

func main() {
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

	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return fmt.Errorf("encode screenshot: %w", err)
	}
	if err := writeScreenshot(path, data.Bytes()); err != nil {
		return err
	}
	fmt.Println("saved", path)
	return nil
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
	for range 40 {
		if ctx.Err() != nil || controls.isPaused() || !heroTabSelected(screen) {
			return nil, image.Point{}, false, nil
		}
		if _, found := findHeroLevelButton(screen); found {
			bounds := screen.Bounds()
			acted, err := controls.runClick(func() error {
				robotgo.Move(bounds.Max.X-10, bounds.Min.Y+bounds.Dy()/2)
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
			if button, stillFound := findHeroLevelButton(capture); stillFound {
				return capture, button, true, nil
			}
			screen = capture
		}
		thumb, height, found := heroScrollbarThumb(screen)
		if !found {
			break
		}
		targetY := max(bounds.Min.Y+bounds.Dy()*30/100, thumb.Y-height*3/4)
		if targetY >= thumb.Y {
			break
		}
		capture, moved, err := drag(thumb, targetY)
		if err != nil || capture == nil {
			return nil, image.Point{}, false, err
		}
		screen = capture
		if !moved {
			break
		}
	}
	return nil, image.Point{}, false, nil
}

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration, heroLevels bool) error {
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
		heroScreen, button, found, err := findHeroButtonWithScroll(ctx, &controls, heroScreen, func(screen image.Image) (bool, error) {
			clicked, err := scanFish(screen)
			fishClicked = fishClicked || clicked
			return clicked, err
		})
		if err != nil {
			return err
		}
		if !found || ctx.Err() != nil {
			if !found && hadHeroTab && !fishClicked && !controls.isPaused() && ctx.Err() == nil {
				nextHeroScan = time.Now().Add(30 * time.Second)
			}
			return nil
		}
		clicked, err := controls.runClick(func() error { return clickAt(button.X, button.Y) })
		if err != nil {
			return fmt.Errorf("click hero level: %w", err)
		}
		if !clicked {
			return nil
		}
		clicks++
		robotgo.Move(heroScreen.Bounds().Max.X-10, heroScreen.Bounds().Min.Y+heroScreen.Bounds().Dy()/2)
		for range 3 {
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
			if !heroListMoved(heroScreen, after) && heroLevelChanged(heroScreen, after, button) {
				heroFailures = 0
				nextHeroScan = time.Now().Add(5 * time.Second)
				fmt.Printf("leveled hero at (%d, %d)\n", button.X, button.Y)
				return nil
			}
		}
		heroFailures++
		if heroFailures >= 3 {
			heroPurchasesEnabled = false
			fmt.Printf("hero level change not confirmed at (%d, %d) three times; hero purchases stopped\n", button.X, button.Y)
			return nil
		}
		nextHeroScan = time.Now().Add(30 * time.Second)
		fmt.Printf("hero level change not confirmed at (%d, %d); retrying hero purchases in 30s\n", button.X, button.Y)
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
