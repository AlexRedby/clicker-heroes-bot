## First Ascension native acceptance

- After Transcension, confirm that passing boss 130 interrupts incomplete hero setup and immediately opens/finishes the first Ascension with a positive reward. If its control is unavailable, confirm that the focused Amenhotep HIRE/MAX path reaches level 150 and exposes it; exact-name/level calibration currently uses native neighboring-name fixtures plus a synthetic target, not an installed-game Amenhotep frame.

## Priority milestone: autonomous Transcension and Outsiders

- Validate the opt-in reset/FEED path in the installed game: a fresh positive reward at a combat wall, unchecked respec, exact Ancient Soul accounting, fixed-quantity sequential purchases, F8/restart reconciliation and return to the zero-soul hero/bootstrap loop. Fixture checks establish software behavior only.
- Calibrate the conservative wall/AS-rate decision against live progression and compare reward/TP/cost estimates with the installed UI. Validate middle Outsider cards, selected fixed quantities and bounded shared scrolling.
- Implement native Ancient summon/offer/confirmation/cancel controls when that unsupported step is reached. The required-only planner and exact Hero Soul receipts exist; owned-Ancient leveling cannot summon missing Ancients. Save a diagnostic and request only the missing screen then. Integrate the unaffordable-offer handoff with ordinary earning and a funded Ascension. Respec remains excluded.

## Automatic gild redistribution native acceptance

- Verify Q + click in the installed roster, the fresh outcome, all-on-target no-op after restart/manual transfers, newly earned gilds, changed target/cycle, insufficient balance/reserve, missed input and F8 recovery. No separate permanent gild history is used.

## Owned dialog interruption checks

- Verify F8 during an owned quantity/Ascension/quest dialog and manual cancellation while a purchase is pending. Never replay a stopped Ancient batch or prestige confirmation. Unsupported dialogs must keep observing without input.

## Current implementation sequence

The main chat coordinates integration, tests and delivery to `main`. Feature owners work in their existing chat worktrees and return reviewed commits; shared CLI/pipeline changes are coordinated by the main chat. Prioritize the complete Transcension/Outsider/restart loop. Timelapse, achievements, Quick Ascension and speed optimization follow this milestone. Native acceptance that needs an unavailable game state remains an explicit gate, not a reason to stop independent implementation.

### 1. Existing features and reproducible bugs

- [ ] Finish Timelapse preparation and paid execution only where native UI evidence exists. Keep purchase execution disabled for missing/ambiguous controls, missing Ancient summons or unverified preparation. Finite allowance and persisted pending debit must survive restart; zero allowance spends nothing. Owner: Timelapse integration and Quick Ascension.
- [ ] Resolve ambiguous dead Mercenary bonus OCR at 640x360 against supplied fixtures if the source preserves enough information. Unknown traits must continue to prevent automatic revive/bury; keep strict row association and paid recovery decisions unchanged. Owner: Automate mercenary quests.
- [ ] Run focused checks for every owner commit, then the integrated native OpenCV/Tesseract suite and build on the authorized Docker host. Transfer source and sanitized test fixtures only; do not send personal saves. Commit and push verified integration to `main`.

### 2. Configurable achievement goals

- [ ] Verify the configured Mercenary goal loop in the installed game: recruitment first, shortest matching valid offer, one fresh export after the collected batch, completion advancing to the next goal and ordinary ruby priorities after all goals. Include restart/profile binding and an unavailable matching offer. The report, bounded save reader, persisted completion and controller preference have code/fixture coverage; native goal acceptance remains open.
- [ ] Implement bounded click, boss-kill, hero-level and zone strategies where native controls and return to ordinary progression are verified. These goals are currently report-only. Unknown counters must not authorize actions; no paid revivals, deliberate Mercenary losses or prestige resets solely for a goal without an explicit policy. Owner: Achievement goals and strategies; main chat integrates shared scheduling.
- [ ] Register exact missing native control evidence for each new execution strategy. Reuse the existing game input queue and save acquisition rather than adding per-row exports or separate input loops.

### 3. Budgeted Quick Ascension

