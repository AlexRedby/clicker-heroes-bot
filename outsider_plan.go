package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
)

type outsiderCheck struct {
	Name          string `json:"name"`
	Level         int    `json:"level"`
	DisplayedCost int    `json:"displayedCost"`
	ExpectedCost  *int   `json:"expectedCost,omitempty"`
}

// A partial UI check cannot establish the hidden roster or authorize input.
type outsiderAdvice struct {
	ReadOnly      bool                       `json:"readOnly"`
	Status        string                     `json:"status"`
	Reason        string                     `json:"reason"`
	SaveHash      string                     `json:"saveHash"`
	Wallet        int                        `json:"observedWallet"`
	Reward        int                        `json:"observedReward"`
	Power         float64                    `json:"observedTPPercent"`
	Quantity      string                     `json:"quantity"`
	SaveReward    *int                       `json:"saveEstimatedReward,omitempty"`
	RewardMatches *bool                      `json:"rewardMatchesSave,omitempty"`
	Checks        []outsiderCheck            `json:"visibleChecks"`
	Plans         *ancientcalc.OutsiderPlans `json:"plans,omitempty"`
}

func reconcileOutsiders(ctx context.Context, ui outsiderObservation, source *ancientcalc.TranscensionPreview) outsiderAdvice {
	out := outsiderAdvice{ReadOnly: true, Status: "unavailable", Wallet: ui.wallet, Reward: ui.gain, Power: ui.power, Quantity: ui.quantity, Checks: []outsiderCheck{}}
	if source != nil {
		out.SaveHash, out.SaveReward = source.SaveHash, source.EstimatedASGain
	}
	if !ui.known {
		out.Reason = "Outsiders UI is unreadable"
		return out
	}
	if source == nil {
		out.Reason = "no save roster; use run -save or the existing -export-dir flow"
		return out
	}
	if source.SaveVersion != 7 || !strings.HasPrefix(source.Build, "1.0e12-") || source.TPBeforePercent == nil || source.Outsiders == nil {
		out.Reason = "save mechanics or Outsider accounting are unavailable"
		return out
	}
	if source.Outsiders.Status != "ok" {
		out.Status, out.Reason = "unsupported", "save allocation is outside the supported AS model"
		return out
	}
	if ui.wallet != source.AncientSouls {
		out.Reason = "UI wallet differs from the save; refresh the roster before planning"
		return out
	}
	// The two-decimal display can round or truncate. Compatibility at display
	// precision is advisory; it does not prove an exact AS total or freshness.
	tp := *source.TPBeforePercent
	if math.IsNaN(tp) || math.IsInf(tp, 0) || (ui.power != math.Round(tp*100)/100 && ui.power != math.Floor(tp*100)/100) {
		out.Reason = "displayed TP differs from the save"
		return out
	}
	quantity := map[string]int{"x1": 1, "x10": 10, "x100": 100, "x1000": 1000}[ui.quantity]
	if quantity == 0 && ui.quantity != "MAX" {
		out.Reason = "selected FEED quantity is unknown"
		return out
	}
	owned := make([]ancientcalc.Level, 0, len(source.Outsiders.Additions))
	for _, row := range source.Outsiders.Additions {
		owned = append(owned, ancientcalc.Level{ID: row.ID, Name: row.Name, Level: strconv.Itoa(row.Current)})
	}
	plans, err := ancientcalc.PlanOutsiders(ctx, source.AncientSoulsTotal, ui.wallet, ui.gain, owned)
	if err != nil {
		out.Reason = err.Error()
		return out
	}
	seen := make(map[string]bool)
	for _, row := range ui.rows {
		var saved *ancientcalc.OutsiderTarget
		for i := range source.Outsiders.Additions {
			candidate := &source.Outsiders.Additions[i]
			if candidate.Name == row.name {
				saved = candidate
				break
			}
		}
		if saved == nil || seen[row.name] || row.level != saved.Current || row.cost <= 0 {
			out.Reason = "visible Outsider levels differ from the save or are ambiguous"
			return out
		}
		seen[row.name] = true
		check := outsiderCheck{Name: row.name, Level: row.level, DisplayedCost: row.cost}
		if quantity > 0 {
			cost, err := ancientcalc.OutsiderFeedCost(saved.ID, row.level, quantity)
			if err != nil || row.cost != cost {
				out.Reason = "displayed FEED cost differs from the calculator for " + row.name
				return out
			}
			check.ExpectedCost = &cost
		}
		out.Checks = append(out.Checks, check)
	}
	if len(out.Checks) == 0 {
		out.Reason = "no visible Outsider rows were checked"
		return out
	}
	if out.SaveReward != nil {
		matches := *out.SaveReward == ui.gain
		out.RewardMatches = &matches
	}
	out.Status = "partial"
	out.Reason = fmt.Sprintf("%d/9 visible rows checked; hidden levels come from the save; informational only", len(out.Checks))
	if quantity == 0 {
		out.Reason += "; MAX cost is not verified"
	} else if len(out.Checks) == 9 {
		out.Status = "matched"
		out.Reason = "all visible levels/costs match the save; reset/respec/recovery remain unverified"
	}
	if plans.Now.Status != "ok" || plans.AfterReward.Status != "ok" {
		out.Status, out.Reason = "unsupported", "current or projected total AS exceeds the allocation model"
	}
	out.Plans = &plans
	return out
}

func outsiderAllocationReport(label string, plan ancientcalc.OutsiderAllocation) string {
	if plan.Status != "ok" {
		return label + ": allocation unavailable (" + plan.Status + ")"
	}
	rows := []string{}
	for _, row := range plan.Additions {
		if row.Target > row.Current {
			rows = append(rows, fmt.Sprintf("%s %d -> %d (%d AS)", row.Name, row.Current, row.Target, row.Cost))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, "no affordable additions")
	}
	return fmt.Sprintf("%s: %s; spent=%d remaining=%d", label, strings.Join(rows, ", "), plan.Spent, plan.Remaining)
}

func (o outsiderAdvice) String() string {
	parts := []string{"Outsider plan: " + o.Status + "; " + o.Reason}
	if o.Plans != nil {
		parts = append(parts, outsiderAllocationReport("current wallet", o.Plans.Now), outsiderAllocationReport(fmt.Sprintf("after displayed +%d AS (conditional)", o.Reward), o.Plans.AfterReward))
		if o.Plans.AfterReward.RequiresRespec {
			parts = append(parts, "ideal allocation requires respec; additions above preserve owned levels")
		}
	}
	if o.RewardMatches != nil && !*o.RewardMatches {
		parts = append(parts, fmt.Sprintf("reward differs: save estimate +%d, UI +%d; projection uses the UI reward", *o.SaveReward, o.Reward))
	}
	return strings.Join(parts, "\n")
}
