package bot

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func gildMovePlan() ancientcalc.GildPlan {
	return ancientcalc.GildPlan{Eligible: true, SaveHash: strings.Repeat("a", 64), Target: ancientcalc.GildHero{ID: 28, Name: "Atlas", Level: 1000}, TotalGilds: 2, MoveGilds: 2, Heroes: []ancientcalc.GildHero{{ID: 2, Gilds: 2}, {ID: 28}}}
}

func gildMoveFrame(t *testing.T, now time.Time, id, layout uint64, roster bool) gameFrame {
	t.Helper()
	s := gildFixture(t, "roster")
	c := gameContext{known: true, window: "Clicker Heroes", bounds: s.Bounds(), heroes: !roster}
	if roster {
		c.modal = gildRosterModal
	}
	return gameFrame{id: id, layout: layout, at: now, image: s, context: c}
}

func TestGildRedistributionRefreshesAfterAttemptWithoutHistory(t *testing.T) {
	for _, inputErr := range []error{nil, errors.New("mouse-up failed")} {
		t.Run(fmtError(inputErr), func(t *testing.T) {
			now := time.Now()
			source := gildMoveFrame(t, now, 1, 1, false)
			plan := gildMovePlan()
			var g gildRedistribution
			if err := g.install(plan, source); err != nil {
				t.Fatal(err)
			}
			// Owned entry changed layout; it must not revoke the fresh preparation.
			roster := gildMoveFrame(t, now.Add(time.Second), 2, 2, true)
			g.observe(gildRedistributionObservation{frame: roster, roster: true, targetFound: true, targetID: 28, target: image.Pt(260, 150), close: image.Pt(1149, 39)})
			cmd, ok := g.next(roster.at)
			if !ok || cmd.action != gildTransferAll {
				t.Fatal(cmd, ok, g.reason)
			}
			g.submitted(cmd, roster.at, inputErr)
			if _, ok := g.next(roster.at); ok {
				t.Fatal("replayed on same frame")
			}
			if err := g.install(plan, source); err == nil {
				t.Fatal("old export accepted as outcome")
			}
			now = roster.at.Add(time.Second)
			roster.id, roster.at = 3, now
			g.observe(gildRedistributionObservation{frame: roster, roster: true, close: image.Pt(1149, 39), targetFound: true, targetID: 28, target: image.Pt(260, 150)})
			cmd, ok = g.next(now)
			if !ok || cmd.action != gildCloseRoster {
				t.Fatal("attempt did not close", cmd, ok)
			}
			g.submitted(cmd, now, nil)
			now = now.Add(time.Second)
			heroes := gildMoveFrame(t, now, 4, 3, false)
			g.observe(gildRedistributionObservation{frame: heroes})
			cmd, ok = g.next(now)
			if !ok || cmd.action != gildRefreshSave {
				t.Fatal("missing fresh outcome", cmd, ok)
			}
			g.submitted(cmd, now, nil)
			plan.Eligible, plan.MoveGilds = false, 0
			plan.Heroes[0].Gilds, plan.Heroes[1].Gilds = 0, 2
			heroes.id++
			heroes.at = now.Add(time.Second)
			if err := g.install(plan, heroes); err != nil || g.active {
				t.Fatal("all-on-target was not no-op", err, g)
			}
		})
	}
}

func fmtError(err error) string {
	if err == nil {
		return "successful input"
	}
	return "uncertain input"
}

func TestGildRedistributionBoundsRetriesAndFailedSearch(t *testing.T) {
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	plan := gildMovePlan()
	for _, blocked := range []bool{false, true} {
		g := gildRedistribution{plan: plan, source: source, attempts: 2, blocked: blocked}
		if blocked {
			g.attempts = 0
		}
		plan.SaveHash = strings.Repeat("b", 64) // Gold/zone changes do not reset a failed distribution.
		if err := g.install(plan, source); err != nil || g.active {
			t.Fatal("unchanged distribution retried", err, g)
		}
		changed := plan
		changed.Heroes = append([]ancientcalc.GildHero(nil), plan.Heroes...)
		changed.Heroes[0].Gilds++
		if err := g.install(changed, source); err != nil || !g.active {
			t.Fatal("new distribution blocked", err, g)
		}
	}
	var g gildRedistribution
	if err := g.install(plan, source); err != nil {
		t.Fatal(err)
	}
	roster := gildMoveFrame(t, now.Add(time.Second), 2, 2, true)
	g.scrolls = 16
	g.observe(gildRedistributionObservation{frame: roster, roster: true, close: image.Pt(1149, 39)})
	cmd, ok := g.next(roster.at)
	if !ok || cmd.action != gildCloseRoster {
		t.Fatal(cmd, ok)
	}
	g.submitted(cmd, roster.at, nil)
	if g.active || !g.blocked {
		t.Fatal("failed search still owns gameplay")
	}
	source.at = roster.at.Add(time.Second)
	if err := g.install(plan, source); err != nil || g.active {
		t.Fatal("failed search restarted on export", err)
	}
	g.interrupt()
	if err := g.install(plan, source); err != nil || !g.active {
		t.Fatal("explicit restart could not retry", err)
	}
}

