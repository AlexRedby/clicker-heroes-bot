package main

import (
	"testing"

	"clicker-heroes-bot/internal/ancientcalc"
)

func TestStartupHeroNameKey(t *testing.T) {
	for _, test := range []struct {
		raw, want string
	}{
		{"SirGeorgellKingsGuard", "sirgeorgeiikingsguard"},
		{"Sir George II, King's Guard", "sirgeorgeiikingsguard"},
		{"CID, the Helpful Adventurer", "eidtheheipfuiadventurer"},
		{"The Masked Samurai", "themaskedsamurai"},
		{"NatalialeeApprentice", "nataiiaieeapprentiee"},
		{"Natalia, Ice Apprentice", "nataiiaieeapprentiee"},
		{"ReferiJeratorleeWizard", "referijeratorieewizard"},
		{"Referi Jerator, Ice Wizard", "referijeratorieewizard"},
		{"Aphrodite, Goddess of Love", "aphroditegoddessofiove"},
	} {
		if got := startupHeroNameKey(test.raw); got != test.want {
			t.Errorf("startupHeroNameKey(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

func TestStartupHeroNameKeyOfficialRosterIsCollisionFree(t *testing.T) {
	names, err := ancientcalc.HeroNames()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string, len(names))
	for _, name := range names {
		key := startupHeroNameKey(name)
		if previous, ok := seen[key]; ok && previous != name {
			t.Fatalf("official hero names %q and %q share key %q", previous, name, key)
		}
		seen[key] = name
	}
	if len(seen) != len(names) {
		t.Fatalf("official roster has %d names but %d distinct keys", len(names), len(seen))
	}
}

func TestStartupHeroNameKeyRejectsTruncatedOrArbitraryNames(t *testing.T) {
	names, err := ancientcalc.HeroNames()
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[startupHeroNameKey(name)] = true
	}
	for _, raw := range []string{
		"SirGeorgellKingsGua",
		"TheMaskedSamura",
		"hBoawlenalBeeen",
		"HIRE",
		"12345!?",
		"AceScout",
	} {
		if key := startupHeroNameKey(raw); known[key] {
			t.Errorf("arbitrary or truncated OCR text %q produced known key %q", raw, key)
		}
	}
}

func TestStartupHeroIceOCRMatchesOnlyExpectedRosterEntry(t *testing.T) {
	names, err := ancientcalc.HeroNames()
	if err != nil {
		t.Fatal(err)
	}
	known := make(map[string]string, len(names))
	for _, name := range names {
		known[startupHeroNameKey(name)] = name
	}
	for raw, expected := range map[string]string{
		"NatalialeeApprentice":   "Natalia, Ice Apprentice",
		"ReferiJeratorleeWizard": "Referi Jerator, Ice Wizard",
	} {
		if got := known[startupHeroNameKey(raw)]; got != expected {
			t.Fatalf("%q matched %q, want %q", raw, got, expected)
		}
	}
	for _, raw := range []string{"lee", "Natalialee", "ReferiJeratorleeWizar", "HIRE"} {
		if got := known[startupHeroNameKey(raw)]; got != "" {
			t.Fatalf("incomplete caption %q matched %q", raw, got)
		}
	}
}
