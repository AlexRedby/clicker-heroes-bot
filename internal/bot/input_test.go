package bot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithHeldKeyReleasesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var states []string
	input := heroInput{keyToggle: func(_, state string) error {
		states = append(states, state)
		if state == "down" {
			cancel()
		}
		return nil
	}}
	err := withHeldKey(ctx, input, "v", time.Second, func() error { return errors.New("action must not run") })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("withHeldKey error = %v, want context cancellation", err)
	}
	if len(states) != 2 || states[0] != "down" || states[1] != "up" {
		t.Fatalf("key states = %v, want [down up]", states)
	}
}
