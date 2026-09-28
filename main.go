package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/go-vgo/robotgo"
)

func main() {
	mode := flag.String("mode", "help", "help, shot, click, or run")
	output := flag.String("out", "artifacts/screenshot.png", "screenshot file for shot mode")
	x := flag.Int("x", 0, "screen X coordinate for click or run")
	y := flag.Int("y", 0, "screen Y coordinate for click or run")
	interval := flag.Duration("interval", 100*time.Millisecond, "time between clicks in run mode")
	duration := flag.Duration("duration", 10*time.Second, "maximum run time")
	delay := flag.Duration("delay", 5*time.Second, "time to focus the game before starting")
	flag.Parse()

	if *mode == "help" {
		flag.Usage()
		return
	}

	if *mode == "click" || *mode == "run" {
		var hasX, hasY bool
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "x":
				hasX = true
			case "y":
				hasY = true
			}
		})
		if !hasX || !hasY {
			log.Fatal("click and run require both -x and -y")
		}
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
		err = runClicks(*x, *y, *interval, *duration)
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

func runClicks(x, y int, interval, duration time.Duration) error {
	if interval <= 0 || duration <= 0 {
		return errors.New("-interval and -duration must be positive")
	}

	interrupt, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interrupt, duration)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("clicking (%d, %d) for up to %s; press Ctrl+C to stop\n", x, y, duration)
	clicks := 0
	for {
		if ctx.Err() != nil {
			fmt.Printf("stopped after %d clicks\n", clicks)
			return nil
		}
		if err := clickAt(x, y); err != nil {
			return fmt.Errorf("click: %w", err)
		}
		clicks++

		select {
		case <-ctx.Done():
			fmt.Printf("stopped after %d clicks\n", clicks)
			return nil
		case <-ticker.C:
		}
	}
}