- [ ] Implement native Go reward/readiness calculations and compare them with current-client/reference data. Evaluate useful gains and the late-game QA-rush interval; respect the reward ceiling beyond zone one million. Owner: Timelapse integration and Quick Ascension; coordinate calculator changes with Ancient calculator correctness.
- [ ] Reuse the persisted ruby spending ledger across Timelapse and Quick Ascension. Add opt-in QA execution with a finite combined allowance, protected balance and per-run/Ascension limits; restarting or earning new rubies must not renew authorization. Avoid a second independent spending mechanism.
- [ ] Add owned Ruby Shop/QA modal transitions through the shared input queue, one-shot purchase submission, fresh save outcome and Ancient replanning without resetting heroes/zones. Require actual offer/confirmation/result evidence before enabling paid input; test cancellation, changed price, stale save, pending debit and zero allowance.

Clans are deferred by user request. Forge Core relic upgrades are excluded by user request. Remove resolved checks and checks explicitly retired by the user; keep unfinished work below.

## Save-guided startup native acceptance

- Confirm that initial Save acquisition produces one fresh export before setup and no duplicate export before Ancient planning. A free footer Auto Clicker can still require one direct footer visit; counts and assignments remain UI observations.
- Start with a missing earlier hire/active skill, then with sufficient levels but an unpurchased upgrade: confirm the first case uses the bounded visual preparation and the second goes directly to Buy Available Upgrades. The latest hero's unavailable ordinary upgrades must remain in normal progression.
- Check zero-gold preparation after Ascension using the reset hero snapshot.

## Footer clicker recognition edge cases

- Verify bounded recovery when the free pool is unreadable or Buy Available Upgrades is clipped/obscured near the optional footer deadline. Keep existing assignments intact and continue ordinary automation when no usable target is recognized.

## Opt-in Timelapse preparation and implementation

Owners: Timelapse integration and Quick Ascension (purchase controller and forecast), Ancient calculator correctness (Hybrid allocation), Gild redistribution (hero/gild preparation), Ascension restart and hero bootstrap (Auto Clicker policy and setup handoff). The main chat owns integration and shared pipeline/CLI changes. Existing ordinary progression remains the default.

- Assess readiness from a fresh local export: current/highest zone, rubies, owned Auto Clickers, hero levels/gilds/upgrades, idle and active Ancients, and Outsiders. The user reports two monsters per normal zone already; measure the remaining return-time bottleneck rather than adding more clicking. Distinguish implemented behavior from advisory previews and installed-game acceptance.
- Validate the native Go reference forecast against installed-game outcomes and measured active return speed. Evaluate actual zones/time saved per ruby; the guide's roughly 50k-zone recommendation is an efficiency guideline, not a fabricated unlock gate. The read-only timelapse-plan command does not establish live readiness.
- Supply native controls for summoning any absent Hybrid Ancient. The explicit Hybrid allocation is available; missing summons are reported rather than executed.
- Complete Timelapse-specific hero preparation and connect its forecast-selected target to the current native gild transfer flow. Ordinary progression already transfers to a supported latest hero; it does not establish Timelapse readiness. Keep earned gift opening separate and account for the transfer soul cost.
- Add a bounded preparation/handoff using the existing shared capture and input queue: startup/Ancient completion -> hero and gild preparation -> preserve at least one unassigned Auto Clicker for Nogardnit -> Timelapse -> fresh outcome/export -> resume ordinary active play. Coordinate periodic clicker recovery so it does not immediately consume the reserved clicker. Verify the actual Timelapse idle calculation before introducing any idle countdown.
- Keep paid execution opt-in with `-timelapse` and an explicit finite ruby allowance; zero allowance must never spend. Define a protected balance, a per-Ascension purchase cap and a persisted spending ledger. Newly earned rubies and restarting the program must not replenish the authorized allowance. Reserve the debit before confirming; an interrupted or uncertain purchase must not be replayed automatically. Account for other ruby spenders and profile identity.
- Collect installed-game evidence: Ruby Shop entry and Timelapse offers with durations/prices, selection/confirmation/cancel screens, resulting summary, Gilds roster and transfer controls, and owned/free Auto Clicker states. Browse/reference existing Downloads before requesting missing states. Do not require a paid purchase merely to obtain a screenshot; a controlled paid acceptance run needs an explicit small ruby allowance.
- Gate paid enablement on forecast/reference comparisons, useful/poor-value readiness cases, insufficient rubies/reserve/cap, changed price, restart/pending debit/F8, modal/fish coexistence, and one bounded installed-game purchase with before/after export. Unsupported or uneconomic Timelapse should leave ordinary play running. Record a precise missing-data list if native evidence is unavailable.

