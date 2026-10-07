package achievementgoal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

func snapshot() ancientcalc.AchievementState {
	return ancientcalc.AchievementState{ProfileID: strings.Repeat("a", 64), Build: "1.0e12-6144", OwnershipKnown: true, Earned: map[int]bool{}, Counters: map[string]ancientcalc.AchievementCounter{
		"goldQuestsCompleted": {Known: true, Value: 4}, "total5MinuteQuests": {Known: true, Value: 9},
		"totalClicks": {Known: true, Value: 0}, "totalBossKills": {Known: true, Value: 0},
		"totalHeroLevels": {Known: true, Value: 0}, "highestFinishedZone": {Known: true, Value: 0},
	}}
}

func TestOrderedGoalCompletionAndRestart(t *testing.T) {
	s, err := ParseState([]byte(`{"config":{"goals":[113,138]}}`))
	if err != nil {
		t.Fatal(err)
	}
	save := snapshot()
	r, err := s.Evaluate(save)
	if err != nil || r.QuestPriority.Reward != "gold" || r.Goals[0].Status != "active" || r.Goals[0].Current != 4 || r.Goals[1].Status != "queued" {
		t.Fatalf("ordered: %+v %v", r, err)
	}
	save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{Known: true, Value: 5}
	r, err = s.Evaluate(save)
	if err != nil || r.Goals[0].Status != "complete" || r.Goals[0].Earned || !r.QuestPriority.FiveMinutes {
		t.Fatalf("counter completion: %+v %v", r, err)
	}
	save.Earned[138] = true // Earned achievement is authoritative even with a missing counter.
	delete(save.Counters, "total5MinuteQuests")
	r, err = s.Evaluate(save)
	if err != nil || r.QuestPriority != (QuestPriority{}) || r.Goals[1].Status != "complete" || !r.Goals[1].Earned {
		t.Fatalf("restore default: %+v %v", r, err)
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	s, err = ParseState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Evaluate(snapshot()) // Lower/current counters cannot undo persisted completion.
	if err != nil || r.QuestPriority != (QuestPriority{}) || len(s.Completed) != 2 || r.Goals[0].Status != "complete" || r.Goals[1].Status != "complete" {
		t.Fatalf("restart: %+v %v", r, err)
	}
	before, _ := json.Marshal(s)
	other := snapshot()
	other.ProfileID = strings.Repeat("b", 64)
	if r, err = s.Evaluate(other); err == nil || r.QuestPriority != (QuestPriority{}) {
		t.Fatalf("wrong profile accepted: %+v %v", r, err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("profile mismatch mutated state")
	}
}

func TestUnsupportedAndNativeGates(t *testing.T) {
	save := snapshot()
	s := State{Config: Config{Goals: []int{999, 9, 113, 138, 5, 21, 29, 1}}}
	save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{Reason: "missing counter"}
	r, err := s.Evaluate(save)
	if err != nil || r.QuestPriority.GoalID != 138 {
		t.Fatalf("supported fallback: %+v %v", r, err)
	}
	for i, reason := range []string{"unknown internal achievement ID", "unsupported counter: totalGold", "missing counter"} {
		if r.Goals[i].Status != "unsupported" || r.Goals[i].Reason != reason || r.Goals[i].CounterKnown {
			t.Fatalf("unsupported %d: %+v", i, r.Goals[i])
		}
	}
	for _, p := range r.Goals[4:] {
		if p.Status != "awaiting_native" || !p.CounterKnown || p.Current != 0 || p.Reason == "" {
			t.Fatalf("native gate: %+v", p)
		}
	}
	s.Config.Goals = []int{113}
	save.Counters["goldQuestsCompleted"] = ancientcalc.AchievementCounter{}
	r, err = s.Evaluate(save)
	if err != nil || r.Goals[0].Reason != "unknown counter" || r.QuestPriority != (QuestPriority{}) {
		t.Fatalf("unknown is not zero: %+v %v", r, err)
	}
	s.Config.Goals = nil
	r, err = s.Evaluate(save)
	if err != nil || len(r.Goals) != 0 || r.QuestPriority != (QuestPriority{}) {
		t.Fatalf("no goal: %+v %v", r, err)
	}
}

func TestQuestPriorityAndCatalog(t *testing.T) {
	catalog := Catalog()
	if len(catalog) != 170 {
		t.Fatal("catalog incomplete")
	}
	seen, steam := map[int]bool{}, 0
	for _, d := range catalog {
		if seen[d.ID] || d.ID <= 0 || d.Name == "" || d.Counter == "" || d.Required == "" {
			t.Fatalf("invalid catalog: %+v", d)
		}
		seen[d.ID] = true
		if d.Live != (d.ID != 112 && d.ID != 143) {
			t.Fatalf("source live flag changed: %+v", d)
		}
		if d.SteamName != "" {
			steam++
		}
		if d.ID == 2 && (d.Required != "25" || d.SteamName != "HIGHEST_ZONE_20") {
			t.Fatal("Steam label replaced internal requirement")
		}
		if d.ID >= 113 && d.ID <= 142 {
			p := questPriority(d)
			if p.GoalID != d.ID {
				t.Fatalf("missing quest priority: %+v", d)
			}
			if p.FiveMinutes {
				if !p.Matches("gold", 5*time.Minute) || p.Matches("gold", 15*time.Minute) || p.Matches("recruitment", 5*time.Minute) || p.Matches("unknown", 5*time.Minute) {
					t.Fatalf("five minutes: %+v", p)
				}
			} else {
				if !p.Matches(p.Reward, 4*time.Hour) || p.Matches(p.Reward, 0) || p.Matches(p.Reward, 49*time.Hour) || p.Matches("unknown", 4*time.Hour) {
					t.Fatalf("reward type: %+v", p)
				}
			}
		}
	}
	if steam != 111 {
		t.Fatalf("Steam mapping count %d", steam)
	}
	if (QuestPriority{}).Matches("rubies", 4*time.Hour) {
		t.Fatal("empty priority changes ordinary policy")
	}
}

func TestInactiveAndUnsupportedBuildCannotActivate(t *testing.T) {
	save := snapshot()
	save.Counters["total5MinuteQuests"] = ancientcalc.AchievementCounter{Known: true, Value: 25000}
	s := State{Config: Config{Goals: []int{112, 143}}}
	r, err := s.Evaluate(save)
	if err != nil || r.QuestPriority != (QuestPriority{}) || len(s.Completed) != 0 {
		t.Fatalf("inactive goal: %+v %v", r, err)
	}
	for _, p := range r.Goals {
		if p.Status != "unsupported" || p.Reason != "inactive achievement in installed-client catalog" {
			t.Fatalf("inactive: %+v", p)
		}
	}
	s = State{Config: Config{Goals: []int{113, 138}}}
	save.Build = "unknown-client"
	delete(save.Counters, "goldQuestsCompleted") // Build takes precedence over missing data.
	r, err = s.Evaluate(save)
	if err != nil || r.QuestPriority != (QuestPriority{}) || len(s.Completed) != 0 {
		t.Fatalf("unknown build: %+v %v", r, err)
	}
	for _, p := range r.Goals {
		if p.CounterKnown || p.Status != "unsupported" || p.Reason != "unsupported save build" {
			t.Fatalf("unknown build: %+v", p)
		}
	}
	save.Earned[113] = true
	r, err = s.Evaluate(save)
	if err != nil || r.Goals[0].Status != "complete" || r.QuestPriority != (QuestPriority{}) {
		t.Fatalf("earned is retained: %+v %v", r, err)
	}
}

func TestStateRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{}`, `{"config":{"goals":[0]}}`, `{"config":{"goals":[113,113]}}`, `{"config":{"goals":[-1]}}`, `{"unknown":1}`, `{"config":{"unknown":1}}`, `{"completed":[113]}`, `{"profileId":"bad"}`, `{} {}`, `{"config":{"goals":[113.5]}}`} {
		_, err := ParseState([]byte(input))
		if input == `{}` {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("invalid state accepted: %s", input)
		}
	}
	save := snapshot()
	save.ProfileID = ""
	s := State{Config: Config{Goals: []int{113}}}
	if r, err := s.Evaluate(save); err == nil || r.QuestPriority != (QuestPriority{}) {
		t.Fatal("unknown identity activates goal")
	}
}
