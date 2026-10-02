package ancientcalc

import (
	"math/big"
	"testing"
)

func TestPlanValidate(t *testing.T) {
	p := Plan{
		Souls: "1e1000", Reserve: "1e990", Spent: "9e999", Remaining: "1e999", Ascensions: 1,
		Owned: []Level{{ID: 1, Name: "Argaiv", Level: "1e500"}},
		Rows:  []Purchase{{ID: 1, Name: "Argaiv", Current: "1e500", Target: "2e500", Quantity: "1e500", Cost: "9e999"}},
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Rows[0].ID = 2
	if p.Validate() == nil {
		t.Fatal("unowned Ancient accepted")
	}
	p.Rows[0].ID = 1
	p.Remaining = "0"
	if p.Validate() == nil {
		t.Fatal("reserve consumed")
	}
	if _, err := Value("NaN"); err == nil {
		t.Fatal("invalid value accepted")
	}
}

func TestInputQuantity(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"1", "1"},
		{"123456", "123456"},
		{"123456789012345678", "1.23456e17"},
		{"100000000", "1e8"},
		{"123450000", "1.2345e8"},
		{"4.55790796093577e32", "4.5579e32"},
	} {
		got, err := InputQuantity(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("InputQuantity(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
		input, _ := new(big.Rat).SetString(tc.in)
		formatted, _ := new(big.Rat).SetString(got)
		if formatted.Cmp(input) > 0 {
			t.Fatalf("InputQuantity(%q) increased quantity: %s > %s", tc.in, got, tc.in)
		}
	}
}

func TestInputQuantityTruncatesFractionBeforeFormatting(t *testing.T) {
	got, err := InputQuantity("123456.9")
	if err != nil || got != "123456" {
		t.Fatalf("InputQuantity fractional value = %q, %v; want 123456", got, err)
	}
}
