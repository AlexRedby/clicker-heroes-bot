package ancientcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

type prestigeStats struct {
	CurrentAscension    *ascensionRecord `json:"currentAscension"`
	CurrentTranscension *struct {
		ID         json.Number                `json:"id"`
		Ascensions map[string]ascensionRecord `json:"ascensions"`
	} `json:"currentTranscension"`
}

type ascensionRecord struct {
	ID           json.Number `json:"id"`
	Transcension json.Number `json:"transcensionId"`
	HighestZone  json.Number `json:"highestZoneEver"`
	StartTime    json.Number `json:"startTime"`
	EndTime      json.Number `json:"endTime"`
	HeroSoulsEnd json.Number `json:"heroSoulsEnd"`
}

type AscensionSummary struct {
	ID                  int `json:"id"`
	HighestZone         int `json:"highestZone"`
	EstimatedASTotal    int `json:"estimatedASTotal"`
	EstimatedASIncrease int `json:"estimatedASIncrease"`
}

type PrestigeHistory struct {
	Available           bool               `json:"available"`
	CompletedAscensions int                `json:"completedAscensions"`
	ASGrowingAscensions int                `json:"asGrowingAscensions"`
	LastCompleted       []AscensionSummary `json:"lastCompleted"`
}

// TranscensionPreview is informational: it never authorizes input or spending.
// Gains use the e12 sacrifice model and remain estimates until matched to UI.
type TranscensionPreview struct {
	SaveHash              string              `json:"saveHash"`
	Build                 string              `json:"build"`
	SaveVersion           int                 `json:"saveVersion"`
	Transcendent          bool                `json:"transcendent"`
	Transcensions         int                 `json:"transcensions"`
	Ascensions            int                 `json:"ascensionsThisTranscension"`
	HighestZone           int                 `json:"highestZone"`
	AncientSouls          int                 `json:"ancientSouls"`
	AncientSoulsTotal     int                 `json:"ancientSoulsTotal"`
	EstimatedASWithoutRun *int                `json:"estimatedASWithoutCurrentRun"`
	EstimatedASGain       *int                `json:"estimatedASGain"`
	ProjectedASWallet     *int                `json:"projectedASWallet"`
	TPBeforePercent       *float64            `json:"estimatedTPBeforePercent"`
	TPAfterPercent        *float64            `json:"estimatedTPAfterPercent"`
	UIVerified            bool                `json:"uiVerified"`
	Recommendation        string              `json:"recommendation"`
	Reasons               []string            `json:"reasons"`
	History               PrestigeHistory     `json:"history"`
	Outsiders             *OutsiderAllocation `json:"outsiders,omitempty"`
	RestoreAfterReset     []string            `json:"restoreAfterReset"`
}

func exactPrestigeNumber(raw json.Number, label string) (*big.Rat, error) {
	if _, err := Value(string(raw)); err != nil {
		return nil, fmt.Errorf("missing or invalid %s", label)
	}
	n, ok := new(big.Rat).SetString(string(raw))
	if !ok {
		return nil, fmt.Errorf("invalid %s", label)
	}
	return n, nil
}

func prestigeInteger(raw json.Number, label string) (int, error) {
	n, err := exactPrestigeNumber(raw, label)
	if err != nil {
		return 0, err
	}
	if !n.IsInt() || !n.Num().IsInt64() {
		return 0, fmt.Errorf("invalid integer %s", label)
	}
	x := n.Num().Int64()
	if x > 9007199254740991 || int64(int(x)) != x {
		return 0, fmt.Errorf("%s exceeds supported integer range", label)
	}
	return int(x), nil
}

// floor(5*log10(H)) == floor(log10(H^5)). Counting digits and comparing
// integers avoids a rounded floating-point logarithm crossing an AS threshold.
func soulsToAS(souls *big.Rat) int {
	if souls.Cmp(big.NewRat(1, 1)) < 0 {
		return 0
	}
	five := big.NewInt(5)
	n := new(big.Int).Exp(souls.Num(), five, nil)
	d := new(big.Int).Exp(souls.Denom(), five, nil)
	exponent := len(n.String()) - len(d.String())
	scaled := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	if n.Cmp(scaled.Mul(scaled, d)) < 0 {
		exponent--
	}
	return max(0, exponent)
}

