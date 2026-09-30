package main

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"reflect"
	"testing"
	"time"
)

func TestHeroRunner(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	// Failed confirmations write diagnostics into this temporary directory.
	t.Chdir(t.TempDir())
	for _, scenario := range []string{"purchase", "saving", "fish before purchase", "fish during confirmation", "pause during gold", "pause during price", "pause during initial level", "pause during final level", "pause during final success", "pause during final fish scan", "cancel during final level", "three failures"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controls := pauseControl{}
			plan := heroRunner{enabled: true, failures: 2}
			var events []string
			reads, fishScans := 0, 0
			interrupt := func() { controls.toggle(); controls.toggle() }
			input := heroInput{
				capture:   func() (image.Image, error) { return screen, nil },
				move:      func(image.Point) error { return nil },
				drag:      func(image.Point, image.Point) error { t.Fatal("already at bottom: unexpected drag"); return nil },
				keyTap:    func(string) error { t.Fatal("already in x1: unexpected quantity change"); return nil },
				keyToggle: func(key, direction string) error { events = append(events, key+" "+direction); return nil },
				click: func(point image.Point) error {
					if point != image.Pt(204, 894) {
						t.Fatalf("wrong purchase target: %v", point)
					}
					events = append(events, "click")
					return nil
				},
			}
			read := heroReaders{
				gold: func(context.Context, image.Image) (float64, error) {
					if scenario == "pause during gold" {
						interrupt()
					}
					return 100, nil
				},
				price: func(context.Context, image.Image, image.Point) (float64, error) {
					if scenario == "pause during price" {
						interrupt()
					}
					if scenario == "saving" {
						return 100.5, nil
					}
					return 102, nil
				},
				level: func(context.Context, image.Image, image.Point) (int, error) {
					reads++
					if scenario == "pause during initial level" {
						interrupt()
					}
					if reads == 6 {
						if scenario == "pause during final level" || scenario == "pause during final success" {
							interrupt()
						}
						if scenario == "cancel during final level" {
							cancel()
						}
					}
					if (scenario == "purchase" && reads > 1) || (scenario == "pause during final success" && reads == 6) {
						return 101, nil
					}
					return 100, nil
				},
			}
			fish := func(image.Image) (bool, error) {
				fishScans++
				if scenario == "pause during final fish scan" && fishScans == 7 {
					interrupt()
				}
				return scenario == "fish before purchase" || (scenario == "fish during confirmation" && fishScans == 3), nil
			}
			started := time.Now()
			acted, err := plan.run(ctx, &controls, controls.snapshot(), input, read, fish)
			if err != nil {
				t.Fatal(err)
			}
			wantClick := scenario != "saving" && scenario != "fish before purchase" && scenario != "pause during gold" && scenario != "pause during price" && scenario != "pause during initial level"
			if acted != wantClick {
				t.Fatalf("acted=%t, want %t", acted, wantClick)
			}
			if wantClick && !reflect.DeepEqual(events, []string{"q down", "click", "q up"}) {
				t.Fatalf("purchase input: %v", events)
			}
			if !wantClick && len(events) != 0 {
				t.Fatalf("unexpected input: %v", events)
			}
			wantFailures, wantEnabled := 2, true
			if scenario == "purchase" {
				wantFailures = 0
			}
			if scenario == "three failures" {
				wantFailures, wantEnabled = 3, false
			}
			if plan.failures != wantFailures || plan.enabled != wantEnabled {
				t.Fatalf("failures=%d enabled=%t; want %d, %t", plan.failures, plan.enabled, wantFailures, wantEnabled)
			}
			if scenario == "three failures" && (plan.nextScan.Before(started.Add(30*time.Second)) || plan.nextScan.After(time.Now().Add(30*time.Second))) {
				t.Fatal("failed purchase did not back off")
			}
			if scenario == "pause during final level" || scenario == "pause during final success" || scenario == "cancel during final level" {
				if reads != 6 {
					t.Fatalf("final confirmation not exercised: reads=%d", reads)
				}
			}
			if scenario == "pause during final fish scan" && fishScans != 7 {
				t.Fatalf("final fish scan not exercised: %d", fishScans)
			}
			// Neither a stopped runner nor one awaiting its deadline should capture or act again.
			input.capture = func() (image.Image, error) { t.Fatal("runner ignored its deadline or disabled state"); return nil, nil }
			if acted, err := plan.run(ctx, &controls, controls.snapshot(), input, read, fish); acted || err != nil {
				t.Fatalf("second run acted=%t error=%v", acted, err)
			}
		})
	}
}

