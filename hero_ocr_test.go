package main

import (
	"bytes"
	"context"
	"errors"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGameNumberAndCroppedOCR(t *testing.T) {
	for input, want := range map[string]float64{"0": math.Inf(-1), "225": 2.3521825, "1.259e71": 71.1000258, "1.168e367": 367.067443} {
		got, ok := parseGameNumber(input)
		if !ok || (math.IsInf(want, -1) && !math.IsInf(got, -1)) || (!math.IsInf(want, -1) && math.Abs(got-want) > 0.001) {
			t.Fatalf("parseGameNumber(%q) = %v, %t", input, got, ok)
		}
	}
	for _, input := range []string{"", "MAX", "1.2e", "1.2e7x", "-5", "91537e69", "9.537069"} {
		if _, ok := parseGameNumber(input); ok {
			t.Fatalf("accepted %q", input)
		}
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal("required Tesseract is not installed")
		}
		t.Skip("Tesseract is not installed")
	}
	screen := loadTestImage(t, "testdata/no-fish-game-screen.jpg")
	gold, err := readHeroGold(context.Background(), screen)
	if err != nil || math.Abs(gold-71.1000258) > 0.001 {
		t.Fatalf("gold = %v, error = %v", gold, err)
	}
	price, err := readHeroPrice(context.Background(), screen, image.Pt(81, 210))
	if err != nil || math.Abs(price-2.3521825) > 0.001 {
		t.Fatalf("first hero price = %v, error = %v", price, err)
	}
	button, found := findHeroLevelButton(screen)
	if !found {
		t.Fatal("runtime hero button missing")
	}
	terraPrice, err := readHeroPrice(context.Background(), screen, button)
	if err != nil || math.Abs(terraPrice-(69+math.Log10(9.537))) > 0.001 {
		t.Fatalf("runtime Terra price=%v error=%v", terraPrice, err)
	}
	largeScreen := loadTestImage(t, "testdata/hero-panel-max.png")
	largeGold, err := readHeroGold(context.Background(), largeScreen)
	if err != nil || math.Abs(largeGold-367.067443) > 0.001 {
		t.Fatalf("large screenshot gold = %v, error = %v", largeGold, err)
	}
	if level, err := readHeroLevel(context.Background(), screen, image.Pt(81, 498)); err != nil || level != 5 {
		t.Fatalf("Terra level = %d, error = %v", level, err)
	}
	if level, err := readHeroLevel(context.Background(), screen, image.Pt(81, 210)); err != nil || level != 1300 {
		t.Fatalf("Frostleaf level = %d, error = %v", level, err)
	}
	if err := checkHeroOCR(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHeroLevelOnRealScreensAndOverlay(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	ctx := context.Background()
	for _, tc := range []struct {
		path    string
		y, want int
	}{
		{"testdata/hero-panel-max.png", 896, 4367},
		{"testdata/hero-owned-disabled.png", 917, 4695},
		{"testdata/hero-tsuchi-x1.png", 894, 11873},
		{"testdata/hero-scrollbar-before.png", 894, 1998},
		{"testdata/fish-over-scrollbar.png", 894, 2035},
	} {
		screen := loadTestImage(t, tc.path)
		got, err := readHeroLevel(ctx, screen, image.Pt(204, tc.y))
		if err != nil || got != tc.want {
			t.Fatalf("%s level=%d error=%v", tc.path, got, err)
		}
	}
	screen := loadTestImage(t, "testdata/no-fish-game-screen.jpg")
	button, _ := findHeroLevelButton(screen)
	fishFile, err := os.ReadFile("assets/orange-fish.png")
	if err != nil {
		t.Fatal(err)
	}
	fish, err := png.Decode(bytes.NewReader(fishFile))
	if err != nil {
		t.Fatal(err)
	}
	small := image.NewRGBA(image.Rect(0, 0, fish.Bounds().Dx()*50/fish.Bounds().Dy(), 50))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), fish, fish.Bounds(), draw.Src, nil)
	after := image.NewRGBA(screen.Bounds())
	draw.Draw(after, after.Bounds(), screen, screen.Bounds().Min, draw.Src)
	pos := image.Pt(screen.Bounds().Dx()*32/100, button.Y-screen.Bounds().Dy()*3/100)
	draw.Draw(after, small.Bounds().Add(pos), small, image.Point{}, draw.Over)
	level, err := readHeroLevel(ctx, after, button)
	if err == nil && level > 5 {
		t.Fatalf("fish overlay falsely increased level: %d", level)
	}
	// A missing/covered level must not be interpreted as zero or a successful purchase.
	draw.Draw(after, image.Rect(266, 480, 390, 510), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if _, err := readHeroLevel(ctx, after, button); err == nil {
		t.Fatal("covered level was accepted")
	}
}

func TestOCRCancellationAndDiagnostics(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix only")
	}
	original := tesseractExecutable
	t.Cleanup(func() { tesseractExecutable = original })
	fake := filepath.Join(t.TempDir(), "tesseract")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	tesseractExecutable = fake
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runTesseract(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("slow OCR cancellation: %v elapsed=%s", err, time.Since(start))
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho missing-traineddata >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = runTesseract(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "missing-traineddata") {
		t.Fatalf("OCR diagnostics lost: %v", err)
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'List of available languages (1):'\necho osd\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkHeroOCR(context.Background()); err == nil {
		t.Fatal("missing English data accepted")
	}
}

func TestHeroEconomyOnUserScreens(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	ctx := context.Background()
	for _, tc := range []struct {
		path     string
		mantissa float64
		exponent int
	}{
		{"testdata/hero-gog-before.png", 2.730, 227},
		{"testdata/hero-gog-tooltip.png", 1.710, 226},
		{"testdata/hero-economy-x1.png", 1.651, 449},
		{"testdata/hero-tsuchi-x1.png", 8.465, 848},
	} {
		screen := loadTestImage(t, tc.path)
		gold, err := readHeroGold(ctx, screen)
		if err != nil || math.Abs(gold-(float64(tc.exponent)+math.Log10(tc.mantissa))) > 0.001 {
			t.Fatalf("%s gold=%v error=%v", tc.path, gold, err)
		}
	}
	screen := loadTestImage(t, "testdata/hero-economy-x1.png")
	current, found := findHeroLevelButton(screen)
	if !found {
		t.Fatal("current hero not found")
	}
	next, found := findNextHeroButton(screen, current)
	if !found {
		t.Fatal("locked next hero not found")
	}
	for _, tc := range []struct {
		button   image.Point
		mantissa float64
		exponent int
	}{
		{current, 2.955, 414},
		{next, 3.828, 499},
	} {
		price, err := readHeroPrice(ctx, screen, tc.button)
		if err != nil || math.Abs(price-(float64(tc.exponent)+math.Log10(tc.mantissa))) > 0.001 {
			t.Fatalf("button=%v price=%v error=%v", tc.button, price, err)
		}
	}
	if level, err := readHeroLevel(ctx, screen, current); err != nil || level != 6122 {
		t.Fatalf("Wepwawet level=%d error=%v", level, err)
	}
	// A fully covered price must remain unreadable rather than turn into a purchase decision.
	covered := image.NewRGBA(screen.Bounds())
	draw.Draw(covered, covered.Bounds(), screen, screen.Bounds().Min, draw.Src)
	b := screen.Bounds()
	draw.Draw(covered, image.Rect(b.Dx()*52/1000, next.Y, b.Dx()*15/100, next.Y+b.Dy()*6/100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	if _, err := readHeroPrice(ctx, covered, next); err == nil {
		t.Fatal("covered price was accepted")
	}
}

func TestOCRProcessBudget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix only")
	}
	original, originalTimeout := tesseractExecutable, ocrTimeout
	t.Cleanup(func() { tesseractExecutable, ocrTimeout = original, originalTimeout })
	t.Setenv("OMP_THREAD_LIMIT", "999")
	t.Setenv("CH_OCR_TEST_ENV", "preserved")
	fake := filepath.Join(t.TempDir(), "tesseract")
	tesseractExecutable = fake
	write := func(script string) {
		t.Helper()
		if err := os.WriteFile(fake, []byte("#!/bin/sh\n"+script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	write("printf '%s:%s' \"$OMP_THREAD_LIMIT\" \"$CH_OCR_TEST_ENV\"\n")
	out, err := runTesseract(context.Background(), nil)
	if err != nil || out != "1:preserved" {
		t.Fatalf("child environment: %q %v", out, err)
	}
	// A valid slow execution must survive the old one-second deadline.
	write("sleep 1.1\necho done\n")
	out, err = runTesseract(context.Background(), nil)
	if err != nil || strings.TrimSpace(out) != "done" {
		t.Fatalf("slow valid OCR: %q %v", out, err)
	}
	write("exec sleep 10\n")
	ocrTimeout = 30 * time.Millisecond
	start := time.Now()
	_, err = runTesseract(context.Background(), nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("execution budget not honored: %v", err)
	}
	// Cancellation remains effective before an execution slot is available.
	ocrSlots <- struct{}{}
	ocrSlots <- struct{}{}
	defer func() { <-ocrSlots; <-ocrSlots }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = runTesseract(ctx, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued cancellation: %v", err)
	}
}

func TestGoldTimeoutDoesNotRetryMask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix only")
	}
	screen := loadTestImage(t, "testdata/hero-tsuchi-x1.png")
	original, originalTimeout := tesseractExecutable, ocrTimeout
	t.Cleanup(func() { tesseractExecutable, ocrTimeout = original, originalTimeout })
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("CH_OCR_TEST_CALLS", calls)
	tesseractExecutable = filepath.Join(t.TempDir(), "tesseract")
	if err := os.WriteFile(tesseractExecutable, []byte("#!/bin/sh\necho call >> \"$CH_OCR_TEST_CALLS\"\nexec sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ocrTimeout = 50 * time.Millisecond
	_, err := readHeroGold(context.Background(), screen)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(calls)
	if err != nil || string(data) != "call\n" {
		t.Fatalf("timeout retried another mask: %q %v", data, err)
	}
}

func TestNonGildedSuccessorPurchaseDecision(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		if os.Getenv("REQUIRE_OCR_TESTS") == "1" {
			t.Fatal(err)
		}
		t.Skip("Tesseract is not installed")
	}
	screen := loadTestImage(t, "testdata/hero-nongilded-successor.jpg")
	now := time.Now()
	frame := gameFrame{id: 1, layout: 1, at: now, image: screen, context: gameContext{known: true, heroes: true, bounds: screen.Bounds()}}
	out, err := readHeroObservation(context.Background(), frame, heroReaders{readHeroGold, readHeroPrice, readHeroLevel}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !out.found || !out.owned || out.level != 99 || math.Abs(out.gold-(1016+math.Log10(9.485))) > 0.001 || math.Abs(out.nextPrice-(1999+math.Log10(2.039))) > 0.001 {
		t.Fatalf("incorrect decision inputs: %+v", out)
	}
	p := heroRunner{enabled: true}
	p.observe(out, observation{}, now)
	a, ok := p.action(now)
	if !ok || a.kind != buyHero || a.point != out.button {
		t.Fatalf("Skogur purchase missing: %+v %t", a, ok)
	}
}
