package ancientcalc

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// RelicBonus levels are fractional, unlike purchased Ancient levels.
type RelicBonus struct {
	Type      int    `json:"type"`
	AncientID int    `json:"ancientId,omitempty"`
	Level     string `json:"level"`
}

type Relic struct {
	UID          int          `json:"uid"`
	Slot         int          `json:"slot"` // Zero preserves an item with no slot; it is not an empty inventory.
	Level        string       `json:"level"`
	Rarity       int          `json:"rarity"`
	ImageID      int          `json:"imageId"`
	UpgradeCount int          `json:"upgradeCount"`
	Bonuses      []RelicBonus `json:"bonuses"`
}

// A snapshot is not live readiness: a relic can arrive after Save.
type RelicSnapshot struct {
	Ascensions     int            `json:"ascensions"`
	Transcendent   bool           `json:"transcendent"`
	EquipmentSlots int            `json:"equipmentSlots"`
	Items          []Relic        `json:"items"`
	AncientLevels  map[int]string `json:"ancientLevels"`
	OutsiderLevels map[int]string `json:"outsiderLevels"`
	HighestZone    int            `json:"highestZone"`
}

type savedRelic struct {
	UID, Level, Rarity, ImageID, UpgradeCount          json.Number
	BonusType1, BonusType2, BonusType3, BonusType4     json.Number
	Bonus1Level, Bonus2Level, Bonus3Level, Bonus4Level json.Number
}

// ReadRelics reuses the bounded save envelope decoder and keeps unrelated save
// data out of the result. Missing or inconsistent inventory never means empty.
func ReadRelics(ctx context.Context, exported []byte) (RelicSnapshot, error) {
	var out RelicSnapshot
	save, err := decodeAncientSave(ctx, exported)
	if err != nil {
		return out, err
	}
	var raw struct {
		EquipmentSlots json.Number            `json:"equipmentSlots"`
		Items          map[string]savedRelic  `json:"items"`
		Slots          map[string]json.Number `json:"slots"`
	}
	if len(save.Items) == 0 || json.Unmarshal(save.Items, &raw) != nil || raw.Items == nil || raw.Slots == nil {
		return out, errors.New("missing or malformed relic inventory")
	}
	if out.EquipmentSlots, err = relicInteger(raw.EquipmentSlots, 1); err != nil || out.EquipmentSlots != 4 {
		return RelicSnapshot{}, errors.New("unsupported relic equipment slot count")
	}
	if out.Ascensions, err = relicInteger(save.NumWorldResets, 0); err != nil {
		return RelicSnapshot{}, errors.New("invalid relic snapshot Ascension count")
	}
	if save.Transcendent == nil {
		return RelicSnapshot{}, errors.New("missing relic snapshot transcendence state")
	}
	out.Transcendent = *save.Transcendent
	out.AncientLevels = map[int]string{}
	out.OutsiderLevels = map[int]string{}
	if out.HighestZone, err = relicInteger(save.HighestFinishedZonePersist, 0); err != nil {
		return RelicSnapshot{}, errors.New("invalid highest zone")
	}
	for key, entry := range save.Ancients.Ancients {
		id, e := strconv.Atoi(key)
		if e != nil {
			return RelicSnapshot{}, errors.New("invalid Ancient identity")
		}
		if _, levelErr := Value(entry.Level.String()); levelErr != nil {
			return RelicSnapshot{}, errors.New("invalid Ancient level")
		}
		out.AncientLevels[id] = entry.Level.String()
	}
	for key, entry := range save.Outsiders.Outsiders {
		id, e := strconv.Atoi(key)
		if e != nil {
			return RelicSnapshot{}, errors.New("invalid Outsider identity")
		}
		if _, levelErr := Value(entry.Level.String()); levelErr != nil {
			return RelicSnapshot{}, errors.New("invalid Outsider level")
		}
		out.OutsiderLevels[id] = entry.Level.String()
	}
	byUID := make(map[int]int, len(raw.Slots))
	for key, value := range raw.Slots {
		slot, err := relicInteger(json.Number(key), 1)
		if err != nil || strconv.Itoa(slot) != key {
			return RelicSnapshot{}, errors.New("invalid relic slot ID")
		}
		uid, err := relicInteger(value, 1)
		if err != nil {
			return RelicSnapshot{}, errors.New("invalid relic slot UID")
		}
		if _, exists := byUID[uid]; exists {
			return RelicSnapshot{}, errors.New("relic UID appears in multiple slots")
		}
		if _, exists := raw.Items[strconv.Itoa(uid)]; !exists {
			return RelicSnapshot{}, errors.New("relic slot references a missing item")
		}
		byUID[uid] = slot
	}
	out.Items = make([]Relic, 0, len(raw.Items))
	for key, item := range raw.Items {
		if ctx != nil && ctx.Err() != nil {
			return RelicSnapshot{}, ctx.Err()
		}
		uid, err := relicInteger(item.UID, 1)
		if err != nil || strconv.Itoa(uid) != key {
			return RelicSnapshot{}, errors.New("invalid relic item UID")
		}
		slot := byUID[uid]
		if _, ok := relicNumber(item.Level.String()); !ok {
			return RelicSnapshot{}, errors.New("invalid relic item level")
		}
		relic := Relic{UID: uid, Slot: slot, Level: item.Level.String(), Bonuses: []RelicBonus{}}
		for i, value := range []json.Number{item.Rarity, item.ImageID, item.UpgradeCount} {
			n, err := relicInteger(value, 0)
			if err != nil {
				return RelicSnapshot{}, errors.New("missing or invalid relic item metadata")
			}
			switch i {
			case 0:
				relic.Rarity = n
			case 1:
				relic.ImageID = n
			case 2:
				relic.UpgradeCount = n
			}
		}
		types := [...]json.Number{item.BonusType1, item.BonusType2, item.BonusType3, item.BonusType4}
		levels := [...]json.Number{item.Bonus1Level, item.Bonus2Level, item.Bonus3Level, item.Bonus4Level}
		seen := make(map[int]bool)
		for i, value := range types {
			kind, err := relicInteger(value, 0)
			level, ok := relicNumber(levels[i].String())
			if err != nil || !ok || (kind == 0 && level.Sign() != 0) || (kind != 0 && seen[kind]) {
				return RelicSnapshot{}, errors.New("missing or invalid relic bonus")
			}
			if kind != 0 {
				seen[kind] = true
				relic.Bonuses = append(relic.Bonuses, RelicBonus{Type: kind, AncientID: relicAncientIDs[kind], Level: levels[i].String()})
			}
		}
		out.Items = append(out.Items, relic)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if out.Items[i].Slot == out.Items[j].Slot {
			return out.Items[i].UID < out.Items[j].UID
		}
		return out.Items[i].Slot < out.Items[j].Slot
	})
	return out, nil
}

