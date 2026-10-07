// Package achievementgoal projects explicit goals without performing input or
// granting permission to spend rubies, reset a run, or lose a mercenary.
package achievementgoal

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"
)

// Internal requirements and separate Steam names from official Unity TextAssets
// 89 and 100, build 6144_new2 (SHA256 b587e1f5a8e4355abc40b39e3738b87badf03f6e96dad4f16ab739e1707149f3):
// https://cdn.clickerheroes.com/gamebuild/builds/6144_new2/Build/6144.data.unityweb
// Reward metadata is deliberately absent: legacy fields do not prove rewards.
//
//go:embed catalog.json
var catalogJSON []byte

type Definition struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Counter   string `json:"counter"`
	Required  string `json:"required"`
	SteamName string `json:"steamName,omitempty"`
	Live      bool   `json:"live"` // _live:0 in the source excludes an inactive achievement.
}

// Catalog returns internal IDs; Steam names are labels, never requirements.
func Catalog() []Definition {
	var out []Definition
	if err := json.Unmarshal(catalogJSON, &out); err != nil {
		panic(err)
	}
	return out
}

type Config struct {
	// Order resolves competing priorities. Unknown IDs remain reportable.
	Goals []int `json:"goals"`
}

func (c Config) Validate() error {
	if len(c.Goals) > 170 {
		return errors.New("at most 170 achievement goals may be configured")
	}
	seen := make(map[int]bool)
	for _, id := range c.Goals {
		if id <= 0 || seen[id] {
			return errors.New("achievement goals must be unique positive internal IDs")
		}
		seen[id] = true
	}
	return nil
}

// State is JSON-serializable. The caller must persist it after Evaluate and
// load it at restart, including Config. Completion belongs to one save profile.
type State struct {
	ProfileID string `json:"profileId"`
	Config    Config `json:"config"`
	Completed []int  `json:"completed"`
}

// ParseState bounds local state/config input and rejects trailing/unknown data.
// For an initial configuration use {"config":{"goals":[117,142]}}.
func ParseState(input []byte) (State, error) {
	var decoded *State
	if len(input) == 0 || len(input) > 32*1024 {
		return State{}, errors.New("achievement state must be non-empty and at most 32 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(&decoded); err != nil {
		return State{}, err
	}
	if decoded == nil {
		return State{}, errors.New("achievement state must be a JSON object")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return State{}, errors.New("trailing achievement state data")
	}
	state := *decoded
	if err := state.validate(); err != nil {
		return State{}, err
	}
	return state, nil
}

func (s State) validate() error {
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if s.ProfileID != "" {
		if len(s.ProfileID) != 64 {
			return errors.New("invalid achievement profile hash")
		}
		for _, r := range s.ProfileID {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return errors.New("invalid achievement profile hash")
			}
		}
	}
	if len(s.Completed) > 170 || len(s.Completed) > 0 && s.ProfileID == "" {
		return errors.New("completed goals require a bound profile")
	}
	seen := make(map[int]bool)
	for _, id := range s.Completed {
		if id <= 0 || id > 170 || seen[id] {
			return errors.New("invalid completed achievement IDs")
		}
		seen[id] = true
	}
	return nil
}

type Progress struct {
	Definition
	Current      int64  `json:"current"`
	CounterKnown bool   `json:"counterKnown"`
	Earned       bool   `json:"earned"`
	Status       string `json:"status"` // complete, active, queued, unsupported, awaiting_native
	Reason       string `json:"reason,omitempty"`
}

type QuestPriority struct {
	GoalID      int    `json:"goalId,omitempty"`
	Reward      string `json:"reward,omitempty"` // Existing controller reward labels.
	FiveMinutes bool   `json:"fiveMinutes,omitempty"`
}

// Matches is only a preference among already recognized, valid offers. The
// controller retains free recruitment first and its existing ranking fallback.
func (p QuestPriority) Matches(reward string, duration time.Duration) bool {
	return p.GoalID > 0 && duration >= 5*time.Minute && duration <= 48*time.Hour &&
		(p.Reward != "" && reward == p.Reward || p.FiveMinutes && duration == 5*time.Minute &&
			(reward == "gold" || reward == "rubies" || reward == "relics" || reward == "skills" || reward == "hero souls"))
}

