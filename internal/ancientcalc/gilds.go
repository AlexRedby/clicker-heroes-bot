package ancientcalc

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Names and upgrade thresholds from the official 1.0e12-6144 client TextAssets:
// https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb
//
//go:embed assets/gild-heroes.json
var gildHeroData []byte

// HeroNames returns the official roster already embedded for gild planning.
func HeroNames() ([]string, error) {
	var heroes []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(gildHeroData, &heroes); err != nil {
		return nil, err
	}
	names := make([]string, len(heroes))
	for i, hero := range heroes {
		names[i] = hero.Name
	}
	return names, nil
}

const maxGildInteger = 9007199254740991 // The client uses native doubles.

type GildHero struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Level  int64  `json:"level"`
	Gilds  int64  `json:"gilds"`
	Locked bool   `json:"locked"`
}

// GildHistory is supplied by the caller. Preview never creates a successful
// transfer receipt. An unresolved attempt must be reconciled before any replay.
type GildHistory struct {
	Transcensions         int64 `json:"transcensions"`
	TranscensionTimestamp int64 `json:"transcensionTimestamp"`
	LastTargetID          int   `json:"lastTargetID"`
	PendingTargetID       int   `json:"pendingTargetID"`
}

type GildPlan struct {
	PreviewOnly           bool         `json:"previewOnly"`
	Eligible              bool         `json:"eligible"`
	SaveHash              string       `json:"saveHash"`
	Ascensions            int64        `json:"ascensions"`
	Transcensions         int64        `json:"transcensions"`
	TranscensionTimestamp int64        `json:"transcensionTimestamp"`
	Target                GildHero     `json:"target"`
	Heroes                []GildHero   `json:"heroes"`
	TotalGilds            int64        `json:"totalGilds"`
	MoveGilds             int64        `json:"moveGilds"`
	UnitCost              int          `json:"unitCost"`
	Cost                  string       `json:"cost"`
	Allowance             string       `json:"allowance"`
	Souls                 string       `json:"souls"`
	Reserve               string       `json:"reserve"`
	Remaining             string       `json:"remaining"`
	MissingUpgrades       []int        `json:"missingUpgrades"`
	History               *GildHistory `json:"history"`
	Reasons               []string     `json:"reasons"`
}

type gildSaveHero struct {
	ID, UID, Level, EpicLevel json.Number
	Locked                    *bool
}

type gildSaveHeroes map[string]gildSaveHero

func (heroes *gildSaveHeroes) UnmarshalJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("gild save is missing a hero roster")
	}
	*heroes = make(gildSaveHeroes)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key := token.(string)
		if _, exists := (*heroes)[key]; exists || len(*heroes) >= 54 {
			return errors.New("duplicate or oversized gild hero roster")
		}
		var hero gildSaveHero
		if err := d.Decode(&hero); err != nil {
			return err
		}
		(*heroes)[key] = hero
	}
	_, err = d.Token()
	return err
}

func gildDecimal(raw, label string) (*big.Rat, error) {
	if _, err := Value(raw); err != nil {
		return nil, fmt.Errorf("invalid %s", label)
	}
	n, ok := new(big.Rat).SetString(raw)
	if !ok {
		return nil, fmt.Errorf("invalid %s", label)
	}
	return n, nil
}

func gildInteger(raw json.Number, label string) (int64, error) {
	n, err := gildDecimal(string(raw), label)
	if err != nil || !n.IsInt() || !n.Num().IsInt64() || n.Num().Int64() > maxGildInteger {
		return 0, fmt.Errorf("invalid %s: expected a bounded nonnegative integer", label)
	}
	return n.Num().Int64(), nil
}

func gildDecimalPlaces(raw string) int {
	mantissa, exponent, _ := strings.Cut(strings.ToLower(raw), "e")
	e, _ := strconv.Atoi(exponent)
	_, fraction, _ := strings.Cut(mantissa, ".")
	return max(0, len(fraction)-e)
}

