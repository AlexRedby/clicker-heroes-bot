package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

type exportFileStamp struct {
	size     int64
	modified time.Time
}

type exportSnapshot map[string]exportFileStamp

var (
	errExportTooLarge = errors.New("exported save exceeds 4 MiB")
	errExportUnstable = errors.New("exported save is still changing")
)

func snapshotExports(dir string) (exportSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(exportSnapshot)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(entry.Name(), "clickerHeroSave") || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		out[path] = exportFileStamp{size: info.Size(), modified: info.ModTime()}
	}
	return out, nil
}

func readFreshExport(ctx context.Context, dir string, before exportSnapshot) ([]byte, string, error) {
	seen := make(exportSnapshot)
	stable := make(map[string]int)
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		current, err := snapshotExports(dir)
		if err != nil {
			return nil, "", err
		}
		for path := range stable {
			if _, ok := current[path]; !ok {
				delete(stable, path)
				delete(seen, path)
			}
		}
		candidates := make([]string, 0, len(current))
		for path, stamp := range current {
			if old, ok := before[path]; ok && old == stamp {
				continue
			}
			if old, ok := seen[path]; ok && old == stamp {
				stable[path]++
			} else {
				seen[path] = stamp
				stable[path] = 1
			}
			if stable[path] >= 2 && stamp.size > 0 {
				candidates = append(candidates, path)
			}
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := current[candidates[i]], current[candidates[j]]
			if a.modified.Equal(b.modified) {
				return candidates[i] > candidates[j]
			}
			return a.modified.After(b.modified)
		})
		for _, path := range candidates {
			data, err := readStableExport(path, current[path])
			if err == nil {
				return data, path, nil
			}
			if errors.Is(err, errExportTooLarge) {
				return nil, "", err
			}
			// A file that changed while being read gets another stability window.
			stable[path] = 0
			break
		}
		wait := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !wait.Stop() {
				<-wait.C
			}
			return nil, "", ctx.Err()
		case <-wait.C:
		}
	}
}

func readStableExport(path string, expected exportFileStamp) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errExportUnstable
	}
	initial := exportFileStamp{size: info.Size(), modified: info.ModTime()}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || initial != expected {
		return nil, errExportUnstable
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errExportUnstable
	}
	data, readErr := io.ReadAll(io.LimitReader(file, ancientcalc.MaxSaveInput+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, errExportUnstable
	}
	if len(data) > ancientcalc.MaxSaveInput {
		return nil, errExportTooLarge
	}
	final, err := os.Lstat(path)
	if err != nil {
		return nil, errExportUnstable
	}
	last := exportFileStamp{size: final.Size(), modified: final.ModTime()}
	if !final.Mode().IsRegular() || final.Mode()&os.ModeSymlink != 0 || last != expected || final.Size() != int64(len(data)) {
		return nil, errExportUnstable
	}
	return data, nil
}
