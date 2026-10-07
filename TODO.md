# TODO

## Current implementation sequence

The main chat coordinates integration, tests and delivery to `main`. Feature owners work in their existing chat worktrees and return reviewed commits; shared CLI/pipeline changes are coordinated by the main chat. Finish actionable existing work before enabling achievement goals, then Quick Ascension. Native acceptance that needs an unavailable game state remains an explicit gate, not a reason to stop independent implementation.

### 1. Existing features and reproducible bugs

- [ ] Integrate the prepared Hybrid Ancient API, native Go Timelapse forecast/controller, persisted finite ruby ledger and forecast-bound gild preview. Reconcile current calculator/export APIs; preserve ordinary Active progression. Expose a usable read-only preparation report with missing prerequisites and exact unsupported cases. Owners: Ancient calculator correctness, Timelapse strategy and automation, Gild redistribution; main chat integrates.
- [ ] Finish Timelapse preparation and paid execution only where native UI evidence exists. Keep purchase execution disabled for missing/ambiguous controls, missing Ancient summons or unverified preparation. Finite allowance and persisted pending debit must survive restart; zero allowance spends nothing. Owner: Timelapse strategy and automation.
- [ ] Measure and improve fish detection on all saved real, rotated, small and scrollbar-overlap frames. Compare working resolution and OpenCV parameters; ship only a measured improvement without lost detections. Add useful p50/p95 capture, analyzer, OCR and queue metrics with bounded storage, plus reasons for idle action scheduling. Owner: Capture and fish performance; main chat wires shared metrics/CLI.
- [ ] Fix the known 640x360 Mercenary Collect/Callect recognition failure using the existing native fixture and focused negative cases. Reuse current visual templates/OCR crops and preserve strict row association. Continue recruitment/recovery only for supplied UI states. Owner: Automate mercenary quests.
- [ ] Recheck remaining startup/footer/Auto Clicker and combat handoff defects against current main and saved fixtures; fix reproducible defects, and identify the precise native states still missing. Do not request deliberate F8 corner-case testing. Owners: Ascension restart and hero bootstrap, Combat and boss recovery.
- [ ] Run focused checks for every owner commit, then the integrated native OpenCV/Tesseract suite and build on the authorized Docker host. Transfer source and sanitized test fixtures only; do not send personal saves. Commit and push verified integration to `main`.

### 2. Configurable achievement goals

- [ ] Read current earned achievements and supported progress counters from exported saves; pin requirements to the installed-client catalog and distinguish internal achievements from Steam mappings. Legacy reward text and mobile currency fields do not prove current desktop rewards.
- [ ] Add a small explicit goal configuration and progress report. Start with Mercenary quest type/count and five-minute quest targets using the existing quest policy; after completion restore normal ruby priorities. Then add supported click, boss-kill, hero-level and zone goals with bounded strategy overrides. Owner: Achievement goals and strategies; main chat integrates CLI/pipeline.
- [ ] Preserve selected goals and completion across restart; unknown counters do not authorize actions. Keep ordinary latest-hero progression unchanged unless an explicit goal requests another strategy. Test already-complete goals, conflicting goals, unsupported IDs, partial progress and restoration after completion. No paid revivals, deliberate Mercenary losses or prestige resets solely for a goal without an explicit policy.
- [ ] Gate each new execution strategy on the available native control evidence. Register missing inputs precisely while delivering the counter reader, policy and report independently.

### 3. Budgeted Quick Ascension

- [ ] Implement native Go reward/readiness calculations and compare them with current-client/reference data. Evaluate useful gains and the late-game QA-rush interval; respect the reward ceiling beyond zone one million. Owner: Timelapse strategy and automation; coordinate calculator changes with Ancient calculator correctness.
- [ ] Reuse the persisted ruby spending ledger across Timelapse and Quick Ascension. Add opt-in QA execution with a finite combined allowance, protected balance and per-run/Ascension limits; restarting or earning new rubies must not renew authorization. Avoid a second independent spending mechanism.
- [ ] Add owned Ruby Shop/QA modal transitions through the shared input queue, one-shot purchase submission, fresh save outcome and Ancient replanning without resetting heroes/zones. Require actual offer/confirmation/result evidence before enabling paid input; test cancellation, changed price, stale save, pending debit and zero allowance.

Clans are deferred by user request. Forge Core relic upgrades are excluded by user request. Existing remaining native acceptance sections below still apply; remove only work that is actually completed and verified.

## Automatic relic equipment native acceptance