## Hero recognition native acceptance

- Run the following cases in the installed game with the integrated boundary reader and production action queue. Capture before/after evidence for any failed transition; fixture tests do not establish native input acceptance.

| Case | Native acceptance gate |
| --- | --- |
| Viewport edges | Complete HIRE/LVL UP captions and upgrade strips at the top, middle and bottom are recognized. Clipped rows are brought into view; overlap advances the bounded sweep. |
| Card variants | Plain/gilded cards, blue/dark captions, dark unavailable slots, gray unlocked artwork, and partial/covered strips. Startup must not read names, levels or purchased checks. |
| Purchases | Startup begins at the bottom and visits rows upward. Owned rows without level-unavailable upgrades receive no levels; missing hires get x1 clicks and unavailable-upgrade rows get Q/MAX, with at most two inputs per viewport, including missed clicks and expanded cards. Ordinary latest-hero Q confirmation still works. |
| Gold and successor | Zero gold, affordable and unaffordable successors, short and growing lists. Starter clicks stop for an affordable purchase, an owned passive hero, pending input or unknown ownership/gold. Startup reads only a locked successor price/gold when needed; numeric level OCR belongs to ordinary latest-hero play. |
| Obstructions | Fish over the caption/level/scrollbar, purchase tooltip and covering modal. Fish recovery stays independent; hero input waits for a covering modal. |
| Navigation and pause | Partial/overlapping rows, missed drag/no motion. Preserve the viewport cursor and attempt count; advance after two inputs instead of looping on one hero. |

- If `hero numbers unreadable` recurs, use the automatically saved `artifacts/hero-unreadable-*.png` and its log containing frame ID, build revision, failed region and OCR output. Collect this analyzed frame rather than a later manual screenshot; reproduce it on the same revision before changing the reader.

## Startup and unattended Ascension validation

- Verify the first post-Transcension Ascension after beating boss 130: a fresh save confirms zero Ascensions in the current Transcension, positive souls allow the reset without waiting for a combat wall, and a validated empty relic inventory skips the missing Relics tab. Confirm later runs retain ordinary wall policy; include F8 and failed-dialog recovery. If the installed game still cancels the first reward dialog, retain its screenshot and the exact reward-read or navigation rejection log; the supplied HUD does not identify that rejection.

- Verify an early growing list after Transcension: recognize its large thumb, reach the complete upgrade footer, visit heroes from bottom to top, give the latest affordable hire an x1 click followed by Q/MAX before earlier skill rows, and revisit unfinished skills while ordinary earning continues.

- Verify bounded startup skill setup in the installed game, including unlocked rows with or without purchased checks, one-slot low-level rows, missing hires, limited gold, two bounded inputs, overlap scrolling and final Buy Available Upgrades. Capture before/after rows if a skill remains locked.
- Verify two consecutive startup/progression cycles with zero gold, short/expanding lists, no-purchase plans, obscured upgrade strips/unreadable prices and fish over controls.
- Extend shared scrolling acceptance beyond the user-observed working cases: current thumb centering, 200 ms source hover, RobotGo smooth drag, 350 ms post-scroll observation settling, partial movement and short no-motion retries. Include Heroes overlap/edges, Ancient wheel/fine arrows and Mercenary edges.
- Verify initial hero/skill upgrade setup at a later zone, including missing earlier heroes, already-disabled Buy Available Upgrades, and an empty Ancient plan that stays on Heroes.
- Obtain a final Ace Scout Dorothy frame before enabling terminal-roster recognition without a successor.

## Remaining feature roadmap

Goal: complete the unattended active-play loop using the existing shared capture, bounded analyzers and serialized input queue. Independent feature work is planned in its own chat; the unattended Ascension milestone is now approved for implementation. Keep working strategies simple: the latest hero, saving for the next hero, native hotkeys/input where available, and no ruby spending outside the authorized Mercenary revival policy or an explicitly enabled and budgeted Timelapse policy.

### Remaining independent gates

