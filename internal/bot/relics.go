package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"clicker-heroes-bot/internal/ancientcalc"
)

func previewRelics(ctx context.Context, savePath string, output io.Writer) error {
	if savePath == "" {
		return errors.New("relics-plan requires -save")
	}
	file, err := os.Open(savePath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > ancientcalc.MaxSaveInput {
		return errors.New("save must be a regular file of at most 4 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, ancientcalc.MaxSaveInput+1))
	if err != nil {
		return err
	}
	preview, err := ancientcalc.PreviewRelics(ctx, data)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(preview)
}

var relicBonusNames = map[int]string{
	1:  "idle DPS",
	2:  "click damage",
	3:  "boss timer",
	4:  "Clickstorm duration",
	5:  "double-ruby chance",
	6:  "starting zone",
	7:  "gilded damage",
	8:  "Metal Detector duration",
	9:  "Golden Clicks duration",
	10: "Lucky Strikes duration",
	11: "Powersurge duration",
	12: "Super Clicks duration",
	13: "boss life reduction",
	14: "Hero Soul DPS",
	15: "critical click damage",
	16: "treasure chest chance",
	17: "Primal Boss chance",
	18: "10x Gold chance",
	19: "hero cost reduction",
	20: "Golden Clicks gold",
	21: "treasure chest gold",
	22: "gold dropped",
	24: "idle gold",
	25: "Primal Hero Souls",
	26: "click combo DPS",
	27: "monsters per zone reduction",
	28: "skill cooldown reduction",
}

func relicReport(preview *ancientcalc.RelicPreview, err error) string {
	if err != nil {
		return fmt.Sprintf("relics: unknown/unreadable inventory: %v; manual review required", err)
	}
	if preview == nil {
		return "relics: unknown/unreadable save; manual review required"
	}
	if len(preview.UnsupportedTypes) > 0 {
		return fmt.Sprintf("relics: manual review required; unsupported bonus types %v; no automatic action", preview.UnsupportedTypes)
	}
	if preview.Suggestion == nil {
		return "relics: no clear upgrade; " + preview.Reason
	}

	suggestion := preview.Suggestion
	var bonuses []string
	for _, item := range preview.Snapshot.Items {
		if item.UID != suggestion.UID {
			continue
		}
		for _, bonus := range item.Bonuses {
			name := relicBonusNames[bonus.Type]
			if name == "" {
				name = fmt.Sprintf("bonus type %d", bonus.Type)
			}
			bonuses = append(bonuses, fmt.Sprintf("%s bonus level=%s", name, bonus.Level))
		}
		break
	}
	bonusText := ""
	if len(bonuses) > 0 {
		bonusText = "; bonuses: " + strings.Join(bonuses, ", ")
	}
	replacement := "empty slot"
	if suggestion.ReplaceUID != 0 {
		replacement = fmt.Sprintf("replace UID %d", suggestion.ReplaceUID)
	}
	return fmt.Sprintf("relics: manual suggestion: equip UID %d in slot %d (%s)%s; apply manually", suggestion.UID, suggestion.Slot, replacement, bonusText)
}
