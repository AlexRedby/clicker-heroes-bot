package ancientcalc

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/ALTree/bigfloat"
)

// Active allocation ported from tomcur/ClickerHeroesCalculator at
// 1c35f5598f210c689a618a7e46a14769f7bf9a39. See THIRD_PARTY_LICENSES.md.
//
//go:embed assets/ancient-data.json
var ancientDataJSON []byte

const ancientPrecision = 384 // More than the reference's 100 decimal digits.

type ancientDefinition struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Formula string `json:"formula"`
}

func aNew() *big.Float { return new(big.Float).SetPrec(ancientPrecision) }
func aConst(s string) *big.Float {
	n, ok := aNew().SetString(s)
	if !ok {
		panic("invalid calculator constant")
	}
	return n
}
func aAdd(x, y *big.Float) *big.Float { return aNew().Add(x, y) }
func aSub(x, y *big.Float) *big.Float { return aNew().Sub(x, y) }
func aMul(x, y *big.Float) *big.Float { return aNew().Mul(x, y) }
func aDiv(x, y *big.Float) *big.Float { return aNew().Quo(x, y) }
func aSquare(x *big.Float) *big.Float { return aMul(x, x) }
func aMax(x, y *big.Float) *big.Float {
	if x.Cmp(y) >= 0 {
		return x
	}
	return y
}
func aMin(x, y *big.Float) *big.Float {
	if x.Cmp(y) <= 0 {
		return x
	}
	return y
}
func aRound(x *big.Float, up bool) *big.Float {
	n, _ := x.Int(nil)
	out := aNew().SetInt(n)
	if up && out.Cmp(x) < 0 {
		out.Add(out, aConst("1"))
	}
	if !up && out.Cmp(x) > 0 {
		out.Sub(out, aConst("1"))
	}
	return out
}
func aString(x *big.Float) string {
	magnitude := aNew().Abs(x)
	if magnitude.Cmp(aConst("1e21")) >= 0 || (x.Sign() != 0 && magnitude.Cmp(aConst("1e-6")) < 0) {
		mantissa, exponent, _ := strings.Cut(x.Text('e', 99), "e")
		mantissa = strings.TrimRight(strings.TrimRight(mantissa, "0"), ".")
		value, _ := strconv.Atoi(exponent)
		return mantissa + fmt.Sprintf("e%+d", value)
	}
	return x.Text('g', 100)
}

func aInput(raw, label string, integer bool) (*big.Float, error) {
	n, err := Value(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", label)
	}
	if integer {
		exact, ok := new(big.Rat).SetString(raw)
		if !ok || !exact.IsInt() {
			return nil, fmt.Errorf("invalid %s", label)
		}
	}
	n, _, err = big.ParseFloat(raw, 10, ancientPrecision, big.ToNearestEven)
	if err != nil || n.IsInf() {
		return nil, fmt.Errorf("invalid %s", label)
	}
	return n, nil
}

// Beyond this point the correction is smaller than the working precision;
// avoid exponentiating huge negative levels outside big.Float's exponent range.
func aDecay(level *big.Float, rate string) *big.Float {
	z := aMul(level, aConst(rate))
	if z.Cmp(aConst("1000")) > 0 {
		return aConst("0")
	}
	return bigfloat.Exp(aNew().Neg(z))
}

