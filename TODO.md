# TODO

## Screen and live-game validation

- Verify the corrected hero loop in a running game: tooltip dismissal after purchase, confirmation of the increased level, the saving decision and transition to the next available hero. Locked next-hero price and gold OCR are covered by real `x1` screenshot regressions.
- Verify that the game registers RobotGo's standard bottom-edge drag and that F8 taking effect after an ongoing native drag finishes is acceptable. Verify bottom-only scrolling, `x1`/`MAX` selected states, actual level confirmation, F8 interruption/resume, and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Measure complete hero/fish cycle latency on the target machine. OCR is cancellable and bounded per read, but the serial cycle can exceed the configured fish interval; tune only after measuring the live workload.
- Resolve macOS capture/input permissions and verify screenshot-pixel to desktop-point conversion, fish clicks, optional monster clicks, and F8 on Windows/macOS with display scaling and multiple displays. Capture and CLI coordinates currently target the primary display.
- Add recognition of a game viewport inside a window before supporting non-full-screen layouts. The current bot requires a recognizable full-screen Heroes layout and skips unknown layouts.
- Support short hero lists without a scrollbar or with a thumb outside the current detector's accepted height range. Capture real examples before changing the detector; a wider height allowance can mistake the gold track border for a thumb.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Detect completed quests, claim their rewards, and send available mercenaries on new quests.
- Choose quests by reward and duration; leave ruby spending and revivals for a separate decision.

## Prestige and later upgrades

- Read expected Hero Souls and recommend Ascension when progress stalls; verify the full restart loop before enabling automatic Ascension.
- Add Ancient spending after Ascension, then consider gilds, relics, and active skills.
- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
