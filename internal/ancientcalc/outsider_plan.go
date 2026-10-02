package ancientcalc

import (
	"context"
	"errors"
	"strconv"
)

const maxOutsiderInteger int64 = 9007199254740991

// OutsiderPlans contains allocations before and after a projected reward.
type OutsiderPlans struct {
	Now         OutsiderAllocation `json:"now"`
	AfterReward OutsiderAllocation `json:"afterReward"`
}

func supportedOutsiderInt(v int) bool {
	x := int64(v)
	return x >= 0 && x <= maxOutsiderInteger && int64(int(x)) == x
}

func addSupportedOutsiderInts(a, b int) (int, error) {
	if !supportedOutsiderInt(a) || !supportedOutsiderInt(b) {
		return 0, errors.New("invalid outsider budget")
	}
	s := int64(a) + int64(b)
	if s > maxOutsiderInteger || int64(int(s)) != s {
		return 0, errors.New("outsider budget exceeds supported integer range")
	}
	return int(s), nil
}

func parseOutsiderLevel(raw string) (int, error) {
	if raw == "" {
		return 0, errors.New("invalid outsider level")
	}
	x, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || x < 0 || x > maxOutsiderInteger || int64(int(x)) != x || strconv.FormatInt(x, 10) != raw {
		return 0, errors.New("outsider level exceeds supported integer range")
	}
	return int(x), nil
}

// PlanOutsiders validates the exact AS ledger and computes both allocations.
func PlanOutsiders(ctx context.Context, total, wallet, gain int, owned []Level) (OutsiderPlans, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return OutsiderPlans{}, err
	}
	if !supportedOutsiderInt(total) || !supportedOutsiderInt(wallet) || !supportedOutsiderInt(gain) {
		return OutsiderPlans{}, errors.New("invalid outsider budget")
	}
	if len(owned) != len(outsiderIDs) {
		return OutsiderPlans{}, errors.New("outsider roster must contain exactly nine entries")
	}
	var current [9]int
	seen := make(map[int]bool, len(outsiderIDs))
	spent := int64(0)
	for _, row := range owned {
		index := -1
		for i, id := range outsiderIDs {
			if row.ID == id {
				index = i
				break
			}
		}
		if index < 0 || seen[row.ID] {
			return OutsiderPlans{}, errors.New("invalid or duplicate outsider roster")
		}
		if row.Name != "" && row.Name != outsiderNames[index] {
			return OutsiderPlans{}, errors.New("outsider name does not match id")
		}
		level, err := parseOutsiderLevel(row.Level)
		if err != nil {
			return OutsiderPlans{}, err
		}
		cost := outsiderCost(index, level)
		if cost < 0 || spent > int64(maxOutsiderInteger)-int64(cost) {
			return OutsiderPlans{}, errors.New("outsider ledger exceeds supported integer range")
		}
		seen[row.ID], current[index], spent = true, level, spent+int64(cost)
	}
	if int64(wallet) > int64(maxOutsiderInteger)-spent || spent+int64(wallet) != int64(total) {
		return OutsiderPlans{}, errors.New("outsider AS ledger does not match total")
	}
	projectedTotal, err := addSupportedOutsiderInts(total, gain)
	if err != nil {
		return OutsiderPlans{}, err
	}
	projectedWallet, err := addSupportedOutsiderInts(wallet, gain)
	if err != nil {
		return OutsiderPlans{}, err
	}
	now, err := allocateOutsiders(ctx, total, wallet, current)
	if err != nil {
		return OutsiderPlans{}, err
	}
	after, err := allocateOutsiders(ctx, projectedTotal, projectedWallet, current)
	if err != nil {
		return OutsiderPlans{}, err
	}
	return OutsiderPlans{Now: now, AfterReward: after}, nil
}

// OutsiderFeedCost returns the incremental cost of feeding quantity levels.
func OutsiderFeedCost(id, level, quantity int) (int, error) {
	if !supportedOutsiderInt(level) || !supportedOutsiderInt(quantity) || quantity <= 0 {
		return 0, errors.New("invalid outsider feed range")
	}
	index := -1
	for i, outsiderID := range outsiderIDs {
		if id == outsiderID {
			index = i
			break
		}
	}
	if index < 0 {
		return 0, errors.New("unknown outsider id")
	}
	target, err := addSupportedOutsiderInts(level, quantity)
	if err != nil {
		return 0, err
	}
	from, to := outsiderCost(index, level), outsiderCost(index, target)
	if from < 0 || to < 0 || to < from {
		return 0, errors.New("outsider feed cost exceeds supported range")
	}
	delta := int64(to) - int64(from)
	if delta > maxOutsiderInteger {
		return 0, errors.New("outsider feed cost exceeds supported range")
	}
	return int(delta), nil
}
