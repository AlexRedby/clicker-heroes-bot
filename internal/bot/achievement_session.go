package bot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"clicker-heroes-bot/internal/achievementgoal"
	"clicker-heroes-bot/internal/ancientcalc"
)

type achievementSession struct {
	path     string
	state    achievementgoal.State
	priority achievementgoal.QuestPriority
	dirty    bool
}

func loadAchievementSession(path string) (*achievementSession, error) {
	if path == "" {
		return nil, nil
	}
	state, err := readAchievementGoals(path)
	if err != nil {
		return nil, err
	}
	return &achievementSession{path: path, state: state, dirty: true}, nil
}

// Runtime only: previews must never call this writer or grant a preference.
// The pipeline must also revoke any queued goal choice when priority is cleared.
func (s *achievementSession) acceptSnapshot(save *ancientcalc.AchievementState, readErr error) error {
	if s == nil {
		return nil
	}
	s.priority, s.dirty = achievementgoal.QuestPriority{}, true
	if readErr != nil {
		return readErr
	}
	if save == nil {
		return errors.New("achievement snapshot unavailable")
	}
	next := s.state
	next.Config.Goals = append([]int(nil), s.state.Config.Goals...)
	next.Completed = append([]int(nil), s.state.Completed...)
	report, err := next.Evaluate(*save)
	if err != nil {
		return err
	}
	if err := writeAchievementState(s.path, next); err != nil {
		return err
	}
	s.state, s.priority, s.dirty = next, report.QuestPriority, false
	return nil
}

// Replace only the explicitly loaded regular goal file, using an adjacent
// synced tempfile. Do not replace a symlink or silently recreate a removed file.
func writeAchievementState(path string, state achievementgoal.State) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("achievement state path must remain a regular file")
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".achievement-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
