package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"
)

type heroRunner struct {
	enabled  bool
	failures int
	nextScan time.Time
}

type heroReaders struct {
	gold  func(context.Context, image.Image) (float64, error)
	price func(context.Context, image.Image, image.Point) (float64, error)
	level func(context.Context, image.Image, image.Point) (int, error)
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
	if !controls.valid(ctx, generation) || !heroTabSelected(screen) {
		return nil, image.Point{}, false, nil
	}
	// A fish can appear after quantity selection and before the first drag.
	if found, err := scanFish(screen); err != nil || found {
		return nil, image.Point{}, false, err
	}
	drag := func(thumb image.Point, targetY int) (image.Image, error) {
		if ctx.Err() != nil {
			return nil, nil
		}
		acted, err := controls.runClick(ctx, generation, func() error {
			return input.drag(thumb, image.Pt(thumb.X, targetY))
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
			return nil, fmt.Errorf("capture hero list after drag: %w", err)
		}
		if capture == nil {
			return nil, errors.New("capture hero list after drag returned no image")
		}
		clicked, err := scanFish(capture)
		if err != nil || clicked {
			return nil, err
		}
		return capture, nil
	}

	bounds := screen.Bounds()
	thumb, _, found := heroScrollbarThumb(screen)
	if !found {
		return nil, image.Point{}, false, nil
	}
	if !heroScrollbarAtBottom(screen) {
		capture, err := drag(thumb, bounds.Max.Y-1)
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

func (p *heroRunner) run(ctx context.Context, controls *pauseControl, generation uint64, input heroInput, read heroReaders, scanFish func(image.Image) (bool, error)) (bool, error) {
	acted, fishPresent := false, false
	if !p.enabled || time.Now().Before(p.nextScan) || !controls.valid(ctx, generation) {
		return acted, nil
	}
	p.nextScan = time.Now().Add(5 * time.Second)
	heroScreen, err := input.capture()
	if err != nil {
		return acted, fmt.Errorf("capture hero screen: %w", err)
	}
	if !controls.valid(ctx, generation) {
		return acted, nil
	}
	if heroScreen == nil {
		return acted, errors.New("capture hero screen returned no image")
	}
	if !heroLayoutValid(heroScreen) {
		return acted, nil
	}
	x1Screen, selected, err := selectHeroX1(ctx, controls, generation, input, heroScreen)
	if err != nil {
		return acted, fmt.Errorf("select x1 hero levels: %w", err)
	}
	if !selected {
		return acted, nil
	}
	x1Screen, button, found, findErr := findHeroButtonWithScroll(ctx, controls, generation, input, x1Screen, func(screen image.Image) (bool, error) {
		present, err := scanFish(screen)
		fishPresent = fishPresent || present
		return present, err
	})
	if !controls.valid(ctx, generation) {
		return acted, nil
	}
	if findErr != nil {
		return acted, findErr
	}
	var gold, nextPrice float64
	var readErr error
	if found && heroRowHasLevel(x1Screen, button.Y) {
		if next, hasNext := findNextHeroButton(x1Screen, button); hasNext {
			gold, readErr = read.gold(ctx, x1Screen)
			if !controls.valid(ctx, generation) {
				return acted, nil
			}
			if readErr != nil {
				readErr = fmt.Errorf("gold: %w", readErr)
			}
			if readErr == nil {
				nextPrice, readErr = read.price(ctx, x1Screen, next)
				if !controls.valid(ctx, generation) {
					return acted, nil
				}
				if readErr != nil {
					readErr = fmt.Errorf("next hero price: %w", readErr)
				}
			}
		}
	}
	if !found {
		if !fishPresent {
			p.nextScan = time.Now().Add(30 * time.Second)
		}
		return acted, nil
	}
	if readErr != nil {
		fmt.Printf("hero numbers unreadable: %v; retrying in 30s\n", readErr)
		p.nextScan = time.Now().Add(30 * time.Second)
		return acted, nil
	}
	if nextPrice > 0 && saveForNextHero(gold, nextPrice) {
		fmt.Println("saving gold for next hero")
		return acted, nil
	}
	heroScreen = x1Screen
	beforeLevel := 0
	if heroRowHasLevel(heroScreen, button.Y) {
		beforeLevel, err = read.level(ctx, heroScreen, button)
		if !controls.valid(ctx, generation) {
			return acted, nil
		}
		if err != nil {
			fmt.Printf("hero level unreadable: %v; retrying in 30s\n", err)
			p.nextScan = time.Now().Add(30 * time.Second)
			return acted, nil
		}
	} else if !heroRowUnowned(heroScreen, button.Y) {
		return acted, nil
	}

	clicked, err := controls.runClick(ctx, generation, func() error { return clickHeroMax(ctx, input, button) })
	if err != nil {
		return acted, fmt.Errorf("click hero level: %w", err)
	}
	if !clicked {
		return acted, nil
	}
	acted = true
	var listStable, levelChanged bool
	var lastAfter image.Image
	for range 5 {
		if !controls.valid(ctx, generation) {
			return acted, nil
		}
		after, err := captureHeroScreen(ctx, controls, generation, input, heroScreen.Bounds())
		if err != nil {
			return acted, fmt.Errorf("capture hero screen after click: %w", err)
		}
		if after == nil || !controls.valid(ctx, generation) {
			return acted, nil
		}
		lastAfter = after
		listStable = heroListStable(heroScreen, after)
		levelChanged = false
		if listStable && heroRowNameMatches(heroScreen, after, button, button) {
			afterLevel, readErr := read.level(ctx, after, button)
			if !controls.valid(ctx, generation) {
				return acted, nil
			}
			levelChanged = readErr == nil && afterLevel > beforeLevel
		}
		if listStable && levelChanged {
			committed, _ := controls.runClick(ctx, generation, func() error {
				p.failures = 0
				p.nextScan = time.Now().Add(5 * time.Second)
				return nil
			})
			if committed {
				fmt.Printf("leveled hero at (%d, %d)\n", button.X, button.Y)
			}
			return acted, nil
		}
		present, err := scanFish(after)
		if err != nil {
			return acted, err
		}
		if present {
			// A fish obscuring confirmation is not a failed hero purchase.
			p.nextScan = time.Now().Add(5 * time.Second)
			return acted, nil
		}
	}
	committed, _ := controls.runClick(ctx, generation, func() error {
		p.failures++
		p.enabled = p.failures < 3
		p.nextScan = time.Now().Add(30 * time.Second)
		return nil
	})
	if !committed {
		return acted, nil
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
	if !p.enabled {
		fmt.Printf("hero level change not confirmed at (%d, %d) three times (list stable=%t, level increased=%t); hero purchases stopped\n", button.X, button.Y, listStable, levelChanged)
		return acted, nil
	}
	fmt.Printf("hero level change not confirmed at (%d, %d) (list stable=%t, level increased=%t); retrying hero purchases in 30s\n", button.X, button.Y, listStable, levelChanged)
	return acted, nil
}