func ancientGoal(name string, base, old, alpha, rate *big.Float, beyond8k bool) *big.Float {
	one := aConst("1")
	logBase := func() *big.Float { return bigfloat.Log(base) }
	correction := func(coefficient, offset, decayRate, logRate, constant string, subtract bool) *big.Float {
		decay := aDecay(old, decayRate)
		offsetValue := aConst(offset)
		if subtract {
			offsetValue = aSub(offsetValue, decay)
		} else {
			offsetValue = aAdd(offsetValue, decay)
		}
		return aSub(aSub(aMul(logBase(), aConst(coefficient)), aMul(bigfloat.Log(offsetValue), aConst(logRate))), aConst(constant))
	}
	switch name {
	case "Fragsworth", "Bhaal", "Argaiv":
		return base
	case "Morgulis", "soulbank":
		return aSquare(base)
	case "Mammon", "Mimzee", "Pluto":
		factor := "0.926"
		if beyond8k {
			factor = "0.905"
		}
		return aMul(base, aConst(factor))
	case "Juggernaut":
		return bigfloat.Pow(base, aConst("0.8"))
	case "Solomon":
		goal := bigfloat.Pow(base, aConst("0.8"))
		if alpha.Sign() > 0 {
			return aDiv(goal, bigfloat.Pow(alpha, aConst("0.4")))
		}
		return aMin(base, aMul(aMul(bigfloat.Pow(bigfloat.Log(aMul(aSquare(base), aConst("3.25"))), aConst("0.4")), goal), aConst("1.15")))
	case "Dora":
		return aMin(correction("2.877", aString(aDiv(aConst("100"), aConst("99"))), "0.002", "1.4365", "9.63", true), aConst("18715"))
	case "Dogcog":
		return aMin(correction("2.844", aString(aDiv(one, aConst("99"))), "0.01", "1.422", "7.232", false), aConst("3743"))
	case "Fortuna":
		return aMin(correction("2.875", aString(aDiv(aConst("10"), aConst("9"))), "0.0025", "1.4375", "9.3", true), aConst("14972"))
	case "Bubos":
		return aMin(correction("2.8", "1", "0.02", "1.4", "5.94", false), aConst("18715"))
	case "Chronos":
		return aMin(correction("2.75", "2", "0.034", "1.375", "5.1", true), aConst("1101"))
	case "Atman", "Kumawakamaru":
		goal := aDiv(logBase(), bigfloat.Log(aConst("2")))
		if alpha.Sign() > 0 {
			if name == "Atman" {
				goal = aSub(correction("2.832", aString(aDiv(aConst("4"), aConst("3"))), "0.013", "1.416", "6.613", true), aMul(bigfloat.Log(alpha), aConst("1.416")))
			} else {
				goal = aSub(correction("2.844", "0.25", "0.01", "1.422", "7.014", false), aMul(bigfloat.Log(alpha), aConst("1.422")))
			}
		}
		cap := aConst("2880")
		if name == "Kumawakamaru" {
			cap = aConst("1498")
		}
		return aMin(goal, cap)
	case "Berserker", "Chawedo", "Energon", "Hecatoncheir", "Kleptos", "Sniperino", "Vaagur":
		if rate.Sign() == 0 {
			return nil
		}
		scaled := aMul(base, aSquare(rate))
		if scaled.Sign() == 0 {
			return old
		}
		// The ordinary skill Ancients share Chronos' uncapped allocation formula.
		goal := aSub(aSub(aMul(bigfloat.Log(scaled), aConst("2.75")), aMul(bigfloat.Log(aSub(aConst("2"), aDecay(old, "0.034"))), aConst("1.375"))), aConst("5.1"))
		if name == "Vaagur" {
			goal = aMin(goal, aConst("1440"))
		}
		return goal
	}
	return nil // Idle-only, obsolete and deliberately excluded Revolc have no Active goal.
}

// Integer powers use stdlib multiplication so exact powers of two stay exact.
func aPowInteger(base *big.Float, exponent *big.Float) *big.Float {
	n, _ := exponent.Int(nil)
	result := aConst("1")
	factor := aNew().Set(base)
	for n.Sign() > 0 {
		if n.Bit(0) == 1 {
			result.Mul(result, factor)
		}
		n.Rsh(n, 1)
		if n.Sign() > 0 {
			factor.Mul(factor, factor)
		}
	}
	return result
}

