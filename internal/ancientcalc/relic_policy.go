package ancientcalc

import (
	"errors"
	"math"
	"math/big"
	"strconv"
)

// Evaluate each single replacement against the complete equipped set and the
// saved Ancient profile. Every returned move must improve the current outfit;
// hypothetical multi-item gains cannot authorize an insignificant first swap.
func activeRelicSuggestion(snapshot RelicSnapshot) (*RelicSuggestion, error) {
	if snapshot.AncientLevels == nil {
		return nil, nil
	}
	if len(snapshot.Items) > 10 {
		return nil, errors.New("relic policy inventory exceeds bounded search")
	}
	if err := validateRelicProfile(snapshot); err != nil {
		return nil, err
	}
	current := make([]Relic, 0, snapshot.EquipmentSlots)
	for _, item := range snapshot.Items {
		if item.Slot >= 1 && item.Slot <= snapshot.EquipmentSlots {
			current = append(current, item)
		}
	}
	for slot := 1; len(current) < snapshot.EquipmentSlots; slot++ {
		found := false
		for _, item := range current {
			if item.Slot == slot {
				found = true
				break
			}
		}
		if !found {
			current = append(current, Relic{Slot: slot})
		}
	}
	baseline := scoreActiveRelics(snapshot, current)
	best := baseline
	var move *RelicSuggestion
	for _, candidate := range snapshot.Items {
		if candidate.Slot >= 1 && candidate.Slot <= snapshot.EquipmentSlots {
			continue
		}
		for i, equipped := range current {
			set := append([]Relic(nil), current...)
			set[i] = candidate
			score := scoreActiveRelics(snapshot, set)
			// Practical tolerances are not transitive. Compare every option with
			// the real outfit as well as the best accepted option.
			if activeScoreGreater(score, baseline) && activeScoreGreater(score, best) {
				best = score
				move = &RelicSuggestion{UID: candidate.UID, Slot: equipped.Slot, ReplaceUID: equipped.UID}
			}
		}
	}
	return move, nil
}

// This is an Active-play priority tuple, not a prediction of run duration.
// Keeping separate effects avoids arbitrary conversions between gold and DPS.
type activeRelicScore [20]float64

func activeScoreGreater(a, b activeRelicScore) bool {
	// Minimum gains in effect units: average monsters, percentage points,
	// skill uptime, cooldown fraction and boss seconds. Damage/gold scores
	// are logarithms, so their minimum is a 0.1% multiplier improvement.
	minimum := [...]float64{.01, .01, .001, .001, .001, .0001, .01, .01, .01, .01, .01, .01}
	for i := range a {
		threshold := math.Log1p(.001)
		if i < len(minimum) {
			threshold = minimum[i]
		}
		if math.Abs(a[i]-b[i]) < threshold {
			continue
		}
		return a[i] > b[i]
	}
	return false
}

func scoreActiveRelics(snapshot RelicSnapshot, set []Relic) activeRelicScore {
	levels := map[int]float64{}
	for id, raw := range snapshot.AncientLevels {
		levels[id] = relicFloat(raw)
	}
	for _, item := range set {
		for _, b := range item.Bonuses {
			levels[b.AncientID] += relicFloat(b.Level)
		}
	}
	outsider := func(id int, rate float64) float64 { return 1 + rate*relicFloat(snapshot.OutsiderLevels[id]) }
	nerfs := math.Floor(float64(snapshot.HighestZone) / 500)
	reduction := capEffect(levels[20], 75, .026, 1) / 100
	uptime := func(id int, cooldown float64) float64 {
		return math.Min(1, (30+2*levels[id])/(cooldown*(1-reduction)))
	}
	var score activeRelicScore
	// Evaluate the useful total effect, rather than capping its delta: an
	// already capped Ancient must not gain value from an unnecessary relic.
	score[0] = math.Min(10+nerfs*.1-2, capEffect(levels[21], 8, .025, outsider(6, .125)))
	score[1] = math.Max(0, math.Min(100, 25-2*nerfs+capEffect(levels[13], 75, .013, outsider(7, .25))))
	// Base cooldowns: official Clicker Heroes skill table. Duration Ancients
	// add two seconds per level; Vaagur reduces each skill's own cooldown.
	// https://blog.clickerheroes.com/hero-skills-in-clicker-heroes-master-every-powerful-buff/
	score[2], score[3] = uptime(25, 1800), uptime(26, 3600) // Lucky Strikes, Golden Clicks
	score[4] = uptime(22, 600) + uptime(24, 600) + uptime(27, 1800) + uptime(23, 3600)
	score[5] = reduction                                         // Also helps Energize, Reload and Dark Ritual.
	score[6] = capEffect(levels[17], 30, .034, outsider(9, .75)) // Chronos
	chest := 1 - .99999999*(1-math.Exp(-.006*nerfs))
	score[7] = math.Min(100, chest*(1+capEffect(levels[14], 9900, .002, 1)/100)*outsider(10, 1))
	score[8] = capEffect(levels[11], 99.99999999, .01, 1)                                // Dogcog
	score[9] = capEffect(levels[12], 100, .0025, 1)                                      // Fortuna
	score[10] = capEffect(levels[31], 96, .01, 1)                                        // Revolc
	score[11] = math.Min(10+nerfs*.4-5, capEffect(levels[18], 5, .002, outsider(8, .5))) // Bubos
	for i, ancient := range []struct {
		id          int
		coefficient float64
	}{
		{28, .02}, {16, .11}, {29, .0001}, {19, .20}, {15, .15}, {10, .30}, {9, .50}, {8, .05},
	} {
		score[12+i] = math.Log1p(ancient.coefficient * levels[ancient.id])
	}
	return score
}

