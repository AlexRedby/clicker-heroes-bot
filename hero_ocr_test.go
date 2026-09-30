package main

import (
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"testing"
)

func TestGameNumberAndCroppedOCR(t *testing.T) {
	for input, want := range map[string]float64{"225": 2.3521825, "1.259e71": 71.1000258, "1.168e367": 367.067443} {
		got, ok := parseGameNumber(input)
		if !ok || math.Abs(got-want) > 0.001 {
			t.Fatalf("parseGameNumber(%q) = %v, %t", input, got, ok)
		}
	}
	for _, input := range []string{"", "MAX", "1.2e", "1.2e7x", "-5"} {
		if _, ok := parseGameNumber(input); ok {
			t.Fatalf("accepted %q", input)
		}
	}
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("Tesseract is not installed")
	}
	file, err := os.Open("testdata/no-fish-game-screen.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	screen, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	gold, err := readHeroGold(screen)
	if err != nil || math.Abs(gold-71.1000258) > 0.001 {
		t.Fatalf("gold = %v, error = %v", gold, err)
	}
	price, err := readHeroPrice(screen, image.Pt(81, 210))
	if err != nil || math.Abs(price-2.3521825) > 0.001 {
		t.Fatalf("first hero price = %v, error = %v", price, err)
	}
	largeFile, err := os.Open("testdata/hero-panel-max.png")
	if err != nil {
		t.Fatal(err)
	}
	defer largeFile.Close()
	largeScreen, err := png.Decode(largeFile)
	if err != nil {
		t.Fatal(err)
	}
	largeGold, err := readHeroGold(largeScreen)
	if err != nil || math.Abs(largeGold-367.067443) > 0.001 {
		t.Fatalf("large screenshot gold = %v, error = %v", largeGold, err)
	}
}
