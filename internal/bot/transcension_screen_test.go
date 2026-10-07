package bot

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"testing"
)

func TestTranscensionNativeConfirmation(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "transcension-confirm.png", width)
			if !transcensionDialog(screen) || outsiderTabSelected(screen) {
				t.Fatal("confirmation/background isolation failed")
			}
			out, err := readTranscensionObservation(context.Background(), gameFrame{image: screen})
			if err != nil || !out.known || !out.confirm || !out.no || !out.respecKnown || out.respec || out.reward != 79 {
				t.Fatalf("native confirmation: %+v %v", out, err)
			}
		})
	}
	for _, name := range []string{"outsiders-top.png", "outsiders-bottom.png", "ascension-confirm.png", "save-menu.png", "ancient-quantity-filled.png"} {
		if transcensionDialog(exportFixture(t, name, 1280)) {
			t.Fatal("unrelated state recognized", name)
		}
	}
	source := exportFixture(t, "transcension-confirm.png", 1280)
	for _, r := range []image.Rectangle{image.Rect(529, 585, 568, 620), image.Rect(729, 192, 929, 216), image.Rect(413, 520, 518, 561)} {
		screen := image.NewRGBA(source.Bounds())
		draw.Draw(screen, screen.Bounds(), source, source.Bounds().Min, draw.Src)
		draw.Draw(screen, r, image.NewUniform(color.Black), image.Point{}, draw.Src)
		out, _ := readTranscensionObservation(context.Background(), gameFrame{image: screen})
		if out.known {
			t.Fatal("unreadable control authorized a reset", r)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readTranscensionObservation(ctx, gameFrame{image: source}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled read", err)
	}
}

func TestOutsiderNativeBottomAndUnaffordableFeed(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "outsiders-bottom.png", width)
			ui, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: gameContext{outsiders: true}})
			want := []outsiderScreenRow{{"Orphalas", 3, 4}, {"Sen-Akhan", 3, 4}}
			if err != nil || !ui.known || ui.gain != 79 || ui.wallet != 0 || ui.power != 4.04 || ui.nextAS != "1.583e77" || ui.quantity != "x1" || !reflect.DeepEqual(ui.rows, want) {
				t.Fatalf("native bottom: %+v %v", ui, err)
			}
			for _, row := range want {
				if _, ok := outsiderFeedPoint(ui, row.name); ok {
					t.Fatal("wallet0 supplied a FEED action")
				}
			}
			if _, found, _ := transcensionControl(screen, transcensionOpen); found {
				t.Fatal("bottom roster invented the offscreen Transcend button")
			}
		})
	}
	screen := exportFixture(t, "outsiders-top.png", 1280)
	if _, found, err := transcensionControl(screen, transcensionOpen); err != nil || !found {
		t.Fatal("native Transcend control", err)
	}
}