func TestHeroRunnerRetry(t *testing.T) {
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	t.Chdir(t.TempDir())
	controls := pauseControl{}
	plan := heroRunner{enabled: true}
	input := heroInput{
		capture:   func() (image.Image, error) { return screen, nil },
		move:      func(image.Point) error { return nil },
		keyToggle: func(string, string) error { return nil },
		click:     func(image.Point) error { return nil },
	}
	read := heroReaders{
		gold:  func(context.Context, image.Image) (float64, error) { return 100, nil },
		price: func(context.Context, image.Image, image.Point) (float64, error) { return 102, nil },
		level: func(context.Context, image.Image, image.Point) (int, error) { return 100, nil },
	}
	for failure := 1; failure <= 3; failure++ {
		plan.nextScan = time.Time{}
		started := time.Now()
		if acted, err := plan.run(context.Background(), &controls, controls.snapshot(), input, read, func(image.Image) (bool, error) { return false, nil }); !acted || err != nil {
			t.Fatalf("retry %d: acted=%t error=%v", failure, acted, err)
		}
		if plan.failures != failure || plan.enabled != (failure < 3) || plan.nextScan.Before(started.Add(30*time.Second)) || plan.nextScan.After(time.Now().Add(30*time.Second)) {
			t.Fatalf("retry %d: %+v", failure, plan)
		}
	}
}

func TestHeroRunnerUnlock(t *testing.T) {
	before := loadTestImage(t, "testdata/hero-gog-before.png")
	after := loadTestImage(t, "testdata/hero-gog-tooltip.png")
	frames := make([]image.Image, 2)
	for i, original := range []image.Image{before, after} {
		frame := image.NewRGBA(original.Bounds())
		draw.Draw(frame, frame.Bounds(), original, original.Bounds().Min, draw.Src)
		b := frame.Bounds()
		// Select x1 and dismiss the tooltip in the captured post-purchase frame.
		for _, quantity := range []int{122, 200, 278, 356, 435} {
			c := color.RGBA{R: 255, G: 210, B: 30, A: 255}
			if quantity == 122 {
				c = color.RGBA{R: 240, G: 140, B: 20, A: 255}
			}
			x, y := b.Dx()*(quantity-25)/1000, b.Dy()*345/1000
			draw.Draw(frame, image.Rect(x-2, y-2, x+3, y+3), image.NewUniform(c), image.Point{}, draw.Src)
		}
		track := image.Rect(b.Dx()*445/1000, b.Dy()*32/100, b.Dx()*495/1000, b.Dy()*965/1000)
		draw.Draw(frame, track, before, track.Min, draw.Src)
		frames[i] = frame
	}
	controls := pauseControl{}
	plan := heroRunner{enabled: true}
	purchased, reads := false, 0
	input := heroInput{
		capture: func() (image.Image, error) {
			if purchased {
				return frames[1], nil
			}
			return frames[0], nil
		},
		move:      func(image.Point) error { return nil },
		keyToggle: func(string, string) error { return nil },
		click:     func(image.Point) error { purchased = true; return nil },
	}
	read := heroReaders{
		gold: func(context.Context, image.Image) (float64, error) {
			t.Fatal("unlock should not compare next hero prices")
			return 0, nil
		},
		level: func(context.Context, image.Image, image.Point) (int, error) {
			if !purchased {
				t.Fatal("unowned hero has no level to read")
			}
			reads++
			return 227, nil
		},
	}
	if acted, err := plan.run(context.Background(), &controls, controls.snapshot(), input, read, func(image.Image) (bool, error) { return false, nil }); !acted || err != nil || reads != 1 || plan.failures != 0 {
		t.Fatalf("unlock: acted=%t error=%v reads=%d failures=%d", acted, err, reads, plan.failures)
	}
}