// Official build 6144 TextAsset itemBonusTypes (path ID 90), SHA256
// a3bac3ba658105c5c33d1f142e3f65a2ae8e4522aa0852f613796161766a00d4.
// Relic bonus IDs differ from Ancient IDs; 23 is absent from this asset.
var relicAncientIDs = map[int]int{
	1: 5, 2: 19, 3: 17, 4: 22, 5: 31, 6: 30, 7: 28,
	8: 27, 9: 26, 10: 25, 11: 24, 12: 23, 13: 18, 14: 16,
	15: 15, 16: 14, 17: 13, 18: 12, 19: 11, 20: 10, 21: 9,
	22: 8, 24: 4, 25: 3, 26: 29, 27: 21, 28: 20,
}

// RelicAncientName resolves the tooltip label from the existing Ancient catalog.
func RelicAncientName(kind int) string {
	id, ok := relicAncientIDs[kind]
	if !ok {
		return ""
	}
	var definitions struct {
		Ancients []ancientDefinition `json:"ancients"`
	}
	if json.Unmarshal(ancientDataJSON, &definitions) != nil {
		return ""
	}
	for _, definition := range definitions.Ancients {
		if definition.ID == id {
			return definition.Name
		}
	}
	return ""
}

type RelicSuggestion struct {
	UID        int `json:"uid"`
	Slot       int `json:"slot"`
	ReplaceUID int `json:"replaceUid"` // Zero means an empty equipment slot in this snapshot.
}

type RelicPreview struct {
	Readiness        string           `json:"readiness"`
	Reason           string           `json:"reason"`
	Snapshot         RelicSnapshot    `json:"snapshot"`
	UnsupportedTypes []int            `json:"unsupportedTypes"`
	Suggestion       *RelicSuggestion `json:"manualSuggestion,omitempty"`
}

// PreviewRelics suggests at most one manual move, then requires another scan.
// Slot layout, current junk membership and native input still need UI proof;
// neither a snapshot nor a suggestion authorizes Ascension or item destruction.
func PreviewRelics(ctx context.Context, exported []byte) (RelicPreview, error) {
	snapshot, err := ReadRelics(ctx, exported)
	if err != nil {
		return RelicPreview{}, err
	}
	preview := previewRelicSnapshot(ctx, snapshot, false)
	if ctx != nil && ctx.Err() != nil {
		return RelicPreview{}, ctx.Err()
	}
	return preview, nil
}