func gildDecimalString(n *big.Rat, places int) string {
	text := n.FloatString(places)
	if places > 0 {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text
}

// CalculateGilds produces a read-only preview, never an executable action plan.
// Eligible means this snapshot fits the policy; live application still needs
// fresh state, modal ownership and native verification in the shared pipeline.
func CalculateGilds(ctx context.Context, exported []byte, reserve string, history *GildHistory) (GildPlan, error) {
	p := GildPlan{PreviewOnly: true, UnitCost: 80, Heroes: []GildHero{}, MissingUpgrades: []int{}, Reasons: []string{}}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return p, err
	}
	var save struct {
		HeroSouls, NumWorldResets, NumberOfTranscensions, TranscensionTimestamp json.Number
		HeroCollection                                                          struct{ Heroes gildSaveHeroes }
		Upgrades                                                                map[string]*bool
	}
	if err := json.Unmarshal(payload, &save); err != nil {
		return p, fmt.Errorf("invalid gild save: %w", err)
	}
	if len(save.HeroCollection.Heroes) == 0 || save.Upgrades == nil {
		return p, errors.New("gild save is missing heroes or upgrades")
	}
	wallet, err := gildDecimal(string(save.HeroSouls), "Hero Souls")
	if err != nil {
		return p, err
	}
	for _, field := range []struct {
		raw  json.Number
		name string
		out  *int64
	}{{save.NumWorldResets, "Ascension count", &p.Ascensions}, {save.NumberOfTranscensions, "Transcension count", &p.Transcensions}, {save.TranscensionTimestamp, "Transcension timestamp", &p.TranscensionTimestamp}} {
		*field.out, err = gildInteger(field.raw, field.name)
		if err != nil {
			return p, err
		}
	}
	var definitions []struct {
		ID       int
		Name     string
		Upgrades []struct {
			ID    int
			Level int64
		}
	}
	if err := json.Unmarshal(gildHeroData, &definitions); err != nil {
		return p, err
	}
	// Missing rows could hide source gilds and understate the transfer cost.
	if len(save.HeroCollection.Heroes) != len(definitions) {
		return p, errors.New("incomplete gild hero roster for client 1.0e12-6144")
	}
	for key := range save.HeroCollection.Heroes {
		id, err := strconv.Atoi(key)
		if err != nil || id < 1 || id > len(definitions) || strconv.Itoa(id) != key {
			return p, errors.New("unknown gild hero ID")
		}
	}
	for key, bought := range save.Upgrades {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id < 1 || id > maxGildInteger || strconv.FormatInt(id, 10) != key || bought == nil {
			return p, errors.New("invalid gild upgrade data")
		}
	}
	for _, def := range definitions {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		hero := save.HeroCollection.Heroes[strconv.Itoa(def.ID)]
		id, e1 := gildInteger(hero.ID, "hero ID")
		uid, e2 := gildInteger(hero.UID, "hero UID")
		level, e3 := gildInteger(hero.Level, "hero level")
		gilds, e4 := gildInteger(hero.EpicLevel, "hero gild count")
		if err := errors.Join(e1, e2, e3, e4); err != nil {
			return p, err
		}
		if id != int64(def.ID) || uid != id || hero.Locked == nil || (*hero.Locked && level > 0) {
			return p, errors.New("mismatched gild hero identity or ownership")
		}
		row := GildHero{def.ID, def.Name, level, gilds, *hero.Locked}
		p.Heroes = append(p.Heroes, row)
		p.TotalGilds += gilds
		if !row.Locked && row.Level > 0 {
			p.Target = row
		}
	}
	if p.Target.ID == 0 {
		p.Reasons = append(p.Reasons, "no purchased hero")
	} else {
		p.MoveGilds = p.TotalGilds - p.Target.Gilds
		if p.Target.ID < 28 || p.Target.ID > 46 {
			p.Reasons = append(p.Reasons, "latest hero is outside the supported Atlas-Xavira progression")
		}
		if p.Target.Level < 1000 {
			p.Reasons = append(p.Reasons, "latest hero has not reached level 1000")
		}
		for _, upgrade := range definitions[p.Target.ID-1].Upgrades {
			bought := save.Upgrades[strconv.Itoa(upgrade.ID)]
			if upgrade.Level <= p.Target.Level && (bought == nil || !*bought) {
				p.MissingUpgrades = append(p.MissingUpgrades, upgrade.ID)
			}
		}
		if len(p.MissingUpgrades) > 0 {
			p.Reasons = append(p.Reasons, "available target upgrades are not purchased")
		}
	}
	if p.TotalGilds == 0 {
		p.Reasons = append(p.Reasons, "no gilds")
	} else if p.Target.ID != 0 && p.MoveGilds == 0 {
		p.Reasons = append(p.Reasons, "all gilds are already on the target")
	}
	reserve = strings.TrimSpace(reserve)
	percent := strings.HasSuffix(reserve, "%")
	rawReserve := strings.TrimSpace(strings.TrimSuffix(reserve, "%"))
	protected, err := gildDecimal(rawReserve, "soul reserve")
	if err != nil {
		return p, err
	}
	places := gildDecimalPlaces(rawReserve)
	if percent {
		if protected.Cmp(big.NewRat(100, 1)) > 0 {
			return p, errors.New("reserve percentage must be from 0 to 100")
		}
		protected.Mul(protected, wallet).Quo(protected, big.NewRat(100, 1))
		places += gildDecimalPlaces(string(save.HeroSouls)) + 2
	}
	places = max(places, gildDecimalPlaces(string(save.HeroSouls)))
	cost := new(big.Int).Mul(big.NewInt(p.MoveGilds), big.NewInt(80))
	remaining := new(big.Rat).Sub(wallet, new(big.Rat).SetInt(cost))
	p.Cost = cost.String()
	p.Allowance = new(big.Int).Mul(big.NewInt(p.TotalGilds), big.NewInt(80)).String()
	p.Souls = string(save.HeroSouls)
	p.Reserve, p.Remaining = gildDecimalString(protected, places), gildDecimalString(remaining, places)
	if remaining.Cmp(protected) < 0 {
		p.Reasons = append(p.Reasons, "transfer would exceed the wallet or protected reserve")
	}
	if history == nil {
		p.Reasons = append(p.Reasons, "transfer history is required; reconcile before application")
	} else {
		validID := func(id int) bool { return id == 0 || (id >= 28 && id <= 46) }
		if history.Transcensions < 0 || history.Transcensions > maxGildInteger || history.TranscensionTimestamp < 0 || history.TranscensionTimestamp > maxGildInteger || !validID(history.LastTargetID) || !validID(history.PendingTargetID) {
			return p, errors.New("invalid gild transfer history")
		}
		copy := *history
		p.History = &copy
		if history.Transcensions != p.Transcensions || history.TranscensionTimestamp != p.TranscensionTimestamp {
			p.Reasons = append(p.Reasons, "Transcension identity changed; reconcile transfer history")
		}
		if history.PendingTargetID != 0 {
			p.Reasons = append(p.Reasons, "an attempted transfer is unresolved")
		}
		if history.LastTargetID != 0 && p.Target.ID <= history.LastTargetID {
			p.Reasons = append(p.Reasons, "target is not later than the last transferred hero")
		}
	}
	p.SaveHash = fmt.Sprintf("%x", sha256.Sum256(exported))
	p.Eligible = len(p.Reasons) == 0
	return p, ctx.Err()
}
