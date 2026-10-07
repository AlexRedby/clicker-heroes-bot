package bot

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"clicker-heroes-bot/internal/achievementgoal"
	"clicker-heroes-bot/internal/ancientcalc"
)

func newAchievementSessionFixture(t *testing.T) (*achievementSession, ancientcalc.AchievementState) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goals.json")
	if err := os.WriteFile(path, []byte(`{"config":{"goals":[113,138]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := loadAchievementSession(path)
	if err != nil {
		t.Fatal(err)
	}
	save := ancientcalc.AchievementState{ProfileID: strings.Repeat("a", 64), Build: ancientcalc.SupportedAchievementBuild, OwnershipKnown: true, Earned: map[int]bool{}, Counters: map[string]ancientcalc.AchievementCounter{
		"goldQuestsCompleted": {Known: true, Value: 4}, "total5MinuteQuests": {Known: true, Value: 9},
	}}
	return s, save
}

func TestAchievementSessionPersistsBeforePreferenceAndRestarts(t *testing.T) {
	if s, err := loadAchievementSession(""); err != nil || s != nil {
		t.Fatalf("default run changed: %+v %v", s, err)
	}
	s, save := newAchievementSessionFixture(t)
	if !s.dirty || s.priority != (achievementgoal.QuestPriority{}) {
		t.Fatal("load granted stale priority")
	}
	checkPersisted := func() {
		t.Helper()
		stored, err := readAchievementGoals(s.path)
		if err != nil || !reflect.DeepEqual(stored, s.state) {
			t.Fatalf("state not persisted: %+v %+v %v", stored, s.state, err)
		}
		info, err := os.Stat(s.path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("mode %o", info.Mode().Perm())
		}
		files, err := os.ReadDir(filepath.Dir(s.path))
		if err != nil || len(files) != 1 {
			t.Fatalf("temporary state leaked: %v %v", files, err)
		}
	}
	if err := s.acceptSnapshot(&save, nil); err != nil {
		t.Fatal(err)
	}
	if s.dirty || s.priority.Reward != "gold" || s.state.ProfileID != save.ProfileID {
		t.Fatalf("fresh preference: %+v", s)
	}
	checkPersisted()
	save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{Known: true, Value: 5}
	if err := s.acceptSnapshot(&save, nil); err != nil {
		t.Fatal(err)
	}
	if !s.priority.FiveMinutes || !reflect.DeepEqual(s.state.Completed, []int{113}) {
		t.Fatalf("completion: %+v", s)
	}
	checkPersisted()
	var err error
	s, err = loadAchievementSession(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.dirty || s.priority != (achievementgoal.QuestPriority{}) {
		t.Fatal("restart reused a stale preference")
	}
	save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{Known: true, Value: 0}
	if err := s.acceptSnapshot(&save, nil); err != nil || !s.priority.FiveMinutes {
		t.Fatalf("persisted completion lost: %+v %v", s, err)
	}
	save.Counters["total5MinuteQuests"] = ancientcalc.AchievementCounter{Known: true, Value: 10}
	if err := s.acceptSnapshot(&save, nil); err != nil || s.dirty || s.priority != (achievementgoal.QuestPriority{}) || len(s.state.Completed) != 2 {
		t.Fatalf("default not restored: %+v %v", s, err)
	}
	checkPersisted()
}

func TestAchievementSessionFailuresRevokePriorityAndPreserveState(t *testing.T) {
	for _, failure := range []string{"read error", "missing snapshot", "profile mismatch", "invalid state", "failed write", "symlink"} {
		t.Run(failure, func(t *testing.T) {
			s, save := newAchievementSessionFixture(t)
			if err := s.acceptSnapshot(&save, nil); err != nil {
				t.Fatal(err)
			}
			originalPath := s.path
			original, err := os.ReadFile(originalPath)
			if err != nil {
				t.Fatal(err)
			}
			var readErr error
			snapshot := &save
			backing := make([]int, 8)
			backing[0] = 138
			s.state.Completed = backing[:1]
			if failure == "read error" {
				readErr = errors.New("export failed")
			}
			if failure == "missing snapshot" {
				snapshot = nil
			}
			if failure == "profile mismatch" {
				save.ProfileID = strings.Repeat("b", 64)
			}
			if failure == "invalid state" {
				s.state.Config.Goals = []int{113, 113}
			}
			if failure == "failed write" {
				s.path = filepath.Join(t.TempDir(), "missing-parent", "goals.json")
			}
			if failure == "symlink" {
				s.path = filepath.Join(t.TempDir(), "goal-link.json")
				if err := os.Symlink(originalPath, s.path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{Known: true, Value: 5}
			before, _ := json.Marshal(s.state)
			if err := s.acceptSnapshot(snapshot, readErr); err == nil {
				t.Fatal("failure accepted")
			}
			after, _ := json.Marshal(s.state)
			if !s.dirty || s.priority != (achievementgoal.QuestPriority{}) || !bytes.Equal(before, after) || backing[1] != 0 {
				t.Fatalf("failed snapshot changed state/priority: %+v", s)
			}
			actual, err := os.ReadFile(originalPath)
			if err != nil || !bytes.Equal(original, actual) {
				t.Fatalf("failure changed original goal file: %v", err)
			}
		})
	}
	if _, err := loadAchievementSession(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("missing configured goal file accepted")
	}
}
