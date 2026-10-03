package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeMercenaryTesseract(t *testing.T, output string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix only")
	}
	path := filepath.Join(t.TempDir(), "tesseract")
	// The TSV is deliberately emitted verbatim. In particular, keep carriage
	// returns in the OCR word text: they are part of the regression input.
	script := "#!/bin/sh\nprintf '%s' '" + strings.ReplaceAll(output, "'", "'\\''") + "'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	original := tesseractExecutable
	tesseractExecutable = path
	t.Cleanup(func() { tesseractExecutable = original })
}

func mercenaryTSVLine(y, height int, text string) string {
	return fmt.Sprintf("5\t1\t1\t1\t2\t2\t202\t%d\t290\t%d\t93.575455\t%s\n", y, height, text)
}

func mercenaryFixtureFrame(t *testing.T) (gameFrame, []image.Point) {
	t.Helper()
	im := loadTestImage(t, "testdata/mercenary-collect.png")
	rows := mercenaryRows(im)
	if len(rows) != 4 {
		t.Fatalf("fixture has %d mercenary rows, want 4", len(rows))
	}
	return gameFrame{image: im, context: gameContext{known: true, mercenaries: true, bounds: im.Bounds()}}, rows
}

func TestReadMercenaryTimerTSVPositionAndTrailingColon(t *testing.T) {
	frame, rows := mercenaryFixtureFrame(t)
	const exactTimerLine = "5\t1\t1\t1\t2\t2\t202\t148\t290\t112\t93.575455\t36:14:32\r\n"
	for name, timerLine := range map[string]string{
		"carriage-return": exactTimerLine,
		"trailing-colon":  mercenaryTSVLine(206, 20, "03:21:22\r :"),
	} {
		t.Run(name, func(t *testing.T) {
			output := timerLine + mercenaryTSVLine(20, 20, "Collect") + mercenaryTSVLine(332, 20, "Start Quest") + mercenaryTSVLine(488, 20, "Start Quest")
			fakeMercenaryTesseract(t, output)
			o, err := readMercenaryObservation(context.Background(), frame)
			if err != nil {
				t.Fatal(err)
			}
			if !o.readable || len(o.running) != 1 || len(o.collect) != 1 || len(o.start) != 2 {
				t.Fatalf("observation=%+v", o)
			}
			if o.running[0] != rows[1] {
				t.Fatalf("timer shifted to row %v, want %v", o.running[0], rows[1])
			}
			if o.collect[0] != rows[0] || o.start[0] != rows[2] || o.start[1] != rows[3] {
				t.Fatalf("adjacent labels shifted: collect=%v start=%v rows=%v", o.collect, o.start, rows)
			}
		})
	}
}

func TestReadMercenaryRejectsMalformedOrAmbiguousTimerTSV(t *testing.T) {
	frame, _ := mercenaryFixtureFrame(t)
	for name, timerLine := range map[string]string{
		"malformed-seconds": mercenaryTSVLine(206, 20, "03:21:99"),
		"midpoint-gap":      mercenaryTSVLine(154, 20, "03:21:22"),
		"oversized-word":    mercenaryTSVLine(148, 200, "36:14:32"),
		"text-garbage":      mercenaryTSVLine(206, 20, "03:21:22 garbage"),
	} {
		t.Run(name, func(t *testing.T) {
			output := timerLine + mercenaryTSVLine(20, 20, "Collect") + mercenaryTSVLine(332, 20, "Start Quest") + mercenaryTSVLine(488, 20, "Start Quest")
			fakeMercenaryTesseract(t, output)
			if o, err := readMercenaryObservation(context.Background(), frame); err == nil || o.readable {
				t.Fatalf("accepted invalid OCR: observation=%+v err=%v", o, err)
			}
		})
	}
}
