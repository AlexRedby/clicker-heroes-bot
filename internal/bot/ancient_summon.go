package bot

import (
	"image"

	"clicker-heroes-bot/internal/transcension"
)

// Only the existing native tab and list arrow are calibrated. No available
// fixture shows the summon entry, offers, price, or confirmation. This adapter
// must not synthesize their observations from owned-Ancient leveling controls.
func nativeAncientSummonObservation(frame gameFrame) transcension.SummonObservation {
	out := transcension.SummonObservation{Frame: frame.id, Generation: frame.generation, Layout: frame.layout, At: frame.at}
	c := frame.context
	if frame.image == nil || !c.known || c.saveMenu || c.ancientDialog || c.ascension || c.relicJunk || c.questDialog || c.mercenaryDialog || c.modal != noGildModal {
		return out
	}
	if c.heroes && heroTabSelected(frame.image) {
		out.Screen, out.Known = transcension.SummonGameScreen, true
	} else if c.ancients && ancientTabSelected(frame.image) {
		out.Screen, out.Known = transcension.SummonAncientsScreen, true
		_, out.UpKnown, _ = ancientScrollArrow(frame.image, -1)
	}
	return out
}

// Reuse the shared Ancient navigation action and listScrollAction dispatch.
// The caller must also recheck planner.Allowed at dequeue and Reserve before
// input. Unsupported summon actions have no coordinates or input path.
func nativeAncientSummonAction(cmd transcension.SummonCommand, frame gameFrame) (gameAction, bool) {
	if frame.id == 0 || cmd.Frame != frame.id || cmd.Generation != frame.generation || cmd.Layout != frame.layout {
		return gameAction{}, false
	}
	o := nativeAncientSummonObservation(frame)
	a := gameAction{kind: handleAncient, frame: frame}
	switch cmd.Action {
	case transcension.OpenAncientTab:
		if o.Screen != transcension.SummonGameScreen || !o.Known {
			return gameAction{}, false
		}
		a.ancient.step, a.point = visitAncients, ancientTabPoint(frame.image, false)
	case transcension.ScrollSummonAncientsUp:
		if o.Screen != transcension.SummonAncientsScreen || !o.Known || !o.UpKnown {
			return gameAction{}, false
		}
		a.ancient.step, a.ancient.direction, a.ancient.fine = scrollAncients, -1, true
		a.point, _, _ = ancientScrollArrow(frame.image, -1)
	default:
		return gameAction{}, false
	}
	return a, a.point != (image.Point{})
}

func nativeAncientSummonActionStable(cmd transcension.SummonCommand, action gameAction, current gameFrame) bool {
	latest, ok := nativeAncientSummonAction(cmd, current)
	return ok && action.frame.id == current.id && action.frame.generation == current.generation && action.frame.layout == current.layout && action.frame.context == current.context && action.point == latest.point && action.kind == latest.kind && action.ancient.step == latest.ancient.step && action.ancient.direction == latest.ancient.direction && action.ancient.fine == latest.ancient.fine
}
