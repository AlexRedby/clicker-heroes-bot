package bot

import (
	"context"
	"errors"
	"image"
	"runtime"
	"time"

	"github.com/go-vgo/robotgo"
)

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
	return waitInput(ctx, 50*time.Millisecond)
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
	defer func() {
		err = errors.Join(err, toggle("left", "up"))
		if err == nil {
			// Leave the pointer and Q unchanged while the game consumes mouse-up.
			err = waitInput(ctx, 100*time.Millisecond)
		}
	}()
	if err = toggle("left", "down"); err != nil {
		return err
	}
	return waitInput(ctx, 100*time.Millisecond)
}

func waitInput(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func withHeldKey(ctx context.Context, input heroInput, key string, delay time.Duration, action func() error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, input.keyToggle(key, "up")) }()
	if err = input.keyToggle(key, "down"); err != nil {
		return err
	}
	if err = waitInput(ctx, delay); err != nil {
		return err
	}
	return action()
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
