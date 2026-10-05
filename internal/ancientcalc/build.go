package ancientcalc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/ALTree/bigfloat"
)

// BuildMode selects the allocation model, independently of native automation.
type BuildMode string

const (
	ActiveBuild BuildMode = "active"
	HybridBuild BuildMode = "hybrid"
)

// BuildOptions controls allocation. An empty mode uses Active; an empty reserve
// uses zero. SkillRate is explicit (zero excludes skill Ancients). HybridRatio
// is Fragsworth/Siyalatas, defaults to the reference's 0.5, and must be positive.
type BuildOptions struct {
	Mode        BuildMode
	Reserve     string
	SkillRate   float64
	Beyond8k    bool
	HybridRatio string
}

type AncientRequirement struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// BuildPlan describes owned-Ancient purchases, not readiness or authorization for
// Timelapse. SaveHash identifies the supplied bytes; acquisition must establish
// freshness. Outsiders and HighestZone come from that same save. Summoning is a
// separate UI dependency: no missing Ancient is added to Plan.Rows. Projected
// levels use Current + Quantity (the entered amount), not the Target ceiling.
// Hybrid needs
// a free Auto Clicker for Nogardnit; its assignment cannot be established here.
type BuildPlan struct {
	Plan                           Plan                 `json:"plan"`
	Mode                           BuildMode            `json:"mode"`
	HybridRatio                    string               `json:"hybridRatio,omitempty"`
	SaveHash                       string               `json:"saveHash"`
	HighestZone                    string               `json:"highestZone"`
	AncientSoulsTotal              string               `json:"ancientSoulsTotal"`
	Outsiders                      []Level              `json:"outsiders"`
	SummoningRequired              []AncientRequirement `json:"summoningRequired"`
	RequiredUnassignedAutoClickers int                  `json:"requiredUnassignedAutoClickers"`
}

// CalculateBuild is the explicit-mode API for preparation consumers. Calculate
// remains the existing Active API. The models are reference rules of thumb,
// with native guarded prices and the retained-Soul/Morgulis benefit comparison.
// If the base Ancient is absent, metadata includes the summoning dependency but
// the returned error means no purchase plan is available.
func CalculateBuild(ctx context.Context, exported []byte, options BuildOptions) (BuildPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	out := BuildPlan{Mode: options.Mode, Outsiders: []Level{}, SummoningRequired: []AncientRequirement{}}
	if out.Mode == "" {
		out.Mode = ActiveBuild
	}
	if out.Mode != ActiveBuild && out.Mode != HybridBuild {
		return out, fmt.Errorf("unsupported Ancient build %q", out.Mode)
	}
	var ratio *big.Float
	if out.Mode == HybridBuild {
		if options.HybridRatio == "" {
			options.HybridRatio = "0.5"
		}
		var err error
		ratio, err = aInput(options.HybridRatio, "Fragsworth/Siyalatas ratio", false)
		if err != nil || ratio.Sign() <= 0 {
			return out, fmt.Errorf("Hybrid ratio must be a positive decimal")
		}
		out.HybridRatio = options.HybridRatio
		out.RequiredUnassignedAutoClickers = 1
	} else if options.HybridRatio != "" {
		return out, fmt.Errorf("Hybrid ratio requires the Hybrid build")
	}
	if options.Reserve == "" {
		options.Reserve = "0"
	}
	save, err := decodeAncientSave(ctx, exported)
	if err != nil {
		return out, err
	}
	digest := sha256.Sum256(exported)
	out.SaveHash = hex.EncodeToString(digest[:])
	out.HighestZone, out.AncientSoulsTotal = string(save.HighestFinishedZonePersist), string(save.AncientSoulsTotal)
	required := []AncientRequirement{{19, "Fragsworth"}}
	if out.Mode == HybridBuild {
		required = []AncientRequirement{{5, "Siyalatas"}, {4, "Libertas"}, {32, "Nogardnit"}, {19, "Fragsworth"}}
	}
	for _, a := range required {
		entry, exists := save.Ancients.Ancients[strconv.Itoa(a.ID)]
		level, levelErr := Value(string(entry.Level))
		if !exists || levelErr == nil && level.Sign() == 0 {
			out.SummoningRequired = append(out.SummoningRequired, a)
		}
	}
	out.Plan, err = planAncientsBuild(ctx, save, options.Reserve, options.SkillRate, options.Beyond8k, out.Mode, ratio)
	if err != nil {
		return out, err
	}
	for i, id := range outsiderIDs {
		level := "0"
		if entry, exists := save.Outsiders.Outsiders[strconv.Itoa(id)]; exists {
			level = string(entry.Level)
		}
		out.Outsiders = append(out.Outsiders, Level{id, outsiderNames[i], level})
	}
	return out, nil
}

// Hybrid goals use Siyalatas as the base, from model.js at the pinned reference
// commit documented in calculator.go. Xyliqil affects idle benefit in the game,
// not the reference's target-level ratios; it is exported for the TL forecast.
func hybridAncientGoal(name string, base, old, alpha, rate, ratio *big.Float, beyond8k bool) *big.Float {
	active := aMul(base, ratio)
	shared := aMax(base, active)
	switch name {
	case "Siyalatas":
		return base
	case "Libertas":
		return ancientGoal("Mammon", base, old, alpha, rate, beyond8k)
	case "Nogardnit":
		return bigfloat.Pow(ancientGoal("Mammon", shared, old, alpha, rate, beyond8k), aConst("0.8"))
	case "Fragsworth", "Bhaal", "Juggernaut":
		return ancientGoal(name, active, old, alpha, rate, beyond8k)
	default:
		return ancientGoal(name, shared, old, alpha, rate, beyond8k)
	}
}
