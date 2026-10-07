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

// TranscensionState contains export facts, not authorization to reset or spend.
// Nil zone/cycle fields and an empty profile hash denote missing metadata.
type TranscensionState struct {
	SaveHash                   string  `json:"saveHash"`
	ProfileID                  string  `json:"profileId,omitempty"`
	Build                      string  `json:"build"`
	SaveVersion                int     `json:"saveVersion"`
	Transcendent               bool    `json:"transcendent"`
	Transcensions              int     `json:"transcensions"`
	Ascensions                 int     `json:"ascensions"`
	AscensionsThisTranscension int     `json:"ascensionsThisTranscension"`
	CurrentZone                *int    `json:"currentZone,omitempty"`
	HighestZone                int     `json:"highestZone"`
	CurrentTranscensionID      *int    `json:"currentTranscensionId,omitempty"`
	CurrentAscensionID         *int    `json:"currentAscensionId,omitempty"`
	CurrentAscensionCycleID    *int    `json:"currentAscensionCycleId,omitempty"`
	AncientSouls               int     `json:"ancientSouls"`
	AncientSoulsTotal          int     `json:"ancientSoulsTotal"`
	HeroSouls                  string  `json:"heroSouls"`
	Ancients                   []Level `json:"ancients"`
	Outsiders                  []Level `json:"outsiders"`
}

func transcensionDefinitions() ([]ancientDefinition, error) {
	var definitions struct {
		Ancients []ancientDefinition `json:"ancients"`
	}
	err := json.Unmarshal(ancientDataJSON, &definitions)
	return definitions.Ancients, err
}

