package bot

import (
	"context"
	"fmt"
	"sync"

	hook "github.com/robotn/gohook"
)

type pauseControl struct {
	mu            sync.Mutex
	paused        bool
	resumeBlocked bool
	pauseReason   string
	generation    uint64
}

func (control *pauseControl) toggle() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused && control.resumeBlocked {
		return true
	}
	control.paused = !control.paused
	control.pauseReason = ""
	if control.paused {
		control.pauseReason = "F8"
	}
	control.generation++
	return control.paused
}

func (control *pauseControl) pause(reason string) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.pauseLocked(reason, false)
}
func (control *pauseControl) block(reason string) {
	control.mu.Lock()
	defer control.mu.Unlock()
	control.pauseLocked(reason, true)
}

// Call only while holding the input/pause mutex.
func (control *pauseControl) pauseLocked(reason string, blocked bool) {
	if control.paused && control.pauseReason == reason && control.resumeBlocked == blocked {
		return
	}
	control.paused = true
	control.pauseReason = reason
	control.resumeBlocked = blocked
	control.generation++
	fmt.Printf("paused: %s\n", reason)
}
func (control *pauseControl) message() string {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused {
		return "paused: " + control.pauseReason
	}
	return "resumed"
}

func (control *pauseControl) snapshot() uint64 {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.generation
}

func (control *pauseControl) valid(ctx context.Context, generation uint64) bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	return !control.paused && ctx.Err() == nil && control.generation == generation
}

func (control *pauseControl) isPaused() bool {
	control.mu.Lock()
	defer control.mu.Unlock()
	return control.paused
}

func (control *pauseControl) runClick(ctx context.Context, generation uint64, action func() error) (bool, error) {
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.paused || ctx.Err() != nil || control.generation != generation {
		return false, nil
	}
	if err := action(); err != nil {
		if ctx.Err() != nil {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isPauseKey(event hook.Event) bool {
	return event.Kind == hook.KeyUp && event.Keycode == hook.Keycode["f8"]
}
