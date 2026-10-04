package main

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"
)

func TestAscensionJunkOwnedContinuation(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/ascension-junk.png")
	frame := gameFrame{id: 2, image: screen, context: gameContext{known: true, bounds: screen.Bounds()}}
	out, err := readAscensionObservation(context.Background(), frame)
	if err != nil || !out.junk || !out.confirm || !out.no {
		t.Fatalf("junk observation: %+v %v", out, err)
	}
	for _, directDialog := range []bool{false, true} {
		p := ascensionPlanner{minimumReward: 60}
		p.sent(openAscension, 1, now)
		p.observe(out, nil, now)
		if p.step != salvageAscensionJunk {
			t.Fatal("owned blocker not salvaged")
		}
		point, found, err := ascensionActionPoint(screen, p.step)
		if err != nil || !found || point != image.Pt(1040, 871) {
			t.Fatalf("native Yes: %v %v %v", point, found, err)
		}
		action := gameAction{kind: handleAscension, frame: frame, ascension: p.step, point: point}
		if !ascensionActionStable(action, frame) {
			t.Fatal("native salvage action rejected")
		}
		p.sent(salvageAscensionJunk, frame.id, now)
		for i := 0; i < 3; i++ {
			out.frame.id++
			p.observe(out, nil, now)
			if p.step != waitAscensionJunk {
				t.Fatal("unchanged prompt replayed Yes")
			}
			if _, found, err := ascensionActionPoint(screen, p.step); found || err != nil {
				t.Fatal("waiting state has input")
			}
		}
		p.observe(ascensionObservation{frame: gameFrame{id: 7}}, errors.New("unreadable"), now)
		if p.step != waitAscensionJunk {
			t.Fatal("unknown frame advanced salvage")
		}
		if !directDialog {
			heroes := gameFrame{id: 8, context: gameContext{known: true, heroes: true}}
			p.observe(ascensionObservation{frame: heroes, zone: 19059}, nil, now)
			if p.step != openAscension || !p.junkReopened || p.minimumReward != 60 {
				t.Fatal("did not reopen with original minimum reward")
			}
			p.sent(openAscension, heroes.id, now)
		}
		dialog := gameFrame{id: 9, context: gameContext{known: true, ascension: true}}
		p.observe(ascensionObservation{frame: dialog, confirm: true, no: true, souls: 65}, nil, now)
		if p.step != confirmAscension {
			t.Fatal("normal reward not independently verified after salvage")
		}
		p.sent(confirmAscension, dialog.id, now)
		reset := gameFrame{id: 10, context: gameContext{known: true, heroes: true}}
		if !p.observe(ascensionObservation{frame: reset, zone: 1}, nil, now) {
			t.Fatal("post-salvage Ascension not completed")
		}
	}
}

func TestAscensionJunkOrphanPauseAndSecondBlocker(t *testing.T) {
	now := time.Now()
	screen := loadTestImage(t, "testdata/ascension-junk.png")
	frame := gameFrame{id: 2, image: screen, context: gameContext{known: true, bounds: screen.Bounds()}}
	out, err := readAscensionObservation(context.Background(), frame)
	if err != nil {
		t.Fatal(err)
	}
	p := ascensionPlanner{}
	p.sent(openAscension, 1, now)
	p.observe(out, nil, now)
	yes, found, err := ascensionActionPoint(screen, p.step)
	if err != nil || !found {
		t.Fatal("salvage point missing")
	}
	action := gameAction{kind: handleAscension, frame: frame, point: yes, ascension: salvageAscensionJunk}
	afterPause := frame
	afterPause.generation++
	if ascensionActionStable(action, afterPause) {
		t.Fatal("F8 kept stale salvage Yes")
	}
	controls := &pauseControl{}
	pipeline := newGamePipeline(controls, heroInput{click: func(image.Point) error { t.Fatal("F8 clicked salvage Yes"); return nil }}, pipelineReaders{}, pipelineOptions{})
	controls.toggle()
	if acted, err := pipeline.execute(context.Background(), action); acted || err != nil {
		t.Fatal("F8 allowed stale salvage input")
	}
	p.interrupt()
	p.observe(out, nil, now)
	if p.step != cancelAscension || !p.active {
		t.Fatal("orphan blocker was not cancelled")
	}
	no, found, err := ascensionActionPoint(screen, p.step)
	if err != nil || !found || no == yes {
		t.Fatal("orphan chose Yes")
	}
	p.sent(cancelAscension, frame.id, now)
	out.frame.id++
	p.observe(out, nil, now)
	if !p.active || p.latest.frame.id != 0 {
		t.Fatal("delayed orphan No replayed")
	}
	p.observe(ascensionObservation{frame: gameFrame{id: 4, context: gameContext{known: true, heroes: true}}, zone: 19059}, nil, now)
	if p.active {
		t.Fatal("closed orphan retained ownership")
	}
	p = ascensionPlanner{minimumReward: 60, junkSalvaged: true, junkReopened: true}
	p.sent(openAscension, 4, now)
	out.frame.id = 5
	p.observe(out, nil, now)
	if p.step != cancelAscension {
		t.Fatal("second blocker allowed another salvage")
	}
}
