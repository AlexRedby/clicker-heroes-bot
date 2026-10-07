package bot

import (
	"context"
	"errors"
	"image"
	"runtime"
	"time"

	"github.com/go-vgo/robotgo"
)

type nativeDragInput struct {
	move        func(image.Point) error
	point       func(image.Point) (image.Point, error)
	sourceReady func(image.Point) error
	guard       func() error
	smooth      func(image.Point)
	release     func() error
}

func robotDragInput(move func(image.Point) error, point func(image.Point) (image.Point, error), sourceReady func(image.Point) error, guard func() error) nativeDragInput {
	return nativeDragInput{
		move: move, point: point, sourceReady: sourceReady, guard: guard,
		smooth: func(target image.Point) {
			high := 0.75
			if runtime.GOOS == "windows" {
				// RobotGo truncates Windows Sleep to integer milliseconds.
				high = 1.25
			}
			robotgo.DragSmooth(target.X, target.Y, 0.25, high)
		},
		release: func() error { return robotgo.Toggle("left", "up") },
	}
}

func runNativeDrag(ctx context.Context, from, to image.Point, input nativeDragInput) error {
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if input.guard != nil {
			return input.guard()
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	// Validate the destination before moving, then revalidate before pressing.
	if _, err := input.point(to); err != nil {
		return err
	}
	if err := input.move(from); err != nil {
		return err
	}
	// Let cursor arrival and hover reach the game before its fixed 50 ms hold.
	if err := waitInput(ctx, 200*time.Millisecond); err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	if input.sourceReady != nil {
		if err := input.sourceReady(from); err != nil {
			return err
		}
	}
	target, err := input.point(to)
	if err != nil {
		return err
	}
	if err := check(); err != nil {
		return err
	}
	err = func() (err error) {
		// DragSmooth owns down/movement/up. Release also on cancellation or panic.
		defer func() { err = errors.Join(err, input.release()) }()
		input.smooth(target)
		return check()
	}()
	if err != nil {
		return err
	}
	if err := waitInput(ctx, 100*time.Millisecond); err != nil {
		return err
	}
	return check()
}