func validateActivePolicyBonuses(items []Relic) error {
	for _, item := range items {
		seen := map[int]bool{}
		for _, bonus := range item.Bonuses {
			if seen[bonus.Type] {
				return errors.New("duplicate relic bonus type")
			}
			seen[bonus.Type] = true
			if _, ok := relicNumber(bonus.Level); !ok || bonus.AncientID != relicAncientIDs[bonus.Type] {
				return errors.New("invalid relic bonus profile")
			}
			// Iris and Solomon need progression state not represented by this
			// snapshot. Refuse the policy rather than treating them as zero.
			if bonus.Type == 6 || bonus.Type == 25 {
				return errors.New("active relic policy does not model Iris or Solomon")
			}
			if relicAncientIDs[bonus.Type] == 0 {
				return errors.New("unsupported relic effect requires manual review")
			}
		}
	}
	return nil
}

func capEffect(level, cap, rate, multiplier float64) float64 {
	return cap * -math.Expm1(-rate*level) * multiplier
}

func validateRelicProfile(snapshot RelicSnapshot) error {
	if snapshot.EquipmentSlots != 4 || !validRelicLayout(snapshot.Items, 4) {
		return errors.New("invalid relic equipment layout")
	}
	if err := validateActivePolicyBonuses(snapshot.Items); err != nil {
		return err
	}
	for _, levels := range []map[int]string{snapshot.AncientLevels, snapshot.OutsiderLevels} {
		for id, raw := range levels {
			if id < 1 {
				return errors.New("invalid relic policy identity")
			}
			if _, err := Value(raw); err != nil {
				return errors.New("invalid relic policy level")
			}
		}
	}
	if snapshot.HighestZone < 0 {
		return errors.New("invalid relic policy zone")
	}
	return nil
}

func relicFloat(raw string) float64 {
	v, err := strconv.ParseFloat(raw, 64)
	if math.IsInf(v, 1) {
		return 1e300
	} // Levels beyond float range already saturate utility effects.
	if err != nil || math.IsNaN(v) {
		return 0
	}
	return math.Min(v, 1e300)
}

// RelicJunkIsDominated is a destruction certificate. Every un-equipped item
// must be component-wise covered by one equipped item across supported Active
// effects; ties are safe, tradeoffs are retained.
func RelicJunkIsDominated(snapshot RelicSnapshot) (bool, error) {
	// Without a validated save profile, comparison cannot authorize destruction.
	if snapshot.AncientLevels == nil {
		return false, nil
	}
	if len(snapshot.Items) > 10 {
		return false, errors.New("relic policy inventory exceeds bounded search")
	}
	if err := validateRelicProfile(snapshot); err != nil {
		return false, err
	}
	current := make([]Relic, 0, snapshot.EquipmentSlots)
	for _, item := range snapshot.Items {
		if item.Slot >= 1 && item.Slot <= snapshot.EquipmentSlots {
			current = append(current, item)
		}
	}
	if len(current) != snapshot.EquipmentSlots {
		return false, nil
	}
	junkCount := 0
	for _, junk := range snapshot.Items {
		if junk.Slot >= 1 && junk.Slot <= snapshot.EquipmentSlots {
			continue
		}
		junkCount++
		covered := false
		for _, equipped := range current {
			if activeRelicCovers(equipped, junk) {
				covered = true
				break
			}
		}
		if !covered {
			return false, nil
		}
	}
	return junkCount > 0, nil
}

func activeRelicBonuses(item Relic) map[int]*big.Rat {
	out := map[int]*big.Rat{}
	for _, bonus := range item.Bonuses {
		if bonus.Type == 1 || bonus.Type == 24 {
			continue
		} // Idle effects do not help Active play.
		out[bonus.Type], _ = relicNumber(bonus.Level)
	}
	return out
}

func activeRelicCovers(equipped, junk Relic) bool {
	e, j := activeRelicBonuses(equipped), activeRelicBonuses(junk)
	for kind, value := range j {
		if current := e[kind]; current == nil || current.Cmp(value) < 0 {
			return false
		}
	}
	return true
}

func validRelicLayout(items []Relic, equipmentSlots int) bool {
	uids, slots := map[int]bool{}, map[int]bool{}
	for _, item := range items {
		if item.UID <= 0 || uids[item.UID] || item.Slot < 0 {
			return false
		}
		uids[item.UID] = true
		if item.Slot >= 1 && item.Slot <= equipmentSlots {
			if slots[item.Slot] {
				return false
			}
			slots[item.Slot] = true
		}
	}
	return true
}
