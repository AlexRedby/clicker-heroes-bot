package bot

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"clicker-heroes-bot/internal/ancientcalc"
)

func TestTimelapsePreviewIsAdvisoryAndProtectsSave(t *testing.T) {
	reference, err := os.ReadFile("../ancientcalc/testdata/hybrid-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct{ Cases []struct{ Save string } }
	if err := json.Unmarshal(reference, &vectors); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(vectors.Cases[0].Save)
	if err != nil {
		t.Fatal(err)
	}
	var save map[string]any
	if err := json.Unmarshal(raw, &save); err != nil {
		t.Fatal(err)
	}
	save["uniqueId"], save["readPatchNumber"] = "synthetic-preview-profile", "1.0e12-6144"
	save["currentZoneHeight"], save["highestFinishedZonePersist"] = 40, 20000
	save["rubies"], save["autoclickers"] = 3303, 3
	save["numWorldResets"], save["numberOfTranscensions"] = 93, 4
	save["stats"] = map[string]any{"currentAscension": map[string]any{"heroSoulsStart": "2.339e58"}}
	options := ancientcalc.BuildOptions{Mode: ancientcalc.HybridBuild, SkillRate: 1}
	path := filepath.Join(t.TempDir(), "save.txt")
	for _, missingBase := range []bool{false, true} {
		if missingBase {
			delete(save["ancients"].(map[string]any)["ancients"].(map[string]any), "5")
		}
		raw, err := json.Marshal(save)
		if err != nil {
			t.Fatal(err)
		}
		exported := []byte(base64.StdEncoding.EncodeToString(raw))
		if err := os.WriteFile(path, exported, 0600); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if err := previewTimelapse(context.Background(), path, "", &stdout, options); err != nil {
			t.Fatal(err)
		}
		var preview timelapsePreview
		if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if !preview.PreviewOnly || len(preview.Blocked) < 4 || len(preview.Forecast.Rows) == 0 || len(preview.Hybrid.SaveHash) != 64 || len(preview.State.ProfileID) != 64 || bytes.Contains(stdout.Bytes(), []byte("synthetic-preview-profile")) || (preview.HybridError != "") != missingBase {
			t.Fatalf("invalid advisory report: %+v", preview)
		}
		if err := previewTimelapse(context.Background(), path, path, nil, options); err == nil {
			t.Fatal("allowed overwriting the save")
		}
		unchanged, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(exported, unchanged) {
			t.Fatal("preview changed the save")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := previewTimelapse(ctx, path, "", &bytes.Buffer{}, options); err != context.Canceled {
		t.Fatalf("cancellation lost: %v", err)
	}
}
