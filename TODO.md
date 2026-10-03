# TODO

## Current runtime errors

- Reproduce the ordinary Heroes scrollbar rejection on the native top-list fixture, share the existing merged-arrow handling with startup and queue checks, and verify regular leveling can request the drag after an Ancient batch. Save the failed shared frame if recognition still fails in the installed game.
- Integrate the Mercenary owner fix for CRLF TSV timer coordinates and noisy running-row timer labels; verify supplied log regressions and existing quest scenarios.
- Review skill 7 activation confirmation with the combat owner; fix reproduced defects and retain a specific native evidence gate for unconfirmed behavior.
- Run the combined OCR regression suite and build, review the changed paths, then commit and push the integrated fixes to main.

## Startup and unattended Ascension validation

- Verify native hire/MAX level confirmation and the full affordable hero sweep using the supplied initial frames. If confirmation fails, capture before/after rows and logs; positive post-hire levels in fixture tests use the existing level reader through controlled stubs.
- Verify two consecutive installed-game cycles: combat wall -> confirmed Ascension -> affordable hero sweep -> Buy Available Upgrades -> enabled progression -> fresh export -> submitted Ancient purchases -> Heroes continuation. Include zero gold, short/expanding lists, no-purchase plans, F8, unreadable names/prices and fish over controls. Code/fixture tests are not native acceptance.
- Recognize the owned Auto Clicker pool and place an available clicker on the approved target without buying new clickers or reclaiming existing assignments. Native placement/target evidence remains necessary; startup currently uses one recognized Buy Available Upgrades click through the shared queue.
- Recognize the native relic-junk blocker and block reset on nonempty or unknown junk with a specific reason. The export preflight is advisory; equipment/salvage UI still needs evidence.
- Obtain a final Ace Scout Dorothy frame before enabling terminal-roster recognition without a successor.

## Remaining feature roadmap

Goal: complete the unattended active-play loop using the existing shared capture, bounded analyzers and serialized input queue. Independent feature work is planned in its own chat; the unattended Ascension milestone is now approved for implementation. Keep working strategies simple: the latest hero, saving for the next hero, native hotkeys/input where available, and no ruby spending.

### Remaining independent gates

| ID | Scope and deliverable | Priority | Completion gate |
| --- | --- | --- | --- |
| R1 | Validate the integrated affordable hero/bulk-upgrade restart, place owned Auto Clickers, and recognize the last hero without a successor. | First | Two native cycles, verified post-hire/MAX confirmation and clicker targets, no regression to latest-hero leveling, F8/missing-input recovery, and terminal-roster evidence. |
| R2 | Validate conservative Ancient price bounds in the installed game. | First | Compare Juggernaut/Solomon bulk purchases and Chor'gorloth discount/balance rounding to fresh before/after exports; preserve the reserve. |
| R3 | Relic management: recognize the inventory/full-junk blocker, determine how equipment and junk handling fit before Ascension, and propose a minimal active-build policy. | First | Verified screens or exact missing-input list; explicit equipment and discard policy; unknown items are not silently destroyed. |
| R4 | Gild redistribution: choose a good target consistent with latest-hero progression, calculate transfer cost from current save/state, and plan UI application. | Next | Keep earned gift opening separate; account for soul costs and reserves; no ruby spending and no repeated transfers to the same target. |
| R5 | Combat and boss recovery: validate active/ready skill states, Energize/Reload waves, variable cooldowns, failed-boss retry and the Ascension handoff. | First | Concrete remaining gaps and fixture/live scenarios; reuse current skill/progression/Ascension policies rather than replacing them without evidence. |
| R6 | Native Windows capture, SIFT, OCR and queue measurement with the integrated changes. | First | Collect p50/p95 timing and timeout rate, with real, rotated, small and scrollbar-overlap fish; no win inferred from fewer detections. |
| R7 | Windowed game and display scaling: locate the game viewport, map screenshot coordinates to input, and plan focus/permission handling across Windows/macOS. | Later | Native coordinate evidence; correct ROI translation, unknown-window behavior and F8; full-screen behavior remains supported. |
| R8 | Transcension and Outsiders: validate the read-only preview against the installed UI and implement gated native stages. | Later | Match reward, TP, costs and respec semantics to UI; automated reset waits for the complete Ascension loop and an explicit enabled mode. |
| R9 | Mercenary recovery and recruitment in the existing mercenary chat: current quest loop validation, dead/missing mercenaries and free recruitment. | Parallel | Preserve saved roster plans and the fixed Okay delay; unsupported screens do not spend rubies; no duplicate quest implementation. |