| ID | Scope and deliverable | Priority | Completion gate |
| --- | --- | --- | --- |
| R1 | Validate remaining hero startup edge cases and recognize the last hero without a successor. | First | Bounded startup inputs and ordinary Q confirmation, missing-input recovery, and terminal-roster evidence. |
| R2 | Validate conservative Ancient price bounds in the installed game. | First | Compare Juggernaut/Solomon bulk purchases and Chor'gorloth discount/balance rounding to fresh before/after exports; preserve the reserve. |
| R4 | Validate automatic gild redistribution against the installed game. | Next | Fresh distribution/outcome, soul costs/reserve, missed input and restart; keep earned gifts separate. |
| R5 | Combat and boss recovery: validate active/ready skill states, Energize/Reload waves, variable cooldowns, failed-boss retry and the Ascension handoff. | First | Concrete remaining gaps and fixture/live scenarios; reuse current skill/progression/Ascension policies rather than replacing them without evidence. |
| R6 | Native Windows capture, SIFT, OCR and queue measurement with the integrated changes. | First | Collect p50/p95 timing and timeout rate, with real, rotated, small and scrollbar-overlap fish; no win inferred from fewer detections. |
| R7 | Windowed game and display scaling: locate the game viewport, map screenshot coordinates to input, and plan focus/permission handling across Windows/macOS. | Later | Native coordinate evidence; correct ROI translation, unknown-window behavior; full-screen behavior remains supported. |
| R8 | Validate opt-in Transcension/Outsiders and supply missing native Ancient summon controls. | Next | Match reward, TP and costs to UI; exact reset/FEED receipts, no respec, bootstrap and missing-UI diagnostics. |
| R9 | Mercenary extra-life chooser and explicit vacancy/in-flight recruitment tracking in the existing mercenary chat. | Parallel | Verified chooser controls, no paid Hire/Reroll, bounded roster tracking, and no duplicate quest implementation. |

### Integration sequence (owned by the main chat)

1. Close the remaining hero startup and combat checks below. Keep R6 measurement independent from gameplay decisions.
2. Verify pending-purchase interruption and cancellation without replay from a stale plan.
3. Verify the remaining Ascension/startup edge cases: missed input, fish over controls and unsupported dialogs.
4. Validate R4 and R8 in the installed game after the repeated loop works. R7 remains independent platform work.

The detailed unfinished implementation and live-validation gates below remain authoritative. Remove completed work; keep missing screenshot/native-game gates visible.

## Shared observation and action pipeline

Architecture: [Automation pipeline](docs/automation-pipeline.md).

- Recheck live OCR timeout rate and capture/CPU load with single-threaded Tesseract, the configurable `-ocr-timeout` budget and capture cadence measured after completion. Measure p50/p95 fish and purchase response and queue delay with `-stats`; verify foreground process/title support and the visual/F8 fallback on Windows/macOS.
- If full-screen SIFT remains the dominant live delay, evaluate reduced working resolution or OpenCV feature configuration against all real, small/rotated and scrollbar-overlap fish cases before adopting it. Preserve recognition quality and bounded input scheduling.

## Earned gild gifts

- Verify the first zone-100 gift after Transcension with chest animation and the remaining earned-gift edge cases in the installed game: a single reward, a missed input. Confirm recovery closes the roster and resumes automation without repeated opening.

## Automatic progression

- Verify A startup/reset enablement and boss fallback/recovery in the installed game, alongside fish, skills and hero purchases. Capture live boss/farm transitions and calibrate the double-damage/new-combat-buff retry policy and increasing cooldown.

## Active skills

- Verify skill 7 after the Ancient-to-Heroes return at the reported zone 549. Absent-toolbar false ready detection is fixed, but the exact live transition remains unconfirmed; collect the expanded before/after state log and `artifacts/skill-7-unconfirmed.png` if it recurs.

- Verify all states and skill transitions in the installed game, including ready icons after cooldown, continuous energized buffs, the second wave after Reload, Dark Ritual's capped no-op, and simultaneous fish/hero actions.

## Screen and live-game validation

- Verify ordinary Heroes scrolling after an Ancient batch in the installed game. The native top-list fixture now passes the common arrow/thumb detector and queue guard. If recognition still fails, collect `artifacts/hero-scrollbar-unrecognized.png` and the log; a drag followed by a missing bottom confirmation is a separate input/transition failure.

