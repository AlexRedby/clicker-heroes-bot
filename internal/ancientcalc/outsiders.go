package ancientcalc

import (
	"context"
	"errors"
	"math"
)

type OutsiderTarget struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Current int    `json:"current"`
	Target  int    `json:"target"`
	Cost    int    `json:"cost"`
}

type OutsiderAllocation struct {
	Status         string           `json:"status"`
	Budget         int              `json:"budget"`
	Spent          int              `json:"spent"`
	Remaining      int              `json:"remaining"`
	RequiresRespec bool             `json:"requiresRespec"`
	Additions      []OutsiderTarget `json:"additions"`
	Ideal          []OutsiderTarget `json:"ideal"`
}

var outsiderNames = [...]string{"Xyliqil", "Chor'gorloth", "Phandoryss", "Ponyboy", "Borb", "Rhageist", "K'Ariqua", "Orphalas", "Sen-Akhan"}
var outsiderIDs = [...]int{1, 2, 3, 5, 6, 7, 8, 9, 10}

func outsiderCost(index, level int) int {
	if index < 0 || index >= len(outsiderNames) || level < 0 {
		return -1
	}
	if index == 2 || level == 0 {
		return level
	}
	a, b := level, level+1
	if b <= 0 {
		return -1
	}
	if a%2 == 0 {
		a /= 2
	} else {
		b /= 2
	}
	if a > math.MaxInt/b {
		return -1
	}
	return a * b
}

func outsiderTargets(levels [9]int) []OutsiderTarget {
	out := make([]OutsiderTarget, len(levels))
	for i := range out {
		out[i] = OutsiderTarget{ID: outsiderIDs[i], Name: outsiderNames[i], Current: levels[i], Target: levels[i]}
	}
	return out
}

func outsiderSpendAS(ratio float64, souls int) int {
	if ratio*float64(souls) < 1 {
		return 0
	}
	return int(math.Floor(math.Sqrt(8*ratio*float64(souls)+1)/2 - .5))
}

// Active allocation formulas from Driej/Clicker-Heroes-Outsiders commit
// fbb5c603d16b914ee57ca9f0360e4dc089ea9507 (Unlicense), without reserve AS
// or Orphalas. Only the AS < 21000 branch is supported.
func outsiderGoal(total int) (tp, zone float64) {
	tp = transcendenceTP(total) / 100
	switch {
	case total == 0:
		zone = 300
	case total < 100:
		zone = (float64(total+42)/5 - 6) * 51.8 * math.Log(1.25) / math.Log(1+tp)
	case total < 10500:
		zone = (1-math.Exp(-float64(total)/3900))*200000 + 4800
	default:
		zone = float64(total)*10.32 + (8000+float64(10500-total)/10500*4000)*12
	}
	return tp, math.Floor(zone)
}

// Core allocation also accepts a fractional prediction budget, as upstream's
// FANT model does. Actual purchases always pass an integer budget.
func outsiderNOS(ctx context.Context, souls, tp, zone float64, levels [3]int) ([3]int, error) {
	hp := math.Min(1.545, 1.145+zone/500000)
	hs := math.Pow(1+tp, .2)
	damage, heroCost := 4.0, 1.07
	if zone > 1200000 {
		damage, heroCost = 1000, 1.22
	} else if zone > 168000 {
		damage = 4.5
	}
	dpsZones := math.Log10(hp) - math.Log10(1.15)*math.Log10(damage)/math.Log10(heroCost)/25
	chor, phan, pony := levels[0], levels[1], levels[2]
	for souls > 0 {
		if err := ctx.Err(); err != nil {
			return [3]int{}, err
		}
		if pony < 1 {
			pony++
			souls -= float64(pony)
			continue
		}
		if phan < 3 {
			phan++
			souls--
			continue
		}
		phanBuff := math.Pow(hs, math.Log10(float64(phan+2)/float64(phan+1))/dpsZones)
		if phan < 50 {
			phanBuff *= math.Pow(1.1, 1/float64(phan))
		}
		ponyBuff := math.Pow((10*math.Pow(float64(pony+1), 2)+1)/(10*math.Pow(float64(pony), 2)+1), 1/float64(pony+1))
		if float64(chor) < souls && chor < 150 {
			chorBuff := math.Pow(1/.95, 1/float64(chor+1))
			if chorBuff >= phanBuff {
				if float64(pony) < souls && ponyBuff >= chorBuff {
					pony++
					souls -= float64(pony)
				} else {
					chor++
					souls -= float64(chor)
				}
				continue
			}
		}
		if float64(pony) < souls && ponyBuff >= phanBuff {
			pony++
			souls -= float64(pony)
		} else {
			phan++
			souls--
		}
	}
	return [3]int{chor, phan, pony}, nil
}