### Integration sequence (owned by the main chat)

1. Validate the integrated hero startup/combat/Ancient handoff and close the remaining Auto Clicker/relic native UI gates below. Keep R6 measurement independent from gameplay decisions.
2. Implement owned Auto Clicker placement and native relic interactions in isolated worktrees; coordinate shared main.go/pipeline.go changes in the main chat.
3. Retain bounded failure pauses and validate startup/export/spending handoffs. A failed purchase must not be replayed from a stale plan.
4. Verify two complete consecutive cycles: combat wall -> confirmed Ascension -> zone 1 -> affordable hero sweep -> Buy Available Upgrades -> enabled progression -> fresh export -> Ancient purchases -> Heroes continuation. Include F8 during transitions, missed input, fish over controls, Explorer focus restoration and unsupported dialogs. Code/fixture checks do not establish native game acceptance.
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

- Verify live recovery when a fish appears over the scrollbar or obscures purchase confirmation: collect it, resume hero actions, and avoid counting the obstruction as a failed purchase. Persistent visible fish should retry after five seconds; vanished fish must not be clicked again. Verify the same recovery on the Ancient scrollbar during a purchase batch: no false missing-row failure, no repeated purchase, and no fish input while a quantity/settings popup covers the game.

- Before implementing the corresponding features, verify the unconfirmed mappings in docs/hotkeys.md against the installed game: Ancient quantity multipliers, and collapse/expand controls.

- Verify repeated Q hero purchases with the 100 ms mouse hold and 100 ms release-settling wait in a running game, including F8 pause/resume. Then verify the full hero loop: tooltip dismissal after purchase, confirmation of the increased level, the saving decision and transition to the next available hero. Locked next-hero price and gold OCR are covered by real `x1` screenshot regressions.
- Verify that the faster scrollbar dragging remains reliable on Windows/macOS and that F8 taking effect after an ongoing native drag finishes is acceptable. Verify bottom-only scrolling, `T` selecting persistent `x1`, `Q` buying MAX levels and returning to `x1`, actual level confirmation, F8 interruption/resume, and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Verify opt-in `-windowed` on native Windows/macOS: permissions, screenshot-pixel to desktop-coordinate conversion, fish and monster clicks, scrollbar drags, hotkeys and F8. Cover Windows 100/125/150/200% DPI, macOS Retina, secondary displays with negative origins, movement/resize, lost focus and same-title windows. Confirm queued decisions are discarded and an uncertain Ancient purchase is not replayed; check export focus restoration and default primary-display behavior too. Geometry, compact-crop, HUD fixture and pipeline regressions pass; native installed-game acceptance remains open.
- Capture real windowed HUDs and dialogs to validate the client-area/centered/bottom-aligned 16:9 candidates before broadening detection. Unknown, ambiguous, clipped or display-straddling windows must remain paused; the Heroes tab is still required for hero actions. After resizing with monster clicks enabled, obtain new crop coordinates and restart.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Run the existing `-mercenaries` loop on the target device before changing its timing: four/five completed quests, mixed Collect/Start Quest/running rows, animated notification, paired Collect/Start clicks, fixed Okay after 300 ms, one bounded scroll sweep and return to Heroes. Include F8 during the Okay delay/scroll, fish between rows and unreadable offers; capture frames/logs for any failure. Preserve the saved visible-row plan without intermediate roster OCR or countdown confirmation.
- Obtain verified game frames for dead cards with Revive/Bury, an actually vacant slot, an all-dead/empty roster, a free recruitment offer, its completed reward, any completion dialog and the new mercenary's Start Quest row. Existing reviewed Mercenary fixtures/Downloads cover ordinary quests, not these transitions. Do not infer a vacancy from a dead card or from the number of rows visible in one viewport.
- After those frames are available, extend the existing reader/planner to skip dead cards explicitly, track verified vacancies and recruitment already in flight, and choose free recruitment only for an available slot. Specify a bounded return to Heroes when no living mercenary or verified free action is available. Keep Revive, Bury, paid Hire and Reroll outside automatic actions; unknown layouts must not spend rubies.
- After the recruitment-completion UI is verified, handle its reward separately from the ordinary same-position Collect/Start pair. Rebuild the saved row plan only when recruitment changes the roster or opens a new UI state, then reuse the existing quest-selection loop for the new mercenary.
- Gate replenishment on real-frame OCR/planner regressions for dead/vacant/full rosters, recruitment already in flight, interrupted completion and unknown dialogs, followed by one native free-recruitment cycle. Until the missing frames and native run are available, leave further Mercenary production changes pending.

