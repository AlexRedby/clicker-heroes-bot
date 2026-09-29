# Clicker Heroes bot

Go bot for a running Clicker Heroes game. It can take a screenshot, make one click, and watch for the clickable orange fish. In `run` mode, it checks the screen for fish repeatedly (once per second by default) and clicks a detected fish. Monster auto-clicks are optional.

## Requirements

- **macOS:** Install Go (`brew install go`), Xcode Command Line Tools (`xcode-select --install`), and OpenCV 4.13 using the [GoCV macOS instructions](https://gocv.io/getting-started/macos/). Grant the app that runs the bot (for example, Terminal) **Accessibility** and **Screen & System Audio Recording** permissions in System Settings > Privacy & Security.
- **Windows:** Install Go (`winget install Golang.Go`) and GCC to build RobotGo (`winget install BrechtSanders.WinLibs.POSIX.UCRT`). Make sure the directory containing `gcc.exe` is in `PATH`.

Fish detection uses GoCV 0.43.0 with native OpenCV 4.13. Install OpenCV before building on every platform; see the [GoCV installation guides](https://gocv.io/getting-started/). On macOS or Linux, verify it with `pkg-config --modversion opencv4` (expect 4.13.x). Also check `go version` and `gcc --version`. If `shot` reports `Capture image not found` on macOS, enable Screen & System Audio Recording for Terminal (or the built app) and restart it. The `click` and `run` modes also require Accessibility permission.

## Run

From the project directory:

```sh
go mod download
go run . -mode help
go run . -mode shot
go run . -mode click -x 700 -y 400
go run . -mode run -duration 10m
go run . -mode run -hero-levels -duration 10m
go run . -mode run -x 700 -y 400 -interval 100ms -fish-interval 1s -duration 10m
```

The example coordinates are placeholders. By default, `shot` saves `artifacts/screenshot.png` and creates the directory if needed. Put other generated test files in `artifacts/` too; Git ignores this directory. You can choose another screenshot path with `-out`. After starting `shot`, `click`, or `run`, you have 5 seconds (`-delay`) to switch to the game. For `click` or optional monster clicks in `run`, use coordinates inside the monster area.

`run` starts paused: press **F8** after the delay to start, and press F8 again to pause or resume from any window. On some Mac keyboards, press Fn+F8. While paused, the bot does not scan or click; `-duration` still counts wall time. If F8 does not resume the bot, check Accessibility permission for the app running it. `run` also stops after `-duration` or Ctrl+C in the terminal. Pause before switching away from the game: clicks go to the current desktop.

Fish detection uses the [Clicker Heroes Orange Fish image from StickPNG](https://www.stickpng.com/img/games/clicker-heroes/clicker-heroes-orange-fish), listed there for personal use only. `run` extracts SIFT features from the fish image and each screenshot, matches them with OpenCV, and clicks the center of a fish when enough matches agree on its position, size, and rotation.

With `-hero-levels`, `run` scrolls the Heroes list to the bottom, searches upward for the latest affordable hero, and clicks its level button. It checks again every 5 seconds after a purchase, or every 30 seconds if none is available. It uses the quantity selected in the game's `Lvl` bar, including `MAX`. Keep the Heroes tab open and the game filling the screen. If the level text does not visibly change after a click, hero purchases stop for that run; fish detection continues. F8 pauses clicks and scrolling.

## Check the project

```sh
go fmt ./...
go test ./...
go build .
```

On headless Linux, run tests with `xvfb-run -a go test ./...` because the keyboard hook needs an X display.

`go.mod` declares the module name and dependency versions; `go.sum` records checksums for downloaded modules. `main.go` contains the command modes, and `fish_sift.go` detects the fish in screenshots.
