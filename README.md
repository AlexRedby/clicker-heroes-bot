# Clicker Heroes bot

Go bot for a running Clicker Heroes game. It can take a screenshot, make one click, and watch for the clickable orange fish. In `run` mode, it checks the screen for fish repeatedly (once per second by default) and clicks a detected fish. Monster auto-clicks are optional.

## Requirements

- **macOS:** Install Go (`brew install go`), Xcode Command Line Tools (`xcode-select --install`), and OpenCV 4.13 using the [GoCV macOS instructions](https://gocv.io/getting-started/macos/). Grant the app that runs the bot (for example, Terminal) **Accessibility** and **Screen & System Audio Recording** permissions in System Settings > Privacy & Security.
- **Windows:** Install Go (`winget install Golang.Go`) and GCC to build RobotGo (`winget install BrechtSanders.WinLibs.POSIX.UCRT`). Make sure the directory containing `gcc.exe` is in `PATH`.

Fish detection uses GoCV 0.43.0 with native OpenCV 4.13. Install OpenCV before building on every platform; see the [GoCV installation guides](https://gocv.io/getting-started/). On macOS or Linux, verify it with `pkg-config --modversion opencv4` (expect 4.13.x). Also check `go version` and `gcc --version`. If `shot` reports `Capture image not found` on macOS, enable Screen & System Audio Recording for Terminal (or the built app) and restart it. The `click` and `run` modes also require Accessibility permission.

Hero leveling, automatic progression, Ascension and Ancient purchases additionally require the `tesseract` executable with English OCR data in `PATH`. Enable **Always use scientific notation** in the game's settings so gold, prices and damage use the same readable format.

Ancient calculations additionally require Node.js 18+ and npm. From the project directory, install the pinned calculator dependencies once:

```sh
npm ci --prefix tools/ancients --ignore-scripts
```

## Run

From the project directory:

```sh
go mod download
go run . -mode help
go run . -mode shot
go run . -mode click -x 700 -y 400
go run . -mode run
go run . -mode run -hero-levels
go run . -mode run -hero-levels -skills -progression -gilds
go run . -mode run -hero-levels -skills -progression -gilds -stats
go run . -mode run -hero-levels -skills -progression -gilds -mercenaries -stats
go run . -mode run -hero-levels -skills -progression -ascension
go run . -mode run -hero-levels -duration 10m
go run . -mode run -x 700 -y 400 -interval 100ms -fish-interval 1s -duration 10m
```

The example coordinates are placeholders. Use pixel coordinates from the saved screenshot of the primary display for `-x` and `-y`, including on Retina displays. The bot validates these coordinates and converts screenshot pixels to desktop points on macOS. By default, `shot` saves `artifacts/screenshot.png` and creates the directory if needed. Put other generated test files in `artifacts/` too; Git ignores this directory. You can choose another screenshot path with `-out`. After starting `shot` or `click`, you have 5 seconds (`-delay`) to switch to the game. `run` has no startup countdown; focus the game and press F8 when ready. For `click` or optional monster clicks in `run`, use coordinates inside the monster area.

`run` starts paused: press **F8** to start, and press F8 again to pause or resume from any window. On some Mac keyboards, press Fn+F8. Pausing blocks new input and invalidates decisions from earlier screenshots. An OCR or detection call already in progress can finish, but its result cannot trigger input after pause/resume. Without `-duration`, it runs until Ctrl+C; a positive `-duration` sets a wall-time limit that also counts while paused. If F8 does not resume the bot, check Accessibility permission for the app running it. Pause before switching away from the game. When native foreground identity is available, input requires the same foreground game process/title observed in the screenshot; if unavailable, the bot reports its visual-context/F8 fallback. This guard does not distinguish windows with identical process/title.

Fish detection uses the [Clicker Heroes Orange Fish image from StickPNG](https://www.stickpng.com/img/games/clicker-heroes/clicker-heroes-orange-fish), listed there for personal use only. A detected fish stays suppressed through small position changes and isolated missed detections; three consecutive fish-free scans rearm collection. If the fish is still recognized after five seconds, the bot retries the click. Fish clicks use the same 100 ms press and release-settling wait as hero purchases. Hero purchases and scrollbar drags require a recent completed fish scan; a visible fish defers hero interaction. A scan in progress does not count as a fish-free result. `run` extracts SIFT features from the fish image and each screenshot, matches them with OpenCV, and clicks the center of a fish when enough matches agree on its position, size, and rotation.

With `-hero-levels`, `run` verifies the full-screen Heroes layout, drags the scrollbar only when needed, and requires a confirmed bottom position. Thumb candidates must have a narrow rectangular shape with aligned sides, so the orange fish is not accepted as a thumb. RobotGo smoothly drags the thumb to the bottom edge of the display with the mouse button held, using 0.5-1.5 ms library step delays for faster smooth movement. F8 takes effect after an ongoing library drag finishes and releases the button. It considers the latest hero and its successor. A missing or obscured row skips the purchase; it never falls back to an earlier owned hero. Keep the Heroes tab open and the game filling the primary display. Lists without a recognizable scrollbar are currently skipped.

The bot keeps the persistent purchase quantity at `x1`, cycling with `T` only if another quantity is selected. It reads gold and the next price from cropped regions, then holds `Q` only during the hero purchase click to buy MAX levels. Hero purchase clicks hold the left button for 100 ms and wait 100 ms after release before moving the pointer or releasing `Q`. Both held inputs are released on errors or cancellation; no MAX selection screenshot is needed. When the next hero costs at most ten times current gold, it waits to buy that hero; otherwise it buys MAX levels of the current hero. It moves the cursor away from hero buttons to dismiss tooltips, then confirms a purchase by reading an increased `Lvl` value with a stable list position. Overlay or animation pixel changes alone do not count. OCR has a configurable three-second limit per execution (`-ocr-timeout`, for example `-ocr-timeout 5s`) and is cancelled when the run stops. Each Tesseract process is limited to one OpenMP thread to avoid competing thread pools alongside SIFT. Gold, price and baseline level share one screenshot and use at most two concurrent Tesseract processes. Confirmation reads only the selected row level and geometry.

It does not buy upgrades. After a confirmed purchase it reads a fresh frame immediately; it checks every 5 seconds while saving, or every 30 seconds when recognition fails or no hero is available. An unconfirmed click saves timestamped `artifacts/hero-failure-*-before.png` (with a red cross at the click target) and matching `*-after.png`, retries later, and stops hero purchases after three consecutive failures; fish detection continues. After a purchase, `x1` remains selected. Pausing invalidates the pending purchase; the next attempt starts from a fresh screenshot. The [hero progression plan](docs/hero-progression.md) describes recognition and its limits.

With `-skills`, OpenCV recognizes ready skill icons; clock overlays and outer glow identify cooldowns, active buffs and energized buffs. Ready short-cooldown skills run independently. The bot refreshes ongoing energized buffs and prioritizes Lucky Strikes + Golden Clicks + Energize + Reload (`3,5,8,9`) when all four are available. Each activation is confirmed from a fresh frame before continuing the combination. Reload prepares a second wave after the current effects finish.

When the pair is unavailable, Energize can strengthen an available core buff, or Reload can follow a confirmed Golden Clicks cast. F8 or an unexpected observed skill activation interrupts a combination. On restart/resume, an ambiguous Energize charge is consumed by a recognized ordinary buff before new utility combinations. Dark Ritual runs separately without Energize/Reload; the game enforces its 20-use limit. A missed/no-op activation retries after 30 seconds without stopping other skills.

Each key is held for 100 ms and released on cancellation/errors. All native input uses one executor; skill analysis and confirmations continue independently of SIFT. Skills do not require timer OCR or Ancient cooldown formulas. Keep the full-screen game HUD visible.

With `-progression`, the bot recognizes the boot toggle and reads the zone from a cropped HUD region every two seconds. It reuses the skill states from the same frame; activation confirmation checks only the boot icon. A known farm state gets one initial attempt to enable progression with `A`; a missing or obscured toggle is skipped. Activation is visually confirmed. An unconfirmed toggle waits at least 30 seconds before another attempt from a fresh farm frame.

An observed progression-to-farm fallback before a boss records that wall. The bot waits for at least twice the displayed damage proxy (the larger of DPS and click damage), or a previously untried active Lucky Strikes/Super Clicks buff, including Energize. It retains the highest observed damage and combat buffs from the failed boss, so expiration and renewal of the same buff do not alone trigger another attempt. Ordinary retries after repeated failures wait 1, 2, 4, 8, then 15 minutes. A previously untried full combat window (active skills `1,2,3,7`) bypasses this delay; after a confirmed full-combat loss, doubled damage or a new stronger combat buff can also retry promptly. This is a retry heuristic: it does not measure click rate or predict a guaranteed kill from boss HP and the timer. Keep monster clicks or in-game Auto Clickers active for a clicking build. In progression-only runs F8 retains the wall; restarting the bot or observing a drop of more than one zone starts a fresh assessment. With automatic Ascension enabled, F8 starts a fresh boss assessment too, so a new observed attempt can authorize a reset. Fish collection keeps priority; hero purchases and skills continue while farming. `-progression` can also run without `-hero-levels` or `-skills`.

With `-ascension`, an observed boss loss after skills `1,2,3,7` overlapped in two boss observations at least two seconds apart starts a World Ascension assessment immediately. Unknown/locked skills and a farm-frame retry baseline do not count as an observed full combat fight. If that proof is unavailable, `-ascension-stall` supplies a three-minute fallback without higher-zone progress. A running boss retry or a fresh stronger retry decision defers the reset. This is a wall heuristic, not a guaranteed optimal Ascension route. It follows the game's [progression guidance](https://blog.clickerheroes.com/how-to-progress-through-clicker-heroes-zones/) to ascend when a boss remains out of reach after skills and upgrades.

Only a reset candidate reads the banked and pending Hero Souls, from two small crops in a shared screenshot. The pending reward must be positive and at least 25% of the soul capital (`-ascension-min-gain 0.25`). Without an export the current unspent soul bank is the available baseline; it does not include souls already spent on Ancients. To include them, add `-save "path/to/fresh-save.txt"`, or use the export already provided by `-ancients-save`. The local calculator supplies the exported wallet plus the sum of owned Ancients' `spentHeroSouls`, never the lifetime sacrificed-souls total. This exported capital remains a floor after a planned Ancient purchase batch; the larger of it and the current bank is used. Export again after manual soul spending or a reset. If the save lacks spending data, the bot uses the HUD bank. The gain threshold is adjustable and is not an estimate of future DPS. Small/zero gains defer the reset and suggest reviewing Ancient allocation and Transcension; they do not authorize Transcension.

`-ascension` requires `-progression`. A disabled toggle by itself is insufficient. Pending inputs, stale damage/reward observations, a visible fish or unreadable soul values defer input. Transient OCR gaps and same-window tab/modal visits preserve the wall history, but a fresh progression observation is required before opening the dialog. Successful hero purchases and ordinary skills no longer repeatedly postpone a proven wall by ten seconds. F8, a changed display/window, or an observed manual zone drop clears the Ascension assessment.

The bot clicks the recognized right-hand red spiral, independently reads the reward from the specific Ascension dialog, and confirms with its green `Yes` only if the reward still meets the minimum gain. A manually opened dialog is never automatically confirmed. All background actions are suspended during this transaction, and the reward must remain visible and unchanged before confirmation. It does not use `Buy Quick Ascension`, spend rubies, transcend, or delete relics. A blocked or missed transition pauses after twenty seconds. Following a confirmed return to zone 1, **the bot pauses** for Hero Souls spending and restart setup. Export a fresh save for the Ancient purchase batch described below; initial hero/autoclicker setup remains manual. Press F8 only after preparing the next run. Real HUD crops, combat-wall/reward decisions and dialog recognition are tested; the actual reset and initial HUD still need live verification.

### Ancient purchases

Export a fresh save using the game settings. Preview an Active-build allocation without game input:

```sh
go run . -mode ancients-plan -save "path/to/clickerHeroSave.txt"
```

The bot runs the [MIT Ancient calculator](https://github.com/tomcur/ClickerHeroesCalculator) locally. It uses saved Ancient/Outsider levels and current Hero Souls, excludes pending Ascension rewards, and retains the calculator's soul bank plus a 1% reserve. `-ancient-reserve` accepts a percentage or an absolute scientific value; `-ancient-skill-rate` sets the skill-Ancient allocation from 0 to 1 (default 1). Set `-ancient-beyond8k` if your best hero is levelled beyond 8000, matching the calculator's Wepwawet setting. Large quantities stay decimal strings. The preview is saved to `artifacts/ancients-plan.json`; the exported save is read only.

Run one purchase batch before ordinary automation:

```sh
go run . -mode run -ancients-save "path/to/clickerHeroSave.txt" -hero-levels -skills -progression
```

Start on Heroes or Ancients in the English full-screen layout, with Ancient cards expanded, and press F8. The bot checks the observed soul balance against the export, visits Ancients, scrolls through the list, holds `V` while clicking an owned Ancient, enters its calculated quantity, and confirms the level increase before the next purchase. Quantities are rounded down to at most 15 significant digits for the input field. Other automation waits during the visit. The bot returns to Heroes and pauses after the batch; F8 then resumes ordinary automation without repeating the batch.

An unreadable or changed row, insufficient balance, unconfirmed input or F8 interruption pauses purchases. Close any quantity window, export a fresh save and restart for another batch. It never summons or respecs Ancients, imports/edits saves or spends rubies. Automatic fresh export after Ascension and initial hero/Auto Clicker setup remain pending; a continuous Ascension/spending/restart loop is not yet enabled. Screen recognition and transaction checks are tested against supplied frames; native purchases need a live game check.

With `-gilds`, the bot checks the earned gift icon every five minutes (`-gild-interval`, for example `-gild-interval 10m`), starting with one check when the run first starts after F8. It clicks the gift, opens the central chest, selects the visible `Open All`, then closes the Gilded Heroes result panel. If a single reward has no `Open All`, it closes the reward using its visible X. Recognition uses small fixed screen regions and OpenCV templates, without OCR or extra captures. Fish, skill, progression, hero and monster actions are suspended while these windows are open; old observations and queued actions are discarded at each transition. Missing/unknown controls are skipped, and an unchanged transaction pauses after three repeated clicks or twenty seconds; inspect the window and press F8 to resume from a fresh frame. It only opens earned rewards and never uses `Get More`, ruby purchases or gild transfers. As with hero recognition, keep the game full-screen on the primary display.

With `-mercenaries`, the bot recognizes the notification over the penultimate tab, collects rewards, sends idle mercenaries on new quests, checks the bottom of the roster, and returns to Heroes. It can also service a Mercenaries tab left open manually. Each click requires a new confirmed screen state; selecting a quest and pressing `Okay` are separate actions, and a countdown confirms dispatch. Game input underneath the quest dialog is suspended. F8 cancels the pending visit; an interrupted quest dialog is closed before resuming.

Free recruitment quests replenish vacant slots. Otherwise, ruby quests follow `4h > 2h > 1h > 30m > 15m > 8h > 5m`; short non-ruby quests precede 24h/48h ruby quests, following the [community ranking](https://clickerheroes.fandom.com/wiki/General_Tips). It never clicks paid rerolls, paid recruitment, revivals or burials. Recognition uses the English full-screen layout shown in the test screenshots; unreadable states end the visit and retry later.

Screen capture has one owner. Fish, skills, progression and due hero analysis consume immutable shared frames in separate bounded workers, each with one running job and one replaceable waiting frame. Results update state as they finish; slow SIFT does not delay skill hotkeys. One deduplicated priority queue feeds sequential input, with fish first. Confirmations consume later shared frames rather than starting private capture/retry loops. A tab, layout, foreground identity or F8 generation change cancels stale decisions. Ordinary capture waits up to 250 ms after the previous capture finishes; slow capture does not immediately trigger another screenshot. Capture also waits during native input and brief UI settling (150-200 ms), so it does not observe a half-drag or temporary Q modifier. `-fish-interval` is a scheduling interval, not a promise that SIFT finishes that quickly. `-stats` prints capture/analysis counts, total durations, queue wait and discarded stale results when the run stops. See [Automation pipeline](docs/automation-pipeline.md).

The [game hotkey reference](docs/hotkeys.md) records keyboard actions, target tabs and sources for future features.

## Check the project

```sh
npm ci --prefix tools/ancients --ignore-scripts
node tools/ancients/plan.test.cjs
go fmt ./...
go test ./...
go build .
```

On headless Linux, run tests with `xvfb-run -a go test ./...` because the keyboard hook needs an X display. `Dockerfile.gocv` includes OpenCV and Tesseract with English data and sets `REQUIRE_OCR_TESTS=1`, so missing OCR dependencies fail the test suite. For a native test environment, use `REQUIRE_OCR_TESTS=1 go test ./...` to require OCR checks too. When running `xvfb-run` as a Docker command, use `docker run --init` so its X server startup signal is handled correctly.

`go.mod` declares the module name and dependency versions; `go.sum` records checksums for downloaded modules. `main.go` contains the command modes, and `fish_sift.go` detects the fish in screenshots.