## Native Ancient calculator review

- Diagnose the live Dora exported-level mismatch during upward Argaiv seeking using the new stall frame; verify name/level row association and clipping before changing pre-purchase checks. Reproduce the root cause in a regression, review and integrate the fix.
- Verify installed-binary identity and close the Hero Souls spending live gates below. Static web-client arithmetic checks do not establish native purchase acceptance.
- Verify live navigation past a top-clipped Ancient level: keep its name as a navigation anchor, wait for a complete level crop before buying, and retain saved/read diagnostics for a genuine mismatch.

## Prestige and later upgrades

### Automatic Ascension

- Verify missed input, manual cancellation and F8 with the Ascension dialog open, followed by the integrated zone-1 hero/bulk-upgrade/export handoff. The user observed one successful full-combat-loss reset; that does not establish two consecutive unattended cycles.
- Capture the relic-junk blocking screen and recognize/report it specifically; keep automatic relic destruction/equipment outside this mode. Unknown or blocked transitions currently time out to a pause.

### Hero Souls spending

- Verify a live large Ancient purchase with six-significant-digit input and the 200 ms entry wait, including closed owned-dialog acknowledgement and F8 interruption. Confirm repeated fresh-export plans stop once large non-exponential increases fall below 0.1%.
- Verify V custom-quantity opening in the running Windows game after the added 100 ms key-settling delay, using a fresh export after Atman's unintended level increase.
- Verify one live `-ancients-save` batch with a fresh export, expanded Ancient cards, V custom quantity, actual text entry, one-shot OK submission acknowledged by a newer ordinary Ancients frame, fresh level/budget checks before each next purchase, one-notch wheel seeking by canonical alphabetical names, name-confirmed movement, bounded wheel-to-arrow recovery and scrollbar-arrow correction for clipped/bracketed/overshot rows and return to Heroes. Include F8, an open dialog timeout and a silently failed purchase that closes the dialog (allowed underbuy, no replay). Supplied frames and regressions cover recognition, budget protection, transaction ownership, stale input and interruption.
- Verify live `-export-dir` acquisition on Windows: menu -> Save -> delayed Explorer foreground -> original game focus -> menu close -> fresh file -> current Hero Souls budget check -> Ancient batch. Verify truncated scientific wallet values against the export and use the `saved`/`read` diagnostics on a mismatch; pending Ascension souls must not authorize spending. Check menu closing while Save is highlighted, the configured export folder and filename, F8 during acquisition, a failed Save, blocked OK and the same sequence after Ascension. Automated regressions cover menu recognition, stale/partial exports, cancellation and queue isolation. Validate the integrated hero sweep/bulk-upgrade handoff and remaining Auto Clicker/relic gates before treating repeated restart cycles as native acceptance.

### Transcension

- Match the fresh `-mode transcension-plan` preview to the installed game's reward/TP display, Outsider costs and respec/refund semantics. Capture reward confirmation, respec controls, one small purchase before/after, immediate manual-reset HUD/export and first-run Ancient summon controls.
- Enable native Transcension and Outsider spending only after two complete consecutive Ascension/restart cycles, a confirmed combat wall and active-play timing evidence, verified reset recovery and an explicit enabled mode. The current preview is informational; save timestamps do not establish a reset decision.
