package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestAutoClickerDisabledFooterPlacement(t *testing.T) {
	requireAncientOCR(t)
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	point, found, err := readHeroUpgradeButton(context.Background(), s)
	if err != nil || !found {
		t.Fatal("fixture footer", found, err)
	}
	disabled := image.NewRGBA(s.Bounds())
	draw.Draw(disabled, disabled.Bounds(), s, s.Bounds().Min, draw.Src)
	r := heroUpgradeButtonRegion(s, point)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			red, g, b := rgb(s.At(x, y))
			if g > 100 && g > red+50 && g > b+50 {
				disabled.Set(x, y, color.RGBA{R: 60, G: 60, B: 60, A: 255})
			}
		}
	}
	located, known, available, err := readHeroUpgradeFooter(context.Background(), disabled)
	if err != nil || !known || available || absDiff(located.Y, point.Y) > s.Bounds().Dy()/100 {
		t.Fatal("disabled footer position", located, point, known, available, err)
	}
	now := time.Now()
	f := gameFrame{id: 1, at: now, image: disabled, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true, autoClickers: true})
	p.frame, p.startup, p.startupCheck = f, startupUpgrades, false
	p.state[autoClickerAnalysis] = observation{frame: f, clickerPool: autoClickerPool{known: true, available: 1, total: 3}}
	p.state[heroAnalysis] = observation{frame: f, startup: startupUpgrades, upgradesKnown: true, point: located}
	p.plan(now)
	a, ok := p.nextAction(now)
	if !ok || a.kind != placeOwnedClicker || a.clicker.target != autoClickerUpgrades || p.startup != startupUpgrades {
		t.Fatal("disabled footer skipped before owned placement", a, ok, p.startup)
	}
}
