package main

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

func TestAutoClickerNativePool(t *testing.T) {
	for _, path := range []string{"testdata/hero-startup-zero.png", "testdata/hero-startup-gold.png"} {
		s := loadTestImage(t, path)
		frame := gameFrame{image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
		pool, err := readAutoClickerPool(context.Background(), frame)
		if err != nil || pool != (autoClickerPool{known: true, available: 3, total: 3}) {
			icon, iconErr := matchControl(s, image.Rect(1197, 368, 1257, 409), "ui/autoclicker-pool.png")
			raw, rawErr := readGameText(context.Background(), s, autoClickerCountRegion(s), max(2, 4096/s.Bounds().Dx()), 7, 180, "0123456789/")
			t.Fatalf("%s pool=%+v error=%v icon=%t %v raw=%q %v", path, pool, err, icon, iconErr, raw, rawErr)
		}
		// The count alone cannot identify the owned pool when its icon is covered.
		covered := image.NewRGBA(s.Bounds())
		draw.Draw(covered, covered.Bounds(), s, s.Bounds().Min, draw.Src)
		draw.Draw(covered, controlRect(s, image.Rect(1197, 368, 1257, 409)), image.NewUniform(color.Black), image.Point{}, draw.Src)
		frame.image = covered
		if got, err := readAutoClickerPool(context.Background(), frame); err != nil || got.known {
			t.Fatalf("covered icon accepted: %+v %v", got, err)
		}
		frame.image, frame.context.saveMenu = s, true
		if got, err := readAutoClickerPool(context.Background(), frame); err != nil || got.known {
			t.Fatalf("menu accepted: %+v %v", got, err)
		}
	}
	for _, raw := range []string{"", "3", "3//3", "3/2", "-1/3", "1/3x", "999999999999999999999/3"} {
		if got := parseAutoClickerPool(raw); got.known {
			t.Fatalf("invalid count %q accepted: %+v", raw, got)
		}
	}
	if got := parseAutoClickerPool(" 0 / 3\n"); got != (autoClickerPool{known: true, total: 3}) {
		t.Fatalf("empty available pool=%+v", got)
	}
}

func TestAutoClickerPlacementAndAcknowledgement(t *testing.T) {
	s := loadTestImage(t, "testdata/hero-startup-zero.png")
	now := time.Now()
	f := gameFrame{id: 1, generation: 2, layout: 3, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	monster := image.Pt(s.Bounds().Dx()*3/4, s.Bounds().Dy()/2)
	footer, found, err := readHeroUpgradeButton(context.Background(), s)
	if err != nil || !found {
		t.Fatalf("native footer: %v %t %v", footer, found, err)
	}
	pool := autoClickerPool{known: true, available: 3, total: 3}
	p := autoClickerPlanner{}
	for _, target := range []autoClickerTarget{autoClickerMonster, autoClickerMonster, autoClickerUpgrades} {
		point := monster
		if target == autoClickerUpgrades {
			if _, ok := p.command(f, pool, autoClickerMonster, monster); ok {
				t.Fatal("last free clicker was not reserved for upgrades")
			}
			point = footer
		}
		a, ok := p.command(f, pool, target, point)
		if !ok {
			t.Fatalf("missing command target=%d pool=%+v", target, pool)
		}
		var trace []string
		input := heroInput{
			keyToggle: func(key, state string) error { trace = append(trace, key+" "+state); return nil },
			click:     func(point image.Point) error { trace = append(trace, fmt.Sprint(point)); return nil },
		}
		p.sent(a, now)
		if _, ok := p.command(f, pool, target, point); ok {
			t.Fatal("submitted command replayed")
		}
		if err := placeAutoClicker(context.Background(), input, a); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(trace, []string{"c down", fmt.Sprint(point), "c up"}) {
			t.Fatalf("placement trace: %v", trace)
		}
		p.interrupt()
		if p.pending == nil {
			t.Fatal("F8 forgot submitted placement")
		}
		pool.available--
		// Controlled pool observations model a successful placement. No native after-placement frame exists yet.
		p.observe(f, pool, now.Add(time.Second))
		if p.pending == nil {
			t.Fatal("same-frame result acknowledged placement")
		}
		f.id++
		f.at = now.Add(time.Second)
		p.observe(f, pool, f.at)
		if p.pending != nil || p.blocked {
			t.Fatalf("decrement not acknowledged: %+v", p)
		}
	}
	if !p.upgrades {
		t.Fatal("footer assignment not remembered")
	}
	pool.available = 1
	if _, ok := p.command(f, pool, autoClickerUpgrades, footer); ok {
		t.Fatal("confirmed footer placement repeated")
	}
	pool.total = 1
	p = autoClickerPlanner{}
	if _, ok := p.command(f, pool, autoClickerMonster, monster); !ok {
		t.Fatal("single owned clicker was not available for combat")
	}
	if _, ok := p.command(f, pool, autoClickerUpgrades, footer); ok {
		t.Fatal("single combat clicker offered for upgrades")
	}
	// Native C on an occupied footer is a no-op: stop trying it, without withholding monsters.
	p = autoClickerPlanner{}
	pool = autoClickerPool{known: true, available: 3, total: 3}
	a, ok := p.command(f, pool, autoClickerUpgrades, footer)
	if !ok {
		t.Fatal("footer not offered")
	}
	p.sent(a, now)
	p.interrupt()
	f.id++
	f.at = now.Add(6 * time.Second)
	p.observe(f, pool, f.at)
	if p.pending != nil || p.blocked || p.upgrades || !p.footerAttempted {
		t.Fatalf("unconfirmed footer: %+v", p)
	}
	if _, ok := p.command(f, pool, autoClickerUpgrades, footer); ok {
		t.Fatal("unconfirmed footer replayed")
	}
	pool.available = 1
	if _, ok := p.command(f, pool, autoClickerMonster, monster); !ok {
		t.Fatal("unconfirmed footer withheld remaining free clicker")
	}
}

func TestAutoClickerNoReplayAndInputGuards(t *testing.T) {
	s := loadTestImage(t, "testdata/hero-startup-zero.png")
	now := time.Now()
	f := gameFrame{id: 1, generation: 2, layout: 3, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	point := image.Pt(s.Bounds().Dx()*3/4, s.Bounds().Dy()/2)
	pool := autoClickerPool{known: true, available: 3, total: 3}
	p := autoClickerPlanner{}
	a, ok := p.command(f, pool, autoClickerMonster, point)
	if !ok || !autoClickerCommandStable(a, f) {
		t.Fatal("valid native command rejected")
	}
	current := f
	current.generation++
	if autoClickerCommandStable(a, current) {
		t.Fatal("old F8 generation accepted")
	}
	current = f
	current.context.saveMenu = true
	if autoClickerCommandStable(a, current) {
		t.Fatal("modal accepted")
	}
	current = f
	covered := image.NewRGBA(s.Bounds())
	draw.Draw(covered, covered.Bounds(), s, s.Bounds().Min, draw.Src)
	draw.Draw(covered, autoClickerCountRegion(s), image.NewUniform(color.Black), image.Point{}, draw.Src)
	current.image = covered
	if autoClickerCommandStable(a, current) {
		t.Fatal("covered count accepted")
	}
	a.point = image.Pt(s.Bounds().Dx()*96/100, s.Bounds().Dy()*55/100)
	if autoClickerTargetValid(a) {
		t.Fatal("pool/removal point accepted as a monster target")
	}
	a.point = point
	p.sent(a, now)
	f.id++
	f.at = now.Add(6 * time.Second)
	p.observe(f, pool, f.at)
	p.interrupt()
	if !p.blocked {
		t.Fatal("unconfirmed placement did not block replay")
	}
	if _, ok := p.command(f, pool, autoClickerMonster, point); ok {
		t.Fatal("unconfirmed input replayed after F8")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var trace []string
	input := heroInput{
		keyToggle: func(key, state string) error {
			trace = append(trace, key+" "+state)
			if state == "down" {
				cancel()
			}
			return nil
		},
		click: func(image.Point) error { t.Fatal("cancelled placement clicked"); return nil },
	}
	if err := placeAutoClicker(ctx, input, a); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(trace, []string{"c down", "c up"}) {
		t.Fatalf("cancel/release: %v %v", trace, err)
	}
}