// ReadTranscensionState shares the bounded decoder and preserves the distinction
// between currentZoneHeight and the historical highest zone. No raw profile ID
// leaves the decoder; ProfileID uses the existing achievement-profile hash.
func ReadTranscensionState(ctx context.Context, exported []byte) (TranscensionState, error) {
	var out TranscensionState
	if ctx == nil {
		ctx = context.Background()
	}
	save, err := decodeAncientSave(ctx, exported)
	if err != nil {
		return out, err
	}
	if save.Transcendent == nil {
		return out, errors.New("save is missing transcendence state")
	}
	if _, err := exactPrestigeNumber(save.HeroSoulsSacrificed, "sacrificed Hero Souls"); err != nil {
		return out, err
	}
	out.Transcendent, out.Build = *save.Transcendent, save.Build
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
		{save.AncientSouls, "Ancient Souls wallet", &out.AncientSouls},
		{save.AncientSoulsTotal, "Ancient Souls total", &out.AncientSoulsTotal},
	} {
		*field.value, err = prestigeInteger(field.raw, field.name)
		if err != nil {
			return TranscensionState{}, err
		}
	}
	if out.SaveVersion != 7 || !strings.HasPrefix(out.Build, "1.0e12-") {
		return TranscensionState{}, errors.New("unsupported Transcension save/mechanics version")
	}
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return out, err
	}
	var metadata struct {
		UniqueID    string      `json:"uniqueId"`
		CurrentZone json.Number `json:"currentZoneHeight"`
	}
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return TranscensionState{}, errors.New("invalid Transcension save metadata")
	}
	if metadata.UniqueID != "" {
		if strings.TrimSpace(metadata.UniqueID) == "" || len(metadata.UniqueID) > 256 {
			return TranscensionState{}, errors.New("invalid Transcension profile identity")
		}
		hash := sha256.Sum256([]byte("clicker-heroes-bot/achievement-profile/" + metadata.UniqueID))
		out.ProfileID = hex.EncodeToString(hash[:])
	}
	if metadata.CurrentZone != "" {
		zone, err := prestigeInteger(metadata.CurrentZone, "current zone")
		if err != nil || zone < 1 {
			return TranscensionState{}, errors.New("invalid current zone")
		}
		out.CurrentZone = &zone
	}
	if save.Stats.CurrentTranscension != nil {
		id, err := prestigeInteger(save.Stats.CurrentTranscension.ID, "current Transcension ID")
		if err != nil {
			return TranscensionState{}, err
		}
		out.CurrentTranscensionID = &id
	}
	if save.Stats.CurrentAscension != nil {
		id, err := prestigeInteger(save.Stats.CurrentAscension.ID, "current Ascension ID")
		if err != nil {
			return TranscensionState{}, err
		}
		cycle, err := prestigeInteger(save.Stats.CurrentAscension.Transcension, "current Ascension Transcension ID")
		if err != nil {
			return TranscensionState{}, err
		}
		out.CurrentAscensionID, out.CurrentAscensionCycleID = &id, &cycle
	}
	out.HeroSouls = string(save.HeroSouls)
	definitions, err := transcensionDefinitions()
	if err != nil {
		return TranscensionState{}, err
	}
	if len(save.Ancients.Ancients) > 100 || len(save.Outsiders.Outsiders) > 10 {
		return TranscensionState{}, errors.New("unsupported prestige roster size")
	}
	known := make(map[int]ancientDefinition, len(definitions))
	for _, def := range definitions {
		known[def.ID] = def
	}
	owned := make(map[int]string)
	for key, entry := range save.Ancients.Ancients {
		if err := ctx.Err(); err != nil {
			return TranscensionState{}, err
		}
		id, err := strconv.Atoi(key)
		level, levelErr := exactPrestigeNumber(entry.Level, "Ancient level")
		if err != nil || strconv.Itoa(id) != key || known[id].Name == "" || levelErr != nil || !level.IsInt() {
			return TranscensionState{}, errors.New("invalid Ancient identity or level")
		}
		if entry.SpentHeroSouls != "" {
			spent, err := exactPrestigeNumber(entry.SpentHeroSouls, "Ancient invested Hero Souls")
			if err != nil || level.Sign() == 0 && spent.Sign() != 0 {
				return TranscensionState{}, errors.New("invalid Ancient invested Hero Souls")
			}
		}
		if level.Sign() > 0 {
			owned[id] = string(entry.Level)
		}
	}
	out.Ancients = []Level{}
	for _, def := range definitions {
		if level, exists := owned[def.ID]; exists {
			out.Ancients = append(out.Ancients, Level{def.ID, def.Name, level})
		}
	}
	levels, complete, err := prestigeOutsiders(save, out.AncientSoulsTotal, out.AncientSouls)
	if err != nil {
		return TranscensionState{}, err
	}
	if !complete {
		return TranscensionState{}, errors.New("Outsider AS ledger is incomplete or does not match total")
	}
	for i, id := range outsiderIDs {
		out.Outsiders = append(out.Outsiders, Level{id, outsiderNames[i], strconv.Itoa(levels[i])})
	}
	hash := sha256.Sum256(exported)
	out.SaveHash = hex.EncodeToString(hash[:])
	if err := out.validate(ctx); err != nil {
		return TranscensionState{}, err
	}
	return out, nil
}

func validStateDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == value
}

