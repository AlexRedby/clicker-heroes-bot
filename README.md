# Clicker Heroes bot

Go bot for a running Clicker Heroes game. It can take a screenshot, make one click, and watch for the clickable orange fish. In `run` mode, it checks the screen for fish repeatedly (once per second by default) and clicks a detected fish. Monster auto-clicks are optional.

## Requirements

- **macOS:** Install Go (`brew install go`), Xcode Command Line Tools (`xcode-select --install`), and OpenCV 4.13 using the [GoCV macOS instructions](https://gocv.io/getting-started/macos/). Grant the app that runs the bot (for example, Terminal) **Accessibility** and **Screen & System Audio Recording** permissions in System Settings > Privacy & Security.
- **Windows:** Install Go (`winget install Golang.Go`) and GCC to build RobotGo (`winget install BrechtSanders.WinLibs.POSIX.UCRT`). Make sure the directory containing `gcc.exe` is in `PATH`.

Fish detection uses GoCV 0.43.0 with native OpenCV 4.13. Install OpenCV before building on every platform; see the [GoCV installation guides](https://gocv.io/getting-started/). On macOS or Linux, verify it with `pkg-config --modversion opencv4` (expect 4.13.x). Also check `go version` and `gcc --version`. If `shot` reports `Capture image not found` on macOS, enable Screen & System Audio Recording for Terminal (or the built app) and restart it. The `click` and `run` modes also require Accessibility permission.

Hero leveling additionally requires the `tesseract` executable with English OCR data in `PATH`. Enable **Always use scientific notation** in the game's settings so gold and prices use the same readable format.

## Run

From the project directory:

```sh
go mod download
go run . -mode help
go run . -mode shot
go run . -mode click -x 700 -y 400
go run . -mode run
go run . -mode run -hero-levels
go run . -mode run -hero-levels -skills
go run . -mode run -hero-levels -duration 10m
go run . -mode run -x 700 -y 400 -interval 100ms -fish-interval 1s -duration 10m
```

The example coordinates are placeholders. Use pixel coordinates from the saved screenshot of the primary display for `-x` and `-y`, including on Retina displays. The bot validates these coordinates and converts screenshot pixels to desktop points on macOS. By default, `shot` saves `artifacts/screenshot.png` and creates the directory if needed. Put other generated test files in `artifacts/` too; Git ignores this directory. You can choose another screenshot path with `-out`. After starting `shot`, `click`, or `run`, you have 5 seconds (`-delay`) to switch to the game. For `click` or optional monster clicks in `run`, use coordinates inside the monster area.

`run` starts paused: press **F8** after the delay to start, and press F8 again to pause or resume from any window. On some Mac keyboards, press Fn+F8. Pausing blocks new input and invalidates decisions from earlier screenshots. An OCR or detection call already in progress can finish, but its result cannot trigger input after pause/resume. Without `-duration`, it runs until Ctrl+C; a positive `-duration` sets a wall-time limit that also counts while paused. If F8 does not resume the bot, check Accessibility permission for the app running it. Pause before switching away from the game: clicks go to the current desktop.

Fish detection uses the [Clicker Heroes Orange Fish image from StickPNG](https://www.stickpng.com/img/games/clicker-heroes/clicker-heroes-orange-fish), listed there for personal use only. A detected fish stays suppressed through small position changes and isolated missed detections; three consecutive fish-free scans rearm collection. `run` extracts SIFT features from the fish image and each screenshot, matches them with OpenCV, and clicks the center of a fish when enough matches agree on its position, size, and rotation.

With `-hero-levels`, `run` verifies the full-screen Heroes layout, drags the scrollbar only when needed, and requires a confirmed bottom position. RobotGo smoothly drags the thumb to the bottom edge of the display with the mouse button held, using 0.5-1.5 ms library step delays for faster smooth movement. F8 takes effect after an ongoing library drag finishes and releases the button. It considers the latest hero and its successor. A missing or obscured row skips the purchase; it never falls back to an earlier owned hero. Keep the Heroes tab open and the game filling the primary display. Lists without a recognizable scrollbar are currently skipped.

The bot keeps the persistent purchase quantity at `x1`, cycling with `T` only if another quantity is selected. It reads gold and the next price from cropped regions, then holds `Q` only during the hero purchase click to buy MAX levels. Hero purchase clicks hold the left button for 100 ms and wait 100 ms after release before moving the pointer or releasing `Q`. Both held inputs are released on errors or cancellation; no MAX selection screenshot is needed. When the next hero costs at most ten times current gold, it waits to buy that hero; otherwise it buys MAX levels of the current hero. It moves the cursor away from hero buttons to dismiss tooltips, then confirms a purchase by reading an increased `Lvl` value with a stable list position. Overlay or animation pixel changes alone do not count. OCR has a one-second limit per read and is cancelled when the run stops; scans and input actions execute sequentially, so a busy cycle can take longer than the configured fish interval.

It does not buy upgrades. It checks again every 5 seconds after a purchase or while saving, or every 30 seconds when recognition fails or no hero is available. An unconfirmed click saves timestamped `artifacts/hero-failure-*-before.png` (with a red cross at the click target) and matching `*-after.png`, retries later, and stops hero purchases after three consecutive failures; fish detection continues. After a purchase, `x1` remains selected. Pausing invalidates the pending purchase; the next attempt starts from a fresh screenshot. The [hero progression plan](docs/hero-progression.md) describes recognition and its limits.

With `-skills`, the bot sends skill hotkeys `1,2,4,6,7,3,5,8,9`, when the recognizable Heroes interface is visible. It holds each key for 100 ms and leaves a 50 ms gap after release. It retries the sequence five seconds after completion; locked skills and skills on cooldown are handled by the game. The final `3,5,8,9` group puts Lucky Strikes and Golden Clicks before Energize + Reload, allowing an energized Reload to shorten both cooldowns when both skills actually activated. This follows the active-play combo in the [PC 1.0e12 community guide](https://www.reddit.com/r/ClickerHeroes/comments/ysawex/zone_1_to_1m_walkthrough_clicker_heroes_pc_v10e12/). The preceding skills are a continuous-activation convenience, rather than the guide's staged boss-push strategy. The bot does not inspect readiness: if a skill was unavailable, Reload can affect a different previously used skill. It does not manage permanent energized buffs or a separate Dark Ritual rotation. It neither unlocks skills nor buys their upgrades. Skill input runs sequentially with fish and hero actions and stops at the next key boundary when F8 is pressed; stopping the run releases any held key. The log reports sent hotkeys, not confirmed skill activations.

The [game hotkey reference](docs/hotkeys.md) records keyboard actions, target tabs and sources for future features.

## Check the project

```sh
go fmt ./...
go test ./...
go build .
```

On headless Linux, run tests with `xvfb-run -a go test ./...` because the keyboard hook needs an X display. `Dockerfile.gocv` includes OpenCV and Tesseract with English data and sets `REQUIRE_OCR_TESTS=1`, so missing OCR dependencies fail the test suite. For a native test environment, use `REQUIRE_OCR_TESTS=1 go test ./...` to require OCR checks too. When running `xvfb-run` as a Docker command, use `docker run --init` so its X server startup signal is handled correctly.

`go.mod` declares the module name and dependency versions; `go.sum` records checksums for downloaded modules. `main.go` contains the command modes, and `fish_sift.go` detects the fish in screenshots.
