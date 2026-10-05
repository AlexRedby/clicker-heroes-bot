package timelapse

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestPinnedDesktopReference(t *testing.T) {
	raw, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		LogHeroSouls float64 `json:"logHeroSouls"`
		Xyliqil      int     `json:"xyliqilLevel"`
		Chor         int     `json:"chorLevel"`
		AC           int     `json:"autoClickers"`
		Rubies       int     `json:"rubyCost"`
		Rows         []struct {
			Duration string `json:"duration"`
			Hero     string `json:"bestHero"`
			Level    int    `json:"heroLevel"`
			Zone     int    `json:"zone"`
		} `json:"timelapses"`
	}
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(strconv.FormatFloat(c.LogHeroSouls, 'f', 0, 64)+"/"+strconv.Itoa(c.AC)+"/"+strconv.Itoa(c.Xyliqil), func(t *testing.T) {
			got, err := Calculate(context.Background(), Input{LogHeroSouls: c.LogHeroSouls, Xyliqil: c.Xyliqil, Chorgorloth: c.Chor, AutoClickers: c.AC})
			if err != nil {
				t.Fatal(err)
			}
			if got.Rubies != c.Rubies || len(got.Rows) != len(c.Rows) {
				t.Fatalf("cost/rows: %d/%d want %d/%d", got.Rubies, len(got.Rows), c.Rubies, len(c.Rows))
			}
			for i, want := range c.Rows {
				row := got.Rows[i]
				hours := 0
				if strings.HasSuffix(want.Duration, "h") {
					hours, _ = strconv.Atoi(strings.TrimSuffix(want.Duration, "h"))
				}
				if row.Hours != hours || row.Hero != want.Hero || row.Level != want.Level || row.Zone != want.Zone {
					t.Errorf("row%d got %+v want %+v", i, row, want)
				}
				if hours == 0 {
					parts := strings.Split(strings.TrimSuffix(want.Duration, " (wait)"), ":")
					if len(parts) == 3 {
						h, _ := strconv.Atoi(parts[0])
						m, _ := strconv.Atoi(parts[1])
						s, _ := strconv.Atoi(parts[2])
						if row.WaitSeconds != h*3600+m*60+s {
							t.Errorf("wait%d=%d want %s", i, row.WaitSeconds, want.Duration)
						}
					}
				}
			}
		})
	}
}

func TestForecastValidation(t *testing.T) {
	for _, in := range []Input{{LogHeroSouls: math.NaN()}, {LogHeroSouls: math.Inf(1)}, {LogHeroSouls: -1}, {LogHeroSouls: 200001}, {AutoClickers: 2000000001}, {Chorgorloth: 151}, {Xyliqil: -1}, {MinZones: -1}} {
		if _, err := Calculate(context.Background(), in); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Calculate(ctx, Input{LogHeroSouls: 58}); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestEconomicsAndRowInvariants(t *testing.T) {
	// The user's October 2 export was 2.339e58 capital. This is illustrative,
	// not permission from a stale save. 50k is not used as an unlock gate.
	poor, err := Calculate(context.Background(), Input{LogHeroSouls: 58.369, Xyliqil: 0, Chorgorloth: 6, AutoClickers: 3})
	if err != nil {
		t.Fatal(err)
	}
	if poor.Rubies != 0 {
		t.Fatalf("poor-value return unexpectedly buys %+v", poor)
	}
	for _, hs := range []float64{400, 1000, 6400, 25600} {
		plan, err := Calculate(context.Background(), Input{LogHeroSouls: hs, AutoClickers: 4})
		if err != nil {
			t.Fatal(err)
		}
		cost, last := 0, 40
		for _, row := range plan.Rows {
			if row.StartZone != last || row.Zone < last || row.Level < 1 {
				t.Fatalf("invalid row %+v", row)
			}
			if row.Hours > 0 {
				if row.Zone-row.StartZone > row.Hours*4500 || row.Rubies <= 0 {
					t.Fatal(row)
				}
			} else if row.Rubies != 0 {
				t.Fatal(row)
			}
			cost += row.Rubies
			last = row.Zone
		}
		if cost != plan.Rubies {
			t.Fatal(plan)
		}
	}
}

func TestCurrentZoneForecastKeepsActualStartingPoint(t *testing.T) {
	for _, start := range []int{300, 5000, 20000} {
		f, err := Calculate(nil, Input{LogHeroSouls: 800, AutoClickers: 4, StartZone: start})
		if err != nil {
			t.Fatal(err)
		}
		last := start
		for _, row := range f.Rows {
			if row.StartZone != last || row.Zone < last {
				t.Fatalf("bad current-zone row %+v", row)
			}
			last = row.Zone
		}
	}
}

func TestAlreadyBeyondIdealCapDoesNotForecastGoingBackward(t *testing.T) {
	f, err := Calculate(nil, Input{LogHeroSouls: 50, StartZone: 20000, AutoClickers: 3})
	if err != nil {
		t.Fatal(err)
	}
	if f.Rubies != 0 || len(f.Rows) != 1 || f.Rows[0].Zone < 20000 || f.Rows[0].WaitSeconds != 0 {
		t.Fatal(f)
	}
}