func (s TranscensionState) validate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validStateDigest(s.SaveHash) || s.ProfileID != "" && !validStateDigest(s.ProfileID) || s.SaveVersion != 7 || !strings.HasPrefix(s.Build, "1.0e12-") {
		return errors.New("invalid or unsupported Transcension state identity")
	}
	for _, value := range []int{s.Transcensions, s.Ascensions, s.AscensionsThisTranscension, s.HighestZone} {
		if !supportedOutsiderInt(value) {
			return errors.New("invalid prestige counter")
		}
	}
	if s.CurrentZone != nil && (!supportedOutsiderInt(*s.CurrentZone) || *s.CurrentZone < 1) {
		return errors.New("invalid current zone")
	}
	for _, field := range []struct {
		actual   *int
		expected int
	}{
		{s.CurrentTranscensionID, s.Transcensions}, {s.CurrentAscensionCycleID, s.Transcensions}, {s.CurrentAscensionID, s.AscensionsThisTranscension},
	} {
		if field.actual != nil && *field.actual != field.expected {
			return errors.New("save cycle metadata does not match prestige counters")
		}
	}
	if _, err := exactPrestigeNumber(json.Number(s.HeroSouls), "Hero Souls wallet"); err != nil {
		return err
	}
	if _, err := PlanOutsiders(ctx, s.AncientSoulsTotal, s.AncientSouls, 0, s.Outsiders); err != nil {
		return err
	}
	for _, row := range s.Outsiders {
		if row.Name == "" {
			return errors.New("missing canonical Outsider name")
		}
	}
	definitions, err := transcensionDefinitions()
	if err != nil {
		return err
	}
	known := make(map[int]string, len(definitions))
	for _, def := range definitions {
		known[def.ID] = def.Name
	}
	if s.Ancients == nil || len(s.Ancients) > len(definitions) {
		return errors.New("missing or unsupported Ancient roster")
	}
	seen := make(map[int]bool, len(s.Ancients))
	for _, row := range s.Ancients {
		if err := ctx.Err(); err != nil {
			return err
		}
		level, err := exactPrestigeNumber(json.Number(row.Level), "Ancient level")
		if err != nil || !level.IsInt() || level.Sign() <= 0 || known[row.ID] != row.Name || row.Name == "" || seen[row.ID] {
			return errors.New("invalid or duplicate owned Ancient")
		}
		seen[row.ID] = true
	}
	return ctx.Err()
}

func validateReceiptPair(ctx context.Context, before, after TranscensionState) error {
	for _, state := range []TranscensionState{before, after} {
		if err := state.validate(ctx); err != nil {
			return err
		}
		if state.ProfileID == "" {
			return errors.New("receipt is missing hashed profile identity")
		}
		if state.CurrentTranscensionID == nil || state.CurrentAscensionID == nil || state.CurrentAscensionCycleID == nil {
			return errors.New("receipt is missing current Transcension/Ascension cycle metadata")
		}
	}
	if before.ProfileID != after.ProfileID || before.Build != after.Build || before.SaveVersion != after.SaveVersion {
		return errors.New("receipt profile/build changed")
	}
	if before.SaveHash == after.SaveHash {
		return errors.New("receipt uses a stale save hash")
	}
	return nil
}

func equalStateLevels(a, b []Level) bool {
	if len(a) != len(b) {
		return false
	}
	levels := make(map[int]Level, len(a))
	for _, row := range a {
		levels[row.ID] = row
	}
	for _, row := range b {
		old, exists := levels[row.ID]
		if !exists || old.Name != row.Name || !equalStateNumber(old.Level, row.Level) {
			return false
		}
	}
	return true
}

func equalStateNumber(a, b string) bool {
	x, ex := exactPrestigeNumber(json.Number(a), "receipt amount")
	y, ey := exactPrestigeNumber(json.Number(b), "receipt amount")
	return ex == nil && ey == nil && x.Cmp(y) == 0
}

