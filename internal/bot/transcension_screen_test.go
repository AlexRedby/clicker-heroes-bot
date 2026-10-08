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
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

func TestTranscensionNativeConfirmation(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "transcension-confirm.png", width)
			if !transcensionDialog(screen) || outsiderTabSelected(screen) {
				t.Fatal("confirmation/background isolation failed")
			}
			frame := gameFrame{id: 1, generation: 1, at: time.Unix(1000, 0), image: screen}
			out, err := readTranscensionObservation(context.Background(), frame)
			if err != nil || !out.known || !out.confirm || !out.no || !out.respecKnown || out.respec || out.reward != 79 {
				t.Fatalf("native confirmation: %+v %v", out, err)
			}
			observed, err := nativeTranscensionObservation(frame, outsiderObservation{}, out, ancientcalc.TranscensionState{})
			if err != nil || observed.Screen != transcension.ConfirmationScreen || !observed.Known || observed.Reward != 79 || observed.Frame != frame.id || observed.Generation != frame.generation {
				t.Fatal("shared-frame confirmation adapter", observed, err)
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

func TestTranscensionLiveAcceptanceIsNotInferred(t *testing.T) {
	n := nativeTranscensionEvidence("1.0e12-6144")
	if n.Build != "1.0e12-6144" || n.ResetRecovery || n.Feed || n.AncientSummon {
		t.Fatal("factory inferred live acceptance from the build", n)
	}
	frame := gameFrame{id: 2, generation: 2}
	o, err := nativeTranscensionObservation(frame, outsiderObservation{}, transcensionObservation{known: true, frame: gameFrame{id: 1, generation: 1}}, ancientcalc.TranscensionState{})
	if err != nil || o.Known {
		t.Fatal("stale analysis supplied an input context", o, err)
	}
}

func TestOutsiderNativeBottomAndUnaffordableFeed(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "outsiders-bottom.png", width)
			ui, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: gameContext{outsiders: true}})
			want := []outsiderScreenRow{{"Orphalas", 3, 4, false}, {"Sen-Akhan", 3, 4, false}}
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

func TestTranscensionModalIsolatesSharedPipeline(t *testing.T) {
	screen := exportFixture(t, "transcension-confirm.png", 1280)
	c, err := recognizedGame(screen)
	if err != nil || !c.known || !c.transcension || c.heroes || c.outsiders || c.ascension || gameScreenVisible(c) || bootstrapHeroes(c) {
		t.Fatalf("Transcension modal leaked game context: %+v %v", c, err)
	}
	now := time.Now()
	clicks := 0
	p := newGamePipeline(&pauseControl{}, heroInput{click: func(image.Point) error { clicks++; return nil }}, pipelineReaders{}, pipelineOptions{heroes: true, monster: true, windowed: true})
	p.frame = gameFrame{id: 2, at: now, image: screen, context: c}
	fish := image.Pt(900, 600)
	p.fishTarget = &fish
	for _, kind := range []actionKind{collectFish, clickMonster, buyHero, handleExport} {
		a := gameAction{kind: kind, frame: p.frame, point: fish, export: &exportCommand{step: exportOpenMenu}}
		p.enqueue(a, now)
		if _, ok := p.nextAction(now); ok {
			t.Fatalf("modal admitted queued action %v", kind)
		}
		if acted, err := p.execute(context.Background(), a); acted || !errors.Is(err, errInputContext) {
			t.Fatalf("modal admitted direct action %v: acted=%t err=%v", kind, acted, err)
		}
	}
	if clicks != 0 || p.fishTarget == nil || *p.fishTarget != fish {
		t.Fatal("covered fish was clicked or its cached position was discarded")
	}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != navigateGame || a.navigation != navigationTranscension {
		t.Fatal("orphan Transcension modal did not select verified No", a, ok)
	}
	if acted, err := p.execute(context.Background(), a); !acted || err != nil || clicks != 1 {
		t.Fatalf("verified cancellation failed: acted=%t err=%v clicks=%d", acted, err, clicks)
	}
}