- During ordinary play, verify that a yellow Relics notification leads to a bounded equipment visit, an accepted native drag for a clear Active improvement and a return to Heroes. Retain the relevant log and before/after frames if a move fails; fixture tests do not prove installed-game drag acceptance.
- Verify equipment selection before an ordinary automatic Ascension and preservation of equipped items through its Junk Pile salvage. The current reader supports up to six visible junk cards in the first row; larger, obscured or ambiguous inventories defer Ascension without destroying items. Extend the reader when an actual unsupported layout is supplied.

## Save-guided startup native acceptance

- Confirm that initial Save acquisition produces one fresh export before setup and no duplicate export before Ancient planning. A free footer Auto Clicker can still require one direct footer visit; counts and assignments remain UI observations.
- Start with a missing earlier hire/active skill, then with sufficient levels but an unpurchased upgrade: confirm the first case uses the bounded visual preparation and the second goes directly to Buy Available Upgrades. The latest hero's unavailable ordinary upgrades must remain in normal progression.
- After a confirmed Ascension, check zero-gold preparation and a new post-startup export; the earlier hero snapshot and Ancient plan must not carry over.

## Cid and footer clicker native acceptance

- Verify recovery to a complete Buy Available Upgrades control after a missed drag and a final drag near the optional pass deadline. The last free clicker must reach the footer when available, and an obscured control must still allow ordinary automation to continue.
- Verify the dim-artwork reader and conditional second skill sweep after Ascension: with initially limited gold, hire a passive hero, revisit Cid, level him and buy skill 1 through Buy Available Upgrades. Capture the full startup log and before/after frames; fixture tests do not establish native input acceptance or sufficient gold during the second pass.
- Verify that a freshly recognized free Auto Clicker reaches Buy Available Upgrades despite an unconfirmed monster placement or delayed pool recognition. Include an unreadable pool; the optional footer pass must remain bounded and ordinary automation must continue.
- If all three clickers again end up on the monster, retain the analyzed `artifacts/auto-clicker-unconfirmed-*.png` and expected/read pool log. Distinguish bot input from manual assignments; the previous screenshot was taken 29 minutes after startup and does not prove one input assigned all three.

## Opt-in Timelapse preparation and implementation

Owners: Timelapse strategy and automation (purchase controller and forecast), Ancient calculator correctness (Hybrid allocation), Gild redistribution (hero/gild preparation), Ascension restart and hero bootstrap (Auto Clicker policy and setup handoff). The main chat owns integration and shared pipeline/CLI changes. Existing ordinary progression remains the default.

- Assess readiness from a fresh local export: current/highest zone, rubies, owned Auto Clickers, hero levels/gilds/upgrades, idle and active Ancients, and Outsiders. The user reports two monsters per normal zone already; measure the remaining return-time bottleneck rather than adding more clicking. Distinguish implemented behavior from advisory previews and installed-game acceptance.
- Implement and compare a native Go Timelapse forecast with the maintained community calculator, checking its formulas against the installed version. Evaluate actual zones/time saved per ruby; the guide's roughly 50k-zone recommendation is an efficiency guideline, not a fabricated unlock gate. Do not depend on a JavaScript sidecar or a remote calculator API.
- Extend the Active-only Ancient calculator with an explicit Hybrid policy for Timelapse readiness. Model Siyalatas, Libertas and Nogardnit with the relevant Outsider effects and retained-Hero-Soul benefit; review the reference formulas instead of guessing allocation ratios. Keep existing Active results unchanged when Timelapse is disabled. Summoning an absent Ancient is a separate verified UI dependency.
- Complete hero/gild preparation: choose and level a suitable Timelapse hero, calculate and apply Hero-Soul-priced gild transfers only when useful, and obtain the actual native transfer controls. Earned gild gift opening does not redistribute existing gilds. The current gild preview supports a limited roster range and does not execute transfers.
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
| Purchases | Startup begins at the top. Owned rows without level-unavailable upgrades receive no levels; missing hires and unavailable-upgrade rows get at most two Q/MAX inputs per viewport, including missed clicks and expanded cards. Ordinary latest-hero Q confirmation still works. |
| Gold and successor | Zero gold, affordable and unaffordable successors, short and growing lists. Starter clicks stop for an affordable purchase, an owned passive hero, pending input or unknown ownership/gold. Startup reads only a locked successor price/gold when needed; numeric level OCR belongs to ordinary latest-hero play. |
| Obstructions | Fish over the caption/level/scrollbar, purchase tooltip and covering modal. Fish recovery stays independent; hero input waits for a covering modal. |
| Navigation and pause | Partial/overlapping rows, missed drag/no motion. Preserve the viewport cursor and attempt count; advance after two inputs instead of looping on one hero. |

