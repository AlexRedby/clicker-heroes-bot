package bot

import (
	"fmt"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/transcension"
)

// Installed-client acceptance remains unproven until actual receipts arrive.
// Opt-in reset/FEED use recognized current-frame controls, not these flags.
// Summon controls remain unsupported and cannot be inferred from the build.
func nativeTranscensionEvidence(build string) transcension.NativeEvidence {
	return transcension.NativeEvidence{Build: build}
}

// Convert accepted shared-frame readings; do not capture or run a second input
// loop. The main pipeline owns modal classification and stale-job rejection.
func nativeTranscensionObservation(frame gameFrame, ui outsiderObservation, confirmation transcensionObservation, state ancientcalc.TranscensionState) (transcension.Observation, error) {
	out := transcension.Observation{Frame: frame.id, Generation: frame.generation, Layout: frame.layout, At: frame.at}
	same := func(source gameFrame) bool {
		return frame.id != 0 && source.id == frame.id && source.generation == frame.generation && source.layout == frame.layout
	}
	if confirmation.known && same(confirmation.frame) {
		out.Screen, out.Known, out.Reward = transcension.ConfirmationScreen, true, confirmation.reward
		out.ConfirmKnown, out.CancelKnown = confirmation.confirm, confirmation.no
		out.RespecKnown, out.Respec = confirmation.respecKnown, confirmation.respec
		return out, nil
	}
	if ui.known && same(ui.frame) {
		out.Screen, out.Known, out.Wallet, out.Reward = transcension.OutsidersScreen, true, ui.wallet, ui.gain
		out.Quantity = map[string]int{"x1": 1, "x10": 10, "x100": 100, "x1000": 1000}[ui.quantity]
		out.QuantityKnown = outsiderQuantityControlsKnown(frame.image)
		_, out.OpenKnown, _ = transcensionControl(frame.image, transcensionOpen)
		out.AtTop = out.OpenKnown
		if thumb, height, found := listScrollbarThumb(frame.image, 445); found {
			b := frame.image.Bounds()
			out.AtBottom = thumb.Y+height/2 >= b.Min.Y+b.Dy()*955/1000
		}
		for _, row := range ui.rows {
			id := 0
			for _, saved := range state.Outsiders {
				if saved.Name == row.name {
					id = saved.ID
				}
			}
			if id == 0 {
				return transcension.Observation{}, fmt.Errorf("unbound Outsider row %q", row.name)
			}
			_, feedKnown := outsiderFeedPoint(ui, row.name)
			out.Rows = append(out.Rows, transcension.Row{ID: id, Name: row.name, Level: row.level, Cost: row.cost, FeedKnown: feedKnown})
		}
		return out, nil
	}
	// An unrecognized popup must not be converted into a game-entry command.
	c := frame.context
	if c.known && c.heroes && !c.transcension && !c.saveMenu && !c.ancientDialog && !c.ascension && !c.relicJunk && !c.questDialog && !c.mercenaryDialog && c.modal == noGildModal {
		out.Screen, out.Known = transcension.GameScreen, true
		_, out.EntryKnown = outsiderEntryPoint(frame.image)
	}
	return out, nil
}
