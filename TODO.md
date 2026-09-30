# TODO

## Shared observation and action pipeline

Architecture: [Automation pipeline](docs/automation-pipeline.md).

- Measure live capture latency, p50/p95 fish and purchase response, CPU/memory and queue delay on the installed game with `-stats`; verify foreground process/title support and the visual/F8 fallback on Windows/macOS.
- If full-screen SIFT remains the dominant live delay, evaluate reduced working resolution or OpenCV feature configuration against all real, small/rotated and scrollbar-overlap fish cases before adopting it. Preserve recognition quality and bounded input scheduling.

## Automatic progression

- Verify A startup/reset enablement and boss fallback/recovery in the installed game, alongside fish, skills and hero purchases. Capture live boss/farm transitions and calibrate the double-damage/new-combat-buff retry policy and increasing cooldown.

## Active skills

- Verify all states and skill transitions in the installed game, including ready icons after cooldown, continuous energized buffs, the second wave after Reload, Dark Ritual's capped no-op, and simultaneous fish/hero actions.

## Screen and live-game validation

- Verify live recovery when a fish appears over the scrollbar or obscures purchase confirmation: collect it, resume hero actions, and avoid counting the obstruction as a failed purchase. Persistent visible fish should retry after five seconds; vanished fish must not be clicked again.

- Before implementing the corresponding features, verify the unconfirmed mappings in docs/hotkeys.md against the installed game: Ancient quantity multipliers, and collapse/expand controls.

- Verify repeated Q hero purchases with the 100 ms mouse hold and 100 ms release-settling wait in a running game, including F8 pause/resume. Then verify the full hero loop: tooltip dismissal after purchase, confirmation of the increased level, the saving decision and transition to the next available hero. Locked next-hero price and gold OCR are covered by real `x1` screenshot regressions.
- Verify that the faster 0.5-1.5 ms step delays remain reliable and that F8 taking effect after an ongoing native drag finishes is acceptable. Verify bottom-only scrolling, `T` selecting persistent `x1`, `Q` buying MAX levels and returning to `x1`, actual level confirmation, F8 interruption/resume, and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Resolve macOS capture/input permissions and verify screenshot-pixel to desktop-point conversion, fish clicks, optional monster clicks, and F8 on Windows/macOS with display scaling and multiple displays. Capture and CLI coordinates currently target the primary display.
- Add recognition of a game viewport inside a window before supporting non-full-screen layouts. The current bot requires a recognizable full-screen game HUD, and the Heroes tab for hero actions; it skips unknown layouts.
- Support short hero lists without a scrollbar or with a thumb outside the current detector's accepted height range. Capture real examples before changing the detector; a wider height allowance can mistake the gold track border for a thumb.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Detect completed quests, claim their rewards, and send available mercenaries on new quests.
- Choose quests by reward and duration; leave ruby spending and revivals for a separate decision.

## Prestige and later upgrades

- Read expected Hero Souls and recommend Ascension when progress stalls; verify the full restart loop before enabling automatic Ascension.
- Add Ancient spending after Ascension, then consider gilds and relics.
- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