func transcendenceTP(total int) float64 {
	return 25 - 23*math.Exp(-0.0003*float64(total))
}

// PreviewTranscension shares save decoding, but does not run Ancient allocation:
// an empty post-Transcension Ancient roster is valid here.
func PreviewTranscension(ctx context.Context, exported []byte) (TranscensionPreview, error) {
	var p TranscensionPreview
	if ctx == nil {
		ctx = context.Background()
	}
	save, err := decodeAncientSave(ctx, exported)
	if err != nil {
		return p, err
	}
	if save.Transcendent == nil {
		return p, errors.New("save is missing transcendence state")
	}
	p.Transcendent, p.Build = *save.Transcendent, save.Build
	for _, item := range []struct {
		raw   json.Number
		label string
		out   *int
	}{
		{save.Version, "save version", &p.SaveVersion},
		{save.NumberOfTranscensions, "Transcension count", &p.Transcensions},
		{save.AscensionsThisTranscension, "current Transcension Ascension count", &p.Ascensions},
		{save.AncientSouls, "Ancient Souls wallet", &p.AncientSouls},
		{save.AncientSoulsTotal, "Ancient Souls total", &p.AncientSoulsTotal},
		{save.HighestFinishedZonePersist, "highest zone", &p.HighestZone},
	} {
		*item.out, err = prestigeInteger(item.raw, item.label)
		if err != nil {
			return p, err
		}
	}
	if p.AncientSouls > p.AncientSoulsTotal {
		return p, errors.New("Ancient Souls wallet exceeds total")
	}
	sum := sha256.Sum256(exported)
	p.SaveHash = hex.EncodeToString(sum[:])
	p.Recommendation = "insufficient_evidence"
	p.Reasons = []string{
		"Reset reward, TP and respec behavior have not been verified against the installed game UI.",
		"An exported file does not prove current game state, a full-combat wall or active-play throughput.",
	}
	p.RestoreAfterReset = []string{
		"Hire and level heroes; unlock upgrades, skills and Amenhotep's Ascension.",
		"Reassign existing Auto Clickers and enable progression.",
		"Earn the first Hero Souls without Quick Ascension; summon active Ancients before allocating levels.",
		"Reacquire earned gilds and relics; restore gild targets and relic equipment. Paid gilds return gradually.",
	}
	if p.SaveVersion != 7 || !strings.HasPrefix(p.Build, "1.0e12-") {
		p.Reasons = append(p.Reasons, "Unsupported save/mechanics version; gain and allocation are unavailable.")
		return p, nil
	}
	tpBefore := 0.0
	if p.Transcendent {
		tpBefore = transcendenceTP(p.AncientSoulsTotal)
	}
	p.TPBeforePercent = &tpBefore
	if len(save.Ancients.Ancients) > 100 || len(save.Outsiders.Outsiders) > 10 {
		return p, errors.New("unsupported prestige roster size")
	}
	var definitions struct {
		Ancients []ancientDefinition `json:"ancients"`
	}
	if err := json.Unmarshal(ancientDataJSON, &definitions); err != nil {
		return p, err
	}
	knownAncients := make(map[int]bool, len(definitions.Ancients))
	for _, def := range definitions.Ancients {
		knownAncients[def.ID] = true
	}
	sacrificed, err := exactPrestigeNumber(save.HeroSoulsSacrificed, "sacrificed Hero Souls")
	if err != nil {
		return p, err
	}
	capital, err := exactPrestigeNumber(save.HeroSouls, "Hero Souls wallet")
	if err != nil {
		return p, err
	}
	for id, entry := range save.Ancients.Ancients {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		key, e := strconv.Atoi(id)
		if e != nil || key <= 0 || strconv.Itoa(key) != id {
			return p, errors.New("invalid Ancient identity")
		}
		level, e := exactPrestigeNumber(entry.Level, "Ancient level")
		if e != nil || !level.IsInt() {
			return p, errors.New("invalid Ancient level")
		}
		spent := new(big.Rat)
		if entry.SpentHeroSouls != "" || level.Sign() != 0 {
			spent, e = exactPrestigeNumber(entry.SpentHeroSouls, "Ancient invested Hero Souls")
			if e != nil {
				return p, e
			}
		}
		if level.Sign() == 0 {
			if spent.Sign() != 0 {
				return p, errors.New("unowned Ancient has invested Hero Souls")
			}
			continue
		}
		if !knownAncients[key] {
			return p, errors.New("unsupported owned Ancient")
		}
		capital.Add(capital, spent)
	}
	primal, err := exactPrestigeNumber(save.PrimalSouls, "pending primal Hero Souls")
	if err != nil {
		return p, err
	}
	heroLevels, err := prestigeInteger(save.TotalHeroLevels, "total hero levels")
	if err != nil {
		return p, err
	}
	primal.Add(primal, new(big.Rat).SetInt64(int64(heroLevels/2000)))
	base := new(big.Rat).Add(sacrificed, capital)
	without := max(p.AncientSoulsTotal, soulsToAS(base))
	projectedTotal := max(p.AncientSoulsTotal, soulsToAS(new(big.Rat).Add(base, primal)))
	gain := projectedTotal - p.AncientSoulsTotal
	wallet := p.AncientSouls + gain
	tp := transcendenceTP(projectedTotal)
	p.EstimatedASWithoutRun, p.EstimatedASGain, p.ProjectedASWallet, p.TPAfterPercent = &without, &gain, &wallet, &tp
	if p.History, err = prestigeHistory(ctx, save, sacrificed, p.Transcensions, p.Ascensions, p.AncientSoulsTotal); err != nil {
		return p, err
	}
	if !p.History.Available {
		p.Reasons = append(p.Reasons, "Closed Ascension history is missing or incomplete.")
	}
	if gain == 0 || p.HighestZone < 300 {
		p.Recommendation = "continue_ascending"
		p.Reasons = append(p.Reasons, "No estimated new Ancient Souls or the zone-300 unlock has not been reached.")
	} else if !p.Transcendent {
		p.Recommendation = "candidate"
		p.Reasons = append(p.Reasons, "First Transcension candidate: zone-300 unlock and a positive estimated reward.")
	} else if p.History.Available && p.History.ASGrowingAscensions < 3 {
		p.Recommendation = "continue_ascending"
		p.Reasons = append(p.Reasons, "Fewer than three completed AS-growing Ascensions; this is a conservative heuristic, not a game rule.")
	} else {
		p.Reasons = append(p.Reasons, "Timing needs a confirmed wall and declining marginal AS per active-play hour; save timestamps include pauses and offline time.")
	}
	levels, accountingKnown, err := prestigeOutsiders(save, p.AncientSoulsTotal, p.AncientSouls)
	if err != nil {
		return p, err
	}
	if accountingKnown {
		allocation, err := allocateOutsiders(ctx, projectedTotal, wallet, levels)
		if err != nil {
			return p, err
		}
		p.Outsiders = &allocation
	} else {
		p.Reasons = append(p.Reasons, "Outsider AS accounting is incomplete or differs from AS total; allocation is unavailable.")
	}
	if err := ctx.Err(); err != nil {
		return p, err
	}
	return p, nil
}