- If `hero numbers unreadable` recurs, use the automatically saved `artifacts/hero-unreadable-*.png` and its log containing frame ID, build revision, failed region and OCR output. Collect this analyzed frame rather than a later manual screenshot; reproduce it on the same revision before changing the reader.
- Complete the two consecutive Ascension/setup cycles tracked below. Request additional screenshots only for a UI state absent from the existing fixtures or automatically saved failure evidence.

## Startup and unattended Ascension validation

- Verify bounded startup skill setup in the installed game, including unlocked rows with or without purchased checks, one-slot low-level rows, missing hires, limited gold, two Q/MAX inputs, overlap scrolling and final Buy Available Upgrades. Capture before/after rows if a skill remains locked.
- Verify two consecutive installed-game cycles: combat wall -> confirmed Ascension -> bounded skill sweep -> newly available upgrades -> enabled progression -> fresh export -> submitted Ancient purchases -> Heroes continuation. Include zero gold, short/expanding lists, no-purchase plans, obscured upgrade strips/unreadable prices and fish over controls. Code/fixture tests are not native acceptance.
- Verify five-minute Auto Clicker recovery in the installed game: remove a monster/footer assignment, check the freshly recognized free pool restores it, and test missed C input and an unavailable footer. Count confirmation does not establish which target the user assigned manually.
- Extend shared scrolling acceptance beyond the user-observed working cases: current thumb centering, 200 ms source hover, RobotGo smooth drag, 350 ms post-scroll observation settling, partial movement and short no-motion retries. Include Heroes overlap/edges, Ancient wheel/fine arrows and Mercenary edges.
- Extend owned Auto Clicker placement acceptance beyond the user-observed working cases: available/owned count before and after C + click, monster and Buy Available Upgrades assignments, a sole clicker, already occupied or unreadable footer and scene background changes. Confirm the optional footer pass hands off within 10 seconds when recognition fails; ordinary periodic upgrade checks remain available. Pool recognition passes supplied native 3/3 frames and placement/queue acknowledgement is covered with modeled observations; actual assigned targets still need native evidence. Existing assignments must remain intact and rubies must not be spent.
- Verify initial hero/skill upgrade setup at a later zone, including missing earlier heroes, already-disabled Buy Available Upgrades, and an empty Ancient plan that stays on Heroes. Confirm a placed footer clicker handles newly unlocked upgrades; ordinary clicks remain a periodic fallback.
- Verify the integrated native Junk Pile salvage during Ascension in the installed game: one Yes, closure, reopened or directly shown reward dialog, then reset and startup. Fixed-text/control recognition, variable reward/background negatives, shared-pipeline isolation and F8 interruption have fixture coverage. Equipped relics must remain untouched.
- Obtain a final Ace Scout Dorothy frame before enabling terminal-roster recognition without a successor.

## Remaining feature roadmap

Goal: complete the unattended active-play loop using the existing shared capture, bounded analyzers and serialized input queue. Independent feature work is planned in its own chat; the unattended Ascension milestone is now approved for implementation. Keep working strategies simple: the latest hero, saving for the next hero, native hotkeys/input where available, and no ruby spending outside the authorized Mercenary revival policy or an explicitly enabled and budgeted Timelapse policy.

### Remaining independent gates

| ID | Scope and deliverable | Priority | Completion gate |
| --- | --- | --- | --- |
| R1 | Validate the integrated bounded hero skill restart, place owned Auto Clickers, and recognize the last hero without a successor. | First | Two native cycles, verified bounded startup inputs and ordinary Q confirmation and clicker targets, no regression to latest-hero leveling, missing-input recovery, and terminal-roster evidence. |
| R2 | Validate conservative Ancient price bounds in the installed game. | First | Compare Juggernaut/Solomon bulk purchases and Chor'gorloth discount/balance rounding to fresh before/after exports; preserve the reserve. |
| R3 | Relic management: verify automatic Active equipment selection and native Junk Pile salvage during Ascension. | First | Verified screens or exact missing-input list; explicit equipment and discard policy; unknown items are not silently destroyed. |
| R4 | Gild redistribution: choose a good target consistent with latest-hero progression, calculate transfer cost from current save/state, and plan UI application. | Next | Keep earned gift opening separate; account for soul costs and reserves; no ruby spending and no repeated transfers to the same target. |
| R5 | Combat and boss recovery: validate active/ready skill states, Energize/Reload waves, variable cooldowns, failed-boss retry and the Ascension handoff. | First | Concrete remaining gaps and fixture/live scenarios; reuse current skill/progression/Ascension policies rather than replacing them without evidence. |
| R6 | Native Windows capture, SIFT, OCR and queue measurement with the integrated changes. | First | Collect p50/p95 timing and timeout rate, with real, rotated, small and scrollbar-overlap fish; no win inferred from fewer detections. |
| R7 | Windowed game and display scaling: locate the game viewport, map screenshot coordinates to input, and plan focus/permission handling across Windows/macOS. | Later | Native coordinate evidence; correct ROI translation, unknown-window behavior; full-screen behavior remains supported. |
| R8 | Transcension and Outsiders: validate the read-only preview against the installed UI and implement gated native stages. | Later | Match reward, TP, costs and respec semantics to UI; automated reset waits for the complete Ascension loop and an explicit enabled mode. |
| R9 | Mercenary recovery and recruitment in the existing mercenary chat: current quest loop validation, dead/missing mercenaries and free recruitment. | Parallel | Preserve saved roster plans and the fixed Okay delay; unsupported screens do not spend rubies; no duplicate quest implementation. |

