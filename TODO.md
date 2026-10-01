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
- Verify that the faster 0.5-1.5 ms step delays remain reliable and that F8 taking effect after an ongoing native drag finishes is acceptable. Verify bottom-only scrolling, `T` selecting persistent `x1`, `Q` buying MAX levels and returning to `x1`, actual level confirmation, F8 interruption/resume, and simultaneous fish collection. Use saved before/after screenshots to diagnose any remaining unconfirmed purchase.
- Resolve macOS capture/input permissions and verify screenshot-pixel to desktop-point conversion, fish clicks, optional monster clicks, and F8 on Windows/macOS with display scaling and multiple displays. Capture and CLI coordinates currently target the primary display.
- Add recognition of a game viewport inside a window before supporting non-full-screen layouts. The current bot requires a recognizable full-screen game HUD, and the Heroes tab for hero actions; it skips unknown layouts.
- Support short hero lists without a scrollbar or with a thumb outside the current detector's accepted height range. Capture real examples before changing the detector; a wider height allowance can mistake the gold track border for a thumb.
- Recognize a verified end of the complete hero roster if there is no successor. The current bot skips an owned candidate without a clearly identified next unowned row.

## Mercenaries

- Verify the live `-mercenaries` loop: notification, Collect, Start Quest, selection, Okay, countdown confirmation, scrolling to the fifth mercenary, returning to Heroes and F8 interruption/resume alongside fish and skills. The four supplied screenshots cover roster and selection OCR at 2560x1440 and 1280x720; native clicks and the scrolled roster still need a running game.
- Capture real death and free-recruitment screens; verify that Revive/Bury remain untouched and recruitment quests are recognized before relying on automatic roster replenishment. Unsupported layouts must end the visit without ruby spending.

## Prestige and later upgrades

### Automatic Ascension

- Use a configurable stall window (initial default: 15 minutes) based on the highest observed zone, recognized boss fallback and existing progression/skill recovery. Pause, missing OCR, tab changes and manual zone navigation must not count as evidence of a stalled run. Do not reset solely because progression is switched off.
- Verify the installed game's Ascension button, confirmation dialog and expected Hero Souls from real screenshots. Recognize the relic-junk blocker and report it without deleting or replacing relics automatically. Distinguish normal Ascension from Quick Ascension, Transcension and any ruby purchase. No confirmed Ascension hotkey has been found; prefer the recognized game control.
- Require working hero leveling and progression management for the opt-in mode. Implement a shared-pipeline transaction: decide -> open -> read reward -> confirm -> observe restart. During modal transitions, invalidate stale actions and suspend unrelated clicks/hotkeys; F8 must interrupt cleanly. Require positive expected souls and a fresh, known game state before confirmation.
- Complete restart support before enabling automatic resets: recognize the initial short hero list, buy the latest affordable hero, dismiss/unlock required controls, restore autoclicker targets if reset clears them, and enable progression. Obtain real immediate-post-reset screenshots for these steps.
- Add real-frame positive/negative recognition and decision tests, including a temporary boss loss, unreadable reward, unrelated/blocked dialog, pause/resume, missed confirmation and reset completion. Run OCR regressions, shared-pipeline tests, race checks, build and vet.
- Verify one complete live Ascension and restart loop before starting automatic Hero Souls spending.

### Hero Souls spending

- After the Ascension/restart loop works live, inspect the Ancients panel and existing Ancient levels. Choose a minimal active-clicker spending policy and verify required hotkeys before implementation; preserve a deliberate unspent soul reserve. Then consider gild transfers and relics.

### Transcension

- Read expected Ancient Souls and recommend Transcension; automate the reset and Outsider spending only after the earlier loops are reliable.
