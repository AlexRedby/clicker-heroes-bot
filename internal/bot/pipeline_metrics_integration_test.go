package bot

import (
	"context"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWaitReasonPrecedence(t *testing.T) {
	p := gamePipeline{settleUntil: time.Now().Add(time.Second)}
	if got := p.waitReason(time.Now()); got != waitSettling {
		t.Fatalf("settling reason = %d", got)
	}
	p.settleUntil = time.Time{}
	if got := p.waitReason(time.Now()); got != waitUnknownContext {
		t.Fatalf("unknown context reason = %d", got)
	}
	p.frame.context.known = true
	p.frame.context.questDialog = true
	if got := p.waitReason(time.Now()); got != waitCoveringModal {
		t.Fatalf("covering dialog reason = %d", got)
	}
	p.frame.context.questDialog = false
	p.hero.pending = &heroAttempt{}
	if got := p.waitReason(time.Now()); got != waitAwaitingConfirmation {
		t.Fatalf("hero confirmation reason = %d", got)
	}
	p.hero.pending = nil
	if got := p.waitReason(time.Now()); got != waitNoDueAction {
		t.Fatalf("idle reason = %d", got)
	}
}

func TestReplaceJobRefreshesQueueTimestamp(t *testing.T) {
	jobs := make(chan analysisJob, 1)
	stale := analysisJob{frame: gameFrame{id: 1}, queuedAt: time.Now().Add(-time.Hour)}
	jobs <- stale
	before := time.Now()
	replaceJob(jobs, analysisJob{frame: gameFrame{id: 2}})
	second := <-jobs
	if second.frame.id != 2 || second.queuedAt.Before(before) {
		t.Fatalf("replacement did not refresh queue timestamp: %+v", second)
	}
}

func TestOCRTimingCollector(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	var wait, execution timingSamples
	ctx := withTimingCollector(context.Background(), &wait, &execution)
	gold, err := readHeroGold(ctx, loadTestImage(t, "../../testdata/no-fish-game-screen.jpg"))
	if err != nil || math.Abs(gold-71.1000258) > 0.001 {
		t.Fatalf("gold=%v error=%v", gold, err)
	}
	if wait.Snapshot().Count == 0 || execution.Snapshot().Count == 0 {
		t.Fatalf("OCR timing samples missing: wait=%+v execution=%+v", wait.Snapshot(), execution.Snapshot())
	}
}

func TestOCRCanceledSemaphoreWaitIsRecorded(t *testing.T) {
	ocrSlots <- struct{}{}
	ocrSlots <- struct{}{}
	defer func() { <-ocrSlots; <-ocrSlots }()
	var wait, execution timingSamples
	ctx, cancel := context.WithTimeout(withTimingCollector(context.Background(), &wait, &execution), 20*time.Millisecond)
	defer cancel()
	_, err := runTesseract(ctx, nil)
	if err == nil || wait.Snapshot().Count != 1 || execution.Snapshot().Count != 0 {
		t.Fatalf("canceled OCR wait samples: err=%v wait=%+v execution=%+v", err, wait.Snapshot(), execution.Snapshot())
	}
}
