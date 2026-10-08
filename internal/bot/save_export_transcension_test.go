package bot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func prestigeExportFixture(t *testing.T, restored bool) []byte {
	t.Helper()
	ancients := map[string]any{}
	payload := map[string]any{
		"uniqueId": "synthetic-export-worker", "version": 7, "readPatchNumber": "1.0e12-6144",
		"transcendent": true, "numberOfTranscensions": 1, "numWorldResets": 0,
		"numAscensionsThisTranscension": 0, "currentZoneHeight": 1, "highestFinishedZonePersist": 300,
		"ancientSouls": 20, "ancientSoulsTotal": 20,
		"heroSouls": "0", "heroSoulsSacrificed": "0", "primalSouls": "0", "totalHeroLevels": 0,
		"ancients": map[string]any{"ancients": ancients}, "outsiders": map[string]any{"outsiders": map[string]any{}},
		"items": map[string]any{"equipmentSlots": 4, "items": map[string]any{}, "slots": map[string]any{}},
		"stats": map[string]any{
			"currentAscension":    map[string]any{"id": 0, "transcensionId": 1},
			"currentTranscension": map[string]any{"id": 1},
		},
	}
	encode := func() []byte {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return []byte(base64.StdEncoding.EncodeToString(data))
	}
	if restored {
		s, err := transcension.ReadSnapshot(context.Background(), encode(), time.Now(), 7)
		if err != nil {
			t.Fatal(err)
		}
		missing, err := ancientcalc.MissingActiveAncients(context.Background(), s.State, 1, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range missing {
			ancients[strconv.Itoa(row.ID)] = map[string]any{"level": "1", "spentHeroSouls": "1"}
		}
		payload["heroSouls"] = "1000"
	}
	return encode()
}

func TestPrestigeExportDeliversResetWithoutAncientAllocation(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		prestigeOnly, restoreAllocation bool
		wantAncientError                bool
	}{
		{name: "prestige only", prestigeOnly: true},
		{name: "restoration still missing", prestigeOnly: true, restoreAllocation: true},
		{name: "ordinary calculation cannot block receipt", wantAncientError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "clickerHeroSave-reset.txt")
			data := prestigeExportFixture(t, false)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			started := time.Now()
			result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "invalid", skillRate: 1}, before: make(exportSnapshot), prestige: true, prestigeOnly: tc.prestigeOnly, restoreAllocation: tc.restoreAllocation, generation: 7})
			if err != nil || result.transcensionErr != nil || result.transcension == nil || result.plan != nil || (result.ancientErr != nil) != tc.wantAncientError {
				t.Fatalf("reset export: %+v %v", result, err)
			}
			s := result.transcension
			hash := sha256.Sum256(data)
			if s.Generation != 7 || s.ExportedAt.Before(started) || s.State.SaveHash != hex.EncodeToString(hash[:]) || len(s.State.Ancients) != 0 || result.prestige == nil || result.prestige.SaveHash != s.State.SaveHash || result.relics == nil || result.relicErr != nil {
				t.Fatal("receipt/preview did not share the fresh export", result)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("generated export retained", err)
			}
		})
	}
}

func TestPrestigeExportAllocatesOnlyRestoredSnapshot(t *testing.T) {
	dir := t.TempDir()
	data := prestigeExportFixture(t, true)
	if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-restored.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "0", skillRate: 1}, before: make(exportSnapshot), prestigeOnly: true, restoreAllocation: true, generation: 7})
	if err != nil || result.transcensionErr != nil || result.ancientErr != nil || result.transcension == nil || result.plan == nil || result.plan.SaveHash != result.transcension.State.SaveHash || result.prestige.SaveHash != result.plan.SaveHash {
		t.Fatalf("restored snapshot allocation: %+v %v", result, err)
	}
}

func TestPrestigeExportWaitsForCompleteSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave-partial.txt")
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	data := prestigeExportFixture(t, false)
	written := make(chan error, 1)
	go func() {
		time.Sleep(450 * time.Millisecond)
		written <- os.WriteFile(path, data, 0600)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "invalid"}, before: make(exportSnapshot), prestigeOnly: true, generation: 7})
	if writeErr := <-written; writeErr != nil {
		t.Fatal(writeErr)
	}
	if err != nil || result.transcensionErr != nil || result.transcension == nil || result.plan != nil {
		t.Fatalf("partial snapshot: %+v %v", result, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("completed generated export retained", err)
	}
}

func TestPrestigeExportRejectsUnsupportedStateWithoutDeletingIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave-unsupported.txt")
	data := []byte(base64.StdEncoding.EncodeToString([]byte(`{"heroSouls":0,"heroSoulsSacrificed":0,"highestFinishedZonePersist":300,"ancientSoulsTotal":0,"numWorldResets":0,"transcendent":false,"ancients":{"ancients":{}},"outsiders":{"outsiders":{}}}`)))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "invalid"}, before: make(exportSnapshot), prestigeOnly: true, restoreAllocation: true, generation: 7})
	if !errors.Is(err, context.DeadlineExceeded) || result.transcension != nil || result.plan != nil {
		t.Fatalf("unsupported snapshot: %+v %v", result, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("unsupported export removed", err)
	}
}