// PlanRelicEquipment uses the same Active comparison as the advisory preview.
// Its caller must establish the live inventory layout and match the move's UID
// before dragging. It never authorizes salvage or spending Forge Cores.
func PlanRelicEquipment(snapshot RelicSnapshot) (*RelicSuggestion, error) {
	if snapshot.EquipmentSlots != 4 {
		return nil, errors.New("unsupported relic equipment slot count")
	}
	if snapshot.AncientLevels != nil {
		return activeRelicSuggestion(snapshot)
	}
	preview := previewRelicSnapshot(nil, snapshot, true)
	if len(preview.UnsupportedTypes) != 0 {
		return nil, errors.New("unsupported relic effects require manual review")
	}
	return preview.Suggestion, nil
}

func previewRelicSnapshot(ctx context.Context, snapshot RelicSnapshot, verifiedJunk bool) RelicPreview {
	out := RelicPreview{
		Readiness: "unknown", Reason: "A save snapshot cannot prove current inventory or authorize Ascension. Suggestions require UI confirmation and manual application.",
		Snapshot: snapshot, UnsupportedTypes: []int{},
	}
	unsupported := map[int]bool{}
	layoutUnknown := false
	for _, item := range snapshot.Items {
		layoutUnknown = layoutUnknown || item.Slot > snapshot.EquipmentSlots
		for _, bonus := range item.Bonuses {
			// Iris changes the starting zone, which the restart loop does not
			// support yet. Solomon's relic effect after Transcension is not verified.
			if relicAncientIDs[bonus.Type] == 0 || bonus.Type == 6 || (bonus.Type == 25 && snapshot.Transcendent) {
				unsupported[bonus.Type] = true
			}
		}
	}
	for kind := range unsupported {
		out.UnsupportedTypes = append(out.UnsupportedTypes, kind)
	}
	sort.Ints(out.UnsupportedTypes)
	if len(out.UnsupportedTypes) > 0 {
		out.Reason = "Unsupported relic effects require manual review; no equipment suggestion or live readiness."
		return out
	}
	if layoutUnknown && !verifiedJunk {
		out.Reason = "Slots beyond the supported equipment range need UI confirmation; no equipment suggestion or live readiness."
		return out
	}
	if snapshot.AncientLevels != nil {
		var err error
		out.Suggestion, err = activeRelicSuggestion(snapshot)
		if err != nil {
			out.Reason, out.Suggestion = err.Error(), nil
		}
		return out
	}
	for slot := 1; slot <= snapshot.EquipmentSlots; slot++ {
		current := Relic{}
		for _, item := range snapshot.Items {
			if item.Slot == slot {
				current = item
				break
			}
		}
		for _, candidate := range snapshot.Items {
			if ctx != nil && ctx.Err() != nil {
				return out
			}
			if candidate.Slot >= 1 && candidate.Slot <= snapshot.EquipmentSlots {
				continue // Never remove a relic already contributing from another equipment slot.
			}
			if relicDominates(candidate, current) {
				out.Suggestion = &RelicSuggestion{UID: candidate.UID, Slot: slot, ReplaceUID: current.UID}
				return out
			}
		}
	}
	return out
}

func relicDominates(candidate, current Relic) bool {
	a, b := activeRelicBonuses(candidate), activeRelicBonuses(current)
	for kind, before := range b {
		if after := a[kind]; (after == nil && before.Sign() > 0) || (after != nil && after.Cmp(before) < 0) {
			return false
		}
	}
	for kind, after := range a {
		if before := b[kind]; (before == nil && after.Sign() > 0) || (before != nil && after.Cmp(before) > 0) {
			return true
		}
	}
	return false // Ties and incomparable active effects preserve current gear.
}

func relicInteger(value json.Number, minimum int) (int, error) {
	integer, ok := relicNumber(value.String())
	if !ok || !integer.IsInt() {
		return 0, errors.New("invalid relic integer")
	}
	n, err := strconv.Atoi(integer.Num().String())
	if err != nil || n < minimum {
		return 0, errors.New("invalid relic integer")
	}
	return n, nil
}

func relicNumber(value string) (*big.Rat, bool) {
	// Client bonuses are bounded decimal values; bound exponent expansion before
	// using exact rationals so tiny losses cannot be rounded into a safe swap.
	if len(value) > 128 || !ancientDecimal.MatchString(value) {
		return nil, false
	}
	if i := strings.IndexAny(value, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(value[i+1:])
		if err != nil || exponent < -308 || exponent > 308 {
			return nil, false
		}
	}
	n, ok := new(big.Rat).SetString(value)
	return n, ok && n.Sign() >= 0
}
