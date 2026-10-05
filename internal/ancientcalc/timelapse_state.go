package ancientcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
)

// TimelapseState contains facts from one export, never permission to spend.
// Actual Hybrid, hero/gild preparation and a free-clicker shared frame are
// separate dependencies; possession of idle Ancients is not optimal allocation.
type TimelapseState struct {
	ProfileID              string `json:"profileId"` // Hash; raw client/account IDs are not returned.
	AscensionID            string `json:"ascensionId"`
	Build                  string `json:"build"`
	Zone, HighestZone      int64
	Rubies                 uint64
	AutoClickers           int64
	LogAscensionStartSouls float64
	Ancients, Outsiders    map[string]string
}

func ReadTimelapseState(ctx context.Context, exported []byte) (TimelapseState, error) {
	var out TimelapseState
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return out, err
	}
	var save struct {
		UniqueID                                                                                                   string `json:"uniqueId"`
		Build                                                                                                      string `json:"readPatchNumber"`
		CurrentZoneHeight, HighestFinishedZonePersist, Rubies, Autoclickers, NumWorldResets, NumberOfTranscensions json.Number
		Stats                                                                                                      struct {
			CurrentAscension struct{ HeroSoulsStart json.Number }
		}
		Ancients  ancientSaveAncients
		Outsiders ancientSaveOutsiders
	}
	if err := json.Unmarshal(payload, &save); err != nil {
		return out, fmt.Errorf("invalid Timelapse save")
	}
	if strings.TrimSpace(save.UniqueID) == "" || len(save.UniqueID) > 256 || strings.TrimSpace(save.Build) == "" || len(save.Build) > 64 {
		return out, fmt.Errorf("Timelapse profile/build identity unavailable")
	}
	hash := sha256.Sum256([]byte("clicker-heroes-bot/timelapse-profile/" + save.UniqueID))
	out.ProfileID = hex.EncodeToString(hash[:])
	out.Build = save.Build
	var resets, trans int64
	for _, f := range []struct {
		raw  json.Number
		name string
		dest *int64
	}{
		{save.CurrentZoneHeight, "current zone", &out.Zone}, {save.HighestFinishedZonePersist, "highest zone", &out.HighestZone},
		{save.Autoclickers, "Auto Clickers", &out.AutoClickers}, {save.NumWorldResets, "Ascensions", &resets}, {save.NumberOfTranscensions, "Transcensions", &trans},
	} {
		value, err := gildInteger(f.raw, f.name)
		if err != nil {
			return TimelapseState{}, err
		}
		*f.dest = value
	}
	balance, err := gildInteger(save.Rubies, "rubies")
	if err != nil {
		return TimelapseState{}, err
	}
	out.Rubies = uint64(balance)
	if out.Zone < 1 || out.Zone > out.HighestZone+1 || out.AutoClickers > 2000000000 {
		return TimelapseState{}, fmt.Errorf("unsupported Timelapse zone/clicker state")
	}
	out.AscensionID = fmt.Sprintf("%d/%d", trans, resets)
	souls, err := Value(string(save.Stats.CurrentAscension.HeroSoulsStart))
	if err != nil || souls.Sign() <= 0 {
		return TimelapseState{}, fmt.Errorf("Ascension-start soul capital unavailable")
	}
	mantissa := new(big.Float)
	exponent := souls.MantExp(mantissa)
	m, _ := mantissa.Float64()
	out.LogAscensionStartSouls = math.Log10(m) + float64(exponent)*math.Log10(2)
	out.Ancients = make(map[string]string)
	out.Outsiders = make(map[string]string)
	for _, set := range []struct {
		entries map[string]ancientSaveEntry
		dest    map[string]string
	}{{save.Ancients.Ancients, out.Ancients}, {save.Outsiders.Outsiders, out.Outsiders}} {
		if set.entries == nil {
			return TimelapseState{}, fmt.Errorf("missing Timelapse Ancient/Outsider state")
		}
		for id, entry := range set.entries {
			if _, err := Value(string(entry.Level)); err != nil {
				return TimelapseState{}, fmt.Errorf("invalid Timelapse Ancient/Outsider level")
			}
			set.dest[id] = string(entry.Level)
		}
	}
	return out, nil
}
