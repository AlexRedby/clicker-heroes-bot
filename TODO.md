# TODO

## R1 implementation in this worktree

- Connect bootstrapPlanner to verified hero/upgrade readers and the shared action queue: targeted support visits, bounded clickHeroLevels purchases with existing heroRunner confirmation, ordinary upgrade clicks and F8/handoff callbacks. Integration remains owned by the main chat.
- Obtain real initial short-list/long-thumb and first-hire frames before enabling new hero geometry; obtain a final Ace Scout Dorothy frame before enabling terminal-roster recognition.
- Implement and confirm existing purchased Auto Clicker placement after native pool/target evidence is available; preserve the user's upgrades assignment and recognize existing hero/skill ownership. Do not buy clickers, reclaim all targets or use bulk upgrades without verified ASCENSION exclusion.
- Verify post-reset progression and Auto Clicker target/count behavior, and bulk-upgrade exclusion of ASCENSION, before wiring bootstrap into main.go/pipeline.go and removing restart pauses. Shared-file integration remains owned by the main chat.

## Remaining feature roadmap

Goal: complete the unattended active-play loop using the existing shared capture, bounded analyzers and serialized input queue. Each feature first gets an actionable plan in its own chat; implementation follows a separate user request. Keep working strategies simple: the latest hero, saving for the next hero, native hotkeys/input where available, and no ruby spending.

### Independent planning streams

| ID | Scope and deliverable | Priority | Completion gate |
| --- | --- | --- | --- |
| R1 | Ascension restart and hero bootstrap: short lists without a scrollbar, first hero purchases, hero skill/upgrades unlocks including Amenhotep, optional Buy Available Upgrades, existing Auto Clicker target setup, and the last hero without a successor. | First | A plan with ordered startup states and verified UI evidence; no regression to the ordinary latest-hero strategy; F8 and missing-input cases specified. Buy Available Upgrades remains optional when the user already assigns an Auto Clicker to it. |
| R2 | Ancient calculator correctness: exact-budget fits, independent allocation/cost/reserve invariants, Juggernaut/Solomon bulk-cost evidence and decoder cancellation propagation. | First | Concrete reproductions and proposed fixes; distinguish frozen-reference parity from independent correctness; six-digit downward purchase input remains safe. |
| R3 | Relic management: recognize the inventory/full-junk blocker, determine how equipment and junk handling fit before Ascension, and propose a minimal active-build policy. | First | Verified screens or exact missing-input list; explicit equipment and discard policy; unknown items are not silently destroyed. |
| R4 | Gild redistribution: choose a good target consistent with latest-hero progression, calculate transfer cost from current save/state, and plan UI application. | Next | Keep earned gift opening separate; account for soul costs and reserves; no ruby spending and no repeated transfers to the same target. |
| R5 | Combat and boss recovery: validate active/ready skill states, Energize/Reload waves, variable cooldowns, failed-boss retry and the Ascension handoff. | First | Concrete remaining gaps and fixture/live scenarios; reuse current skill/progression/Ascension policies rather than replacing them without evidence. |
| R6 | Capture and fish performance: measure capture, SIFT, OCR and queue delays, then propose only demonstrated reductions in repeated work or latency. | First | Reproducible baseline and quality-preserving experiments over real, rotated, small and scrollbar-overlap fish; no performance win inferred from fewer detections. |
| R7 | Windowed game and display scaling: locate the game viewport, map screenshot coordinates to input, and plan focus/permission handling across Windows/macOS. | Later | Native coordinate evidence; correct ROI translation, unknown-window behavior and F8; full-screen behavior remains supported. |
| R8 | Transcension and Outsiders: determine when Transcension beats another Ascension, decode relevant save fields, and plan Outsider allocation plus UI stages. | Later | Research and preview/recommendation first; automated reset waits for the complete Ascension loop, verified UI evidence and an explicit enabled mode. |
| R9 | Mercenary recovery and recruitment in the existing mercenary chat: current quest loop validation, dead/missing mercenaries and free recruitment. | Parallel | Preserve saved roster plans and the fixed Okay delay; unsupported screens do not spend rubies; no duplicate quest implementation. |

### Integration sequence (owned by the main chat)

1. Accept R1/R2/R3/R5 plans and close the corresponding live gates below. Keep R6 measurement independent from gameplay decisions.
2. Implement ready feature slices in isolated worktrees. Feature chats own feature files; the main chat coordinates changes to main.go, pipeline.go, shared context/assets and root TODO.md so parallel work does not overwrite them.
3. Replace the intentional pauses after reset and the Ancient batch with explicit bootstrap handoffs only after bootstrap, export/spending, required unlocks and relic blocking are reliable. A failed purchase must not be replayed from a stale plan.
4. Verify two complete consecutive cycles: combat wall -> confirmed Ascension -> zone 1 -> fresh export -> Ancient purchases -> bootstrap/upgrades -> existing Auto Clicker targets -> automatic progression. Include F8 during transitions, missed input, fish over controls, Explorer focus restoration and unsupported dialogs. Code/fixture checks do not establish native game acceptance.
5. Add R4 when the repeated loop works. R7 remains independent platform work. Enable R8 automation only after the loop and recommendation/Outsider preview are accepted.

The detailed unfinished implementation and live-validation gates below remain authoritative. Remove completed work; keep missing screenshot/native-game gates visible.

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

- Verify a live large Ancient purchase with six-significant-digit input and the 200 ms entry wait, including level confirmation and F8 interruption.
- Verify V custom-quantity opening in the running Windows game after the added 100 ms key-settling delay, using a fresh export after Atman's unintended level increase.
- Verify one live `-ancients-save` batch with a fresh export, expanded Ancient cards, V custom quantity, actual text entry, visible level confirmation, scrolling through all planned rows and return to Heroes. Include F8 and a missed input; obtain post-purchase frames if level OCR needs calibration. Supplied frames and regressions cover recognition, budget protection, transaction ownership, stale input and interruption.
- Verify live `-export-dir` acquisition on Windows: menu -> Save -> delayed Explorer foreground -> original game focus -> menu close -> fresh file -> current Hero Souls budget check -> Ancient batch. Verify truncated scientific wallet values against the export and use the `saved`/`read` diagnostics on a mismatch; pending Ascension souls must not authorize spending. Check menu closing while Save is highlighted, the configured export folder and filename, F8 during acquisition, a failed Save, blocked OK and the same sequence after Ascension. Automated regressions cover menu recognition, stale/partial exports, cancellation and queue isolation. Initial hero buying, Amenhotep unlock and Auto Clicker setup still gate unattended restart; consider gild transfers and relics after that loop works.

### Transcension

- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
