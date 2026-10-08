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
)

func TestOutsiderRealScreen(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "outsiders-top.png", width)
			c, err := recognizedGame(screen)
			if err != nil || !c.known || !c.outsiders || c.heroes || c.ancients || c.saveMenu {
				t.Fatalf("classification: %+v %v", c, err)
			}
			out, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: c})
			if err != nil {
				t.Fatal(err)
			}
			if !out.known || out.wallet != 0 || out.gain != 20 || out.power != 4.04 || out.sacrificed != "1.227e62" || out.nextAS != "9.291e65" || out.quantity != "x1" {
				t.Fatalf("header: %+v", out)
			}
			want := []outsiderScreenRow{{"Xyliqil", 0, 1, false}, {"Chor'gorloth", 6, 7, false}}
			if !reflect.DeepEqual(out.rows, want) {
				t.Fatalf("rows: %+v want %+v", out.rows, want)
			}
		})
	}
}

func TestOutsiderPostTranscensionScreen(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			screen := exportFixture(t, "outsiders-post-transcension.png", width)
			c, err := recognizedGame(screen)
			if err != nil || !c.known || !c.outsiders || c.transcension {
				t.Fatalf("post-reset classification: %+v %v", c, err)
			}
			out, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: c})
			if err != nil || !out.known || out.wallet != 75 || out.gain != 0 || out.power != 4.55 || out.quantity != "x1" || out.sacrificed != "2.789e78" || out.nextAS != "1.191e78" {
				t.Fatalf("post-reset header: %+v %v", out, err)
			}
			want := []outsiderScreenRow{{"Xyliqil", 0, 1, true}, {"Chor'gorloth", 7, 8, true}}
			if !reflect.DeepEqual(out.rows, want) {
				t.Fatalf("post-reset rows: %+v want %+v", out.rows, want)
			}
		})
	}
}

func TestOutsiderRejectsOtherTabsAndUnreadableFields(t *testing.T) {
	requireAncientOCR(t)
	for _, name := range []string{"hero-economy-x1.png", "ascension-ancients.png", "save-menu.png", "ancient-quantity-filled.png"} {
		if outsiderTabSelected(exportFixture(t, name, 1280)) {
			t.Fatal("unrelated screen recognized", name)
		}
	}
	for _, region := range []image.Rectangle{
		image.Rect(470, 180, 605, 198), image.Rect(450, 200, 600, 220),
		image.Rect(292, 180, 382, 198), image.Rect(442, 420, 520, 453), image.Rect(480, 502, 530, 525),
		image.Rect(35, 380, 562, 713),
	} {
		source := exportFixture(t, "outsiders-top.png", 1280)
		screen := image.NewRGBA(source.Bounds())
		draw.Draw(screen, screen.Bounds(), source, source.Bounds().Min, draw.Src)
		draw.Draw(screen, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
		out, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: gameContext{outsiders: true}})
		if out.known {
			t.Fatalf("unreadable %v became known: %+v %v", region, out, err)
		}
	}
	source := exportFixture(t, "outsiders-top.png", 1280)
	dimmed := image.NewRGBA(source.Bounds())
	for y := 0; y < dimmed.Bounds().Dy(); y++ {
		for x := 0; x < dimmed.Bounds().Dx(); x++ {
			r, g, b := rgb(source.At(x, y))
			dimmed.SetRGBA(x, y, color.RGBA{uint8(r / 2), uint8(g / 2), uint8(b / 2), 255})
		}
	}
	if outsiderTabSelected(dimmed) {
		t.Fatal("dimmed modal background recognized as active Outsiders")
	}
	screen := exportFixture(t, "outsiders-top.png", 1280)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := readOutsiderObservation(ctx, gameFrame{image: screen, context: gameContext{outsiders: true}})
	if !errors.Is(err, context.Canceled) || out.known {
		t.Fatalf("cancellation: %+v %v", out, err)
	}
}

func TestOutsiderQuantityMetadata(t *testing.T) {
	source := exportFixture(t, "outsiders-top.png", 1280)
	screen := image.NewRGBA(source.Bounds())
	draw.Draw(screen, screen.Bounds(), source, source.Bounds().Min, draw.Src)
	orange := image.NewUniform(color.RGBA{238, 149, 32, 255})
	gold := image.NewUniform(color.RGBA{255, 220, 43, 255})
	draw.Draw(screen, image.Rect(122, 262, 130, 280), gold, image.Point{}, draw.Src)
	draw.Draw(screen, image.Rect(220, 262, 228, 280), orange, image.Point{}, draw.Src)
	if outsiderQuantity(screen) != "x10" {
		t.Fatal("bulk FEED cost would be reported as x1")
	}
	draw.Draw(screen, image.Rect(122, 262, 130, 280), orange, image.Point{}, draw.Src)
	if outsiderQuantity(screen) != "" {
		t.Fatal("ambiguous quantity was accepted")
	}
}

