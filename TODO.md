# TODO

## Shared observation and action pipeline

Architecture: [Automation pipeline](docs/automation-pipeline.md).

- Recheck live OCR timeout rate and capture/CPU load with single-threaded Tesseract, the configurable `-ocr-timeout` budget and capture cadence measured after completion. Measure p50/p95 fish and purchase response and queue delay with `-stats`; verify foreground process/title support and the visual/F8 fallback on Windows/macOS.
- If full-screen SIFT remains the dominant live delay, evaluate reduced working resolution or OpenCV feature configuration against all real, small/rotated and scrollbar-overlap fish cases before adopting it. Preserve recognition quality and bounded input scheduling.

## Earned gild gifts

- Verify the full `-gilds` batch in the running game: gift -> central chest -> Open All -> close the Gilded Heroes roster; include a single reward, missed input and F8 while a modal is open. Screenshot recognition, queue isolation, retries and pause/resume are covered by regression tests.

## Automatic progression

- Verify A startup/reset enablement and boss fallback/recovery in the installed game, alongside fish, skills and hero purchases. Capture live boss/farm transitions and calibrate the double-damage/new-combat-buff retry policy and increasing cooldown.

## Active skills

- Verify all states and skill transitions in the installed game, including ready icons after cooldown, continuous energized buffs, the second wave after Reload, Dark Ritual's capped no-op, and simultaneous fish/hero actions.

## Screen and live-game validation

- Reproduce the reported failure to scroll from the top of the list using an actual pre-drag screenshot. Distinguish `hero scrollbar not recognized` from an emitted `dragged hero scrollbar` followed by missing bottom confirmation. The queue regression covers a relocated real thumb with slow SIFT, but does not establish that the reported live geometry is recognized.

- Verify live recovery when a fish appears over the scrollbar or obscures purchase confirmation: collect it, resume hero actions, and avoid counting the obstruction as a failed purchase. Persistent visible fish should retry after five seconds; vanished fish must not be clicked again.

- Before implementing the corresponding features, verify the unconfirmed mappings in docs/hotkeys.md against the installed game: Ancient quantity multipliers, and collapse/expand controls.

- Verify repeated Q hero purchases with the 100 ms mouse hold and 100 ms release-settling wait in a running game, including F8 pause/resume. Then verify the full hero loop: tooltip dismissal after purchase, confirmation of the increased level, the saving decision and transition to the next available hero. Locked next-hero price and gold OCR are covered by real `x1` screenshot regressions.
- Verify that the faster scrollbar dragging remains reliable on Windows/macOS and that F8 taking effect after an ongoing native drag finishes is acceptable. Verify bottom-only scrolling, `T` selecting persistent `x1`, `Q` buying MAX levels and returning to `x1`, actual level confirmation, F8 interruption/resume, and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Resolve macOS capture/input permissions and verify screenshot-pixel to desktop-point conversion, fish clicks, optional monster clicks, and F8 on Windows/macOS with display scaling and multiple displays. Capture and CLI coordinates currently target the primary display.
- Add recognition of a game viewport inside a window before supporting non-full-screen layouts. The current bot requires a recognizable full-screen game HUD, and the Heroes tab for hero actions; it skips unknown layouts.
- Support short hero lists without a scrollbar or with a thumb outside the current detector's accepted height range. Capture real examples before changing the detector; a wider height allowance can mistake the gold track border for a thumb.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Verify the live `-mercenaries` loop: notification, saved visible-row plan, paired Collect/Start Quest clicks, offer selection, fixed Okay after 300 ms, faster scrolling to the fifth mercenary, returning to Heroes and F8 interruption/resume alongside fish and skills. Supplied screenshots and regression tests cover initial roster/offer OCR, four/five-mercenary sweeps without intermediate roster OCR, unreadable offers, interruptions and transient context recovery; native timing still needs a running game on the target device.
- Capture real death and free-recruitment screens; verify that Revive/Bury remain untouched and recruitment quests are recognized before relying on automatic roster replenishment. Unsupported layouts must end the visit without ruby spending.

## Native Ancient calculator review

- Fix exact-budget allocations in `internal/ancientcalc/calculator.go`: search uses `spent < available` and rejects a fitting plan. Reproduction: Fragsworth=1, Morgulis=1, wallet=5, reserve=0, skill rate=0 returns no purchases; Fragsworth=2 and Morgulis=4 cost exactly 5. Add an independent regression, then rerun the frozen-reference cases and review intended output changes.
- Verify the installed game's custom-quantity cost for Juggernaut/Solomon before treating `polynomial1_5` estimates as purchase budgets. The inherited cumulative approximation gives 143 at level 10 versus 146 for the sum of individually rounded level costs; a wallet=83 plan reports spending 82 but individually priced levels cost 84. Compare bulk pricing and Chor'gorloth rounding against the client or an observed purchase; keep the reserve safe without iterating through enormous levels.
- Add independent allocation invariants and small-budget cases alongside reference parity: affordable exact fits, row quantity versus target delta, row costs versus total spending, and remaining souls versus reserve. Frozen outputs alone also preserve upstream mistakes.
- Preserve context cancellation when `zlib.NewReader` fails in `internal/ancientcalc/save.go`; a canceled context currently becomes `invalid compressed save`. Add a decoder regression that covers cancellation during initialization.

## Prestige and later upgrades

### Automatic Ascension

- Verify one live opt-in `-ascension` reset: the observed full-combat boss loss (or fallback stall), meaningful Hero Souls gain using the chosen bank/export baseline, red spiral -> independently verified Hero Souls reward -> green Yes -> zone 1 -> automatic pause. Test a missed click, manual cancellation and F8 with the dialog open. Real-frame recognition, modal isolation, reward changes and decision cancellation are covered by regression tests.
- Capture the immediate post-reset HUD to verify zone-1 OCR and initial controls in the installed game. The first implementation pauses after reset; complete initial short-list hero buying, Amenhotep's Ascension unlock and Auto Clicker target setup before enabling repeated unattended runs.
- Capture the relic-junk blocking screen and recognize/report it specifically; keep automatic relic destruction/equipment outside this mode. Unknown or blocked transitions currently time out to a pause.
- Verify the Ascension/restart stages and fresh-export spending live before enabling their unattended integration.

### Hero Souls spending

- Verify one live `-ancients-save` batch with a fresh export, expanded Ancient cards, V custom quantity, actual text entry, visible level confirmation, scrolling through all planned rows and return to Heroes. Include F8 and a missed input; obtain real filled-quantity and post-purchase frames if OCR needs calibration. Supplied frames and regressions cover recognition, budget protection, transaction ownership, stale input and interruption.
- Acquire a fresh exported state after each confirmed Ascension and integrate spending only after live validation. The current command runs one batch from a user export and then pauses; repeated unattended resets still require initial hero buying, Amenhotep unlock and Auto Clicker setup. Consider gild transfers and relics after that loop works.

### Transcension

- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