func prestigeOutsiders(save ancientSave, total, wallet int) ([9]int, bool, error) {
	var levels [9]int
	ledger, known := big.NewInt(int64(wallet)), true
	for id, entry := range save.Outsiders.Outsiders {
		key, err := strconv.Atoi(id)
		if err != nil || key <= 0 || strconv.Itoa(key) != id {
			return levels, false, errors.New("invalid Outsider identity")
		}
		level, err := prestigeInteger(entry.Level, "Outsider level")
		if err != nil {
			return levels, false, err
		}
		index := -1
		for i, activeID := range outsiderIDs {
			if key == activeID {
				index = i
				break
			}
		}
		if index < 0 && (key != 4 || level != 0) {
			return levels, false, errors.New("unsupported owned Outsider")
		}
		if index >= 0 {
			levels[index] = level
		}
		if entry.SpentAncientSouls == "" {
			known = false
			continue
		}
		spent, err := prestigeInteger(entry.SpentAncientSouls, "Outsider invested Ancient Souls")
		if err != nil {
			return levels, false, err
		}
		expected := big.NewInt(int64(level))
		if index != 2 {
			expected.Mul(expected, big.NewInt(int64(level)+1))
			expected.Quo(expected, big.NewInt(2))
		}
		if expected.Cmp(big.NewInt(int64(spent))) != 0 {
			known = false
		}
		ledger.Add(ledger, big.NewInt(int64(spent)))
	}
	if ledger.Cmp(big.NewInt(int64(total))) > 0 {
		return levels, false, errors.New("Outsider spending exceeds Ancient Souls total")
	}
	return levels, known && ledger.Cmp(big.NewInt(int64(total))) == 0, nil
}