type Report struct {
	Build           string        `json:"build"`
	OwnershipKnown  bool          `json:"ownershipKnown"`
	OwnershipReason string        `json:"ownershipReason,omitempty"`
	Goals           []Progress    `json:"goals"`
	QuestPriority   QuestPriority `json:"questPriority"`
}

// Evaluate uses one fresh snapshot. Missing/unknown counters never activate a
// priority. A complete goal stays complete after an Ascension or process restart.
func (s *State) Evaluate(save ancientcalc.AchievementState) (Report, error) {
	out := Report{Build: save.Build, OwnershipKnown: save.OwnershipKnown, OwnershipReason: save.OwnershipReason, Goals: []Progress{}}
	if s == nil {
		return out, errors.New("missing achievement state")
	}
	if err := s.validate(); err != nil {
		return out, err
	}
	identity := State{ProfileID: save.ProfileID}
	if save.ProfileID == "" || identity.validate() != nil {
		return out, errors.New("missing or invalid achievement save profile")
	}
	if s.ProfileID != "" && s.ProfileID != save.ProfileID {
		return out, errors.New("achievement state belongs to another profile")
	}
	s.ProfileID = save.ProfileID
	definitions := make(map[int]Definition)
	for _, d := range Catalog() {
		definitions[d.ID] = d
	}
	completed := make(map[int]bool)
	for _, id := range s.Completed {
		completed[id] = true
	}
	for _, id := range s.Config.Goals {
		d, found := definitions[id]
		p := Progress{Definition: d, Status: "unsupported"}
		p.ID = id
		if !found {
			p.Reason = "unknown internal achievement ID"
			out.Goals = append(out.Goals, p)
			continue
		}
		counter, exists := save.Counters[d.Counter]
		if save.Build != ancientcalc.SupportedAchievementBuild {
			counter.Known, counter.Reason = false, "unsupported save build"
		} else if counter.Known && counter.Value < 0 {
			counter.Known, counter.Reason = false, "malformed counter"
		}
		p.Current, p.CounterKnown = counter.Value, counter.Known
		p.Earned = save.OwnershipKnown && save.Earned[id]
		if !d.Live {
			p.Reason = "inactive achievement in installed-client catalog"
			out.Goals = append(out.Goals, p)
			continue
		}
		target, err := strconv.ParseInt(d.Required, 10, 64)
		if completed[id] || p.Earned || err == nil && counter.Known && counter.Value >= target {
			p.Status = "complete"
			if !completed[id] {
				s.Completed = append(s.Completed, id)
				completed[id] = true
			}
		} else if save.Build != ancientcalc.SupportedAchievementBuild {
			p.Reason = "unsupported save build"
		} else if !exists {
			p.Reason = "unsupported counter: " + d.Counter
		} else if !counter.Known {
			p.Reason = counter.Reason
			if p.Reason == "" {
				p.Reason = "unknown counter"
			}
		} else {
			priority := questPriority(d)
			if priority.GoalID == 0 {
				p.Status, p.Reason = "awaiting_native", strategyGate(d.Counter)
			} else if out.QuestPriority.GoalID != 0 {
				p.Status, p.Reason = "queued", fmt.Sprintf("waiting for goal %d", out.QuestPriority.GoalID)
			} else {
				p.Status, out.QuestPriority = "active", priority
			}
		}
		out.Goals = append(out.Goals, p)
	}
	return out, nil
}

func strategyGate(counter string) string {
	switch counter {
	case "totalClicks":
		return "requires verified bounded monster input and owned Auto Clicker placement"
	case "totalBossKills":
		return "requires verified boss selection, repeat kills and return to progression"
	case "totalHeroLevels":
		return "requires verified early-hero level purchases and restoration of latest-hero progression"
	case "highestFinishedZone", "transcendentHighestFinishedZone":
		return "requires verified bounded zone/progression controls; no reset solely for a goal"
	default:
		return "execution strategy requires native control evidence"
	}
}

func questPriority(d Definition) QuestPriority {
	p := QuestPriority{GoalID: d.ID}
	switch d.Counter {
	case "goldQuestsCompleted":
		p.Reward = "gold"
	case "relicQuestsCompleted":
		p.Reward = "relics"
	case "rubyQuestsCompleted":
		p.Reward = "rubies"
	case "skillQuestsCompleted":
		p.Reward = "skills"
	case "heroSoulQuestsCompleted":
		p.Reward = "hero souls"
	case "total5MinuteQuests":
		p.FiveMinutes = true
	default:
		return QuestPriority{}
	}
	return p
}
