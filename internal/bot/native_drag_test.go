package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"testing"
	"time"
)

func TestNativeDragSettlesAndReleases(t *testing.T) {
	from, to, desktop := image.Pt(20, 30), image.Pt(20, 90), image.Pt(120, 190)
	var events []string
	var movedAt, draggedAt, releasedAt time.Time
	input := nativeDragInput{
		point: func(p image.Point) (image.Point, error) {
			if p != to {
				t.Fatalf("unexpected endpoint %v", p)
			}
			events = append(events, "endpoint")
			return desktop, nil
		},
		move: func(p image.Point) error {
			if p != from {
				t.Fatalf("unexpected source %v", p)
			}
			movedAt = time.Now()
			events = append(events, "move")
			return nil
		},
		sourceReady: func(p image.Point) error {
			events = append(events, "verify source")
			return nil
		},
		smooth: func(p image.Point) {
			if p != desktop {
				t.Fatalf("unmapped smooth endpoint %v", p)
			}
			draggedAt = time.Now()
			events = append(events, "smooth")
		},
		release: func() error {
			releasedAt = time.Now()
			events = append(events, "up")
			return nil
		},
	}
	if err := runNativeDrag(context.Background(), from, to, input); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(events) != "[endpoint move verify source endpoint smooth up]" {
		t.Fatalf("unsafe drag ordering: %v", events)
	}
	if draggedAt.Sub(movedAt) < 200*time.Millisecond || time.Since(releasedAt) < 100*time.Millisecond {
		t.Fatalf("drag did not settle: source=%s release=%s", draggedAt.Sub(movedAt), time.Since(releasedAt))
	}
}

func TestNativeDragRejectsStaleSourceBeforePressing(t *testing.T) {
	failure := errors.New("native cursor or scene changed")
	for _, scenario := range []string{"cancelled before", "endpoint invalid", "move failed", "cancelled after move", "guard changed", "source drifted", "cancelled during source check", "endpoint changed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled before" {
				cancel()
			}
			moved, endpoints := false, 0
			input := nativeDragInput{
				point: func(p image.Point) (image.Point, error) {
					endpoints++
					if scenario == "endpoint invalid" || scenario == "endpoint changed" && endpoints == 2 {
						return image.Point{}, failure
					}
					return p, nil
				},
				move: func(image.Point) error {
					moved = true
					if scenario == "cancelled after move" {
						cancel()
					}
					if scenario == "move failed" {
						return failure
					}
					return nil
				},
				guard: func() error {
					if scenario == "guard changed" && moved {
						return failure
					}
					return nil
				},
				sourceReady: func(image.Point) error {
					if scenario == "source drifted" {
						return failure
					}
					if scenario == "cancelled during source check" {
						cancel()
					}
					return nil
				},
				smooth:  func(image.Point) { t.Fatal("pressed on an invalid source") },
				release: func() error { t.Fatal("released before any press"); return nil },
			}
			err := runNativeDrag(ctx, image.Pt(1, 2), image.Pt(1, 8), input)
			want := failure
			if scenario == "cancelled before" || scenario == "cancelled after move" || scenario == "cancelled during source check" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
			if (scenario == "endpoint invalid" || scenario == "cancelled before") && moved {
				t.Fatal("moved before endpoint/context validation")
			}
		})
	}
}

func TestNativeDragAlwaysReleasesAfterSmooth(t *testing.T) {
	failure := errors.New("scene or release failed")
	for _, scenario := range []string{"cancelled during smooth", "guard changed", "release failed", "cancelled during release", "panic"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dragged, released := false, false
			input := nativeDragInput{
				point: func(p image.Point) (image.Point, error) { return p, nil },
				move:  func(image.Point) error { return nil },
				guard: func() error {
					if dragged && scenario == "guard changed" {
						return failure
					}
					return nil
				},
				smooth: func(image.Point) {
					dragged = true
					if scenario == "cancelled during smooth" {
						cancel()
					}
					if scenario == "panic" {
						panic(failure)
					}
				},
				release: func() error {
					released = true
					if scenario == "release failed" {
						return failure
					}
					if scenario == "cancelled during release" {
						cancel()
					}
					return nil
				},
			}
			defer func() {
				panicValue := recover()
				if !dragged || !released {
					t.Fatal("held mouse button was stranded")
				}
				if scenario == "panic" && panicValue != failure || scenario != "panic" && panicValue != nil {
					t.Fatalf("unexpected panic: %v", panicValue)
				}
			}()
			err := runNativeDrag(ctx, image.Pt(1, 2), image.Pt(1, 8), input)
			want := failure
			if scenario == "cancelled during smooth" || scenario == "cancelled during release" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
		})
	}
}
