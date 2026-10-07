package bot

import (
	"image"
	"testing"
	"time"
)

func TestAutoClickerRecoveryRequiresFreshRecognizedPool(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	now := time.Now()
	frame := gameFrame{id: 10, generation: 2, layout: 3, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	pool := autoClickerPool{known: true, available: 1, total: 3}
	p := autoClickerPlanner{blocked: true, footerAttempted: true}
	p.rememberPool(frame, pool)

	if p.recover(frame, pool) {
		t.Fatal("same frame recovered a failed planner")
	}
	fresh := frame
	fresh.id++
	fresh.at = now.Add(time.Minute)
	changedPool := autoClickerPool{known: true, available: 0, total: 3}
	if !p.recover(fresh, changedPool) || p.blocked || p.footerAttempted {
		t.Fatalf("fresh changed pool did not recover planner: %+v", p)
	}
}

func TestAutoClickerFailedMonsterCanRecoverAndRetry(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	now := time.Now()
	frame := gameFrame{id: 40, generation: 2, layout: 3, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	point := image.Pt(s.Bounds().Dx()*3/4, s.Bounds().Dy()/2)
	pool := autoClickerPool{known: true, available: 2, total: 3}
	p := autoClickerPlanner{}
	a, ok := p.command(frame, pool, autoClickerMonster, point)
	if !ok {
		t.Fatal("monster placement was not planned")
	}
	p.sent(a, now)
	p.rememberPool(frame, pool)
	failed := frame
	failed.id++
	failed.at = now.Add(6 * time.Second)
	p.observe(failed, pool, failed.at)
	if p.pending != nil || !p.blocked {
		t.Fatalf("timed out monster placement was not resolved: %+v", p)
	}
	recovery := failed
	recovery.id++
	recovery.at = now.Add(11 * time.Second)
	if !p.recover(recovery, pool) {
		t.Fatal("fresh pool did not clear failed monster placement")
	}
	if _, ok := p.command(recovery, pool, autoClickerMonster, point); !ok {
		t.Fatal("recovered planner did not allow monster retry")
	}
}

func TestAutoClickerRecoveryPreservesPendingPlacement(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	now := time.Now()
	frame := gameFrame{id: 20, generation: 2, layout: 3, at: now, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	point := image.Pt(s.Bounds().Dx()*3/4, s.Bounds().Dy()/2)
	pool := autoClickerPool{known: true, available: 2, total: 3}
	p := autoClickerPlanner{}
	a, ok := p.command(frame, pool, autoClickerMonster, point)
	if !ok {
		t.Fatal("monster placement was not planned")
	}
	p.sent(a, now)
	p.rememberPool(frame, pool)
	fresh := frame
	fresh.id++
	fresh.at = now.Add(5 * time.Minute)
	if p.recover(fresh, pool) {
		t.Fatal("recovery cleared a submitted placement")
	}
	if p.pending == nil {
		t.Fatal("pending placement was discarded by recovery")
	}
}

func TestAutoClickerFooterPassReleasesLastClicker(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	frame := gameFrame{id: 30, generation: 2, layout: 3, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	point := image.Pt(s.Bounds().Dx()*3/4, s.Bounds().Dy()/2)
	p := autoClickerPlanner{}
	p.noteFooterUnavailable()
	if _, ok := p.command(frame, autoClickerPool{known: true, available: 1, total: 3}, autoClickerMonster, point); !ok {
		t.Fatal("last free clicker remained reserved after bounded footer pass")
	}
	if _, ok := p.command(frame, autoClickerPool{known: true, available: 1, total: 3}, autoClickerUpgrades, point); ok {
		t.Fatal("missing footer was converted into an upgrade target")
	}
}

func TestAutoClickerRecoveryCanRestoreRemovedFooter(t *testing.T) {
	s := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	f := gameFrame{id: 1, image: s, context: gameContext{known: true, heroes: true, bounds: s.Bounds()}}
	p := autoClickerPlanner{upgrades: true, footerAttempted: true}
	p.rememberPool(f, autoClickerPool{known: true, total: 3})
	f.id++
	pool := autoClickerPool{known: true, available: 1, total: 3}
	if !p.recover(f, pool) {
		t.Fatal("freed clicker did not reopen recovery")
	}
	point := image.Pt(s.Bounds().Dx()*34/100, s.Bounds().Dy()/2)
	if _, ok := p.command(f, pool, autoClickerUpgrades, point); !ok {
		t.Fatal("cached footer assignment blocked restoration")
	}
}
