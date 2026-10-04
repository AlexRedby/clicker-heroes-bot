# Automation pipeline

`pipeline.go` owns capture, observations, scheduling and the action queue. Feature controllers in `hero_runner.go`, `skills.go`, `progression.go` and `gilds.go` retain their decision policies and pending confirmations.

```text
Shared capture -> immutable frame -> bounded analyzer workers
                                          |
                                          v
                          observations -> coordinator state
                                          |
                                          v
                          deduplicated queue -> single input executor
                                          |
                                          v
                          confirmation on later shared frames
```

## Frames and context

A frame carries its image, ID, capture time, F8 generation and layout revision. The coordinator is the only state writer. Each analyzer has one running job and one replaceable waiting frame; intermediate screenshots are skipped rather than queued indefinitely. Results are accepted once per analyzer/frame and rejected after context changes or dependent input invalidation.

The boot icon recognizes the full-screen game HUD without requiring Heroes. Heroes analysis additionally requires the Heroes tab. The earned-gift and Gilded Heroes modals are explicit contexts. While open, they suspend every background analyzer and permit only the enabled gift transaction. Unknown layout stops recognition jobs and input. Tab, image dimensions or foreground process/title changes discard pending panel decisions. Native foreground identity is checked again at the input boundary. If unavailable, visual context and explicit F8 pausing are the documented fallback; identical process/title windows are not distinguished.

Ordinary capture waits 250 ms (or the shorter configured fish interval) after capture/context recognition finishes. A slow capture therefore cannot consume its own interval and cause continuous capture. Post-action refresh remains immediate after UI settling. It pauses during a native input transaction and for 150-200 ms of UI settling. Analyzers can finish while input runs. F8 invalidates actionable results; an ongoing native transaction finishes and releases held inputs before F8 takes effect.

## Analysis

- **Fish:** one SIFT worker, scheduled by `-fish-interval` on every recognized game tab, including exclusive Ancient visits and pending confirmations. A found position is retained until collection, including across tabs, covering dialogs and settings. No further scans are scheduled while that position is known. Covering dialogs, settings and Explorer suppress fish input; closing them allows collection on the next shared game frame. Viewport changes discard old coordinates. Slow OCR and export-file reads do not block this worker or its priority input. Three completed negative observations rearm collection; a persistent visible fish can retry after five seconds.
- **Skills:** one recognition of all nine states per shared frame. Progression reuses the same frame's states; there is no duplicate strip analysis.
- **Owned Auto Clickers:** progression pool recognition on shared Heroes frames. Available/total OCR runs independently every five seconds, or every 300 ms while awaiting placement acknowledgement. Placement uses C + click through the common executor; a fresh count decrement acknowledges it. Submitted placement ownership survives F8, and ambiguous input is not replayed. Existing assignments and rubies are preserved.
- **Heroes:** initial/reset top-to-bottom affordable sweep followed by the bulk upgrade footer; otherwise only when due on Heroes. Gold, successor price and baseline level share one frame. At most two Tesseract executions run concurrently, each with one OpenMP thread and a three-second execution limit (`-ocr-timeout`). Purchase confirmation reads only row stability and level.
- **Gild gifts:** an infrequent local icon check on a shared frame (included in `-progression`, configured by `-gild-interval`). Modal steps use local templates on later shared frames; they have no OCR or separate capture loop. Opening/advancing a gift invalidates panel observations and actions while preserving any known fish position.
- **Ancients:** an empty plan finishes on Heroes without a tab visit. Nonempty plans read names and levels for navigation and compare the saved level before purchases. The monetary budget comes only from the fresh save: exact decimal arithmetic subtracts guarded costs of submitted rows and preserves the reserve; wallet HUD OCR is not used. CLI and autoexport share the final calculator plan: after six-digit quantity truncation, a non-exponential current level of at least `1e9` requires a relative entered increase of at least 0.1%. Skipped rows retain their souls; totals sum only the original guarded costs of retained rows, without a second allocation search. Small/exponential integer purchases remain eligible. After the owned OK click, the worker uses only shared-frame context: a newer recognized ordinary Ancients tab with the quantity dialog closed retires the item as submitted, without name, level or wallet OCR. The next item requires a fresh name/level read. Navigation reads names independently of numeric OCR and seeks the alphabetically first remaining canonical name from the fresh export, while allowing other readable nearby planned purchases. It sends one native wheel notch, keeps the cursor over the list for the library's 150 ms post-event delay, then parks it and checks name order and displacement on a newer shared frame; one notch can move more than one card. Clipped/bracketed targets and direction reversals use recognized scrollbar-arrow clicks with name-only observations. A complete button with a matching name requests a fresh full read before buying; unreadable numbers get bounded retries. No thumb geometry or initial top sweep is required. Two confirmed wheel no-ops switch the current target to recognized arrow input and reset the no-motion count; arrows get three independent no-motion chances before stopping. Arrow recovery allows at most 128 total inputs for that target (normal wheel navigation allows 64), while its 24 local-correction limit counts only inputs within the visible canonical name range. Distant arrow transit still checks names and displacement after every input. Repeated unknown/misordered names and bounded input/local/direction-change counts stop with target, direction and readable-neighbor diagnostics. Submitted rows are never replayed. Fixture models exercise several wheel/arrow displacements; they do not calibrate native increments.
- **Progression:** zone and conditional damage OCR every two seconds, using shared combat buffs. An `A` confirmation checks only the boot icon and can proceed independently of the skill worker.