func prestigeHistory(ctx context.Context, save ancientSave, sacrificed *big.Rat, transcensions, ascensions, total int) (PrestigeHistory, error) {
	h := PrestigeHistory{LastCompleted: []AscensionSummary{}}
	current, cycle := save.Stats.CurrentAscension, save.Stats.CurrentTranscension
	if current == nil || cycle == nil || cycle.Ascensions == nil {
		return h, nil
	}
	if len(cycle.Ascensions) > 10000 {
		return h, errors.New("Ascension history is too large")
	}
	currentID, err := prestigeInteger(current.ID, "current Ascension ID")
	if err != nil {
		return h, err
	}
	currentCycle, err := prestigeInteger(current.Transcension, "current Ascension Transcension ID")
	if err != nil {
		return h, err
	}
	cycleID, err := prestigeInteger(cycle.ID, "history Transcension ID")
	if err != nil {
		return h, err
	}
	if currentID != ascensions || currentCycle != transcensions || cycleID != transcensions {
		return h, errors.New("Ascension history does not match prestige counters")
	}
	var ids []int
	for key, record := range cycle.Ascensions {
		id, err := prestigeInteger(record.ID, "Ascension history ID")
		if err != nil || strconv.Itoa(id) != key {
			return h, errors.New("invalid Ascension history identity")
		}
		cycleID, err := prestigeInteger(record.Transcension, "Ascension history Transcension ID")
		if err != nil || cycleID != transcensions || id > currentID {
			return h, errors.New("invalid Ascension history cycle")
		}
		// The live record may already have endTime; exclude it by ID.
		if id < currentID {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	best := total
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return h, err
		}
		r := cycle.Ascensions[strconv.Itoa(id)]
		zone, err := prestigeInteger(r.HighestZone, "historical highest zone")
		if err != nil {
			return h, err
		}
		souls, err := exactPrestigeNumber(r.HeroSoulsEnd, "historical Hero Souls")
		if err != nil {
			return h, err
		}
		after := max(best, soulsToAS(new(big.Rat).Add(sacrificed, souls)))
		if after > best {
			h.ASGrowingAscensions++
		}
		h.LastCompleted = append(h.LastCompleted, AscensionSummary{id, zone, after, after - best})
		best = after
	}
	h.Available, h.CompletedAscensions = len(ids) == currentID, len(ids)
	for i, id := range ids {
		if i != id {
			h.Available = false
		}
	}
	if len(h.LastCompleted) > 3 {
		h.LastCompleted = h.LastCompleted[len(h.LastCompleted)-3:]
	}
	return h, nil
}
