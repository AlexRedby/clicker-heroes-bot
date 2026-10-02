package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clicker-heroes-bot/internal/ancientcalc"
)

func TestInvalidSaveCannotProduceRelicPreview(t *testing.T) {
	file := filepath.Join(t.TempDir(), "save.txt")
	if err := os.WriteFile(file, []byte("not a save"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", file, t.TempDir()} {
		var output bytes.Buffer
		if err := previewRelics(context.Background(), path, &output); err == nil || output.Len() != 0 {
			t.Fatalf("invalid input produced output: %v, %q", err, output.String())
		}
	}
	if err := os.Truncate(file, ancientcalc.MaxSaveInput+1); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := previewRelics(context.Background(), file, &output); err == nil || output.Len() != 0 {
		t.Fatal("oversized save produced a preview")
	}
}

func TestRelicReportIsConciseAndAdvisory(t *testing.T) {
	preview := &ancientcalc.RelicPreview{
		Readiness:  "unknown",
		Snapshot:   ancientcalc.RelicSnapshot{Items: []ancientcalc.Relic{{UID: 8, Bonuses: []ancientcalc.RelicBonus{{Type: 2, Level: "30"}}}}},
		Suggestion: &ancientcalc.RelicSuggestion{UID: 8, Slot: 1, ReplaceUID: 7},
	}
	line := relicReport(preview, nil)
	for _, want := range []string{"relics:", "UID 8", "slot 1", "UID 7", "click damage bonus level=30", "apply manually"} {
		if !strings.Contains(line, want) {
			t.Fatalf("report %q misses %q", line, want)
		}
	}
	if line := relicReport(&ancientcalc.RelicPreview{UnsupportedTypes: []int{23}}, nil); !strings.Contains(line, "manual review") || strings.Contains(line, "ready") {
		t.Fatalf("unsupported report = %q", line)
	}
	if line := relicReport(&ancientcalc.RelicPreview{}, nil); !strings.Contains(line, "no clear upgrade") || strings.Contains(line, "ready") {
		t.Fatalf("no-upgrade report = %q", line)
	}
	if line := relicReport(nil, os.ErrNotExist); !strings.Contains(line, "unknown/unreadable") || strings.Contains(line, "ready") {
		t.Fatalf("error report = %q", line)
	}
}