func ancientCostSum(formula string, level *big.Float) *big.Float {
	switch formula {
	case "one":
		return level
	case "linear":
		return aMul(aMul(level, aAdd(level, aConst("1"))), aConst("0.5"))
	case "exponential":
		return aSub(aPowInteger(aConst("2"), aAdd(level, aConst("1"))), aConst("1"))
	case "polynomial1_5":
		root := aNew().Sqrt(level)
		power := aMul(level, root)
		sum := aAdd(aAdd(aMul(aMul(level, power), aConst("0.4")), aMul(power, aConst("0.5"))), aMul(root, aConst("0.125")))
		return aRound(aAdd(sum, aDiv(aConst("1"), aMul(aConst("1920"), power))), true)
	}
	panic("unsupported allocation cost formula")
}

// Bound the official 6144 client's price, including its binary64
// operations and the addends it drops beyond a ten-exponent gap. Allocation
// stays at 384 bits; these prices deliberately include a rounding allowance.
func ancientPrice(formula string, old, target, multiplier *big.Float) (*big.Float, error) {
	// These small integer operations and their decimal alignment stay exact.
	if multiplier.Cmp(aConst("1")) == 0 && ((formula == "one" && target.Cmp(aConst("10")) <= 0) || (formula == "linear" && target.Cmp(aConst("4")) <= 0)) {
		return aSub(ancientCostSum(formula, target), ancientCostSum(formula, old)), nil
	}
	if formula == "exponential" && target.Cmp(aConst("4000000")) > 0 {
		return nil, errors.New("Ancient exponential cost exceeds supported exponent range")
	}
	interval := func(level *big.Float) (*big.Float, *big.Float) {
		if formula != "polynomial1_5" {
			sum := ancientCostSum(formula, level)
			error := aMul(sum, aConst("2e-13"))
			if formula == "linear" && level.Cmp(aConst("1e9")) >= 0 {
				error = aAdd(error, aMul(level, aConst("0.5")))
			}
			if formula == "exponential" {
				// The client uses 2^(x+1)-2; its power's weighted roundoff
				// grows with x, and subtraction can discard the two.
				sum = aSub(sum, aConst("1"))
				error = aMul(aMul(aAdd(sum, aConst("2")), aAdd(level, aConst("1"))), aConst("2e-13"))
				if level.Cmp(aConst("32")) >= 0 {
					error = aAdd(error, aConst("2"))
				}
			}
			return aSub(sum, error), aAdd(sum, error)
		}
		y := aAdd(level, aConst("1"))
		root := aNew().Sqrt(y)
		power := aMul(y, root)
		sum := aSub(aAdd(aSub(aMul(aMul(y, power), aConst("0.4")), aMul(power, aConst("0.5"))), aMul(root, aConst("0.125"))), aDiv(aConst("0.00052"), power))
		// More than 1,000 binary64 unit roundoffs cover input conversion,
		// the formula's operations, normalization and integer ceiling.
		error := aAdd(aMul(sum, aConst("2e-13")), aConst("0.001"))
		if level.Cmp(aConst("10000")) >= 0 {
			error = aAdd(error, aMul(root, aConst("0.125")))
		}
		if level.Cmp(aConst("1e9")) >= 0 {
			// Dropping x+1's one changes the sum by at most y^(3/2);
			// dropping the subtracted term changes it by 0.5*y^(3/2).
			error = aAdd(error, aMul(power, aConst("1.5")))
		}
		lower := aSub(sum, error)
		// The client's Ceiling is a no-op once the exponent reaches 15.
		lower = aRound(lower, aAdd(sum, error).Cmp(aConst("1e15")) < 0)
		return lower, aRound(aAdd(sum, error), true)
	}
	lowOld, highOld := interval(old)
	lowTarget, highTarget := interval(target)
	if lowTarget.Cmp(highOld) <= 0 {
		return nil, errors.New("Ancient purchase is below supported client precision")
	}
	// Cover normalized endpoint representations and range subtraction too.
	rangeCost := aSub(highTarget, lowOld)
	if highTarget.Cmp(aMul(lowOld, aConst("1e10"))) >= 0 {
		// The subtraction can discard the entire old endpoint.
		rangeCost = highTarget
	}
	rangeCost = aAdd(rangeCost, aMul(aAdd(highTarget, highOld), aConst("2e-13")))
	// Percent5 computes 1 - (100 * (1 - 0.95^level)) * 0.01. Its
	// absolute binary64 error is bounded even when the power is tiny.
	discount := multiplier
	if multiplier.Cmp(aConst("1")) < 0 {
		discount = aMin(aConst("1"), aAdd(multiplier, aConst("1e-13")))
	}
	cost := aRound(aMul(rangeCost, discount), true)
	return aRound(aMul(cost, aConst("1.0000000000002")), true), nil
}

