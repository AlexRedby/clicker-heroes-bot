package ancientcalc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// Unsupported complete saves fall back to visual setup; decoding failures can
// still mean the game's export file is only partially written.
var ErrHeroSetupUnsupported = errors.New("unsupported hero setup snapshot")

type HeroSetupHero struct {
	ID            int
	Level         int64
	RequiredLevel int64
	NeedsLevels   bool
}

type HeroSetupPlan struct {
	Heroes          []HeroSetupHero
	MissingUpgrades []int
	PassiveReady    bool
	NeedsLevels     bool
}

type setupUpgrade struct {
	ID            int
	Level         int64
	Prerequisites []int
	Kind          string
}

type setupHero struct {
	ID       int
	Upgrades []setupUpgrade
}

// PlanHeroSetup only describes skill preparation. Future heroes beyond the
// latest hire belong to ordinary latest-hero progression, not the startup sweep.
func PlanHeroSetup(ctx context.Context, exported []byte) (HeroSetupPlan, error) {
	p := HeroSetupPlan{Heroes: []HeroSetupHero{}, MissingUpgrades: []int{}}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := decodeSavePayload(ctx, exported)
	if err != nil {
		return p, err
	}
	if !json.Valid(payload) {
		return p, errors.New("hero setup payload is not complete JSON")
	}
	var save struct {
		HeroCollection struct{ Heroes gildSaveHeroes }
		Upgrades       map[string]*bool
	}
	if err := json.Unmarshal(payload, &save); err != nil {
		return p, fmt.Errorf("%w: %v", ErrHeroSetupUnsupported, err)
	}
	var definitions []setupHero
	if err := json.Unmarshal(gildHeroData, &definitions); err != nil {
		return p, fmt.Errorf("%w: invalid hero catalog", ErrHeroSetupUnsupported)
	}
	if len(definitions) != 54 || len(save.HeroCollection.Heroes) != len(definitions) || save.Upgrades == nil {
		return p, fmt.Errorf("%w: incomplete hero roster or upgrades", ErrHeroSetupUnsupported)
	}
	levels := make(map[int]int64, len(definitions))
	latest := 0
	for _, def := range definitions {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		hero, exists := save.HeroCollection.Heroes[strconv.Itoa(def.ID)]
		id, e1 := gildInteger(hero.ID, "hero ID")
		uid, e2 := gildInteger(hero.UID, "hero UID")
		level, e3 := gildInteger(hero.Level, "hero level")
		if !exists || errors.Join(e1, e2, e3) != nil || id != int64(def.ID) || uid != id ||
			hero.Locked == nil || *hero.Locked && level > 0 || len(def.Upgrades) == 0 {
			return p, fmt.Errorf("%w: invalid hero %d", ErrHeroSetupUnsupported, def.ID)
		}
		levels[def.ID] = level
		if level > 0 {
			latest = max(latest, def.ID)
			p.PassiveReady = p.PassiveReady || def.ID > 1
		}
	}
	for key, bought := range save.Upgrades {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || id <= 0 || id > maxGildInteger || strconv.FormatInt(id, 10) != key || bought == nil {
			return p, fmt.Errorf("%w: invalid upgrade ownership", ErrHeroSetupUnsupported)
		}
	}
	if !p.PassiveReady {
		latest = max(latest, 2) // Cid alone cannot generate passive starter gold.
	}
	upgrades := make(map[int]setupUpgrade)
	owners := make(map[int]int)
	for _, def := range definitions {
		for _, upgrade := range def.Upgrades {
			upgrades[upgrade.ID], owners[upgrade.ID] = upgrade, def.ID
		}
	}
	required := make(map[int]int64)
	seen := make(map[int]bool)
	var need func(int) error
	need = func(id int) error {
		if upgrades[id].Kind == "reset" || id == 132 {
			return nil // Reset buttons are never skill preparation purchases.
		}
		if seen[id] {
			return nil
		}
		seen[id] = true
		if bought := save.Upgrades[strconv.Itoa(id)]; bought != nil && *bought {
			return nil
		}
		upgrade, exists := upgrades[id]
		if !exists {
			return fmt.Errorf("%w: unknown prerequisite %d", ErrHeroSetupUnsupported, id)
		}
		p.MissingUpgrades = append(p.MissingUpgrades, id)
		owner := owners[id]
		required[owner] = max(required[owner], max(1, upgrade.Level))
		for _, prerequisite := range upgrade.Prerequisites {
			if err := need(prerequisite); err != nil {
				return err
			}
		}
		return nil
	}
	for _, def := range definitions {
		if def.ID > latest {
			break
		}
		for _, upgrade := range def.Upgrades {
			// Ordinary progression levels the latest hero. Preparation only needs
			// its unlocked upgrades and any missing active skill.
			if p.PassiveReady && def.ID == latest && levels[def.ID] < upgrade.Level && upgrade.Kind != "skill" {
				continue
			}
			if err := need(upgrade.ID); err != nil {
				return HeroSetupPlan{}, err
			}
		}
	}
	for _, def := range definitions {
		minimum, missing := required[def.ID]
		if !missing {
			continue
		}
		row := HeroSetupHero{ID: def.ID, Level: levels[def.ID], RequiredLevel: minimum, NeedsLevels: levels[def.ID] < minimum}
		p.Heroes = append(p.Heroes, row)
		p.NeedsLevels = p.NeedsLevels || row.NeedsLevels
	}
	p.NeedsLevels = p.NeedsLevels || !p.PassiveReady
	sort.Ints(p.MissingUpgrades)
	return p, nil
}
