package bot

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeroUnreadableDiagnosticsUseSharedFrame(t *testing.T) {
	screen := loadTestImage(t, "../../testdata/hero-startup-gold.png")
	now := time.Now()
	captures := 0
	p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { captures++; return screen, nil }}, pipelineReaders{}, pipelineOptions{heroes: true})
	p.startup, p.layout = startupHeroes, 1
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	readError := errors.New("startup HIRE caption unreadable")
	accept := func(err error) {
		t.Helper()
		p.frame = frame
		if e := p.accept(context.Background(), observation{kind: heroAnalysis, startup: startupHeroes, frame: frame, hero: heroObservation{frame: frame, startup: true}, err: err}, frame.at); e != nil {
			t.Fatal(e)
		}
		frame.id++
		frame.at = frame.at.Add(time.Second)
	}
	accept(readError)
	if len(p.diagnostics) != 1 || captures != 0 {
		t.Fatalf("diagnostics=%d captures=%d", len(p.diagnostics), captures)
	}
	diagnostic := <-p.diagnostics
	if diagnostic.after.frame.id != 1 || diagnostic.after.frame.image != screen || !errors.Is(diagnostic.readError, readError) {
		t.Fatal("diagnostic lost the analyzed frame or reason")
	}
	accept(readError)
	p.hero.interrupt()
	accept(readError)
	if len(p.diagnostics) != 0 {
		t.Fatal("unchanged read failure saved repeatedly or after F8")
	}
	accept(nil)
	accept(readError)
	if len(p.diagnostics) != 1 || captures != 0 {
		t.Fatal("recovered reader did not start a new failure episode")
	}
	<-p.diagnostics
	accept(errors.New("startup hero name unreadable"))
	if len(p.diagnostics) != 1 {
		t.Fatal("changed read failure was not recorded")
	}
	blocked := errors.New("startup caption crop unreadable")
	accept(blocked)
	<-p.diagnostics
	accept(blocked)
	if len(p.diagnostics) != 1 {
		t.Fatal("busy worker permanently suppressed new failure evidence")
	}

	t.Chdir(t.TempDir())
	saveHeroReadFailure(diagnostic.after.frame, diagnostic.readError)
	paths, err := filepath.Glob("artifacts/hero-unreadable-*.png")
	if err != nil || len(paths) != 1 {
		t.Fatalf("saved failure frames: %v %v", paths, err)
	}
	f, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	saved, err := png.Decode(f)
	if err != nil || saved.Bounds() != screen.Bounds() {
		t.Fatalf("saved frame bounds: %v", err)
	}
	r, g, b, a := screen.At(204, 655).RGBA()
	sr, sg, sb, sa := saved.At(204, 655).RGBA()
	if r != sr || g != sg || b != sb || a != sa {
		t.Fatal("diagnostic changed the screenshot")
	}
}