### Integration sequence (owned by the main chat)

1. Validate the integrated hero startup/combat/Ancient handoff and close the remaining Auto Clicker/relic native UI gates below. Keep R6 measurement independent from gameplay decisions.
2. Validate the integrated owned Auto Clicker placement and obtain native relic interaction evidence; coordinate shared main.go/pipeline.go changes in the main chat.
3. Retain bounded failure pauses and validate startup/export/spending handoffs. A failed purchase must not be replayed from a stale plan.
4. Verify two complete consecutive cycles: combat wall -> confirmed Ascension -> zone 1 -> bounded skill sweep -> newly available upgrades -> enabled progression -> fresh export -> Ancient purchases -> Heroes continuation. Include missed input, fish over controls, Explorer focus restoration and unsupported dialogs. Code/fixture checks do not establish native game acceptance.
5. Add R4 when the repeated loop works. R7 remains independent platform work. Enable R8 automation only after the loop and recommendation/Outsider preview are accepted.

The detailed unfinished implementation and live-validation gates below remain authoritative. Remove completed work; keep missing screenshot/native-game gates visible.

## Shared observation and action pipeline

Architecture: [Automation pipeline](docs/automation-pipeline.md).

- Recheck live OCR timeout rate and capture/CPU load with single-threaded Tesseract, the configurable `-ocr-timeout` budget and capture cadence measured after completion. Measure p50/p95 fish and purchase response and queue delay with `-stats`; verify foreground process/title support and the visual/F8 fallback on Windows/macOS.
- If full-screen SIFT remains the dominant live delay, evaluate reduced working resolution or OpenCV feature configuration against all real, small/rotated and scrollbar-overlap fish cases before adopting it. Preserve recognition quality and bounded input scheduling.

## Earned gild gifts

- Verify the remaining earned-gift edge cases in the installed game: a single reward, a missed input. Confirm recovery closes the roster and resumes automation without repeated opening.

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
- Capture real windowed HUDs and dialogs to validate the client-area/centered/bottom-aligned 16:9 candidates before broadening detection. Unknown, ambiguous, clipped or display-straddling windows must remain paused; the Heroes tab is still required for hero actions. After resizing with monster clicks enabled, obtain new crop coordinates and restart.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Fix ordinary Mercenary button OCR at 640x360: in the scaled Emma fixture, Collect can be read as Callect. Preserve strict row association; use a visual button template or improve the crop rather than accepting arbitrary labels. The death detector itself passes at this size.
- Verify dead-card handling in the installed game: mixed living/dead roster, death notification, bounded all-dead return to Heroes and fish during the visit. Real-frame and pipeline regressions pass; no live recovery action has been tested.
- Verify one installed-game paid revive and one burial/free-replacement cycle, using the integrated policy and actual displayed price/balance. Capture the extra-life chooser before enabling it; cards with remaining lives are deferred. Native paid revive, Bury, post-burial roster, paid Hire negative and free recruitment offer fixtures are covered; recruitment completion is still missing.
- Run the existing `-mercenaries` loop on the target device before changing its timing: four/five completed quests, mixed Collect/Start Quest/running rows, animated notification, paired Collect/Start clicks, fixed Okay after 300 ms, one bounded scroll sweep and return to Heroes. Include fish between rows and unreadable offers; capture frames/logs for any failure. Preserve the saved visible-row plan without intermediate roster OCR or countdown confirmation.
- Obtain verified game frames for an actually vacant slot, an all-dead/empty roster, a free recruitment offer, its completed reward, any completion dialog and the new mercenary's Start Quest row. The Emma death frame is covered; the post-burial roster and free offer are covered, but vacancy tracking and recruitment completion remain unverified. Do not infer a vacancy from a dead card or from the number of rows visible in one viewport.
- After those frames are available, extend the existing reader/planner to track verified vacancies and recruitment already in flight, and choose free recruitment only for an available slot. Preserve the bounded return to Heroes when no living mercenary or verified free action is available. Preserve the verified Revive/Bury flow; paid Hire and Reroll remain excluded. Unknown layouts must not spend rubies.
- After the recruitment-completion UI is verified, handle its reward separately from the ordinary same-position Collect/Start pair. Rebuild the saved row plan only when recruitment changes the roster or opens a new UI state, then reuse the existing quest-selection loop for the new mercenary.
- Gate replenishment on real-frame OCR/planner regressions for dead/vacant/full rosters, recruitment already in flight, interrupted completion and unknown dialogs, followed by one native free-recruitment cycle. Until the missing frames and native run are available, leave further Mercenary production changes pending.

