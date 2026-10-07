package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"clicker-heroes-bot/internal/vision"

	"github.com/go-vgo/robotgo"
	hook "github.com/robotn/gohook"
)

func runBot(options pipelineOptions, duration time.Duration, stats bool) error {
	if duration < 0 {
		return errors.New("-duration must be non-negative")
	}

	sift, err := vision.NewFishDetector()
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
	if options.heroes || options.progression || options.mercenaries || options.ancientPlan != nil || options.export != nil {
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
			return runNativeDrag(ctx, from, to, robotDragInput(moveAt, mousePoint, nil, nil))
		},
	}
	controls := pauseControl{paused: true}
	windowReader := foregroundGameWindow
	if options.windowed {
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
	}, options)
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
