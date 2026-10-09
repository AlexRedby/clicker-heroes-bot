package bot

import (
	"context"
	"image"
	"testing"
)

func TestEarlyHeroLargeScrollbar(t *testing.T) {
	f := startupFrame(t, "../../testdata/hero-early-large-scrollbar.png")
	thumb, height, found := heroScrollbarThumb(f.image)
	if !found || absDiff(thumb.X, 1171) > 4 || height < 350 || height > 450 {
		t.Fatalf("early list thumb: point=%v height=%d found=%t", thumb, height, found)
	}
	if heroScrollbarAtBottom(f.image) {
		t.Fatal("top mistaken for bottom")
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
	out := p.analyze(context.Background(), heroAnalysis, analysisJob{frame: f, startup: startupUpgrades})
	if out.err != nil || out.hero.bottom || out.hero.startupScroll == (image.Point{}) || out.hero.startupScroll.Y <= thumb.Y {
		t.Fatalf("footer seek did not scroll: %+v", out)
	}
}
