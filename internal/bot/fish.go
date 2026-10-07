package bot

import (
	"image"
	"time"
)

type fishClickTracker struct {
	last        image.Point
	clicked     bool
	misses      int
	lastClickAt time.Time
}

func (tracker *fishClickTracker) shouldClick(point image.Point, found bool) bool {
	if !found {
		tracker.misses++
		if tracker.misses >= 3 {
			tracker.clicked = false
		}
		return false
	}
	tracker.misses = 0
	// A fish rotates and detection jitters; a nearby match is still the same fish.
	delta := point.Sub(tracker.last)
	// Retry only a fish still recognized on a fresh frame; the game may miss input.
	return !tracker.clicked || delta.X*delta.X+delta.Y*delta.Y > 150*150 || time.Since(tracker.lastClickAt) >= 5*time.Second
}

func (tracker *fishClickTracker) recordClick(point image.Point) {
	tracker.last = point
	tracker.lastClickAt = time.Now()
	tracker.clicked = true
	tracker.misses = 0
}