- Verify live recovery when a fish appears over the scrollbar or obscures purchase confirmation: collect it, resume hero actions, and avoid counting the obstruction as a failed purchase. Persistent visible fish should retry after five seconds; vanished fish must not be clicked again. Verify the same recovery on the Ancient scrollbar during a purchase batch: no false missing-row failure, no repeated purchase, and no fish input while a quantity/settings popup covers the game.

- Before implementing the corresponding features, verify the unconfirmed mappings in docs/hotkeys.md against the installed game: Ancient quantity multipliers, and collapse/expand controls.

- Verify repeated Q hero purchases with the 100 ms mouse hold and 100 ms release-settling wait in a running game. Then verify the full hero loop: tooltip dismissal after purchase, confirmation of the increased level, the saving decision and transition to the next available hero. Locked next-hero price and gold OCR are covered by real `x1` screenshot regressions.
- Verify that the faster scrollbar dragging remains reliable on Windows/macOS. Verify bottom-only scrolling, `T` selecting persistent `x1`, `Q` buying MAX levels and returning to `x1`, actual level confirmation and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Verify opt-in `-windowed` on native Windows/macOS: permissions, screenshot-pixel to desktop-coordinate conversion, fish and monster clicks, scrollbar drags and hotkeys. Cover Windows 100/125/150/200% DPI, macOS Retina, secondary displays with negative origins, movement/resize, lost focus and same-title windows. Confirm queued decisions are discarded and an uncertain Ancient purchase is not replayed; check export focus restoration and default primary-display behavior too. Geometry, compact-crop, HUD fixture and pipeline regressions pass; native installed-game acceptance remains open.
- Capture real windowed HUDs and dialogs to validate the client-area/centered/bottom-aligned 16:9 candidates before broadening detection. Unknown, ambiguous, clipped or display-straddling windows must suppress input while observation continues; the Heroes tab is still required for hero actions. After resizing with monster clicks enabled, obtain new crop coordinates and restart.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Obtain the extra-life chooser before enabling it; cards with remaining lives must stay deferred rather than being buried.
- Extend the existing reader/planner with explicit verified-vacancy and recruitment-in-flight tracking. Do not infer a vacancy from a dead card or one visible viewport. Preserve ordinary free recruitment, bounded Heroes return and the existing Revive/Bury policy; paid Hire and Reroll remain excluded.
- Add dedicated recruitment-reward handling if a completion dialog differs from the ordinary Collect/Start flow. Rebuild the saved row plan when recruitment changes the roster, then reuse ordinary quest selection.
- Cover explicit vacancy/in-flight tracking with dead/vacant/full roster, interrupted-completion and unknown-dialog regressions before enabling the additional logic. Unknown layouts must not spend rubies.

## Ancient fresh-save budget acceptance

- Verify a last Ancient purchase close to the displayed balance; preserve the reserve and collect before/after exports for any failed purchase. Spending uses the freshly exported save, without monetary HUD corroboration.

## Native Ancient calculator review

- Verify installed-binary identity and close the Hero Souls spending live gates below. Static web-client arithmetic checks do not establish native purchase acceptance.
- Verify live navigation past a top-clipped Ancient level: keep its name as a navigation anchor, wait for a complete level crop before buying, and retain saved/read diagnostics for a genuine mismatch.

## Prestige and later upgrades

### Automatic Ascension

- Verify recovery from missed input and manual cancellation with the Ascension dialog open.

### Hero Souls spending

- Verify benefit-based residual Morgulis purchases in the installed game with fresh before/after exports. Compare the native Soul damage bonus and billing to the guarded plan, confirm the optional explicit reserve and no repeated significant purchases without new souls. The Active allocation remains an approximate farming model, not a globally optimal damage/gold/skill solver.

- Verify a live large Ancient purchase with six-significant-digit input and the 200 ms entry wait, including closed owned-dialog acknowledgement. Confirm repeated fresh-export plans stop once large non-exponential increases fall below 0.1%.
- Verify Ancient purchase recovery after an open quantity-dialog timeout or a silently failed purchase that closes the dialog: allow underbuy without replay. Supplied frames and regressions cover transaction ownership, stale input and interruption.
- Verify rejection of stale/partial export files and pending Ascension souls during spending decisions; interrupted purchases must not be replayed.
