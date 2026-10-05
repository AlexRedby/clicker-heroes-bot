// Package timelapse forecasts prepared Hybrid progression. It never authorizes input.
package timelapse

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
)

// ReferenceRevision pins Driej/Clicker-Heroes-Timelapses desktop js/main.js.
// The source is public domain (Unlicense). Its ideal allocation, maximal
// Dogcog discount, estimated gild count and 2-MPZ speed are assumptions, not
// evidence that the installed client or the current preparation matches them.
const ReferenceRevision = "9fa560705139957b0c2021b99f51b3b12754fd47"

//go:embed heroes.json
var heroData []byte

var tables = func() struct {
	Costs     map[string]float64 `json:"costs"`
	BaseCosts map[string]float64 `json:"baseCosts"`
	DPS       map[string]float64 `json:"dps"`
	Order     []string           `json:"order"`
} {
	var t struct {
		Costs     map[string]float64 `json:"costs"`
		BaseCosts map[string]float64 `json:"baseCosts"`
		DPS       map[string]float64 `json:"dps"`
		Order     []string           `json:"order"`
	}
	if err := json.Unmarshal(heroData, &t); err != nil {
		panic(err)
	}
	return t
}()

type Input struct {
	StartZone int // Zero uses the reference startup zone 40; fresh exports may supply the actual zone.
	// Ascension-start capital, not pending primal souls or the current wallet.
	LogHeroSouls                       float64
	Xyliqil, Chorgorloth, AutoClickers int
	MinZones                           int // Zero uses the reference default 20000.
}

type Row struct {
	Hours       int    `json:"hours"` // Zero means ordinary progression, never a shop purchase.
	Hero        string `json:"hero"`
	Level       int    `json:"level"`
	StartZone   int    `json:"startZone"`
	Zone        int    `json:"zone"`
	Rubies      int    `json:"rubies"`
	WaitSeconds int    `json:"waitSeconds,omitempty"`
}

type Forecast struct {
	Reference string `json:"reference"`
	Rows      []Row  `json:"rows"`
	Rubies    int    `json:"rubies"`
	// No mercenary earnings are subtracted from costs; earnings are not permission.
	Assumptions []string `json:"assumptions"`
}

type model struct {
	input               Input
	hs, cps, xyl, gilds float64
	active              bool
	tlZone, highest     int
}

