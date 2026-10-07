package ancientcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const SupportedAchievementBuild = "1.0e12-6144"

// AchievementCounter distinguishes an actual zero from unavailable data.
type AchievementCounter struct {
	Value  int64  `json:"value"`
	Known  bool   `json:"known"`
	Reason string `json:"reason,omitempty"`
}

type AchievementState struct {
	ProfileID       string                        `json:"profileId"`
	Build           string                        `json:"build"`
	Earned          map[int]bool                  `json:"earned"`
	OwnershipKnown  bool                          `json:"ownershipKnown"`
	OwnershipReason string                        `json:"ownershipReason,omitempty"`
	Counters        map[string]AchievementCounter `json:"counters"`
}

// ReadAchievementState uses the shared bounded export decoder. Counter paths
// below were observed in the 1.0e12-6144 desktop export, not inferred from names.
// A changed build retains ownership but cannot authorize a strategy override.
func ReadAchievementState(ctx context.Context, exported []byte) (AchievementState, error) {
	var out AchievementState
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return out, err
	}
	var save map[string]json.RawMessage
	if err := json.Unmarshal(payload, &save); err != nil || save == nil {
		return out, errors.New("invalid achievement save JSON object")
	}
	var id string
	if json.Unmarshal(save["uniqueId"], &id) != nil || strings.TrimSpace(id) == "" || len(id) > 256 {
		return out, errors.New("achievement profile identity unavailable")
	}
	hash := sha256.Sum256([]byte("clicker-heroes-bot/achievement-profile/" + id))
	out.ProfileID = hex.EncodeToString(hash[:])
	_ = json.Unmarshal(save["readPatchNumber"], &out.Build)
	out.Earned = make(map[int]bool)
	var ownership map[string]*bool
	out.OwnershipKnown = json.Unmarshal(save["achievements"], &ownership) == nil && ownership != nil
	for key, value := range ownership {
		id, err := strconv.Atoi(key)
		if err != nil || id <= 0 || strconv.Itoa(id) != key || value == nil {
			out.OwnershipKnown = false
			break
		}
		out.Earned[id] = *value
	}
	if !out.OwnershipKnown {
		out.Earned = make(map[int]bool)
		out.OwnershipReason = "missing or malformed achievements ownership"
	}
	out.Counters = make(map[string]AchievementCounter)
	for _, name := range []string{
		"goldQuestsCompleted", "relicQuestsCompleted", "rubyQuestsCompleted",
		"skillQuestsCompleted", "heroSoulQuestsCompleted", "total5MinuteQuests",
		"totalClicks", "totalBossKills", "totalHeroLevels", "highestFinishedZone",
		"transcendentHighestFinishedZone",
	} {
		counter := AchievementCounter{}
		var n json.Number
		switch {
		case out.Build != SupportedAchievementBuild:
			counter.Reason = "unsupported save build"
		case len(save[name]) == 0:
			counter.Reason = "missing counter"
		case json.Unmarshal(save[name], &n) != nil:
			counter.Reason = "malformed counter"
		default:
			value, err := gildInteger(n, name)
			if err != nil {
				counter.Reason = "malformed counter"
			} else {
				counter.Value, counter.Known = value, true
			}
		}
		out.Counters[name] = counter
	}
	if ctx != nil && ctx.Err() != nil {
		return AchievementState{}, ctx.Err()
	}
	return out, nil
}
