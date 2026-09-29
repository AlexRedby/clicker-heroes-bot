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
	duration := flag.Duration("duration", 10*time.Second, "maximum run time")
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
		err = runBot(*x, *y, hasX, *interval, *fishInterval, *duration)
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
	return !tracker.clicked || point != tracker.last
}

func (tracker *fishClickTracker) recordClick(point image.Point) {
	tracker.last = point
	tracker.clicked = true
}

func runBot(x, y int, monsterClicks bool, interval, fishInterval, duration time.Duration) error {
	if fishInterval <= 0 || duration <= 0 || (monsterClicks && interval <= 0) {
		return errors.New("-fish-interval and -duration must be positive; -interval must be positive when monster clicks are enabled")
	}
	fish, err := loadFish()
	if err != nil {
		return fmt.Errorf("load fish image: %w", err)
	}

	interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupt, duration)
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

	fmt.Printf("paused for up to %s; press F8 to start or pause, Ctrl+C to stop\n", duration)
	clicks := 0
	fishClicks := fishClickTracker{}
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
		point, found := findFish(screenshot, fish)
		if ctx.Err() == nil && fishClicks.shouldClick(point, found) {
			clicked, err := controls.runClick(func() error { return clickAt(point.X, point.Y) })
			if err != nil {
				return fmt.Errorf("click fish: %w", err)
			}
			if clicked {
				fishClicks.recordClick(point)
				fmt.Printf("clicked fish at (%d, %d)\n", point.X, point.Y)
				clicks++
			}
		}
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
