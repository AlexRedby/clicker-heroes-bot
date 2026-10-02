package ancientcalc

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

type Level struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Level string `json:"level"`
}
type Purchase struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Current  string `json:"current"`
	Target   string `json:"target"`
	Quantity string `json:"quantity"`
	Cost     string `json:"cost"`
}
type Plan struct {
	Souls      string     `json:"souls"`
	Invested   string     `json:"invested,omitempty"`
	Reserve    string     `json:"reserve"`
	Spent      string     `json:"spent"`
	Remaining  string     `json:"remaining"`
	Ascensions int        `json:"ascensions"`
	Owned      []Level    `json:"owned"`
	Rows       []Purchase `json:"rows"`
}

var ancientDecimal = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,5})?$`)

// Value parses a finite, nonnegative decimal amount without using OCR log10 values.
func Value(s string) (*big.Float, error) {
	if len(s) > 10000 || !ancientDecimal.MatchString(s) {
		return nil, errors.New("invalid Ancient quantity")
	}
	n, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil || n.IsInf() || n.Sign() < 0 {
		return nil, errors.New("invalid Ancient quantity")
	}
	return n, nil
}

// Validate checks plan identities, amounts and reserve limits.
func (p Plan) Validate() error {
	if p.Ascensions < 0 || len(p.Owned) == 0 || len(p.Owned) > 100 || len(p.Rows) > len(p.Owned) {
		return errors.New("invalid Ancient plan roster")
	}
	totals := make([]*big.Float, 4)
	for i, s := range []string{p.Souls, p.Reserve, p.Spent, p.Remaining} {
		v, err := Value(s)
		if err != nil {
			return err
		}
		totals[i] = v
	}
	if p.Invested != "" {
		if _, err := Value(p.Invested); err != nil {
			return err
		}
	}
	if totals[2].Cmp(totals[0]) > 0 || totals[3].Cmp(totals[1]) < 0 {
		return errors.New("Ancient plan exceeds soul budget or reserve")
	}
	owned := map[int]Level{}
	for _, a := range p.Owned {
		if a.ID <= 0 || a.Name == "" || len(a.Name) > 60 || strings.ContainsAny(a.Name, "\r\n") {
			return errors.New("invalid Ancient identity")
		}
		level, err := Value(a.Level)
		if err != nil || level.Sign() <= 0 {
			return errors.New("invalid owned Ancient level")
		}
		if _, exists := owned[a.ID]; exists {
			return errors.New("duplicate Ancient identity")
		}
		owned[a.ID] = a
	}
	seen := map[int]bool{}
	for _, a := range p.Rows {
		old, ok := owned[a.ID]
		if !ok || old.Name != a.Name || old.Level != a.Current || seen[a.ID] {
			return errors.New("Ancient plan targets an unknown or duplicate Ancient")
		}
		seen[a.ID] = true
		current, err := Value(a.Current)
		if err != nil {
			return err
		}
		target, err := Value(a.Target)
		if err != nil {
			return err
		}
		quantity, err := Value(a.Quantity)
		if err != nil {
			return err
		}
		cost, err := Value(a.Cost)
		if err != nil {
			return err
		}
		if quantity.Sign() <= 0 || cost.Sign() < 0 || target.Cmp(current) <= 0 {
			return errors.New("Ancient purchase must increase an owned level")
		}
	}
	return nil
}

// InputQuantity rounds down to fit the visible text field without increasing
// calculator spending. The omitted digits are below the game's displayed precision.
func InputQuantity(quantity string) (string, error) {
	if _, err := Value(quantity); err != nil {
		return "", err
	}
	rat, ok := new(big.Rat).SetString(quantity)
	if !ok {
		return "", errors.New("invalid Ancient purchase quantity")
	}
	integer := new(big.Int).Quo(rat.Num(), rat.Denom())
	if integer.Sign() <= 0 {
		return "", errors.New("Ancient quantity is below one level")
	}
	digits := integer.String()
	if len(digits) <= 15 {
		return digits, nil
	}
	return digits[:1] + "." + digits[1:15] + fmt.Sprint("e", len(digits)-1), nil
}
