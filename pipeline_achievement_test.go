package main

import (
	"context"
	"encoding/base64"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/achievementgoal"
	"clicker-heroes-bot/internal/ancientcalc"
)

func TestPipelineAchievementGoalRefreshAndCompletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goals.json")
	if err := os.WriteFile(path, []byte(`{"config":{"goals":[140,126]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	goals, err := loadAchievementSession(path)
	if err != nil {
		t.Fatal(err)
	}
	p := newGamePipeline(&pauseControl{}, heroInput{}, pipelineReaders{}, pipelineOptions{achievements: goals, mercenaries: true, export: &saveExportOptions{dir: t.TempDir()}})
	p.frame = testPipelineFrame()
	p.frame.context.heroes = true
	p.layout = p.frame.layout
	if !p.export.goalsOnly || !p.export.requested || goals.priority.GoalID != 0 {
		t.Fatal("startup didn't request a fresh goal-only snapshot")
	}
	save := ancientcalc.AchievementState{ProfileID: strings.Repeat("a", 64), Build: ancientcalc.SupportedAchievementBuild, OwnershipKnown: true, Counters: map[string]ancientcalc.AchievementCounter{"total5MinuteQuests": {Known: true, Value: 249}, "rubyQuestsCompleted": {Known: true, Value: 10}}}
	accept := func(readErr error) {
		t.Helper()
		p.export.active, p.export.step, p.export.jobFrame, p.export.window = true, exportReadFile, p.frame.id, "game"
		if err := p.accept(context.Background(), observation{kind: exportAnalysis, frame: p.frame, export: exportResult{achievements: &save, achievementErr: readErr}}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	accept(nil)
	p.plan(time.Now())
	if goals.priority.GoalID != 140 || p.mercenary.questPriority.GoalID != 140 || goals.dirty || p.export.requested || p.ancient.plan != nil {
		t.Fatal("goal snapshot installed an Ancient batch or lost priority")
	}
	p.actionCompleted(actionResult{action: gameAction{kind: handleMercenary, frame: p.frame, mercenary: mercenaryCommand{step: claimAndOpenMercenaryQuest}}, acted: true}, time.Now())
	p.mercenary.active = true
	p.planAchievementRefresh()
	if p.export.requested || !goals.dirty {
		t.Fatal("export interrupted an unfinished roster visit")
	}
	p.mercenary.active, p.mercenary.pending = false, nil
	p.settleUntil = time.Time{} // The modeled visit has returned after input settling.
	p.frame.id++
	p.planAchievementRefresh()
	if !p.export.requested || !p.export.goalsOnly || goals.priority != (achievementgoal.QuestPriority{}) {
		t.Fatal("finished collection did not request one refresh and revoke stale priority")
	}
	save.Counters["total5MinuteQuests"] = ancientcalc.AchievementCounter{Known: true, Value: 250}
	accept(nil)
	p.plan(time.Now())
	if goals.priority.GoalID != 126 || p.mercenary.questPriority.GoalID != 126 || p.export.requested {
		t.Fatalf("completed goal didn't advance: priority=%+v planner=%+v export=%+v", goals.priority, p.mercenary.questPriority, p.export)
	}
	restarted, err := loadAchievementSession(path)
	if err != nil || len(restarted.state.Completed) != 1 || restarted.state.Completed[0] != 140 {
		t.Fatalf("completion didn't survive restart: %+v %v", restarted, err)
	}
	goals.dirty = true
	p.planAchievementRefresh()
	save.ProfileID = strings.Repeat("b", 64)
	accept(nil)
	p.plan(time.Now())
	if goals.priority.GoalID != 0 || p.mercenary.questPriority.GoalID != 0 || p.export.requested {
		t.Fatal("foreign profile kept a priority or retried exports forever")
	}
	save.ProfileID = strings.Repeat("a", 64)
	save.Counters["rubyQuestsCompleted"] = ancientcalc.AchievementCounter{Known: true, Value: 500}
	if err := goals.acceptSnapshot(&save, nil); err != nil {
		t.Fatal(err)
	}
	p.actionCompleted(actionResult{action: gameAction{kind: handleMercenary, frame: p.frame, mercenary: mercenaryCommand{step: claimMercenaryReward}}, acted: true}, time.Now())
	p.mercenary.pending, p.settleUntil = nil, time.Time{}
	p.plan(time.Now())
	if goals.dirty || goals.priority.GoalID != 0 || p.export.requested || p.mercenary.questPriority.GoalID != 0 {
		t.Fatal("completed goals caused more exports or kept a preference")
	}
	for _, opts := range []pipelineOptions{{achievements: goals, fishInterval: time.Second}, {achievements: goals, mercenaries: true, fishInterval: time.Second}, {achievements: goals, export: &saveExportOptions{dir: "exports"}, fishInterval: time.Second}} {
		if _, err := configureRun(opts); err == nil {
			t.Fatal("goal run accepted missing Mercenaries or export directory")
		}
	}
}

func TestAchievementOnlyExportSkipsAncientAndRelicPlanning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clickerHeroSave.txt")
	payload := `{"uniqueId":"synthetic-goal-export","readPatchNumber":"1.0e12-6144","achievements":{},"total5MinuteQuests":15}`
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString([]byte(payload))), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := readExport(exportJob{ctx: ctx, options: saveExportOptions{dir: dir, reserve: "invalid"}, before: make(exportSnapshot), goalsOnly: true, achievements: true})
	if err != nil || result.achievements == nil || result.achievementErr != nil || result.plan != nil || result.relics != nil || result.prestige != nil {
		t.Fatalf("goal export touched unrelated plans: %+v %v", result, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("owned generated export was not cleaned up")
	}
}

func TestAchievementPriorityRevokedOnResetAndWindowChange(t *testing.T) {
	for _, test := range []struct {
		name                 string
		generation           uint64
		window               string
		ownedExport, revoked bool
	}{
		{name: "F8 generation", generation: 1, revoked: true},
		{name: "same generation reset"},
		{name: "another game window", window: "another-game", revoked: true},
		{name: "owned Explorer focus", window: "!outside-game", ownedExport: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			goals, save := newAchievementSessionFixture(t)
			if err := goals.acceptSnapshot(&save, nil); err != nil {
				t.Fatal(err)
			}
			frame := testPipelineFrame()
			frame.context.heroes = true
			p := newGamePipeline(&pauseControl{}, heroInput{capture: func() (image.Image, error) { return frame.image, nil }}, pipelineReaders{context: func(image.Image) (gameContext, error) { return frame.context, nil }, window: func() string { return test.window }}, pipelineOptions{achievements: goals, mercenaries: true, export: &saveExportOptions{dir: t.TempDir()}, fishInterval: time.Second})
			p.frame, p.layout = frame, frame.layout
			p.export.requested = false
			p.mercenary.questPriority = goals.priority
			p.queue[handleMercenary] = gameAction{kind: handleMercenary, mercenary: mercenaryCommand{step: selectMercenaryQuest}}
			if test.window == "" {
				p.reset(test.generation)
			} else {
				if test.ownedExport {
					p.export.active, p.export.step, p.export.window = true, exportRestoreGame, "game"
				}
				if err := p.capture(context.Background(), time.Now(), mercenaryRecoveryJobs()); err != nil {
					t.Fatal(err)
				}
			}
			if test.revoked {
				if goals.priority.GoalID != 0 || p.mercenary.questPriority.GoalID != 0 || !goals.dirty {
					t.Fatal("context change restored a stale preference")
				}
				if _, ok := p.queue[handleMercenary]; ok {
					t.Fatal("stale goal choice remained queued")
				}
				save.ProfileID = strings.Repeat("b", 64)
				if err := goals.acceptSnapshot(&save, nil); err == nil || goals.priority.GoalID != 0 {
					t.Fatal("foreign profile reactivated old goal")
				}
			} else if goals.priority.GoalID != 113 || goals.dirty {
				t.Fatal("owned reset/focus handoff revoked a valid snapshot")
			}
		})
	}
}