// Calculate decodes an exported save and plans purchases for an Active build.
func Calculate(ctx context.Context, exported []byte, reserve string, skillRate float64, beyond8k bool) (Plan, error) {
	save, err := decodeAncientSave(ctx, exported)
	if err != nil {
		return Plan{}, err
	}
	return planAncients(ctx, save, reserve, skillRate, beyond8k)
}

func planAncients(ctx context.Context, save ancientSave, reserve string, skillRate float64, beyond8k bool) (Plan, error) {
	return planAncientsBuild(ctx, save, reserve, skillRate, beyond8k, ActiveBuild, nil)
}

func planAncientsBuild(ctx context.Context, save ancientSave, reserve string, skillRate float64, beyond8k bool, mode BuildMode, ratio *big.Float) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	wallet, err := aInput(string(save.HeroSouls), "Hero Souls", false)
	if err != nil {
		return Plan{}, err
	}
	// AddSouls rounds the balance after subtraction. Allow its half-unit
	// rounding, normalization and save-export error, using the initial wallet.
	loss := aMax(aConst("1"), aMul(wallet, aConst("2e-13")))
	if wallet.Cmp(aConst("10")) < 0 {
		if _, accuracy := wallet.Int(nil); accuracy == big.Exact {
			loss = aConst("0") // These integer balances and charges stay exact.
		}
	}
	price := func(formula string, old, target, multiplier *big.Float) (*big.Float, error) {
		cost, err := ancientPrice(formula, old, target, multiplier)
		if err != nil {
			return nil, err
		}
		return aRound(aAdd(cost, loss), true), nil
	}
	p, err := planAncientsBuildWithPrice(ctx, save, reserve, skillRate, beyond8k, mode, ratio, price)
	if err != nil {
		return p, err
	}
	return addProfitableMorgulis(ctx, save, p, price)
}

