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
- **Heroes:** only when due on Heroes. Gold, successor price and baseline level share one frame. At most two Tesseract executions run concurrently, each with one OpenMP thread and a three-second execution limit (`-ocr-timeout`). Purchase confirmation reads only row stability and level.
- **Gild gifts:** an infrequent local icon check on a shared frame (`-gilds`, `-gild-interval`). Modal steps use local templates on later shared frames; they have no OCR or separate capture loop. Opening/advancing a gift invalidates panel observations and actions while preserving any known fish position.
- **Progression:** zone and conditional damage OCR every two seconds, using shared combat buffs. An `A` confirmation checks only the boot icon and can proceed independently of the skill worker.

Named PNG templates under `assets/` are embedded in the executable and decoded once into immutable Go images. Each file contains only its recognition sample; screen regions and click points remain in the feature code. Native OpenCV matrices remain local to each call and are closed. Actions and confirmations do not wait for SIFT. Decisions retain their source frame and expire by age or dependent input/context changes; there is no persistent cross-decision OCR cache.

## Input and confirmation

The queue holds at most one pending intent per action kind, plus the executing action. New observations replace older proposals. Priority is fish, earned gifts, skill, progression, hero interaction, then optional monster clicks. Independent valid intents may execute consecutively; actions invalidate only affected observations.

Hero purchase, scrollbar and quantity actions require recognizable Heroes context. No feature waits for a completed fish scan. Coordinate intents are checked against current row/scrollbar geometry before input. Cached fish input is rebuilt from the current shared frame, so tab changes and modal delays do not expire its position.

`Q down -> click -> Q up` is one transaction with guaranteed cleanup. Energize and its consumer retain their sequence; Reload waits for confirmed prerequisite casts. Native drag is never interrupted halfway.

Confirmation is controller state, not a private capture/sleep loop. Later shared frames confirm increased hero level, skill cooldown, `x1`, scrollbar bottom or progression mode. Unreadable/no-op results use bounded attempts and the existing retry policies. Ancient OCR errors and an unreadable scrollbar retry within the existing transaction deadline while fish detection continues independently. Collection discards obscured panel observations and rereads controls while preserving the pending transaction. Diagnostic PNG encoding runs outside the coordinator with a bounded queue.

## Measurement

Run with `-stats` to print capture and analyzer counts, cumulative capture/analysis/input durations, cumulative queue wait and discarded stale work on shutdown. The configured interval schedules observations; a slow analyzer skips frames and cannot overlap itself.

Fixture checks cover bounded workers, shared frame identity, context/F8 invalidation, priority, modifier cleanup and shared confirmations. Live capture latency, focus support, p50/p95 response and CPU/memory still need measurement on the installed game. Further SIFT tuning must retain the real, rotated, small and scrollbar-overlap recognition cases.
