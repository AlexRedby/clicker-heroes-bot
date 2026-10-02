package main

import (
	"clicker-heroes-bot/internal/ancientcalc"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadFreshExportIgnoresStaleAndFindsNew(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "clickerHeroSave-old.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, time.Now().Add(24*time.Hour), time.Now().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotExports(dir)
	if err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(dir, "clickerHeroSave-new.txt")
	want := []byte("new synthetic export")
	if err := os.WriteFile(newPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, path, err := readFreshExport(ctx, dir, before)
	if err != nil || path != newPath || string(got) != string(want) {
		t.Fatalf("got path=%q data=%q err=%v", path, got, err)
	}
}

func TestReadFreshExportChangedAndFiltersNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotExports(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, gotPath, err := readFreshExport(ctx, dir, before)
	if err != nil || gotPath != path || string(got) != "changed" {
		t.Fatalf("got path=%q data=%q err=%v", gotPath, got, err)
	}
}

func TestReadFreshExportRejectsOversizeAndCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave-large.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", ancientcalc.MaxSaveInput+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := readFreshExport(ctx, dir, nil); !errors.Is(err, errExportTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, _, err := readFreshExport(cancelled, dir, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestReadFreshExportDoesNotReuseUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "clickerHeroSave-old.txt"), []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotExports(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if data, path, err := readFreshExport(ctx, dir, before); !errors.Is(err, context.DeadlineExceeded) || data != nil || path != "" {
		t.Fatalf("reused stale export: %q %v", path, err)
	}
}