func validFloat(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Calculate ports the default desktop forecast including ordinary wait rows.
// Advanced Ascension-time/MPZ/QA heuristics are deliberately outside this model.
// Installed-game prices and actual preparation must be checked separately.
func Calculate(ctx context.Context, in Input) (Forecast, error) {
	out := Forecast{Reference: ReferenceRevision, Assumptions: []string{
		"ideal Hybrid Ancient allocation", "maximal Dogcog discount", "estimated gild count",
		"two monsters per ordinary zone", "prepared hero upgrades and gilds", "Nogardnit free-clicker effect",
		"desktop reference prices require installed-client verification",
	}}
	if ctx == nil {
		ctx = context.Background()
	}
	if !validFloat(in.LogHeroSouls) || in.LogHeroSouls < 0 || in.LogHeroSouls > 200000 || in.Xyliqil < 0 || in.Xyliqil > 1000000 || in.Chorgorloth < 0 || in.Chorgorloth > 150 || in.AutoClickers < 0 || in.AutoClickers > 2000000000 || in.MinZones < 0 || in.MinZones > 756000 || in.StartZone < 0 || in.StartZone > 2147000000 {
		return out, fmt.Errorf("unsupported Timelapse input or scaling")
	}
	if in.MinZones == 0 {
		in.MinZones = 20000
	}
	ac := float64(in.AutoClickers)
	cps := math.Min(math.Max(1+math.Log10(ac), 1+(ac-1)*math.Log10(1.5)), math.Log10(1.7976931348623157)+308)
	xyl := math.Log10(1.5) * float64(in.Xyliqil)
	previous := (in.LogHeroSouls - 5) / math.Log10(1.25) * 5
	m := model{input: in, hs: in.LogHeroSouls + math.Log10(1/0.95)*float64(in.Chorgorloth) - 2, cps: cps, xyl: xyl, gilds: math.Max(1, math.Floor(previous/10)-9)}
	m.active = in.LogHeroSouls*0.5+cps-0.3701813447471219227682305382593-math.Log10(1.5)*3.3895*float64(in.Xyliqil)-math.Log10(1.5)*ac > 0
	initial := in.StartZone
	if initial == 0 {
		initial = 40
	}
	start := initial
	revived := false
	gold := 0.0
	for iteration := 0; iteration < 4096; iteration++ {
		if err := ctx.Err(); err != nil {
			return Forecast{}, err
		}
		if !revived {
			extra := math.Log10(1.6 / 0.6)
			if start > 140 {
				extra = math.Log10(1.15 / 0.15)
			}
			gold = m.monsterGold(start) + math.Max(extra+xyl, cps+1)
		}
		hero, kind := bestHero(gold)
		level := heroLevel(gold, hero, kind)
		m.highest = bossAligned(m.idleZone(heroDPS(hero, level, kind, m.gilds)))
		gained := m.highest - start
		hours, cost, capacity := offer(gained, in.MinZones)
		if hours > 0 {
			gained = min(gained, capacity)
			m.highest = start + gained
			out.Rows = append(out.Rows, Row{Hours: hours, Hero: displayHero(hero), Level: level, StartZone: start, Zone: m.highest, Rubies: cost})
			out.Rubies += cost
			start = m.highest
			revived = false
		} else {
			if revived {
				out.Rows = out.Rows[:len(out.Rows)-1]
				start = initial
				if len(out.Rows) > 0 {
					start = out.Rows[len(out.Rows)-1].Zone
				}
				break
			}
			next, nextOK := nextHero(hero)
			if !nextOK {
				break
			}
			required := m.zoneRequired(next)
			gold = tables.Costs[next]
			cap, _, _, err := m.softCap(ctx, start)
			if err != nil {
				return Forecast{}, err
			}
			if required >= cap || required <= start {
				break
			}
			out.Rows = append(out.Rows, Row{Hero: displayHero(hero), Level: level, StartZone: start, Zone: required, WaitSeconds: int(math.Ceil(float64(required-start) / 8050 * 3600))})
			start = required
			revived = true
		}
		if !revived && gained < in.MinZones {
			break
		}
		if iteration == 4095 {
			return Forecast{}, fmt.Errorf("forecast step bound exceeded")
		}
	}
	m.tlZone = start
	cap, hero, level, err := m.softCap(ctx, start)
	if err != nil {
		return Forecast{}, err
	}
	out.Rows = append(out.Rows, Row{Hero: displayHero(hero), Level: level, StartZone: start, Zone: max(start, m.highest), WaitSeconds: int(math.Ceil(float64(max(0, cap-start)) / 8050 * 3600))})
	return out, nil
}

func displayHero(hero string) string {
	if hero == "Wepwawet2" {
		return "Wepwawet"
	}
	return hero
}
func offer(gain, minimum int) (hours, cost, capacity int) {
	if gain <= minimum {
		return
	}
	switch {
	case gain >= 360000 && minimum <= 756000:
		return 168, 500, 756000
	case gain >= 162000 && minimum <= 216000:
		return 48, 300, 216000
	case gain >= 72000 && minimum <= 108000:
		return 24, 200, 108000
	case minimum <= 36000:
		return 8, 100, 36000
	default:
		return
	}
}
func bestHero(gold float64) (string, string) {
	hero, kind := "Samurai", "old"
	for _, name := range tables.Order {
		if tables.Costs[name] <= gold {
			hero = name
			if name == "Xavira0" {
				kind = "e10"
			}
			if name == "Rose0" {
				kind = "e11"
			}
		}
	}
	return hero, kind
}
func nextHero(hero string) (string, bool) {
	if hero == "Dorothy5" {
		return "", false
	}
	for _, name := range tables.Order {
		if tables.Costs[name] > tables.Costs[hero] && name != "Dorothy1" && name != "Dorothy2" && name != "dorothy4" {
			return name, true
		}
	}
	return "", false
}
func heroLevel(gold float64, hero, kind string) int {
	base, ok := tables.BaseCosts[hero[:len(hero)-1]]
	if !ok {
		base = tables.BaseCosts[hero]
	}
	scale := 1.07
	if kind == "e11" {
		scale = 1.22
	}
	return int(math.Max(1, math.Floor((gold-base)/math.Log10(scale))))
}
func heroDPS(hero string, level int, kind string, gilds float64) float64 {
	mult := 4.0
	if kind == "e10" {
		mult = 4.5
	}
	if kind == "e11" {
		mult = 1000
	}
	bonus := math.Log10(10/mult) * math.Min(math.Floor(float64(level)/1000), 8)
	if kind == "e11" {
		bonus = 0
	}
	return tables.DPS[hero] + math.Log10(float64(level)) + math.Log10(mult)*math.Floor(math.Max(float64(level)-175, 0)/25) + bonus + math.Log10(gilds)
}
func highestZone(dps float64) int {
	hp500 := 1 + math.Log10(1.55)*139 + math.Log10(1.145)*360
	hp200k := math.Log10(1.24) + 25409
	if dps < 1+math.Log10(1.145)+math.Log10(1.55)*139+1 {
		return 130
	}
	if dps <= hp500 {
		return 500
	}
	zone := 199999.0
	if dps < hp200k {
		health := hp500
		for z := 501; z < 200000; z++ {
			health += math.Log10(1.145 + 0.001*math.Floor(float64(z-1)/500))
			if health >= dps {
				zone = float64(z)
				break
			}
		}
	} else {
		zone = math.Min((dps-hp200k)/math.Log10(1.545)+200000, 2.147e9)
	}
	boss := math.Floor(zone/500)*0.4 + 5
	return int(math.Floor(zone - math.Log(boss)/math.Log(1.545)))
}
func bossAligned(zone int) int { return zone - zone%5 + 4 }
func (m *model) idleZone(dps float64) int {
	return highestZone(dps + m.hs*2.4 + math.Log10(1.5)*2*float64(m.input.Xyliqil) - 1.4457374595558059145523937994475 + math.Log10(1.5)*float64(m.input.AutoClickers))
}
func (m *model) monsterGold(zone int) float64 {
	ancient := m.hs*1.5 - 1.1105440342413657683046916147778 - math.Log10(15)
	if zone < 140 {
		return math.Log10(1.6)*float64(zone-1) + ancient
	}
	return math.Log10(1.15)*float64(zone-140) + math.Log10(1.6)*139 + ancient
}
func (m *model) zoneRequired(hero string) int {
	bonus := m.xyl
	if m.active {
		bonus = m.cps
	}
	return int(math.Floor((tables.Costs[hero] - math.Log10(1.15/0.15) - bonus - (m.hs*1.5 - 1.1105440342413657683046916147778) - math.Log10(1.6/1.15)*139) / math.Log10(1.15)))
}
func (m *model) softCap(ctx context.Context, start int) (int, string, int, error) {
	m.tlZone = start
	for i := 0; i < 4096; i++ {
		if err := ctx.Err(); err != nil {
			return 0, "", 0, err
		}
		bonus := m.xyl
		if m.active {
			bonus = m.cps
		}
		gold := m.monsterGold(start) + math.Log10(1.15/0.15) + bonus
		hero, kind := bestHero(gold)
		level := heroLevel(gold, hero, kind)
		dps := heroDPS(hero, level, kind, m.gilds)
		combo := math.Min(math.Log10(math.Max(float64(start-m.tlZone)/2.25, 1))+m.cps, 307)
		if m.active {
			m.highest = highestZone(dps + combo + m.cps + m.hs*2.9 - 1.8159188043029278373206243377068)
		} else {
			m.highest = m.idleZone(dps)
		}
		m.highest = bossAligned(m.highest)
		if m.highest-start <= 10 {
			return start, hero, level, nil
		}
		start = m.highest
	}
	return 0, "", 0, fmt.Errorf("soft-cap iteration bound exceeded")
}
