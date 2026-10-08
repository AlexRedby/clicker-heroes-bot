package bot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGildExportReadsCurrentDistributionWithoutAncientAllocation(t *testing.T) {
	dir := t.TempDir()
	data := testIntegratedGildSave(t, true)
	path := filepath.Join(dir, "clickerHeroSave-gilds.txt")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "0"}, before: make(exportSnapshot), gildsOnly: true})
	if err != nil || result.gildErr != nil || result.gilds == nil || result.plan != nil || result.ancientErr != nil {
		t.Fatalf("gild-only export: %+v, %v", result, err)
	}
	if result.gilds.MoveGilds != 2 || result.gilds.SaveHash != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("distribution was not bound to this export", result.gilds)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("generated export was retained", err)
	}
}
