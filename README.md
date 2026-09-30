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
go run . -mode run -hero-levels -duration 10m
go run . -mode run -x 700 -y 400 -interval 100ms -fish-interval 1s -duration 10m
```

The example coordinates are placeholders. Use pixel coordinates from the saved screenshot of the primary display for `-x` and `-y`, including on Retina displays. The bot validates these coordinates and converts screenshot pixels to desktop points on macOS. By default, `shot` saves `artifacts/screenshot.png` and creates the directory if needed. Put other generated test files in `artifacts/` too; Git ignores this directory. You can choose another screenshot path with `-out`. After starting `shot`, `click`, or `run`, you have 5 seconds (`-delay`) to switch to the game. For `click` or optional monster clicks in `run`, use coordinates inside the monster area.

`run` starts paused: press **F8** after the delay to start, and press F8 again to pause or resume from any window. On some Mac keyboards, press Fn+F8. Pausing blocks new input and invalidates decisions from earlier screenshots. An OCR or detection call already in progress can finish, but its result cannot trigger input after pause/resume. Without `-duration`, it runs until Ctrl+C; a positive `-duration` sets a wall-time limit that also counts while paused. If F8 does not resume the bot, check Accessibility permission for the app running it. Pause before switching away from the game: clicks go to the current desktop.

Fish detection uses the [Clicker Heroes Orange Fish image from StickPNG](https://www.stickpng.com/img/games/clicker-heroes/clicker-heroes-orange-fish), listed there for personal use only. A detected fish stays suppressed through small position changes and isolated missed detections; three consecutive fish-free scans rearm collection. `run` extracts SIFT features from the fish image and each screenshot, matches them with OpenCV, and clicks the center of a fish when enough matches agree on its position, size, and rotation.

With `-hero-levels`, `run` verifies the full-screen Heroes layout, drags the scrollbar only when needed, and requires a confirmed bottom position. It considers the latest hero and its successor. A missing or obscured row skips the purchase; it never falls back to an earlier owned hero. Keep the Heroes tab open and the game filling the primary display. Lists without a recognizable scrollbar are currently skipped.

The bot verifies `x1` selection before reading gold and the next price from cropped regions, then verifies `MAX` before purchasing. When the next hero costs at most ten times current gold, it waits to buy that hero; otherwise it buys MAX levels of the current hero. It checks that the same hero is still present, then confirms a purchase by reading an increased `Lvl` value with a stable list position. Overlay or animation pixel changes alone do not count. OCR has a one-second limit per read and is cancelled when the run stops; scans and input actions execute sequentially, so a busy cycle can take longer than the configured fish interval.

It does not buy upgrades. It checks again every 5 seconds after a purchase or while saving, or every 30 seconds when recognition fails or no hero is available. An unconfirmed click saves timestamped `artifacts/hero-failure-*-before.png` (with a red cross at the click target) and matching `*-after.png`, retries later, and stops hero purchases after three consecutive failures; fish detection continues. If pausing interrupts `x1`/`MAX` restoration, the next attempt verifies the quantity again before any purchase. The [hero progression plan](docs/hero-progression.md) describes recognition and its limits.

## Check the project

```sh
go fmt ./...
go test ./...
go build .
```

On headless Linux, run tests with `xvfb-run -a go test ./...` because the keyboard hook needs an X display. `Dockerfile.gocv` includes OpenCV and Tesseract with English data and sets `REQUIRE_OCR_TESTS=1`, so missing OCR dependencies fail the test suite. For a native test environment, use `REQUIRE_OCR_TESTS=1 go test ./...` to require OCR checks too. When running `xvfb-run` as a Docker command, use `docker run --init` so its X server startup signal is handled correctly.

`go.mod` declares the module name and dependency versions; `go.sum` records checksums for downloaded modules. `main.go` contains the command modes, and `fish_sift.go` detects the fish in screenshots.
