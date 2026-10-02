package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewOutputPreservesSaveAliases(t *testing.T) {
	// Use the sanitized prestige fixture; no account data is needed.
	save := filepath.Join(t.TempDir(), "save.txt")
	original, err := os.ReadFile("../../internal/ancientcalc/testdata/prestige-save.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(save, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"source", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			out := save
			if kind != "source" {
				out = filepath.Join(filepath.Dir(save), kind)
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(save, out); err != nil {
					t.Skipf("%s unsupported: %v", kind, err)
				}
			}
			if err := run(context.Background(), save, out, nil); err == nil {
				t.Fatalf("accepted output %s", kind)
			}
			unchanged, err := os.ReadFile(save)
			if err != nil || string(unchanged) != string(original) {
				t.Fatal("source save changed")
			}
		})
	}
	out := filepath.Join(filepath.Dir(save), "preview.json")
	if err := run(context.Background(), save, out, nil); err != nil {
		t.Fatal(err)
	}
	if result, err := os.ReadFile(out); err != nil || len(result) == 0 {
		t.Fatal("missing preview")
	}
	var stdout bytes.Buffer
	if err := run(context.Background(), save, "", &stdout); err != nil {
		t.Fatal(err)
	}
	var preview map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil || preview["uiVerified"] != false {
		t.Fatal("invalid stdout preview")
	}
}
