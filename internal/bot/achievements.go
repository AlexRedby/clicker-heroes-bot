package bot

import (
	"context"
	"errors"
	"io"
	"os"

	"clicker-heroes-bot/internal/achievementgoal"
	"clicker-heroes-bot/internal/ancientcalc"
)

type achievementPreview struct {
	PreviewOnly bool                   `json:"previewOnly"`
	Report      achievementgoal.Report `json:"report"`
	NextState   achievementgoal.State  `json:"nextState"`
}

// Without a goal file, report the whole catalog. A preview never changes the
// goal file or installs the reported quest preference in the running game.
func previewAchievements(ctx context.Context, savePath, goalPath, output string, stdout io.Writer) error {
	exported, err := readPreviewSave(savePath)
	if err != nil {
		return err
	}
	save, err := ancientcalc.ReadAchievementState(ctx, exported)
	if err != nil {
		return err
	}
	state := achievementgoal.State{}
	inputs := []string{savePath}
	if goalPath == "" {
		for _, goal := range achievementgoal.Catalog() {
			state.Config.Goals = append(state.Config.Goals, goal.ID)
		}
	} else {
		state, err = readAchievementGoals(goalPath)
		if err != nil {
			return err
		}
		inputs = append(inputs, goalPath)
	}
	report, err := state.Evaluate(save)
	if err != nil {
		return err
	}
	return writeReadOnlyPreview(ctx, achievementPreview{PreviewOnly: true, Report: report, NextState: state}, output, stdout, inputs)
}

func readAchievementGoals(path string) (achievementgoal.State, error) {
	file, err := os.Open(path)
	if err != nil {
		return achievementgoal.State{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return achievementgoal.State{}, err
	}
	const limit = 32 * 1024
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > limit {
		return achievementgoal.State{}, errors.New("achievement state must be a nonempty regular file no larger than 32 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return achievementgoal.State{}, err
	}
	return achievementgoal.ParseState(data)
}

// Reuse an already requested export. Otherwise refresh once after collection,
// when the roster visit has returned to Heroes and no other transaction owns UI.
func (p *gamePipeline) planAchievementRefresh() {
	goals := p.options.achievements
	if goals == nil || !goals.dirty || p.export.requested || p.startup != noStartup || p.startupCheck || !p.frame.context.heroes || !p.frame.context.known {
		return
	}
	if p.mercenary.active || p.mercenary.pending != nil || p.ascension.active || p.relic.active || p.gild.active || p.ancient.active || p.ancient.plan != nil && !p.ancient.finished {
		return
	}
	if p.hero.pending != nil || p.skill.pending != nil || p.progression.pending != nil || p.clickers.pending != nil {
		return
	}
	p.invalidateAchievementGoals()
	p.export.requested, p.export.goalsOnly, p.export.relicsOnly = true, true, false
}

func (p *gamePipeline) invalidateAchievementGoals() {
	if goals := p.options.achievements; goals != nil {
		_ = goals.acceptSnapshot(nil, errInputContext)
		p.mercenary.questPriority = goals.priority
		delete(p.queue, handleMercenary)
	}
}
