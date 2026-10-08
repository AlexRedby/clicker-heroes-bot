package bot

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"

	"github.com/go-vgo/robotgo"
)

// Main runs the command-line modes and the shared game automation pipeline.
func Main() {
	robotgo.Scale = false
	mode := flag.String("mode", "help", "help, shot, click, run, ancients-plan, relics-plan, transcension-plan, timelapse-plan, or achievements-plan")
	output := flag.String("out", "artifacts/screenshot.png", "screenshot file for shot or preview JSON for transcension-plan, timelapse-plan or achievements-plan mode")
	save := flag.String("save", "", "exported save for preview modes, run Outsider roster, or Ascension invested-soul baseline (read only)")
	achievementGoals := flag.String("achievement-goals", "", "ordered goal state/config JSON for achievements-plan or run (run persists progress)")
	outsiderShot := flag.String("screenshot", "", "existing Outsiders screenshot to reconcile with -save in transcension-plan mode (read only)")
	ancientReserve := flag.String("ancient-reserve", "0", "Optional Hero Souls spending floor beyond the calculator soul bank")
	ancientSkillRate := flag.Float64("ancient-skill-rate", 1, "calculator allocation to skill Ancients, from 0 to 1")
	ancientBeyond8k := flag.Bool("ancient-beyond8k", false, "best hero is levelled beyond 8000; changes calculator gold allocation")
	ancientSave := flag.String("ancients-save", "", "exported save for one Ancient purchase batch before normal run actions")
	exportDir := flag.String("export-dir", "", "Save folder; suggest relics, buy Ancients at startup/after Ascension, and inspect relics before Ascension")
	ancientPlanOutput := flag.String("ancient-plan-out", "artifacts/ancients-plan.json", "Ancient purchase plan with gild and Transcension previews")
	x := flag.Int("x", 0, "screen X coordinate for click or optional monster clicks in run mode")
	y := flag.Int("y", 0, "screen Y coordinate for click or optional monster clicks in run mode")
	interval := flag.Duration("interval", 100*time.Millisecond, "time between optional monster clicks in run mode")
	fishInterval := flag.Duration("fish-interval", time.Second, "time between fish scans in run mode")
	progression := flag.Bool("progression", false, "automate heroes, skills, owned Auto Clickers, gild gifts, boss progression, Ascension and Ancients; requires -export-dir")
	transcension := flag.Bool("transcension", false, "enable guarded Transcension integration; reset remains blocked until native recovery and spending are verified")
	gildInterval := flag.Duration("gild-interval", 5*time.Minute, "time between earned gild gift checks")
	ascensionMinGain := flag.Float64("ascension-min-gain", 0.25, "minimum Ascension reward as a fraction of soul capital (0.25 means 25%)")
	ascensionStall := flag.Duration("ascension-stall", 3*time.Minute, "fallback stall before Ascension when a full combat failure was not observed")
	mercenaries := flag.Bool("mercenaries", false, "collect mercenary rewards and send new quests without spending rubies")
	duration := flag.Duration("duration", 0, "maximum run time (0 means unlimited)")
	stats := flag.Bool("stats", false, "print pipeline timing and analysis counters when run stops")
	windowed := flag.Bool("windowed", false, "detect a visible game viewport on Windows/macOS; -x/-y use viewport screenshot pixels")
	delay := flag.Duration("delay", 5*time.Second, "time to focus the game before shot or click (run waits for F8)")
	flag.DurationVar(&ocrTimeout, "ocr-timeout", ocrTimeout, "maximum time per Tesseract execution (excluding queue wait)")
	flag.Parse()

	if *mode == "help" {
		flag.Usage()
		return
	}
	if *achievementGoals != "" && *mode != "achievements-plan" && *mode != "run" {
		log.Fatal("-achievement-goals requires achievements-plan or run mode")
	}
	if *outsiderShot != "" && *mode != "transcension-plan" {
		log.Fatal("-screenshot requires transcension-plan mode")
	}
	if *windowed {
		if *mode != "shot" && *mode != "click" && *mode != "run" {
			log.Fatal("-windowed requires shot, click or run mode")
		}
		if err := windowedPreflight(); err != nil {
			log.Fatal(err)
		}
	}

	var hasX, hasY bool
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "x":
			hasX = true
		case "y":
			hasY = true
		}
	})
	if *mode == "click" && (!hasX || !hasY) {
		log.Fatal("click requires both -x and -y")
	}
	if *mode == "run" && hasX != hasY {
		log.Fatal("run requires both -x and -y when monster clicks are enabled")
	}
	if *mode == "shot" || *mode == "click" {
		if *delay < 0 {
			log.Fatal("-delay must be non-negative")
		}
		fmt.Printf("starting %s in %s; focus the game now\n", *mode, *delay)
		time.Sleep(*delay)
	}

	var err error
	switch *mode {
	case "ancients-plan":
		if *save == "" {
			err = errors.New("ancients-plan requires -save")
			break
		}
		var plan ancientPlan
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		plan, err = calculateAncients(ctx, *save, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
		if err == nil {
			err = writeAncientPlan(*ancientPlanOutput, plan)
		}
	case "achievements-plan", "timelapse-plan", "transcension-plan":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		path := ""
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "out" {
				path = *output
			}
		})
		switch *mode {
		case "achievements-plan":
			err = previewAchievements(ctx, *save, *achievementGoals, path, os.Stdout)
		case "timelapse-plan":
			err = previewTimelapse(ctx, *save, path, os.Stdout, ancientcalc.BuildOptions{Mode: ancientcalc.HybridBuild, Reserve: *ancientReserve, SkillRate: *ancientSkillRate, Beyond8k: *ancientBeyond8k})
		case "transcension-plan":
			if *outsiderShot != "" {
				err = previewOutsiders(ctx, *save, *outsiderShot, path, os.Stdout)
			} else {
				err = previewTranscension(ctx, *save, path, os.Stdout)
			}
		}
	case "relics-plan":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		err = previewRelics(ctx, *save, os.Stdout)
	case "shot":
		if *windowed {
			var capture windowCapture
			var img image.Image
			img, err = capture.capture()
			if err == nil {
				if shot := img.(*viewportImage); shot.reason != "" {
					err = errors.New(shot.reason)
				} else {
					err = saveImage(*output, shot.Image)
					if err == nil {
						fmt.Println("saved viewport pixels", *output)
					}
				}
			}
		} else {
			err = saveScreenshot(*output)
		}
	case "click":
		if *windowed {
			var capture windowCapture
			var img image.Image
			img, err = capture.capture()
			if err == nil {
				shot := img.(*viewportImage)
				if shot.reason != "" {
					err = errors.New(shot.reason)
				} else {
					err = bindViewportInput(context.Background(), heroInput{}, shot.geometry).click(image.Pt(*x, *y))
				}
			}
		} else {
			err = clickAt(context.Background(), *x, *y)
		}
	case "run":
		var export *saveExportOptions
		if *exportDir != "" {
			if *ancientSave != "" {
				err = errors.New("use either -export-dir or -ancients-save")
				break
			}
			if _, e := snapshotExports(*exportDir); e != nil {
				err = fmt.Errorf("export directory: %w", e)
				break
			}
			if _, e := ancientcalc.Value(strings.TrimSuffix(*ancientReserve, "%")); e != nil {
				err = fmt.Errorf("soul reserve: %w", e)
				break
			}
			if !(*ancientSkillRate >= 0 && *ancientSkillRate <= 1) {
				err = errors.New("-ancient-skill-rate must be between 0 and 1")
				break
			}
			export = &saveExportOptions{dir: *exportDir, reserve: *ancientReserve, skillRate: *ancientSkillRate, beyond8k: *ancientBeyond8k, planOutput: *ancientPlanOutput}
		}
		var goals *achievementSession
		if *achievementGoals != "" {
			goals, err = loadAchievementSession(*achievementGoals)
			if err != nil {
				break
			}
		}
		options, e := configureRun(pipelineOptions{
			progression: *progression, transcension: *transcension, mercenaries: *mercenaries, monster: hasX,
			monsterPoint: image.Pt(*x, *y), clickInterval: *interval, fishInterval: *fishInterval,
			gildInterval: *gildInterval, ascensionStall: *ascensionStall, ascensionMinGain: *ascensionMinGain,
			export: export, windowed: *windowed, achievements: goals,
		})
		if e != nil {
			err = e
			break
		}
		var plan *ancientPlan
		if *ancientSave != "" {
			value, e := calculateAncients(context.Background(), *ancientSave, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
			if e != nil {
				err = e
				break
			}
			plan = &value
			if err = writeAncientPlan(*ancientPlanOutput, value); err != nil {
				break
			}
		}
		capitalPlan := plan
		if *progression && *save != "" {
			value, e := calculateAncients(context.Background(), *save, *ancientReserve, *ancientSkillRate, *ancientBeyond8k)
			if e != nil {
				err = e
				break
			}
			capitalPlan = &value
		}
		capital, e := ascensionSoulCapital(capitalPlan)
		if e != nil {
			err = e
			break
		}
		var outsiderBase *ancientcalc.TranscensionPreview
		if *save != "" {
			if *progression {
				outsiderBase = capitalPlan.Transcension
				if capitalPlan.TranscensionError != "" {
					fmt.Println("Outsider save preview unavailable:", capitalPlan.TranscensionError)
				}
			} else {
				value, e := readTranscensionPreview(context.Background(), *save)
				if e != nil {
					fmt.Println("Outsider save preview unavailable:", e)
				} else {
					outsiderBase = &value
				}
			}
		}

		options.ascensionCapital, options.ancientPlan, options.outsiderBase = capital, plan, outsiderBase
		err = runBot(options, *duration, *stats)
	default:
		err = fmt.Errorf("unknown mode %q", *mode)
	}
	if err != nil {
		log.Fatal(err)
	}
}

func saveScreenshot(path string) error {
	img, err := robotgo.CaptureImg()
	if err != nil {
		return fmt.Errorf("capture screen: %w", err)
	}
	if img == nil {
		return errors.New("capture screen returned no image")
	}
	if err := saveImage(path, img); err != nil {
		return err
	}
	fmt.Println("saved", path)
	return nil
}

func saveImage(path string, img image.Image) error {
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		return fmt.Errorf("encode screenshot: %w", err)
	}
	return writeScreenshot(path, data.Bytes())
}

func writeScreenshot(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create screenshot directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write screenshot: %w", err)
	}
	return nil
}
