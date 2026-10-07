package bot

import (
	"bytes"
	"image"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func mercenaryPlannerFrame(id uint64, at time.Time, c gameContext) gameFrame {
	return gameFrame{id: id, at: at, context: c}
}

func captureMercenaryPlannerStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestMercenaryPlannerSkipsDeadRows(t *testing.T) {
	now := time.Now()
	bounds := image.Rect(0, 0, 1000, 1000)
	c := gameContext{known: true, mercenaries: true, bounds: bounds, window: "game"}
	dead := image.Pt(700, 400)
	livingCollect := image.Pt(700, 550)
	deadStart := image.Pt(700, 700)
	livingStart := image.Pt(700, 850)
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	o := mercenaryObservation{
		frame:    mercenaryPlannerFrame(1, now, c),
		readable: true,
		top:      true,
		bottom:   true,
		dead:     []image.Point{dead, deadStart},
		collect:  []image.Point{livingCollect},
		start:    []image.Point{livingStart},
	}
	p.observe(o, now)

	a, ok := p.action(now)
	if !ok || a.mercenary.step != claimAndOpenMercenaryQuest || a.point != livingCollect {
		t.Fatalf("wanted living Collect row, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	questContext := gameContext{known: true, mercenaries: true, questDialog: true, bounds: bounds, window: "game"}
	p.observe(mercenaryObservation{
		frame:    mercenaryPlannerFrame(2, now.Add(time.Second), questContext),
		readable: true,
		selected: -1,
		quests:   []mercenaryQuest{{reward: "gold", duration: time.Hour, point: image.Pt(400, 400)}},
	}, now.Add(time.Second))
	a, ok = p.action(now.Add(time.Second))
	if !ok || a.mercenary.step != selectMercenaryQuest {
		t.Fatalf("wanted quest selection after living Collect row, got %+v ok=%t", a, ok)
	}
	p.sent(a, now.Add(time.Second))
	p.observe(mercenaryObservation{
		frame:    mercenaryPlannerFrame(3, now.Add(2*time.Second), c),
		readable: true,
		top:      true,
		bottom:   true,
		dead:     []image.Point{dead, deadStart},
		start:    []image.Point{livingStart},
	}, now.Add(2*time.Second))
	a, ok = p.action(now.Add(2 * time.Second))
	if !ok || a.mercenary.step != openMercenaryQuest || a.point != livingStart {
		t.Fatalf("wanted living Start row, got %+v ok=%t", a, ok)
	}
}

func TestMercenaryPlannerAllDeadScrollsOnceAndReturns(t *testing.T) {
	now := time.Now()
	bounds := image.Rect(0, 0, 1000, 1000)
	c := gameContext{known: true, mercenaries: true, bounds: bounds, window: "game"}
	heroes := gameContext{known: true, heroes: true, bounds: bounds, window: "game"}
	dead := []image.Point{image.Pt(700, 400), image.Pt(700, 550)}
	thumb := image.Pt(450, 450)
	p := mercenaryPlanner{active: true, returnHeroes: true}
	roster := func(id uint64, at time.Time, top, bottom bool) mercenaryObservation {
		return mercenaryObservation{frame: mercenaryPlannerFrame(id, at, c), readable: true, top: top, bottom: bottom, thumbFound: true, thumb: thumb, dead: append([]image.Point(nil), dead...)}
	}
	p.observe(roster(1, now, false, false), now)
	a, ok := p.action(now)
	if !ok || a.mercenary.step != scrollMercenariesTop {
		t.Fatalf("wanted top scroll, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	now = now.Add(time.Second)
	p.observe(roster(2, now, true, false), now)
	a, ok = p.action(now)
	if !ok || a.mercenary.step != scrollMercenariesBottom {
		t.Fatalf("wanted bottom scroll, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	now = now.Add(time.Second)
	p.observe(roster(3, now, true, true), now)
	a, ok = p.action(now)
	if !ok || a.mercenary.step != returnToHeroes {
		t.Fatalf("wanted return to Heroes after all-dead sweep, got %+v ok=%t", a, ok)
	}
	p.sent(a, now)
	now = now.Add(time.Second)
	p.observe(mercenaryObservation{frame: mercenaryPlannerFrame(4, now, heroes)}, now)
	if p.active || p.returnHeroes || p.roster != nil {
		t.Fatalf("planner did not finish all-dead visit: %+v", p)
	}
}

func TestMercenaryPlannerSavedDeadRowsAndVisitLog(t *testing.T) {
	now := time.Now()
	bounds := image.Rect(0, 0, 1000, 1000)
	c := gameContext{known: true, mercenaries: true, bounds: bounds, window: "game"}
	heroes := gameContext{known: true, heroes: true, bounds: bounds, window: "game"}
	dead := []image.Point{image.Pt(700, 400)}
	living := image.Pt(700, 550)
	p := mercenaryPlanner{active: true, returnHeroes: true, topVisited: true, bottomVisited: true}
	firstLog := captureMercenaryPlannerStdout(t, func() {
		p.observe(mercenaryObservation{
			frame: mercenaryPlannerFrame(1, now, c), readable: true, top: true, bottom: true,
			dead: dead, collect: []image.Point{living},
		}, now)
	})
	if p.roster == nil || len(p.roster.dead) != 1 || p.roster.dead[0] != dead[0] {
		t.Fatalf("saved roster lost dead rows: %+v", p.roster)
	}
	repeatedLog := captureMercenaryPlannerStdout(t, func() {
		p.captured(mercenaryPlannerFrame(2, now.Add(time.Second), c), now.Add(time.Second))
		p.captured(mercenaryPlannerFrame(3, now.Add(2*time.Second), c), now.Add(2*time.Second))
	})
	if p.roster == nil || len(p.roster.dead) != 1 || p.roster.dead[0] != dead[0] {
		t.Fatalf("repeated capture discarded saved dead rows: %+v", p.roster)
	}
	if strings.TrimSpace(firstLog) == "" {
		t.Fatal("first readable visit did not log dead mercenary status")
	}
	if repeatedLog != "" {
		t.Fatalf("repeated capture spammed dead mercenary log: %q", repeatedLog)
	}

	// Complete the visit through the normal return action so the next visit
	// gets a fresh one-shot status log.
	p.latest.collect = nil
	p.roster.collect = nil
	a, ok := p.action(now.Add(2 * time.Second))
	if !ok || a.mercenary.step != returnToHeroes {
		t.Fatalf("wanted return action for fresh visit, got %+v ok=%t", a, ok)
	}
	p.sent(a, now.Add(2*time.Second))
	p.observe(mercenaryObservation{frame: mercenaryPlannerFrame(4, now.Add(3*time.Second), heroes)}, now.Add(3*time.Second))
	if p.active || p.roster != nil {
		t.Fatalf("return did not reset visit: %+v", p)
	}

	p.active = true
	p.returnHeroes = true
	p.topVisited, p.bottomVisited = true, true
	secondLog := captureMercenaryPlannerStdout(t, func() {
		p.observe(mercenaryObservation{frame: mercenaryPlannerFrame(5, now.Add(5*time.Second), c), readable: true, top: true, bottom: true, dead: dead}, now.Add(5*time.Second))
	})
	if strings.TrimSpace(secondLog) == "" {
		t.Fatal("fresh visit did not log dead mercenary status")
	}
}
