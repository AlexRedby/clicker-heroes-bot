package bot

import (
	"context"
	"fmt"
	"io"
	"math/big"

	"clicker-heroes-bot/internal/ancientcalc"
	"clicker-heroes-bot/internal/timelapse"
)

type timelapsePreview struct {
	PreviewOnly bool                       `json:"previewOnly"`
	State       ancientcalc.TimelapseState `json:"state"`
	Hybrid      ancientcalc.BuildPlan      `json:"hybrid"`
	HybridError string                     `json:"hybridError,omitempty"`
	Forecast    timelapse.Forecast         `json:"forecast"`
	Gilds       *ancientcalc.GildPlan      `json:"gilds,omitempty"`
	GildError   string                     `json:"gildError,omitempty"`
	Blocked     []string                   `json:"blocked"`
}

// This report never acquires game input, allocates a spending allowance or
// treats the reference's ideal hero/gild assumptions as observed preparation.
func previewTimelapse(ctx context.Context, savePath, output string, stdout io.Writer, options ancientcalc.BuildOptions) error {
	exported, err := readPreviewSave(savePath)
	if err != nil {
		return err
	}
	preview, err := calculateTimelapsePreview(ctx, exported, options)
	if err != nil {
		return err
	}
	return writeReadOnlyPreview(ctx, preview, output, stdout, []string{savePath})
}

func calculateTimelapsePreview(ctx context.Context, exported []byte, options ancientcalc.BuildOptions) (timelapsePreview, error) {
	p := timelapsePreview{PreviewOnly: true, Blocked: []string{
		"native Shop offers, confirmation, cancellation and outcome are not verified",
		"current hero, upgrades and gilds must match a prepared Hybrid forecast",
		"a free Auto Clicker and native Nogardnit behavior require shared-frame verification",
		"paid automation requires an explicit finite persisted ruby allowance",
	}}
	var err error
	p.State, err = ancientcalc.ReadTimelapseState(ctx, exported)
	if err != nil {
		return p, err
	}
	if p.State.Zone > 2147000000 {
		return p, fmt.Errorf("unsupported Timelapse starting zone")
	}
	outsider := func(id string) (int, error) {
		raw, exists := p.State.Outsiders[id]
		if !exists {
			return 0, nil
		}
		n, err := ancientcalc.Value(raw)
		if err != nil {
			return 0, err
		}
		level, accuracy := n.Int64()
		if accuracy != big.Exact || level < 0 || level > 2147000000 {
			return 0, fmt.Errorf("unsupported Timelapse Outsider level")
		}
		return int(level), nil
	}
	xyl, err := outsider("1")
	if err != nil {
		return p, err
	}
	chor, err := outsider("2")
	if err != nil {
		return p, err
	}
	p.Forecast, err = timelapse.Calculate(ctx, timelapse.Input{StartZone: int(p.State.Zone), LogHeroSouls: p.State.LogAscensionStartSouls, Xyliqil: xyl, Chorgorloth: chor, AutoClickers: int(p.State.AutoClickers)})
	if err != nil {
		return p, err
	}
	p.Hybrid, err = ancientcalc.CalculateBuild(ctx, exported, options)
	if err != nil {
		// Missing Siyalatas still leaves a useful report of acquisition needs.
		if len(p.Hybrid.SummoningRequired) == 0 || err.Error() != "Siyalatas must be owned" {
			return p, err
		}
		p.HybridError = err.Error()
	}
	for _, missing := range p.Hybrid.SummoningRequired {
		p.Blocked = append(p.Blocked, "summon "+missing.Name+"; summoning is not automated")
	}
	if len(p.Hybrid.Plan.Rows) > 0 {
		p.Blocked = append(p.Blocked, "Hybrid Ancient purchases are projected, not applied")
	}
	gilds, err := ancientcalc.CalculateGilds(ctx, exported, options.Reserve, nil)
	if err != nil {
		p.GildError = err.Error()
	} else {
		p.Gilds = &gilds
	}
	if err := ctx.Err(); err != nil {
		return p, err
	}
	return p, nil
}
