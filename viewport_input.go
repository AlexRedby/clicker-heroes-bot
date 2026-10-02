package main

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"time"

	"github.com/go-vgo/robotgo"
)

// Each transaction gets closures bound to its immutable frame, never the latest capture.
func bindViewportInput(ctx context.Context, base heroInput, g viewportGeometry) heroInput {
	guard := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		scene, err := readNativeScene()
		if err != nil || scene != g.Scene || g.ROI.Empty() {
			return errInputContext
		}
		return nil
	}
	argument := func(p image.Point) (image.Point, error) {
		if err := guard(); err != nil {
			return image.Point{}, err
		}
		desktop, err := g.point(p)
		if err != nil {
			return image.Point{}, err
		}
		if !nativeTargetVisible(g.Scene, desktop) {
			return image.Point{}, errInputContext
		}
		return nativeMouseArgument(desktop)
	}
	move := func(p image.Point) error {
		target, err := argument(p)
		if err != nil {
			return err
		}
		robotgo.Move(target.X, target.Y)
		return nil
	}
	settle := func(p image.Point, delay time.Duration) error {
		if err := move(p); err != nil {
			return err
		}
		time.Sleep(delay)
		if _, err := argument(p); err != nil {
			return err
		}
		want, err := g.point(p)
		if err != nil {
			return err
		}
		actual, err := nativeMousePosition()
		if err != nil {
			return err
		}
		if absDiff(actual.X, want.X) > 1 || absDiff(actual.Y, want.Y) > 1 {
			return fmt.Errorf("native cursor %v did not reach %v", actual, want)
		}
		return nil
	}
	base.bind = nil
	base.move = move
	base.click = func(p image.Point) error {
		if err := settle(p, 50*time.Millisecond); err != nil {
			return err
		}
		return clickLeft(ctx, func(args ...interface{}) error {
			if len(args) > 1 && args[1] == "down" {
				if _, err := argument(p); err != nil {
					return err
				}
			}
			return robotgo.Toggle(args...)
		})
	}
	base.monsterClick = base.click
	base.drag = func(from, to image.Point) error {
		// Validate both endpoints before mouse-down. Keep RobotGo's native drag.
		if _, err := argument(to); err != nil {
			return err
		}
		if err := settle(from, 100*time.Millisecond); err != nil {
			return err
		}
		target, err := argument(to)
		if err != nil {
			return err
		}
		defer robotgo.Toggle("left", "up")
		high := 0.75
		if runtime.GOOS == "windows" {
			high = 1.25
		}
		robotgo.DragSmooth(target.X, target.Y, 0.25, high)
		return guard()
	}
	keyTap, keyToggle, typeText := base.keyTap, base.keyToggle, base.typeText
	base.keyTap = func(key string) error {
		if err := guard(); err != nil {
			return err
		}
		return keyTap(key)
	}
	base.keyToggle = func(key, state string) error {
		// Releases must survive lost focus/cancellation so no held input is stranded.
		if state != "up" {
			if err := guard(); err != nil {
				return err
			}
		}
		return keyToggle(key, state)
	}
	base.typeText = func(text string) error {
		if err := guard(); err != nil {
			return err
		}
		return typeText(text)
	}
	return base
}