## Ancient fresh-save budget acceptance

- Verify two consecutive live Ancient batches, including a last purchase close to the displayed balance; preserve the reserve and collect before/after exports for any failed purchase. Spending uses the freshly exported save, without monetary HUD corroboration.

## Native Ancient calculator review

- Verify installed-binary identity and close the Hero Souls spending live gates below. Static web-client arithmetic checks do not establish native purchase acceptance.
- Verify live navigation past a top-clipped Ancient level: keep its name as a navigation anchor, wait for a complete level crop before buying, and retain saved/read diagnostics for a genuine mismatch.

## Prestige and later upgrades

### Automatic Ascension

- Verify missed input and manual cancellation with the Ascension dialog open, followed by the integrated zone-1 hero/bulk-upgrade/export handoff. The user observed one successful full-combat-loss reset; that does not establish two consecutive unattended cycles.
- Verify the integrated Junk Pile salvage -> Ascension transition in the installed game after the supplied native blocker passes fixture and queue tests. Equipment preflight must complete before junk salvage; unknown dialogs must not trigger salvage.

### Hero Souls spending

- Verify benefit-based residual Morgulis purchases in the installed game with fresh before/after exports. Compare the native Soul damage bonus and billing to the guarded plan, confirm the optional explicit reserve and no repeated significant purchases without new souls. The Active allocation remains an approximate farming model, not a globally optimal damage/gold/skill solver.

- Verify a live large Ancient purchase with six-significant-digit input and the 200 ms entry wait, including closed owned-dialog acknowledgement. Confirm repeated fresh-export plans stop once large non-exponential increases fall below 0.1%.
- Verify V custom-quantity opening in the running Windows game after the added 100 ms key-settling delay, using a fresh export after Atman's unintended level increase.
- Verify one live `-ancients-save` batch with a fresh export, expanded Ancient cards, V custom quantity, actual text entry, one-shot OK submission acknowledged by a newer ordinary Ancients frame, fresh level checks and the calculated save budget before each next purchase, one-notch wheel seeking by canonical alphabetical names, name-confirmed movement, bounded wheel-to-arrow recovery and scrollbar-arrow correction for clipped/bracketed/overshot rows and return to Heroes. Include an open dialog timeout and a silently failed purchase that closes the dialog (allowed underbuy, no replay). Supplied frames and regressions cover recognition, budget protection, transaction ownership, stale input and interruption.
- Verify live `-export-dir` acquisition on Windows: menu -> Save -> delayed Explorer foreground -> original game focus -> menu close -> fresh file -> calculated Hero Souls budget -> Ancient batch. Verify the export-backed budget and reserve, without wallet HUD OCR; pending Ascension souls must not authorize spending. Check menu closing while Save is highlighted, the configured export folder and filename, a failed Save, blocked OK and the same sequence after Ascension. Automated regressions cover menu recognition, stale/partial exports, cancellation and queue isolation. Validate the integrated hero sweep/bulk-upgrade handoff and remaining Auto Clicker/relic gates before treating repeated restart cycles as native acceptance.

### Transcension

- Match the fresh `-mode transcension-plan` preview to the installed game's reward/TP display, Outsider costs and respec/refund semantics. Capture reward confirmation, respec controls, one small purchase before/after, immediate manual-reset HUD/export and first-run Ancient summon controls.
- Enable native Transcension and Outsider spending only after two complete consecutive Ascension/restart cycles, a confirmed combat wall and active-play timing evidence, verified reset recovery and an explicit enabled mode. The current preview is informational; save timestamps do not establish a reset decision.