Named PNG templates under `assets/` are embedded in the executable and decoded once into immutable Go images. Each file contains only its recognition sample; screen regions and click points remain in the feature code. Native OpenCV matrices remain local to each call and are closed. Actions and confirmations do not wait for SIFT. Decisions retain their source frame and expire by age or dependent input/context changes; there is no persistent cross-decision OCR cache.

## Input and confirmation

The queue holds at most one pending intent per action kind, plus the executing action. New observations replace older proposals. Priority is fish, earned gifts, skill, progression, owned Auto Clicker placement, exclusive transactions, hero interaction, then optional monster clicks. Independent valid intents may execute consecutively; actions invalidate only affected observations.

Hero purchase, scrollbar and quantity actions require recognizable Heroes context. No feature waits for a completed fish scan. Coordinate intents are checked against current row/scrollbar geometry before input. Cached fish input is rebuilt from the current shared frame, so tab changes and modal delays do not expire its position.

`Q down -> click -> Q up` is one transaction with guaranteed cleanup. Energize and its consumer retain their sequence; Reload waits for confirmed prerequisite casts. Native drag is never interrupted halfway.

Confirmation is controller state, not a private capture/sleep loop. Later shared frames confirm increased hero level, skill cooldown, `x1`, scrollbar bottom or progression mode. Unreadable/no-op results use bounded attempts and the existing retry policies. Ancient pre-purchase OCR errors and unreadable navigation controls retry within the existing transaction deadline while fish detection continues independently. After OK, an open dialog or unknown/foreign/modal context waits within the same deadline, then blocks the batch without repeating OK. F8, stale frames and changed window geometry cannot acknowledge submission. Dialog closure does not prove a level increase: a silent failed purchase can underbuy, but the submitted item is not retried. Collection discards obscured panel observations and rereads controls while preserving the pending transaction. Diagnostic PNG encoding runs outside the coordinator with a bounded queue.

## Measurement

Run with `-stats` to print capture and analyzer counts, cumulative capture/analysis/input durations, cumulative queue wait and discarded stale work on shutdown. The configured interval schedules observations; a slow analyzer skips frames and cannot overlap itself.

Fixture checks cover bounded workers, shared frame identity, context/F8 invalidation, priority, modifier cleanup and shared confirmations. Live capture latency, focus support, p50/p95 response and CPU/memory still need measurement on the installed game. Further SIFT tuning must retain the real, rotated, small and scrollbar-overlap recognition cases.
