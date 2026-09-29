# Clicker Heroes bot

Go bot for a running Clicker Heroes game. It can take a screenshot, make one click, and watch for the orange fish clickable. In `run` mode, it scans the screen once per second and clicks a detected fish. Monster auto-clicks are optional.

## Requirements

- **macOS:** Install Go (`brew install go`) and Xcode Command Line Tools (`xcode-select --install`). Grant the app that runs the bot (for example, Terminal) **Accessibility** and **Screen & System Audio Recording** permissions in System Settings > Privacy & Security.
- **Windows:** Install Go (`winget install Golang.Go`) and GCC to build RobotGo (`winget install BrechtSanders.WinLibs.POSIX.UCRT`). Make sure the directory containing `gcc.exe` is in `PATH`.

Check the installation with `go version` and `gcc --version`. If `shot` reports `Capture image not found` on macOS, enable Screen & System Audio Recording for Terminal (or the built app) and restart it. The `click` and `run` modes also require Accessibility permission.

## Run

From the project directory:

```sh
go mod download
go run . -mode help
go run . -mode shot
go run . -mode click -x 700 -y 400
go run . -mode run -duration 10m
go run . -mode run -x 700 -y 400 -interval 100ms -fish-interval 1s -duration 10m
```

The example coordinates are placeholders. By default, `shot` saves `artifacts/screenshot.png` and creates the directory if needed. Put other generated test files in `artifacts/` too; Git ignores this directory. You can choose another screenshot path with `-out`. After starting `shot`, `click`, or `run`, you have 5 seconds (`-delay`) to switch to the game. For `click` or optional monster clicks in `run`, use coordinates inside the monster area.

`run` starts paused: press **F8** after the delay to start, and press F8 again to pause or resume from any window. On some Mac keyboards, press Fn+F8. While paused, the bot does not scan or click; `-duration` still counts wall time. If F8 does not resume the bot, check Accessibility permission for the app running it. `run` also stops after `-duration` or Ctrl+C in the terminal. Pause before switching away from the game: clicks go to the current desktop.

Fish detection uses the [Clicker Heroes Orange Fish image from StickPNG](https://www.stickpng.com/img/games/clicker-heroes/clicker-heroes-orange-fish), listed there for personal use only. The bot searches several sizes and orientations around the full circle in 5-degree steps, so display scaling or a different in-game fish appearance may need further calibration.

## Compare fish detection methods

The `gocv` build tag runs an experimental GoCV matcher next to the existing matcher. It does not change `run`. [GoCV 0.43.0](https://github.com/hybridgroup/gocv/releases/tag/v0.43.0) targets OpenCV 4.13.0; OpenCV 5.0 is tested separately through its Python binding because this GoCV release does not support OpenCV 5. Both experiments use the same generated game-screen fixtures, saved under the ignored `artifacts/gocv-comparison/` directory.

With a local Docker daemon, run these commands from the project directory:

```sh
docker build -f Dockerfile.gocv -t clicker-bot-gocv .
docker run --rm -v "$PWD":/workspace -w /workspace clicker-bot-gocv go test -tags gocv fish.go fish_gocv_test.go -run '^TestCompareFishMatchers$' -v -count=1
docker build -f experiments/Dockerfile.opencv5 -t clicker-bot-opencv5 .
docker run --rm -v "$PWD":/workspace -w /workspace clicker-bot-opencv5 python experiments/compare_opencv5.py
docker run --rm -v "$PWD":/workspace -w /workspace clicker-bot-opencv5 python experiments/compare_features.py
docker run --rm -v "$PWD":/workspace -w /workspace clicker-bot-gocv go test fish.go fish_benchmark_test.go -run '^$' -bench '^BenchmarkFish$' -benchtime=3x -count=3
```

The GoCV test first writes the fixtures. Times include screen conversion and template preparation. The real game screenshot without a fish checks false positives; fish-positive fixtures place the PNG programmatically on that screenshot. Results on live game captures still need verification. Both OpenCV experiments use a score threshold of 0.97 fitted to these fixtures. At 0.99, both found only the upright fish. The GoCV experiment reuses the existing template-generation function; the OpenCV 5 experiment uses OpenCV resizing and rotation, so the difference between their times cannot be attributed to library version alone.

One run on the same remote Linux x86-64 host gave these per-frame times:

| Frame | Existing Go matcher | GoCV / OpenCV 4.13 | OpenCV 5 |
| --- | ---: | ---: | ---: |
| No fish | 2.47 s | 122.66 s | 85.45 s |
| Upright fish | 0.019 s | 0.381 s | 0.310 s |
| Fish rotated 17 degrees | 0.110 s | 5.21 s | 3.80 s |
| Fish rotated 180 degrees | 1.253 s | 64.22 s | 45.43 s |

All three classified these four frames correctly at the fitted threshold. The OpenCV versions perform dense matching at every location for every scale and angle, while the existing matcher rejects most locations after checking four pixels. These measurements do not establish accuracy on real fish screenshots or performance on macOS and Windows.

The feature experiment compares SIFT and ORB from OpenCV 5. It detects keypoints on resized fish references (50, 75, and 200 pixels high for SIFT; 75 and 200 for ORB), matches them to each frame, then checks whether an affine transform has at least four consistent matches and a plausible fish size. The references are prepared before timing each frame. This avoids scanning every position, size, and angle. A simple HSV connected-component search was also tried, but the fish merged with orange UI or the monster in these fixtures, so it did not provide useful candidate regions.

Repeated measurements on the same Linux host and four shared fixtures gave these approximate median per-frame times. The Go benchmark ran three batches of three iterations; SIFT and ORB ran five iterations per frame:

| Frame | Existing Go matcher | SIFT | ORB |
| --- | ---: | ---: | ---: |
| No fish | 2.43 s | 0.14 s | 0.025 s |
| Upright fish | 0.020 s | 0.13 s | missed |
| Fish rotated 17 degrees | 0.105 s | 0.13 s | missed |
| Fish rotated 180 degrees | 1.24 s | 0.13 s | missed |

SIFT found all three inserted fish and rejected the fish-free screenshot. It also found an extra 50-pixel fish rendered with bilinear rotation, unlike the original overlays. ORB rejected the fish-free screenshot but missed every fish. These results use one real fish-free game screenshot plus synthetic fish overlays, so false-positive rate and live-game accuracy remain unknown. The SIFT parameters were selected using these fixtures and require validation on real fish captures. `run` still uses the existing Go matcher.

## Check the project

```sh
go fmt ./...
go test ./...
go build .
```

`go.mod` declares the module name and dependency versions; `go.sum` records checksums for downloaded modules. `main.go` contains the command modes, and `fish.go` detects the fish in screenshots.
