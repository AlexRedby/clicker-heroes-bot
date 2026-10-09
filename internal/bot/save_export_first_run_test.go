package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitialExportPreservesFirstRunWithoutTranscensionOption(t *testing.T) {
	dir := t.TempDir()
	data := prestigeExportFixture(t, false)
	if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-first-run.txt"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, initialSetup: true, before: make(exportSnapshot), options: saveExportOptions{dir: dir, reserve: "0", skillRate: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if result.plan != nil || result.ancientErr == nil || !strings.Contains(result.ancientErr.Error(), "Fragsworth must be owned") {
		t.Fatalf("missing Ancient did not remain an independent startup error: %+v", result)
	}
	hash := sha256.Sum256(data)
	if result.transcension != nil || result.prestige == nil || result.prestige.SaveHash != hex.EncodeToString(hash[:]) || !result.prestige.Transcendent || result.prestige.Ascensions != 0 {
		t.Fatalf("ordinary initial export lost fresh first-run metadata: %+v", result)
	}
	p := ascensionPlanner{}
	p.observeSave(result)
	if !p.firstRun {
		t.Fatal("ordinary fresh export did not establish first-run Ascension")
	}
}
