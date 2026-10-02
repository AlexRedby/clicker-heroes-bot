package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"clicker-heroes-bot/internal/ancientcalc"
)

func previewTranscension(ctx context.Context, savePath, output string, stdout io.Writer) error {
	if savePath == "" {
		return errors.New("-save is required")
	}
	file, err := os.Open(savePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > ancientcalc.MaxSaveInput {
		return errors.New("save must be a nonempty regular file no larger than 4 MiB")
	}
	exported, err := io.ReadAll(io.LimitReader(file, ancientcalc.MaxSaveInput+1))
	if err != nil {
		return err
	}
	preview, err := ancientcalc.PreviewTranscension(ctx, exported)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if output == "" {
		_, err = stdout.Write(data)
		return err
	}
	source, err := filepath.Abs(savePath)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	targetInfo, _ := os.Stat(target)
	if source == target || (targetInfo != nil && os.SameFile(info, targetInfo)) {
		return errors.New("preview output must not overwrite the exported save")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}
