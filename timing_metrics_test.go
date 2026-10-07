package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTimingCollectorIsolatedByContext(t *testing.T) {
	var waitA, execA, waitB, execB timingSamples
	ctxA := withTimingCollector(context.Background(), &waitA, &execA)
	ctxB := withTimingCollector(context.Background(), &waitB, &execB)
	wait, execution := timingCollector(ctxA)
	if wait != &waitA || execution != &execA {
		t.Fatalf("context A collector pointers changed")
	}
	wait.Record(time.Millisecond)
	execution.Record(2 * time.Millisecond)
	wait, execution = timingCollector(ctxB)
	if wait != &waitB || execution != &execB {
		t.Fatalf("context B collector pointers changed")
	}
	wait.Record(3 * time.Millisecond)
	execution.Record(4 * time.Millisecond)
	if waitA.Snapshot().Count != 1 || execA.Snapshot().Count != 1 || waitB.Snapshot().Count != 1 || execB.Snapshot().Count != 1 {
		t.Fatalf("timing collectors crossed contexts")
	}
	if wait, execution = timingCollector(context.Background()); wait != nil || execution != nil {
		t.Fatalf("background context unexpectedly has collector")
	}
}

func TestTimingSamplesNearestRankAndRollover(t *testing.T) {
	var samples timingSamples
	for i := 1; i <= timingSampleWindow+4; i++ {
		samples.Record(time.Duration(i) * time.Millisecond)
	}
	got := samples.Snapshot()
	if got.Count != timingSampleWindow+4 || got.Window != timingSampleWindow || got.P50 != 132*time.Millisecond || got.P95 != 248*time.Millisecond || got.Min != 5*time.Millisecond || got.Max != 260*time.Millisecond {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if got.String() != "count=260 window=256 p50=132ms p95=248ms" {
		t.Fatalf("unexpected string: %q", got.String())
	}
}

func TestTimingSamplesEmptyNegativeAndConcurrent(t *testing.T) {
	var samples timingSamples
	samples.Record(-time.Second)
	if got := samples.Snapshot(); got != (timingSummary{}) {
		t.Fatalf("negative duration was recorded: %+v", got)
	}
	var counters waitReasonCounters
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				samples.Record(time.Duration(j) * time.Millisecond)
				counters.Record(waitPaused)
				_ = samples.Snapshot()
				_ = counters.String()
			}
		}()
	}
	wg.Wait()
	if got := samples.Snapshot(); got.Count != 800 || got.Window != 256 {
		t.Fatalf("unexpected concurrent summary: %+v", got)
	}
	text := counters.String()
	if !strings.HasPrefix(text, "paused=800 input-busy=0 settling=0") {
		t.Fatalf("unstable or incorrect counter string: %q", text)
	}
}
