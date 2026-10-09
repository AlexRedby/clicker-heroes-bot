package bot

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
)

func TestStartupSeedClicksWhenHeroWorkUnavailable(t *testing.T) {
	now := time.Now()
	for _, name := range []string{"needed", "unknown", "affordable", "passive", "pending", "scroll", "quantity", "failed", "layout", "generation", "menu"} {
		t.Run(name, func(t *testing.T) {
			f := testPipelineFrame()
			f.context.heroes = true
			p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{heroes: true})
			p.frame, p.layout = f, f.layout
			p.beginStartup()
			p.startup = startupHeroes
			out := observation{kind: heroAnalysis, frame: f, startup: startupHeroes, hero: heroObservation{frame: f, startup: true, x1: true}}
			p.state[heroAnalysis] = out
			switch name {
			case "unknown":
				p.state[heroAnalysis] = observation{}
			case "affordable":
				out.found = true
				out.hero.found = true
				p.state[heroAnalysis] = out
				p.enqueue(gameAction{kind: buyHero, frame: f}, now)
			case "passive":
				p.startupPassive = true
			case "pending":
				p.hero.pending = &heroAttempt{action: gameAction{kind: buyHero, frame: f}}
			case "scroll":
				out.hero.startupScroll = image.Pt(1, 2)
				p.state[heroAnalysis] = out
				p.enqueue(gameAction{kind: scrollHeroes, frame: f}, now)
			case "quantity":
				p.enqueue(gameAction{kind: selectQuantity, frame: f}, now)
			case "failed":
				out.frame.id++
				out.err = errors.New("unreadable hero")
				if err := p.accept(context.Background(), out, now); err != nil {
					t.Fatal(err)
				}
			case "layout":
				p.state[heroAnalysis].frame.layout++
				p.state[heroAnalysis].hero.frame.layout++
			case "generation":
				p.state[heroAnalysis].frame.generation++
				p.state[heroAnalysis].hero.frame.generation++
			case "menu":
				p.frame.context.saveMenu = true
			}
			// Revalidate previously queued seed clicks against the current observation.
			p.enqueue(gameAction{kind: clickMonster, frame: f}, now)
			p.planStartup(now)
			if name == "affordable" || name == "scroll" || name == "quantity" {
				if p.monsterAssistReady() {
					t.Fatalf("%s transaction did not take priority over combat assist", name)
				}
				return
			}
			a, ok := p.nextAction(now)
			clicked := ok && a.kind == clickMonster
			want := name != "affordable" && name != "pending" && name != "scroll" && name != "quantity" && name != "menu"
			if clicked != want {
				t.Fatalf("seed click=%t action=%+v", clicked, a)
			}
		})
	}
	// Explicit ordinary combat clicking retains its separate purpose.
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{monster: true, monsterPoint: image.Pt(12, 34), clickInterval: time.Second})
	p.frame = testPipelineFrame()
	p.layout = p.frame.layout
	p.plan(now)
	if a, ok := p.nextAction(now); !ok || a.kind != clickMonster || a.point != p.options.monsterPoint {
		t.Fatalf("ordinary combat click lost: %+v %t", a, ok)
	}
}

func TestStartupVisualAffordabilityAndOwnershipNeedNoNumericalOCR(t *testing.T) {
	requireAncientOCR(t)
	source := loadTestImage(t, "../../testdata/hero-startup-gold.png")
	frame := func(s image.Image) gameFrame {
		return gameFrame{id: 1, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	}
	numericalReads := 0
	read := heroReaders{
		level: func(context.Context, image.Image, image.Point) (int, error) {
			numericalReads++
			return 0, errors.New("unreadable level")
		},
		gold: func(context.Context, image.Image) (float64, error) {
			numericalReads++
			return 0, errors.New("unreadable gold")
		},
		price: func(context.Context, image.Image, image.Point) (float64, error) {
			numericalReads++
			return 0, errors.New("unreadable price")
		},
	}
	affordable, err := readStartupHeroObservation(context.Background(), frame(source), read, nil, startupSweep{top: true})
	if err != nil || !affordable.found || affordable.passiveReady || affordable.startupComplete {
		t.Fatalf("affordable HIRE: %+v %v", affordable, err)
	}
	// An actual owned card below affordable Cid must establish passive readiness
	// before selecting Cid, without reading that lower card's name or LVL.
	combined := image.NewRGBA(source.Bounds())
	draw.Draw(combined, combined.Bounds(), source, source.Bounds().Min, draw.Src)
	lower := startupTranslatedCard(t, "../../testdata/hero-startup-bottom-hire.png", 709, 865, false)
	r := image.Rect(89, 757, 1126, 987)
	draw.Draw(combined, r, lower, r.Min, draw.Src)
	passive, err := readStartupHeroObservation(context.Background(), frame(combined), read, nil, startupSweep{top: true})
	if err != nil || !passive.found || absDiff(passive.button.Y, 655) > 4 || !passive.passiveReady || passive.startupComplete || numericalReads != 0 {
		t.Fatalf("passive below Cid: %+v %v numerical OCR=%d", passive, err, numericalReads)
	}
	// Change only the blue button body; keep the native HIRE artwork and names.
	dark := image.NewRGBA(source.Bounds())
	draw.Draw(dark, dark.Bounds(), source, source.Bounds().Min, draw.Src)
	for y := 550; y < dark.Bounds().Max.Y; y++ {
		for x := 140; x < 350; x++ {
			r, g, b := rgb(dark.At(x, y))
			if b > 150 && b > r+40 && b >= g-10 && g > 90 {
				dark.Set(x, y, color.RGBA{45, 60, 70, 255})
			}
		}
	}
	short, err := readStartupHeroObservation(context.Background(), frame(dark), read, nil, startupSweep{top: true})
	if err != nil || short.found || short.passiveReady || short.startupComplete {
		t.Fatalf("unavailable HIRE: %+v %v", short, err)
	}
	obscured := image.NewRGBA(dark.Bounds())
	draw.Draw(obscured, obscured.Bounds(), dark, dark.Bounds().Min, draw.Src)
	draw.Draw(obscured, image.Rect(179, 825, 294, 875), image.NewUniform(color.RGBA{45, 60, 70, 255}), image.Point{}, draw.Src)
	uncertain, err := readStartupHeroObservation(context.Background(), frame(obscured), read, nil, startupSweep{top: true})
	if err != nil || uncertain.found || uncertain.passiveReady || uncertain.startupComplete {
		t.Fatalf("covered lower ownership inferred a purchase or completion: %+v %v", uncertain, err)
	}
	if numericalReads != 0 {
		t.Fatalf("native button affordability required %d numerical reads", numericalReads)
	}
}
