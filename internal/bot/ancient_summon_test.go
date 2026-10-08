package bot

import (
	"fmt"
	"testing"
	"time"

	"clicker-heroes-bot/internal/transcension"
)

func TestAncientSummonNativeNavigationUsesSharedActions(t *testing.T) {
	requireAncientOCR(t)
	for _, width := range []int{1280, 1920, 2560} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			for _, tc := range []struct {
				fixture string
				action  transcension.SummonAction
				step    ancientStep
			}{
				{"hero-startup-zero.png", transcension.OpenAncientTab, visitAncients},
				{"ascension-ancients.png", transcension.ScrollSummonAncientsUp, scrollAncients},
			} {
				screen := exportFixture(t, tc.fixture, width)
				c, err := recognizedGame(screen)
				if err != nil || !c.known {
					t.Fatal("native fixture context", err)
				}
				f := gameFrame{id: 2, generation: 3, layout: 4, at: time.Unix(1000, 0), image: screen, context: c}
				o := nativeAncientSummonObservation(f)
				if !o.Known || o.OpenKnown || o.ConfirmKnown || o.CancelKnown || len(o.Offers) != 0 || o.Wallet != "" {
					t.Fatal("owned roster invented summon controls", o)
				}
				cmd := transcension.SummonCommand{Action: tc.action, Frame: f.id, Generation: f.generation, Layout: f.layout}
				a, ok := nativeAncientSummonAction(cmd, f)
				if !ok || a.kind != handleAncient || a.ancient.step != tc.step || !nativeAncientSummonActionStable(cmd, a, f) {
					t.Fatal("shared native navigation", a, ok)
				}
				if tc.step == scrollAncients {
					scroll, ok := listScrollAction(a)
					if !ok || scroll.mode != listScrollFine || scroll.direction != -1 {
						t.Fatal("Ancient summon did not use shared list dispatch", scroll, ok)
					}
				}
				for _, change := range []func(*gameFrame){
					func(f *gameFrame) { f.id++ },
					func(f *gameFrame) { f.generation++ },
					func(f *gameFrame) { f.layout++ },
					func(f *gameFrame) { f.context.known = false },
				} {
					latest := f
					change(&latest)
					if nativeAncientSummonActionStable(cmd, a, latest) {
						t.Fatal("stale native navigation retained authority")
					}
				}
				for _, action := range []transcension.SummonAction{transcension.OpenSummonOffers, transcension.ChooseAncientOffer, transcension.ConfirmAncientSummon} {
					cmd.Action = action
					if _, ok := nativeAncientSummonAction(cmd, f); ok {
						t.Fatal("uncalibrated summon input invented", action)
					}
				}
			}
		})
	}
}

func TestAncientSummonDoesNotNavigateThroughOtherModals(t *testing.T) {
	requireAncientOCR(t)
	for _, fixture := range []string{"ancient-quantity.png", "ascension-confirm.png", "transcension-confirm.png", "relic-salvage-dialog.png", "mercenary-quests.png", "save-menu.png"} {
		screen := exportFixture(t, fixture, 1280)
		c, _ := recognizedGame(screen)
		f := gameFrame{id: 1, generation: 1, image: screen, context: c}
		if o := nativeAncientSummonObservation(f); o.Known {
			t.Fatal("modal became Ancient summon navigation", fixture, o)
		}
	}
	if nativeTranscensionEvidence("1.0e12-6144").AncientSummon {
		t.Fatal("owned-Ancient navigation enabled native summon acceptance")
	}
}
