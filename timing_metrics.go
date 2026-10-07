package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type timingCollectorKey struct{}

func withTimingCollector(ctx context.Context, wait, execution *timingSamples) context.Context {
	return context.WithValue(ctx, timingCollectorKey{}, [2]*timingSamples{wait, execution})
}

func timingCollector(ctx context.Context) (*timingSamples, *timingSamples) {
	v, _ := ctx.Value(timingCollectorKey{}).([2]*timingSamples)
	return v[0], v[1]
}

const timingSampleWindow = 256

// timingSamples keeps total observations while quantiles use only the latest 256 samples.
type timingSamples struct {
	mu    sync.Mutex
	data  [timingSampleWindow]time.Duration
	start int
	size  int
	total uint64
}

func (s *timingSamples) Record(d time.Duration) {
	if d < 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size < timingSampleWindow {
		s.data[(s.start+s.size)%timingSampleWindow] = d
		s.size++
	} else {
		s.data[s.start] = d
		s.start = (s.start + 1) % timingSampleWindow
	}
	s.total++
}

type timingSummary struct {
	Count  uint64
	Window int
	P50    time.Duration
	P95    time.Duration
	Min    time.Duration
	Max    time.Duration
}

func (s *timingSamples) Snapshot() timingSummary {
	s.mu.Lock()
	values := make([]time.Duration, s.size)
	for i := range values {
		values[i] = s.data[(s.start+i)%timingSampleWindow]
	}
	total := s.total
	s.mu.Unlock()

	if len(values) == 0 {
		return timingSummary{Count: total}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	nearestRank := func(percent int) time.Duration {
		rank := (percent*len(values) + 99) / 100
		return values[rank-1]
	}
	return timingSummary{
		Count:  total,
		Window: len(values),
		P50:    nearestRank(50),
		P95:    nearestRank(95),
		Min:    values[0],
		Max:    values[len(values)-1],
	}
}

func (s timingSummary) String() string {
	return fmt.Sprintf("count=%d window=%d p50=%s p95=%s", s.Count, s.Window, s.P50, s.P95)
}

type actionWaitReason uint8

const (
	waitPaused actionWaitReason = iota
	waitInputBusy
	waitSettling
	waitUnknownContext
	waitCoveringModal
	waitAwaitingConfirmation
	waitNoDueAction
	waitReasonCount
)

var waitReasonNames = [...]string{
	"paused",
	"input-busy",
	"settling",
	"unknown-context",
	"covering-modal",
	"awaiting-confirmation",
	"no-due-action",
}

// waitReasonCounters counts timer poll observations; it does not imply duration or causality.
type waitReasonCounters struct {
	mu     sync.Mutex
	counts [waitReasonCount]uint64
}

func (c *waitReasonCounters) Record(reason actionWaitReason) {
	if reason >= waitReasonCount {
		return
	}
	c.mu.Lock()
	c.counts[reason]++
	c.mu.Unlock()
}

func (c *waitReasonCounters) String() string {
	c.mu.Lock()
	counts := c.counts
	c.mu.Unlock()
	parts := make([]string, len(waitReasonNames))
	for i, name := range waitReasonNames {
		parts[i] = fmt.Sprintf("%s=%d", name, counts[i])
	}
	return strings.Join(parts, " ")
}