// VerifyTranscensionReceipt proves the observed positive reward and a new cycle
// with respec disabled. Missing current-zone/profile/cycle metadata is a blocker;
// timestamps and reward estimates never substitute for receipt evidence.
func VerifyTranscensionReceipt(ctx context.Context, before, after TranscensionState, observedReward int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReceiptPair(ctx, before, after); err != nil {
		return err
	}
	if !supportedOutsiderInt(observedReward) || observedReward <= 0 {
		return errors.New("Transcension receipt requires an observed positive AS reward")
	}
	cycle, err := addSupportedOutsiderInts(before.Transcensions, 1)
	if err != nil || after.Transcensions != cycle || !after.Transcendent || after.AscensionsThisTranscension != 0 {
		return errors.New("Transcension receipt did not establish the next fresh cycle")
	}
	if after.CurrentZone == nil {
		return errors.New("Transcension receipt is missing currentZoneHeight reset metadata")
	}
	if *after.CurrentZone != 1 {
		return errors.New("Transcension receipt current zone is not 1")
	}
	if !equalStateNumber(after.HeroSouls, "0") {
		return errors.New("Transcension receipt Hero Souls wallet did not reset to zero")
	}
	total, err := addSupportedOutsiderInts(before.AncientSoulsTotal, observedReward)
	if err != nil {
		return err
	}
	wallet, err := addSupportedOutsiderInts(before.AncientSouls, observedReward)
	if err != nil {
		return err
	}
	if after.AncientSoulsTotal != total || after.AncientSouls != wallet || !equalStateLevels(before.Outsiders, after.Outsiders) || len(after.Ancients) != 0 {
		return errors.New("Transcension receipt AS, Outsider roster or Ancient reset mismatch")
	}
	return ctx.Err()
}

// VerifyOutsiderFeedReceipt accepts one row and an explicit positive quantity.
// It rejects other roster, soul, zone, build or cycle changes during the feed.
func VerifyOutsiderFeedReceipt(ctx context.Context, before, after TranscensionState, id, quantity int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReceiptPair(ctx, before, after); err != nil {
		return err
	}
	if before.Transcensions != after.Transcensions || before.Ascensions != after.Ascensions || before.AscensionsThisTranscension != after.AscensionsThisTranscension || before.Transcendent != after.Transcendent || before.HighestZone != after.HighestZone || before.AncientSoulsTotal != after.AncientSoulsTotal || !equalStateNumber(before.HeroSouls, after.HeroSouls) || !equalStateLevels(before.Ancients, after.Ancients) {
		return errors.New("Outsider feed receipt contains unrelated state changes")
	}
	if before.CurrentZone == nil != (after.CurrentZone == nil) || before.CurrentZone != nil && *before.CurrentZone != *after.CurrentZone {
		return errors.New("Outsider feed receipt current zone changed")
	}
	expected := append([]Level(nil), before.Outsiders...)
	for i, row := range expected {
		if row.ID != id {
			continue
		}
		level, err := parseOutsiderLevel(row.Level)
		if err != nil {
			return err
		}
		cost, err := OutsiderFeedCost(id, level, quantity)
		if err != nil {
			return err
		}
		if cost > before.AncientSouls || after.AncientSouls != before.AncientSouls-cost {
			return errors.New("Outsider feed receipt wallet does not match exact cost")
		}
		expected[i].Level = strconv.Itoa(level + quantity)
		if !equalStateLevels(expected, after.Outsiders) {
			return errors.New("Outsider feed receipt level or row mismatch")
		}
		return ctx.Err()
	}
	return errors.New("unknown Outsider feed ID")
}

// MissingActiveAncients lists policy dependencies, never summon prices or
// purchase instructions. Skill Ancients are included only for a positive rate.
func MissingActiveAncients(ctx context.Context, state TranscensionState, skillRate float64, beyond8k bool) ([]AncientRequirement, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !(skillRate >= 0 && skillRate <= 1) {
		return nil, errors.New("Ancient skill rate must be between 0 and 1")
	}
	if err := state.validate(ctx); err != nil {
		return nil, err
	}
	definitions, err := transcensionDefinitions()
	if err != nil {
		return nil, err
	}
	owned := make(map[int]bool, len(state.Ancients))
	for _, row := range state.Ancients {
		owned[row.ID] = true
	}
	out := []AncientRequirement{}
	rate := aConst(strconv.FormatFloat(skillRate, 'g', -1, 64))
	for _, def := range definitions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !owned[def.ID] && ancientGoal(def.Name, aConst("10"), aConst("1"), aConst("1"), rate, beyond8k) != nil {
			out = append(out, AncientRequirement{def.ID, def.Name})
		}
	}
	return out, nil
}