// Retain the Active RoT allocation, then compare an affordable Morgulis batch
// with leaving its souls in the wallet. Official client 6144 combines them as
// 1 + 0.10*wallet + 0.11*Morgulis; gold, farm and skill effects stay unchanged.
func addProfitableMorgulis(ctx context.Context, save ancientSave, p Plan, price func(string, *big.Float, *big.Float, *big.Float) (*big.Float, error)) (Plan, error) {
	entry, owned := save.Ancients.Ancients["16"]
	if !owned || aConst(string(entry.Level)).Sign() == 0 {
		return p, nil
	}
	old := aConst(string(entry.Level))
	rowIndex := -1
	previousQuantity, previousCost := aConst("0"), aConst("0")
	for i, row := range p.Rows {
		if row.ID == 16 {
			rowIndex = i
			previousQuantity, previousCost = aConst(row.Quantity), aConst(row.Cost)
			break
		}
	}
	wallet, reserved := aConst(p.Souls), aConst(p.Reserve)
	otherCost := aSub(aConst(p.Spent), previousCost)
	budget := aSub(aSub(wallet, reserved), otherCost)
	if budget.Sign() <= 0 {
		return p, nil
	}
	chor := aConst("0")
	if outsider, ok := save.Outsiders.Outsiders["2"]; ok {
		chor = aConst(string(outsider.Level))
	}
	multiplier := bigfloat.Pow(aConst("0.95"), chor)
	left, right := aConst("0"), aRound(aDiv(budget, multiplier), false)
	currentExact, _ := new(big.Rat).SetString(string(entry.Level))
	if old.Cmp(aConst("1e9")) >= 0 {
		minimum := new(big.Rat).Quo(currentExact, big.NewRat(1000, 1))
		integer := new(big.Int).Quo(new(big.Int).Add(minimum.Num(), new(big.Int).Sub(minimum.Denom(), big.NewInt(1))), minimum.Denom())
		left = aNew().SetInt(integer)
		if right.Cmp(left) < 0 {
			return p, nil // Even the ideal discounted quantity cannot pass the input gate.
		}
		cost, err := price("one", old, aAdd(old, left), multiplier)
		if err != nil {
			return p, err
		}
		if cost.Cmp(budget) > 0 {
			return p, nil
		}
	}
	// The discounted linear cost bounds affordable levels from above; retain
	// enough search precision for the field's six significant digits.
	initial := aMax(right, aConst("1"))
	for iteration := 0; right.Cmp(left) > 0 && aDiv(aSub(right, left), initial).Cmp(aConst("1e-10")) > 0; iteration++ {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if iteration >= 1024 {
			return p, errors.New("Morgulis search did not converge")
		}
		mid := aRound(aDiv(aAdd(aAdd(left, right), aConst("1")), aConst("2")), false)
		cost, err := price("one", old, aAdd(old, mid), multiplier)
		if err != nil {
			return p, err
		}
		if cost.Cmp(budget) <= 0 {
			left = mid
		} else {
			right = aSub(mid, aConst("1"))
		}
	}
	if left.Cmp(previousQuantity) <= 0 {
		return p, nil
	}
	integer, _ := left.Int(nil)
	quantity, err := InputQuantity(integer.String())
	if err != nil {
		return p, err
	}
	entered := aConst(quantity)
	if entered.Cmp(previousQuantity) <= 0 {
		return p, nil
	}
	enteredExact, _ := new(big.Rat).SetString(quantity)
	if old.Cmp(aConst("1e9")) >= 0 && new(big.Rat).Mul(enteredExact, big.NewRat(1000, 1)).Cmp(currentExact) < 0 {
		return p, nil
	}
	target := aAdd(old, entered)
	cost, err := price("one", old, target, multiplier)
	if err != nil {
		return p, err
	}
	baselineCost := aConst("0")
	if previousQuantity.Sign() > 0 {
		baselineCost, err = price("one", old, aAdd(old, previousQuantity), multiplier)
		if err != nil {
			return p, err
		}
	}
	additionalCost, err := price("one", aAdd(old, previousQuantity), target, multiplier)
	if err != nil {
		return p, err
	}
	// Check the merged purchase and its additional benefit. Using exact
	// decimal arithmetic also avoids cancellation at enormous levels.
	newCost, _ := new(big.Rat).SetString(aString(cost))
	oldQuantity, _ := new(big.Rat).SetString(aString(previousQuantity))
	oldCost, _ := new(big.Rat).SetString(aString(baselineCost))
	extraCost, _ := new(big.Rat).SetString(aString(additionalCost))
	benefit := func(q, c *big.Rat) bool {
		return new(big.Rat).Mul(q, big.NewRat(11, 1)).Cmp(new(big.Rat).Mul(c, big.NewRat(10, 1))) > 0
	}
	extraQuantity := new(big.Rat).Sub(enteredExact, oldQuantity)
	if cost.Cmp(budget) > 0 || !benefit(enteredExact, newCost) || !benefit(extraQuantity, new(big.Rat).Sub(newCost, oldCost)) || !benefit(extraQuantity, extraCost) {
		return p, nil
	}
	row := Purchase{ID: 16, Name: "Morgulis", Current: aString(old), Target: aString(target), Quantity: quantity, Cost: aString(cost)}
	if rowIndex >= 0 {
		p.Rows[rowIndex] = row
	} else {
		p.Rows = append(p.Rows, row)
	}
	spent := aAdd(otherCost, cost)
	p.Spent, p.Remaining = aString(spent), aString(aSub(wallet, spent))
	return p, p.Validate()
}