func TestOutsiderScrolledAndClippedCards(t *testing.T) {
	requireAncientOCR(t)
	source := exportFixture(t, "outsiders-top.png", 1280)
	screen := image.NewRGBA(source.Bounds())
	draw.Draw(screen, screen.Bounds(), source, source.Bounds().Min, draw.Src)
	// Shift the list up by 80 reference pixels. The first card is clipped, the
	// second remains readable at its new position; the fixed header is unchanged.
	draw.Draw(screen, image.Rect(35, 380, 562, 713), source, image.Pt(35, 460), draw.Src)
	out, err := readOutsiderObservation(context.Background(), gameFrame{image: screen, context: gameContext{outsiders: true}})
	if err != nil || !out.known || !reflect.DeepEqual(out.rows, []outsiderScreenRow{{"Chor'gorloth", 6, 7, false}}) {
		t.Fatalf("scrolled/clipped: %+v %v", out, err)
	}
}

func TestOutsiderIntegersRejectAmbiguity(t *testing.T) {
	for _, raw := range []string{"", "O", "-1", "1e6", "01", "1,23", "1.0", "9007199254740992"} {
		if _, err := parseOutsiderInteger(raw); err == nil {
			t.Fatal("accepted ambiguous integer", raw)
		}
	}
	for raw, want := range map[string]int{"0": 0, "7": 7, "1,234": 1234} {
		if got, err := parseOutsiderInteger(raw); err != nil || got != want {
			t.Fatalf("%s: %d %v", raw, got, err)
		}
	}
}

func TestOutsiderPipelineReadOnlyAndContextOwnership(t *testing.T) {
	ctx := context.Background()
	frame := testPipelineFrame()
	frame.context.outsiders = true
	current := frame.context
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }}, pipelineReaders{
		context: func(image.Image) (gameContext, error) { return current, nil },
		window:  func() string { return current.window },
		outsiders: func(_ context.Context, f gameFrame) (outsiderObservation, error) {
			return outsiderObservation{frame: f, known: true, wallet: 0, gain: 20, power: 4.04, quantity: "x1"}, nil
		},
	}, pipelineOptions{fishInterval: time.Second})
	jobs := make([]chan analysisJob, analysisCount)
	for i := range jobs {
		jobs[i] = make(chan analysisJob, 1)
	}
	now := time.Now()
	if err := p.capture(ctx, now, jobs); err != nil {
		t.Fatal(err)
	}
	job := <-jobs[outsiderAnalysis]
	if job.frame.id != p.frame.id || job.frame.image != p.frame.image {
		t.Fatal("Outsiders did not share the capture")
	}
	if err := p.capture(ctx, now.Add(6*time.Second), jobs); err != nil || len(jobs[outsiderAnalysis]) != 0 {
		t.Fatal("duplicate in-flight OCR", err)
	}
	out := p.analyze(ctx, outsiderAnalysis, job)
	if err := p.accept(ctx, out, now); err != nil || !p.state[outsiderAnalysis].outsider.known || len(p.queue) != 0 {
		t.Fatal("read-only observation", err)
	}
	if err := p.capture(ctx, now.Add(7*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[outsiderAnalysis]
	// An OCR failure invalidates the previous values without stopping the bot.
	if err := p.accept(ctx, observation{kind: outsiderAnalysis, frame: job.frame, err: errors.New("clipped cost")}, now); err != nil || p.state[outsiderAnalysis].outsider.known {
		t.Fatal("stale values survived failed OCR", err)
	}
	current.outsiders, current.heroes = false, true
	if err := p.capture(ctx, now.Add(8*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if err := p.accept(ctx, p.analyze(ctx, outsiderAnalysis, job), now); err != nil || p.state[outsiderAnalysis].outsider.known || len(p.queue) != 0 {
		t.Fatal("stale tab result accepted", err)
	}
	current.outsiders, current.heroes = true, false
	if err := p.capture(ctx, now.Add(9*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[outsiderAnalysis]
	current.window = "!outside-game"
	if err := p.capture(ctx, now.Add(10*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if err := p.accept(ctx, p.analyze(ctx, outsiderAnalysis, job), now); err != nil || p.state[outsiderAnalysis].outsider.known || len(jobs[outsiderAnalysis]) != 0 {
		t.Fatal("foreground context loss accepted or scheduled Outsider OCR", err)
	}
	current.window = "game"
	if err := p.capture(ctx, now.Add(11*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[outsiderAnalysis]
	current.modal = unknownGildModal
	if err := p.capture(ctx, now.Add(12*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	if err := p.accept(ctx, p.analyze(ctx, outsiderAnalysis, job), now); err != nil || p.state[outsiderAnalysis].outsider.known || len(jobs[outsiderAnalysis]) != 0 {
		t.Fatal("modal accepted or scheduled Outsider OCR", err)
	}
	current.modal = noGildModal
	if err := p.capture(ctx, now.Add(13*time.Second), jobs); err != nil {
		t.Fatal(err)
	}
	job = <-jobs[outsiderAnalysis]
	p.controls.toggle()
	p.reset(p.controls.snapshot())
	if err := p.accept(ctx, p.analyze(ctx, outsiderAnalysis, job), now); err != nil || p.state[outsiderAnalysis].outsider.known {
		t.Fatal("old pause generation accepted", err)
	}
}
