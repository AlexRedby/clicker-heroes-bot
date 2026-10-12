# TODO

## Performance

- Measure the reported live response delays with `-stats`: capture cost, OCR timeout rate, fish/purchase response and queue latency. Optimize the measured bottleneck; compare recognition quality against the existing real fish corpus if changing SIFT resolution/features.

## Hero roster completion

- Recognize the final hero without requiring a successor. Obtain an Ace Scout Dorothy frame before implementing terminal-roster detection.

## Ancient purchase pricing

- Establish installed-game bulk pricing for Juggernaut/Solomon and Chor'gorloth rounding using fresh before/after exports. These costs remain an approximation; use the result to correct purchase budgets if necessary.

## Mercenaries

- Resolve the reported ambiguous dead-Mercenary bonus OCR at 640x360 if the supplied source preserves enough information. Unknown traits must not authorize revive/bury.
- Implement the extra-life chooser once its actual controls are available; defer cards with remaining lives until then.
- Add explicit vacancy and recruitment-in-flight tracking, rebuilding the row plan when recruitment changes the roster. Reuse existing free quest selection; paid Hire/Reroll remain excluded.

## Achievement strategies

- Implement bounded click, boss-kill, hero-level and zone strategies; these goals are currently report-only. Reuse the shared input queue and save snapshots. Unknown counters must not authorize input; no paid revivals, deliberate losses or prestige resets solely for a goal without an explicit policy.
- Obtain the actual missing controls for each execution strategy before enabling it.

## Budgeted Timelapse

- Assess readiness from fresh save facts and measured return speed: zones, rubies, owned Auto Clickers, hero/gild/upgrade state, active/idle Ancients and Outsiders.
- Complete forecast-selected hero/gild preparation and the Hybrid Ancient summon controls. Coordinate Auto Clicker assignment so Nogardnit retains an unassigned clicker during Timelapse preparation.
- Compare the native Go forecast with reference data and observed outcomes to establish useful zones/time saved per ruby.
- Implement opt-in execution with a finite combined Timelapse/Quick Ascension allowance, protected balance, per-Ascension limits and a persisted pending debit. Restarting or earning rubies must not renew authorization; uncertain purchases must not be replayed.
- Obtain actual Ruby Shop entry, duration/price offers, confirmation/cancel and result controls before enabling paid input. A paid acceptance run requires an explicitly authorized small ruby allowance; zero allowance spends nothing.
- Reuse shared navigation/input and refresh the save after a purchase, then resume ordinary play. Unsupported or uneconomic offers must leave ordinary progression running.

## Budgeted Quick Ascension

- Implement native Go reward/readiness calculations and compare them with current-client/reference data, including the reward ceiling beyond zone one million.
- Reuse the Timelapse ruby ledger and add per-run/Ascension purchase limits.
- Implement owned Ruby Shop/QA transitions, one-shot submission and fresh-save outcome/Ancient replanning without resetting heroes or zones. Obtain actual offer/confirmation/result controls before enabling paid input.

## Windowed platform support

- Calibrate actual windowed HUD/dialog geometry and screenshot-to-input coordinates for supported Windows/macOS scaling and displays before claiming native support. Unknown or clipped geometry must suppress input.

Clans are deferred by user request. Forge Core Relic upgrades are excluded by user request. Keep only concrete unfinished work and indispensable missing evidence; remove fixed bugs once regression tests pass. Future hero/skill/scroll defects are handled when reported, not tracked as speculative acceptance checks.