// Keep allocation separate from pricing so frozen legacy allocations and actual
// client budgets can be checked with independent price policies.
func planAncientsWithPrice(ctx context.Context, save ancientSave, reserve string, skillRate float64, beyond8k bool, price func(string, *big.Float, *big.Float, *big.Float) (*big.Float, error)) (Plan, error) {
	return planAncientsBuildWithPrice(ctx, save, reserve, skillRate, beyond8k, ActiveBuild, nil, price)
}

func planAncientsBuildWithPrice(ctx context.Context, save ancientSave, reserve string, skillRate float64, beyond8k bool, mode BuildMode, ratio *big.Float, price func(string, *big.Float, *big.Float, *big.Float) (*big.Float, error)) (Plan, error) {
	p := Plan{Rows: []Purchase{}, Owned: []Level{}}
	if err := ctx.Err(); err != nil {
		return p, err
	}
	var definitions struct {
		Ancients  []ancientDefinition `json:"ancients"`
		Outsiders []int               `json:"outsiders"`
	}
	if err := json.Unmarshal(ancientDataJSON, &definitions); err != nil {
		return p, err
	}
	if !(skillRate >= 0 && skillRate <= 1) {
		return p, errors.New("-ancient-skill-rate must be between 0 and 1")
	}
	souls, err := aInput(string(save.HeroSouls), "Hero Souls", false)
	if err != nil {
		return p, err
	}
	_, err = aInput(string(save.HeroSoulsSacrificed), "sacrificed Hero Souls", false)
	if err != nil {
		return p, err
	}
	zone, err := aInput(string(save.HighestFinishedZonePersist), "highest zone", true)
	if err != nil {
		return p, err
	}
	ancientSouls, err := aInput(string(save.AncientSoulsTotal), "Ancient Souls", false)
	if err != nil {
		return p, err
	}
	resets, err := aInput(string(save.NumWorldResets), "Ascension count", true)
	if err != nil {
		return p, err
	}
	count, acc := resets.Int64()
	if acc != big.Exact || count > 9007199254740991 || int64(int(count)) != count {
		return p, errors.New("Ascension count exceeds supported integer range")
	}
	p.Ascensions = int(count)
	reserve = strings.TrimSpace(reserve)
	percent := strings.HasSuffix(reserve, "%")
	reserved, err := aInput(strings.TrimSpace(strings.TrimSuffix(reserve, "%")), "soul reserve", false)
	if err != nil {
		return p, err
	}
	if percent {
		if reserved.Cmp(aConst("100")) > 0 {
			return p, errors.New("reserve percentage must be from 0 to 100")
		}
		if mode == HybridBuild && reserved.Cmp(aConst("100")) == 0 {
			// Multiplying and dividing an enormous balance can round a full
			// reserve above the wallet. A 100% reserve is the wallet itself.
			reserved = aNew().Set(souls)
		} else {
			reserved = aDiv(aMul(reserved, souls), aConst("100"))
		}
	}
	if reserved.Cmp(souls) > 0 {
		return p, errors.New("reserve exceeds available Hero Souls")
	}
	available := aSub(souls, reserved)
	byID := map[int]ancientDefinition{}
	for _, def := range definitions.Ancients {
		byID[def.ID] = def
	}
	levels := map[int]*big.Float{}
	invested := aConst("0")
	investedKnown := true
	for id, entry := range save.Ancients.Ancients {
		key, e := strconv.Atoi(id)
		if e != nil || key <= 0 || strconv.Itoa(key) != id {
			return p, errors.New("malformed Ancient identity")
		}
		level, e := aInput(string(entry.Level), "Ancient level", true)
		if e != nil {
			return p, e
		}
		if _, ok := byID[key]; !ok && level.Sign() > 0 {
			return p, errors.New("unknown owned Ancient")
		}
		levels[key] = level
		if level.Sign() > 0 {
			if entry.SpentHeroSouls == "" {
				investedKnown = false
			} else {
				spent, e := aInput(string(entry.SpentHeroSouls), "invested Hero Souls", false)
				if e != nil {
					return p, e
				}
				invested = aAdd(invested, spent)
			}
		}
	}
	knownOutsiders := map[int]bool{}
	for _, id := range definitions.Outsiders {
		knownOutsiders[id] = true
	}
	discountLevel := aConst("0")
	for id, entry := range save.Outsiders.Outsiders {
		key, e := strconv.Atoi(id)
		if e != nil || key <= 0 || strconv.Itoa(key) != id {
			return p, errors.New("malformed Outsider identity")
		}
		level, e := aInput(string(entry.Level), "Outsider level", true)
		if e != nil {
			return p, e
		}
		if !knownOutsiders[key] && level.Sign() > 0 {
			return p, errors.New("unknown owned Outsider")
		}
		if key == 2 {
			discountLevel = level
		}
	}
	baseID, baseName := 19, "Fragsworth"
	if mode == HybridBuild {
		baseID, baseName = 5, "Siyalatas"
	}
	if levels[baseID] == nil || levels[baseID].Sign() == 0 {
		return p, fmt.Errorf("%s must be owned", baseName)
	}
	// Bound the discount's exponent before calling the arbitrary-precision library.
	chor, _ := discountLevel.Float64()
	if math.IsInf(chor, 0) || chor > 4000000 {
		return p, errors.New("Ancient cost discount exceeds supported exponent range")
	}
	multiplier := bigfloat.Pow(aConst("0.95"), discountLevel)
	tp := aConst("0")
	if save.Transcendent != nil && *save.Transcendent {
		tp = aMax(aMul(aAdd(aMul(aDecay(ancientSouls, "0.0003"), aConst("-0.23")), aConst("0.25")), aConst("100")), aConst("1"))
	}
	hp := aAdd(aMul(aRound(aDiv(zone, aConst("500")), false), aConst("0.005")), aConst("1.145"))
	alphaScale := "1.4067"
	if beyond8k {
		alphaScale = "1.1085"
	}
	alpha := aDiv(aMul(bigfloat.Log(aAdd(aDiv(tp, aConst("100")), aConst("1"))), aConst(alphaScale)), bigfloat.Log(hp))
	rate := aConst(strconv.FormatFloat(skillRate, 'g', -1, 64))
	defs := append([]ancientDefinition{}, definitions.Ancients...)
	if levels[16] == nil || levels[16].Sign() == 0 {
		defs = append(defs, ancientDefinition{ID: -1, Name: "soulbank", Formula: "one"})
		levels[-1] = aConst("0")
	}
	targets, costs := map[int]*big.Float{}, map[int]*big.Float{}
	compute := func(addLevels *big.Float) (*big.Float, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := aAdd(levels[baseID], addLevels)
		total := aConst("0")
		targets, costs = map[int]*big.Float{}, map[int]*big.Float{}
		if base.Sign() <= 0 {
			return total, nil
		}
		for _, def := range defs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			old := levels[def.ID]
			if old == nil || (old.Sign() == 0 && def.ID != -1) {
				continue
			}
			var goal *big.Float
			if mode == HybridBuild {
				goal = hybridAncientGoal(def.Name, base, old, alpha, rate, ratio, beyond8k)
			} else {
				goal = ancientGoal(def.Name, base, old, alpha, rate, beyond8k)
			}
			if goal == nil {
				continue
			}
			target := aMax(old, aRound(goal, true))
			if def.ID != -1 && def.Formula != "exponential" && target.Cmp(aConst("1e9")) >= 0 && aSub(target, old).Cmp(aMax(aConst("4"), aMul(old, aConst("1e-10")))) < 0 {
				// Skip unresolved client increments; interval pricing
				// can support them once its endpoint errors are correlated.
				target = old
			}
			targets[def.ID] = target
			cost := aConst("0")
			if target.Cmp(old) > 0 {
				if def.ID == -1 {
					cost = aSub(target, old)
				} else {
					var e error
					cost, e = price(def.Formula, old, target, multiplier)
					if e != nil {
						return nil, e
					}
				}
			}
			costs[def.ID] = cost
			total = aAdd(total, cost)
		}
		return total, nil
	}
	left := aNew().Neg(levels[baseID])
	right := aConst("0")
	if available.Sign() > 0 {
		right = aRound(aNew().Sqrt(aAdd(aMul(aDiv(available, multiplier), aConst("2")), aSquare(levels[baseID]))), true)
	}
	initialDiff := aSub(right, left)
	var spent *big.Float
	lowFit, highFit := bigfloat.Exp(aConst("-0.1")), bigfloat.Exp(aConst("0.1"))
	for iteration := 0; aSub(right, left).Cmp(aConst("1")) > 0 && aDiv(aSub(right, left), initialDiff).Cmp(aConst("1e-10")) > 0; iteration++ {
		if iteration >= 1024 {
			return p, errors.New("Ancient search did not converge")
		}
		mid := aRound(aDiv(aAdd(right, left), aConst("2")), false)
		if spent != nil && available.Sign() > 0 {
			ratio := aDiv(spent, available)
			interval := aSub(right, left)
			if ratio.Cmp(lowFit) < 0 {
				mid = aRound(aAdd(left, aDiv(interval, aConst("1.25"))), false)
			} else if ratio.Cmp(highFit) > 0 {
				mid = aRound(aAdd(left, aDiv(interval, aConst("4"))), false)
			}
		}
		spent, err = compute(mid)
		if err != nil {
			return p, err
		}
		if spent.Cmp(available) <= 0 {
			left = mid
		} else {
			right = mid
		}
	}
	spent, err = compute(left)
	if err != nil {
		return p, err
	}
	if bank := targets[-1]; bank != nil {
		spent = aSub(spent, bank)
	}
	if spent.Sign() < 0 || spent.Cmp(available) > 0 {
		return p, errors.New("calculated spend exceeds available Hero Souls")
	}
	p.Souls, p.Reserve = aString(souls), aString(reserved)
	if investedKnown {
		p.Invested = aString(invested)
	}
	spent = aConst("0")
	for _, def := range definitions.Ancients {
		old := levels[def.ID]
		if old == nil || old.Sign() == 0 {
			continue
		}
		p.Owned = append(p.Owned, Level{def.ID, def.Name, aString(old)})
		target, cost := targets[def.ID], costs[def.ID]
		if target == nil || cost.Sign() == 0 || target.Cmp(old) <= 0 {
			continue
		}
		quantity := aSub(target, old)
		// Reuse the native input formatter on an integer, avoiding float text
		// rounding before truncating the quantity to the field's six digits.
		integer, _ := quantity.Int(nil)
		text, err := InputQuantity(integer.String())
		if err != nil {
			return p, err
		}
		// Filter only the finished input plan; preserve allocation and its price ceilings.
		if def.Formula != "exponential" && old.Cmp(aConst("1e9")) >= 0 {
			// Both decimal integers were validated by aInput/InputQuantity. Exact
			// comparison keeps the 0.1% boundary despite binary rounding at huge levels.
			current, _ := new(big.Rat).SetString(string(save.Ancients.Ancients[strconv.Itoa(def.ID)].Level))
			entered, _ := new(big.Rat).SetString(text)
			if new(big.Rat).Mul(entered, big.NewRat(1000, 1)).Cmp(current) < 0 {
				continue
			}
		}

		p.Rows = append(p.Rows, Purchase{def.ID, def.Name, aString(old), aString(target), text, aString(cost)})
		spent = aAdd(spent, cost)
	}
	p.Spent, p.Remaining = aString(spent), aString(aSub(souls, spent))
	return p, p.Validate()
}
