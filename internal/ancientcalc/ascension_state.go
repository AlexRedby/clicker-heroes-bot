package ancientcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
	"strings"
)

// AscensionState contains export facts independently of Ancient allocation.
// Missing optional identity/zone fields remain unknown. SoulCapital is known
// only when the complete Ancient investment ledger is available.
type AscensionState struct {
	SaveHash                   string `json:"saveHash"`
	ProfileID                  string `json:"profileId,omitempty"`
	Build                      string `json:"build"`
	SaveVersion                int    `json:"saveVersion"`
	Transcendent               bool   `json:"transcendent"`
	Transcensions              int    `json:"transcensions"`
	Ascensions                 int    `json:"ascensions"`
	AscensionsThisTranscension int    `json:"ascensionsThisTranscension"`
	HighestZone                int    `json:"highestZone"`
	CurrentZone                *int   `json:"currentZone,omitempty"`
	CurrentTranscensionID      *int   `json:"currentTranscensionId,omitempty"`
	CurrentAscensionID         *int   `json:"currentAscensionId,omitempty"`
	CurrentAscensionCycleID    *int   `json:"currentAscensionCycleId,omitempty"`
	PendingHeroSouls           string `json:"pendingHeroSouls"`
	HeroSouls                  string `json:"heroSouls"`
	SoulCapital                string `json:"soulCapital,omitempty"`
	CapitalKnown               bool   `json:"capitalKnown"`
}

// ascensionReward shares the established e12 reward model with Transcension.
// Small differences from the client's float32/BigNumber rounding are accepted.
func ascensionReward(save ancientSave) (*big.Rat, error) {
	pending, err := exactPrestigeNumber(save.PrimalSouls, "pending primal Hero Souls")
	if err != nil {
		return nil, err
	}
	levels, err := prestigeInteger(save.TotalHeroLevels, "total hero levels")
	if err != nil {
		return nil, err
	}
	return pending.Add(pending, new(big.Rat).SetInt64(int64(levels/2000))), nil
}

// Compact quantities keep large exponents bounded for the existing log policy.
func ascensionQuantity(value *big.Rat) string {
	return new(big.Float).SetPrec(256).SetRat(value).Text('g', -1)
}

func ReadAscensionState(ctx context.Context, exported []byte) (AscensionState, error) {
	var out AscensionState
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return out, err
	}
	var save struct {
		ancientSave
		UniqueID    string      `json:"uniqueId"`
		CurrentZone json.Number `json:"currentZoneHeight"`
	}
	if err := json.Unmarshal(payload, &save); err != nil {
		return out, errors.New("invalid Ascension save JSON")
	}
	if save.Transcendent == nil {
		return out, errors.New("save is missing transcendence state")
	}
	out.Build, out.Transcendent = save.Build, *save.Transcendent
	for _, field := range []struct {
		raw   json.Number
		name  string
		value *int
	}{
		{save.Version, "save version", &out.SaveVersion},
		{save.NumberOfTranscensions, "Transcension count", &out.Transcensions},
		{save.NumWorldResets, "Ascension count", &out.Ascensions},
		{save.AscensionsThisTranscension, "current Transcension Ascension count", &out.AscensionsThisTranscension},
		{save.HighestFinishedZonePersist, "highest zone", &out.HighestZone},
	} {
		*field.value, err = prestigeInteger(field.raw, field.name)
		if err != nil {
			return AscensionState{}, err
		}
	}
	if out.SaveVersion != 7 || !strings.HasPrefix(out.Build, "1.0e12-") {
		return AscensionState{}, errors.New("unsupported Ascension save/mechanics version")
	}
	if save.UniqueID != "" {
		if strings.TrimSpace(save.UniqueID) == "" || len(save.UniqueID) > 256 {
			return AscensionState{}, errors.New("invalid Ascension profile identity")
		}
		hash := sha256.Sum256([]byte("clicker-heroes-bot/achievement-profile/" + save.UniqueID))
		out.ProfileID = hex.EncodeToString(hash[:])
	}
	if save.CurrentZone != "" {
		zone, err := prestigeInteger(save.CurrentZone, "current zone")
		if err != nil || zone < 1 {
			return AscensionState{}, errors.New("invalid current zone")
		}
		out.CurrentZone = &zone
	}
	if save.Stats.CurrentTranscension != nil {
		id, err := prestigeInteger(save.Stats.CurrentTranscension.ID, "current Transcension ID")
		if err != nil || id != out.Transcensions {
			return AscensionState{}, errors.New("current Transcension metadata does not match counters")
		}
		out.CurrentTranscensionID = &id
	}
	if save.Stats.CurrentAscension != nil {
		id, err := prestigeInteger(save.Stats.CurrentAscension.ID, "current Ascension ID")
		if err != nil || id != out.AscensionsThisTranscension {
			return AscensionState{}, errors.New("current Ascension metadata does not match counters")
		}
		cycle, err := prestigeInteger(save.Stats.CurrentAscension.Transcension, "current Ascension Transcension ID")
		if err != nil || cycle != out.Transcensions {
			return AscensionState{}, errors.New("current Ascension Transcension metadata does not match counters")
		}
		out.CurrentAscensionID, out.CurrentAscensionCycleID = &id, &cycle
	}
	pending, err := ascensionReward(save.ancientSave)
	if err != nil {
		return AscensionState{}, err
	}
	wallet, err := exactPrestigeNumber(save.HeroSouls, "Hero Souls wallet")
	if err != nil {
		return AscensionState{}, err
	}
	out.PendingHeroSouls, out.HeroSouls = ascensionQuantity(pending), string(save.HeroSouls)
	capital := new(big.Rat).Set(wallet)
	out.CapitalKnown = save.Ancients.Ancients != nil
	if len(save.Ancients.Ancients) > 100 {
		return AscensionState{}, errors.New("unsupported Ancient roster size")
	}
	for key, entry := range save.Ancients.Ancients {
		if err := ctx.Err(); err != nil {
			return AscensionState{}, err
		}
		id, err := strconv.Atoi(key)
		level, levelErr := exactPrestigeNumber(entry.Level, "Ancient level")
		if err != nil || id <= 0 || strconv.Itoa(id) != key || levelErr != nil || !level.IsInt() {
			return AscensionState{}, errors.New("invalid Ancient identity or level")
		}
		if entry.SpentHeroSouls == "" {
			if level.Sign() > 0 {
				out.CapitalKnown = false
			}
			continue
		}
		spent, err := exactPrestigeNumber(entry.SpentHeroSouls, "Ancient invested Hero Souls")
		if err != nil || level.Sign() == 0 && spent.Sign() != 0 {
			return AscensionState{}, errors.New("invalid Ancient invested Hero Souls")
		}
		capital.Add(capital, spent)
	}
	if out.CapitalKnown {
		out.SoulCapital = ascensionQuantity(capital)
	}
	hash := sha256.Sum256(exported)
	out.SaveHash = hex.EncodeToString(hash[:])
	if err := ctx.Err(); err != nil {
		return AscensionState{}, err
	}
	return out, nil
}