func TestGildRedistributionStaleAndRevokedPreparationOnlyRefresh(t *testing.T) {
	for _, change := range []string{"expired", "generation", "window", "bounds", "interrupt"} {
		t.Run(change, func(t *testing.T) {
			now := time.Now()
			source := gildMoveFrame(t, now, 1, 1, false)
			var g gildRedistribution
			if err := g.install(gildMovePlan(), source); err != nil {
				t.Fatal(err)
			}
			frame := gildMoveFrame(t, now.Add(time.Second), 2, 2, false)
			switch change {
			case "expired":
				frame.at = now.Add(31 * time.Second)
			case "generation":
				frame.generation++
			case "window":
				frame.context.window = "other"
			case "bounds":
				frame.context.bounds.Max.X++
			case "interrupt":
				g.interrupt()
			}
			g.observe(gildRedistributionObservation{frame: frame, entry: image.Pt(116, 434)})
			cmd, ok := g.next(frame.at)
			if !ok || cmd.action != gildRefreshSave {
				t.Fatal("revoked state can spend", cmd, ok, g.reason)
			}
		})
	}
}

func TestGildRedistributionMissingControlsAndRepeatedExpiryReleaseOwnership(t *testing.T) {
	now := time.Now()
	source := gildMoveFrame(t, now, 1, 1, false)
	plan := gildMovePlan()
	for _, closePresent := range []bool{false, true} {
		var g gildRedistribution
		if err := g.install(plan, source); err != nil {
			t.Fatal(err)
		}
		frame := gildMoveFrame(t, now.Add(time.Second), 2, 2, true)
		u := gildRedistributionObservation{frame: frame, roster: true}
		if closePresent {
			u.close = image.Pt(1149, 39)
		}
		g.observe(u)
		cmd, ok := g.next(frame.at)
		if closePresent {
			if !ok || cmd.action != gildCloseRoster {
				t.Fatal("missing down/target did not exit", cmd, ok)
			}
			g.submitted(cmd, frame.at, nil)
		} else if ok {
			t.Fatal("missing close emitted input", cmd)
		}
		if g.active {
			t.Fatal("missing controls retained modal ownership")
		}
	}
	var g gildRedistribution
	for i := 0; i < 2; i++ {
		if err := g.install(plan, source); err != nil {
			t.Fatal(err)
		}
		frame := gildMoveFrame(t, source.at.Add(31*time.Second), uint64(2+i), 2, false)
		g.observe(gildRedistributionObservation{frame: frame})
		cmd, ok := g.next(frame.at)
		if i == 0 {
			if !ok || cmd.action != gildRefreshSave {
				t.Fatal(cmd, ok)
			}
			g.submitted(cmd, frame.at, nil)
		} else if ok || g.active || !g.blocked {
			t.Fatal("repeated expiry restarted preparation", cmd, ok, g)
		}
		source.id = frame.id + 1
		source.at = frame.at.Add(time.Second)
	}
}

func TestGildRedistributionNativeEntryAndExactNames(t *testing.T) {
	requireAncientOCR(t)
	ctx := context.Background()
	now := time.Now()
	entry := loadTestImage(t, "../../testdata/hero-startup-zero.png")
	u, err := readGildRedistribution(ctx, gameFrame{image: entry, context: gameContext{known: true, heroes: true}}, ancientcalc.GildHero{ID: 28, Name: "Atlas"})
	if err != nil || absDiff(u.entry.X, 232) > 3 || absDiff(u.entry.Y, 867) > 3 {
		t.Fatal("native entry", u.entry, err)
	}
	frame := gildMoveFrame(t, now, 2, 2, true)
	for _, target := range []ancientcalc.GildHero{{ID: 2, Name: "Treebeast"}, {ID: 7, Name: "The Masked Samurai"}, {ID: 11, Name: "Natalia, Ice Apprentice"}} {
		u, err := readGildRedistribution(ctx, frame, target)
		if err != nil || !u.targetFound || u.targetID != target.ID || !u.target.In(image.Rect(143, 129, 1098, 515)) || u.down == (image.Point{}) || u.close != (image.Pt(1149, 39)) {
			t.Fatal("named tile", target, u, err)
		}
		cmd := gildRedistributionCommand{action: gildTransferAll, frame: frame, point: u.target, region: u.targetRegion}
		if !gildRedistributionCommandValid(cmd, frame) {
			t.Fatal("native name unstable")
		}
		changed := image.NewRGBA(frame.image.Bounds())
		draw.Draw(changed, changed.Bounds(), frame.image, frame.image.Bounds().Min, draw.Src)
		draw.Draw(changed, u.targetRegion, image.NewUniform(color.Black), image.Point{}, draw.Src)
		bad := frame
		bad.image = changed
		if gildRedistributionCommandValid(cmd, bad) {
			t.Fatal("covered/replaced target authorized")
		}
	}
	u, err = readGildRedistribution(ctx, frame, ancientcalc.GildHero{ID: 28, Name: "Atlas"})
	if err != nil || u.targetFound {
		t.Fatal("offscreen target inferred from roster order", u, err)
	}
}

func TestGildRedistributionReleasesQOnClickErrorAndCancellation(t *testing.T) {
	for _, cancelClick := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var keys []string
		input := heroInput{keyToggle: func(k, state string) error { keys = append(keys, k+" "+state); return nil }, click: func(image.Point) error {
			if cancelClick {
				cancel()
			}
			return errors.New("click failed")
		}}
		err := executeGildRedistribution(ctx, input, gildRedistributionCommand{action: gildTransferAll, point: image.Pt(260, 150)})
		cancel()
		if err == nil || strings.Join(keys, ",") != "q down,q up" {
			t.Fatal("held key stranded", keys, err)
		}
	}
}
