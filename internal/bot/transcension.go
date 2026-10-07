package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"

	"clicker-heroes-bot/internal/ancientcalc"
)

func previewTranscension(ctx context.Context, savePath, output string, stdout io.Writer) error {
	preview, err := readTranscensionPreview(ctx, savePath)
	if err != nil {
		return err
	}
	return writeReadOnlyPreview(ctx, preview, output, stdout, []string{savePath})
}

func readTranscensionPreview(ctx context.Context, savePath string) (ancientcalc.TranscensionPreview, error) {
	exported, err := readPreviewSave(savePath)
	if err != nil {
		return ancientcalc.TranscensionPreview{}, err
	}
	return ancientcalc.PreviewTranscension(ctx, exported)
}

func readPreviewSave(savePath string) ([]byte, error) {
	if savePath == "" {
		return nil, errors.New("-save is required")
	}
	file, err := os.Open(savePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > ancientcalc.MaxSaveInput {
		return nil, errors.New("save must be a nonempty regular file no larger than 4 MiB")
	}
	exported, err := io.ReadAll(io.LimitReader(file, ancientcalc.MaxSaveInput+1))
	if err != nil {
		return nil, err
	}
	if len(exported) > ancientcalc.MaxSaveInput {
		return nil, errors.New("save exceeds 4 MiB")
	}
	return exported, nil
}

func previewOutsiders(ctx context.Context, savePath, screenshotPath, output string, stdout io.Writer) error {
	preview, err := readTranscensionPreview(ctx, savePath)
	if err != nil {
		return err
	}
	screen, err := readOutsiderScreenshot(screenshotPath)
	if err != nil {
		return err
	}
	c, err := recognizedGame(screen)
	if err != nil {
		return err
	}
	if !c.outsiders {
		return errors.New("screenshot is not a supported active Outsiders tab")
	}
	ui, err := readOutsiderObservation(ctx, gameFrame{image: screen, context: c})
	if err != nil {
		return err
	}
	advice := reconcileOutsiders(ctx, ui, &preview)
	return writeReadOnlyPreview(ctx, advice, output, stdout, []string{savePath, screenshotPath})
}

func readOutsiderScreenshot(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const limit = 32 << 20
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > limit {
		return nil, errors.New("screenshot must be a nonempty regular file no larger than 32 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read bounded screenshot: %w", err)
	}
	if len(data) > limit {
		return nil, errors.New("screenshot exceeds 32 MiB")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width < 640 || config.Height < 360 || config.Width > 8192 || config.Height > 8192 || config.Width*config.Height > 16<<20 {
		return nil, errors.New("unsupported screenshot dimensions")
	}
	screen, _, err := image.Decode(bytes.NewReader(data))
	return screen, err
}

func writeReadOnlyPreview(ctx context.Context, preview any, output string, stdout io.Writer, inputs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(preview, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if output == "" {
		if stdout == nil {
			return errors.New("preview output writer is required")
		}
		_, err = stdout.Write(data)
		return err
	}
	target, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	targetInfo, _ := os.Stat(target)
	for _, input := range inputs {
		source, err := filepath.Abs(input)
		if err != nil {
			return err
		}
		info, err := os.Stat(source)
		if err != nil {
			return err
		}
		if source == target || (targetInfo != nil && os.SameFile(info, targetInfo)) {
			return errors.New("preview output must not overwrite an input file")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}
