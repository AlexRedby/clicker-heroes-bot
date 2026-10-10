package ancientcalc

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strconv"
)

// Official https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.wasm.unityweb:
// AncientsController..ctor's 35 uint32 prices are
// metadata field 33671, SHA256 3F463C3966043DDA8E62BA30266ACC9A63258F2635F5EED6C7FD14D474C390CB.
// GetPriceOfAllAncients sums prices[ownedCount:totalCount]. PurchaseAllRemainingAncients
// skips the TextAsset ancients' nonTranscendent rows (3, 6, 7, 30), leaving 26.
// OnSummonAllClicked purchases immediately; PurchaseAncient initializes level 1.
var summonAncientPrices6144 = [...]int{
	1, 2, 4, 8, 16, 35, 70, 125, 250, 500, 800, 1200, 1700, 2200,
	2750, 3400, 4100, 5000, 6000, 7500, 10000, 12500, 16000, 25000,
	35000, 50000, 70000, 100000, 150000, 250000, 400000, 750000,
	1400000, 2700000, 5200000,
}

type SummonAllPlan struct {
	Cost     string  `json:"cost"`
	Missing  []Level `json:"missing"`
	Eligible bool    `json:"eligible"`
}

// PlanSummonAll quotes the immediate purchase without authorizing native input.
// Restoration supports the verified transcendent roster of client 6144 only.
func PlanSummonAll(ctx context.Context, state TranscensionState) (SummonAllPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := state.validate(ctx); err != nil {
		return SummonAllPlan{}, err
	}
	if state.Build != "1.0e12-6144" || !state.Transcendent {
		return SummonAllPlan{}, errors.New("Summon All requires the verified transcendent client 6144")
	}
	definitions, err := transcensionDefinitions()
	if err != nil {
		return SummonAllPlan{}, err
	}
	owned := make(map[int]bool, len(state.Ancients))
	for _, row := range state.Ancients {
		if row.ID == 3 || row.ID == 6 || row.ID == 7 || row.ID == 30 {
			return SummonAllPlan{}, errors.New("Summon All source owns a nonTranscendent Ancient")
		}
		owned[row.ID] = true
	}
	plan := SummonAllPlan{Missing: []Level{}}
	total := 0
	for _, def := range definitions {
		if def.ID == 3 || def.ID == 6 || def.ID == 7 || def.ID == 30 {
			continue
		}
		total++
		if !owned[def.ID] {
			plan.Missing = append(plan.Missing, Level{ID: def.ID, Name: def.Name, Level: "1"})
		}
	}
	if total != 26 {
		return SummonAllPlan{}, errors.New("Summon All catalog does not match verified client roster")
	}
	cost := 0
	for _, price := range summonAncientPrices6144[len(owned):total] {
		cost += price
	}
	plan.Cost = strconv.Itoa(cost)
	wallet, err := exactPrestigeNumber(json.Number(state.HeroSouls), "Hero Souls wallet")
	if err != nil {
		return SummonAllPlan{}, err
	}
	plan.Eligible = len(plan.Missing) > 0 && wallet.Cmp(big.NewRat(int64(cost), 1)) >= 0
	return plan, ctx.Err()
}

// VerifySummonAllReceipt proves the reserved immediate debit and every new level,
// retaining existing levels while ordinary combat may advance zones.
func VerifySummonAllReceipt(ctx context.Context, before, after TranscensionState, reserved SummonAllPlan) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReceiptPair(ctx, before, after); err != nil {
		return err
	}
	plan, err := PlanSummonAll(ctx, before)
	if err != nil {
		return err
	}
	if !plan.Eligible || !reserved.Eligible || reserved.Cost != plan.Cost || !equalStateLevels(reserved.Missing, plan.Missing) {
		return errors.New("Summon All receipt does not match reserved client purchase")
	}
	if before.CurrentZone == nil || after.CurrentZone == nil || before.Transcensions != after.Transcensions || before.Ascensions != after.Ascensions || before.AscensionsThisTranscension != after.AscensionsThisTranscension || before.Transcendent != after.Transcendent || after.HighestZone < before.HighestZone || before.AncientSouls != after.AncientSouls || before.AncientSoulsTotal != after.AncientSoulsTotal || !equalStateLevels(before.Outsiders, after.Outsiders) {
		return errors.New("Summon All receipt contains unrelated state changes")
	}
	wallet, _ := exactPrestigeNumber(json.Number(before.HeroSouls), "Hero Souls wallet")
	cost, _ := exactPrestigeNumber(json.Number(plan.Cost), "Summon All price")
	remaining, _ := exactPrestigeNumber(json.Number(after.HeroSouls), "Hero Souls wallet")
	if new(big.Rat).Sub(wallet, cost).Cmp(remaining) != 0 {
		return errors.New("Summon All receipt wallet does not match exact debit")
	}
	expected := append(append([]Level(nil), before.Ancients...), plan.Missing...)
	if !equalStateLevels(expected, after.Ancients) {
		return errors.New("Summon All receipt roster or initial level mismatch")
	}
	return ctx.Err()
}
