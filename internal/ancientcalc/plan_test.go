package ancientcalc

import "testing"

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
		{"123456789012345678", "1.23456789012345e17"},
	} {
		got, err := InputQuantity(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("InputQuantity(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}