func outsiderIdeal(ctx context.Context, total int) ([9]int, error) {
	tp, zone := outsiderGoal(total)
	levels := math.Floor((math.Log10(1+tp)*zone/5+6)/math.Log10(2)-3/math.Log10(2)) - 1
	kuma := -8 * (1 - math.Exp(-.025*levels))
	atman := 75 * (1 - math.Exp(-.013*levels))
	bubos := -5 * (1 - math.Exp(-.002*levels))
	dora := 9900 * (1 - math.Exp(-.002*levels))
	nerfs := math.Floor(zone / 500)
	mpz := math.Round((10+nerfs*.1)*100) / 100
	chest := 1 - .99999999*(1-math.Exp(-.006*nerfs))
	borbCap := math.Max(0, math.Ceil(((mpz-2.1)/-kuma-1)/.125))
	if total >= 10500 {
		borbCap = math.Ceil((zone - 500) / 5000)
	}
	rhCap := math.Ceil(((100-(25-nerfs*2))/atman - 1) / .25)
	kaCap := math.Ceil((((10+nerfs*.4)-5)/-bubos - 1) / .5)
	senCap := math.Max(1, math.Ceil(100/chest/(dora/100+1)-1))
	borbRatio := .99
	if total < 50 {
		borbRatio = .5
	}
	borb := int(math.Min(float64(outsiderSpendAS(borbRatio, total)), borbCap+1))
	if total <= 2000 {
		n, err := outsiderNOS(ctx, float64(total)*.5, tp, 100, [3]int{})
		if err != nil {
			return [9]int{}, err
		}
		s := math.Pow(1+tp, 2)
		logHS := math.Log10(20*(10*math.Pow(float64(n[2]), 2)+1)*(s+s*s+s*s*s)) + math.Log10(math.Pow(1/.95, float64(n[0])))
		kumaFant := math.Max(1, math.Floor(logHS/math.Log10(2)-3/math.Log(2))-1)
		required := math.Ceil((1/(1-math.Exp(-.025*kumaFant)) - 1) * 8)
		fant := int(math.Min(float64(outsiderSpendAS(.35, total)), required))
		borb = max(borb, fant)
	}
	if outsiderCost(4, borb) > total-5 {
		borb = outsiderSpendAS(1, total-5)
	}
	remaining := total - outsiderCost(4, borb)
	scale := 1.0
	if total < 100 {
		scale = float64(total) / 100
	}
	capLevel := func(cap, ratio float64) int {
		if float64(outsiderCost(0, int(cap))) > ratio*float64(remaining) {
			return outsiderSpendAS(ratio, remaining)
		}
		return int(cap)
	}
	rh, ka, sen := capLevel(rhCap, .2*scale), capLevel(kaCap, .01*scale), capLevel(senCap, .05*scale)
	remaining -= outsiderCost(5, rh) + outsiderCost(6, ka) + outsiderCost(8, sen)
	n, err := outsiderNOS(ctx, float64(remaining), tp, zone, [3]int{})
	if err != nil {
		return [9]int{}, err
	}
	return [9]int{0, n[0], n[1], n[2], borb, rh, ka, 0, sen}, nil
}

func allocateOutsiders(ctx context.Context, total, wallet int, current [9]int) (OutsiderAllocation, error) {
	if err := ctx.Err(); err != nil {
		return OutsiderAllocation{}, err
	}
	if total < 0 || wallet < 0 {
		return OutsiderAllocation{}, errors.New("invalid outsider budget")
	}
	invested := 0
	for i, level := range current {
		c := outsiderCost(i, level)
		if c < 0 || invested > total-c {
			return OutsiderAllocation{}, errors.New("outsider levels exceed soul ledger")
		}
		invested += c
	}
	if wallet > total-invested {
		return OutsiderAllocation{}, errors.New("outsider wallet exceeds soul ledger")
	}
	a := OutsiderAllocation{Status: "ok", Budget: wallet, Remaining: wallet}
	if total >= 21000 {
		a.Status = "unsupported"
		return a, nil
	}
	ideal, err := outsiderIdeal(ctx, total)
	if err != nil {
		return OutsiderAllocation{}, err
	}
	a.Ideal = outsiderTargets(ideal)
	idealSpent := 0
	for i := range ideal {
		c := outsiderCost(i, ideal[i])
		if c < 0 || idealSpent > total-c {
			return OutsiderAllocation{}, errors.New("ideal outsider allocation exceeds budget")
		}
		idealSpent += c
		a.Ideal[i].Current, a.Ideal[i].Cost = 0, c
		a.RequiresRespec = a.RequiresRespec || current[i] > ideal[i]
	}
	plan, remaining := current, wallet
	// Fill utility targets first, then apply the same core marginal-benefit
	// formula from the owned levels; existing investment is never refunded.
	for _, i := range [...]int{4, 5, 6, 8} {
		for plan[i] < ideal[i] && plan[i]+1 <= remaining {
			if err := ctx.Err(); err != nil {
				return OutsiderAllocation{}, err
			}
			plan[i]++
			remaining -= plan[i]
		}
	}
	tp, zone := outsiderGoal(total)
	n, err := outsiderNOS(ctx, float64(remaining), tp, zone, [3]int{plan[1], plan[2], plan[3]})
	if err != nil {
		return OutsiderAllocation{}, err
	}
	plan[1], plan[2], plan[3] = n[0], n[1], n[2]
	a.Additions = outsiderTargets(plan)
	for i := range plan {
		c := outsiderCost(i, plan[i]) - outsiderCost(i, current[i])
		if c < 0 || a.Spent > wallet-c {
			return OutsiderAllocation{}, errors.New("outsider additions exceed wallet")
		}
		a.Additions[i].Current, a.Additions[i].Cost = current[i], c
		a.Spent += c
	}
	a.Remaining = wallet - a.Spent
	return a, nil
}
