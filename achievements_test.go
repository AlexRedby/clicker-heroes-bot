package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"clicker-heroes-bot/internal/ancientcalc"
)

func TestAchievementPreviewCompletionAndInputProtection(t *testing.T) {
	dir := t.TempDir()
	savePath, goalsPath := filepath.Join(dir, "save.txt"), filepath.Join(dir, "goals.json")
	goals := []byte(`{"config":{"goals":[140,126]}}`)
	if err := os.WriteFile(goalsPath, goals, 0600); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"uniqueId": "synthetic-achievement-preview", "readPatchNumber": ancientcalc.SupportedAchievementBuild, "achievements": map[string]bool{}, "total5MinuteQuests": 249, "rubyQuestsCompleted": 15}
	writeSave := func() []byte {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		exported := []byte(base64.StdEncoding.EncodeToString(raw))
		if err := os.WriteFile(savePath, exported, 0600); err != nil {
			t.Fatal(err)
		}
		return exported
	}
	exported := writeSave()
	evaluate := func(goalPath string) achievementPreview {
		t.Helper()
		var stdout bytes.Buffer
		if err := previewAchievements(context.Background(), savePath, goalPath, "", &stdout); err != nil {
			t.Fatal(err)
		}
		var preview achievementPreview
		if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if !preview.PreviewOnly || len(preview.NextState.ProfileID) != 64 || bytes.Contains(stdout.Bytes(), []byte("synthetic-achievement-preview")) {
			t.Fatal("invalid preview or exposed profile")
		}
		return preview
	}
	preview := evaluate(goalsPath)
	if preview.Report.QuestPriority.GoalID != 140 || !preview.Report.QuestPriority.FiveMinutes || preview.Report.Goals[1].Status != "queued" {
		t.Fatalf("wrong preference: %+v", preview.Report)
	}
	for _, path := range []string{savePath, goalsPath} {
		if err := previewAchievements(context.Background(), savePath, goalsPath, path, nil); err == nil {
			t.Fatalf("allowed overwriting %s", path)
		}
	}
	for path, expected := range map[string][]byte{savePath: exported, goalsPath: goals} {
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(expected, actual) {
			t.Fatalf("changed input %s", path)
		}
	}
	payload["total5MinuteQuests"] = 250
	writeSave()
	preview = evaluate(goalsPath)
	if preview.Report.QuestPriority.GoalID != 126 || len(preview.NextState.Completed) != 1 || preview.NextState.Completed[0] != 140 {
		t.Fatalf("did not advance: %+v", preview)
	}
	persisted, err := json.Marshal(preview.NextState)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goalsPath, persisted, 0600); err != nil {
		t.Fatal(err)
	}
	payload["total5MinuteQuests"] = 0
	writeSave()
	if restored := evaluate(goalsPath); restored.Report.Goals[0].Status != "complete" || restored.Report.QuestPriority.GoalID != 126 {
		t.Fatalf("completion lost: %+v", restored)
	}
	payload["uniqueId"] = "another-profile"
	writeSave()
	if err := previewAchievements(context.Background(), savePath, goalsPath, "", &bytes.Buffer{}); err == nil {
		t.Fatal("accepted another profile")
	}
	if all := evaluate(""); len(all.Report.Goals) != 170 {
		t.Fatal("default catalog missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := previewAchievements(ctx, savePath, "", "", &bytes.Buffer{}); err != context.Canceled {
		t.Fatalf("cancellation lost: %v", err)
	}
	if err := os.WriteFile(goalsPath, bytes.Repeat([]byte(" "), 32769), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readAchievementGoals(goalsPath); err == nil {
		t.Fatal("oversized goal input accepted")
	}
}
