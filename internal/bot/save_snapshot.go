package bot

import (
	"errors"
	"fmt"
	"time"
)

// Accepted save facts survive UI navigation; mutations invalidate only their consumers.
type saveSnapshot struct {
	result                   exportResult
	wallZone, highestZone    int
	rewardDirty, relicsDirty bool
	relicAlertHandled        bool
}

func (s *saveSnapshot) accept(result exportResult, wall, highest int) {
	handled := s.relicAlertHandled
	if s.result.relics != nil && result.relics != nil && !relicSameItems(s.result.relics.Snapshot, result.relics.Snapshot) {
		handled = false
	}
	*s = saveSnapshot{result: result, wallZone: wall, highestZone: highest, relicAlertHandled: handled}
}

func (s *saveSnapshot) ascensionCurrent(wall int) bool {
	return s.result.ascension != nil && s.result.ascensionErr == nil && !s.rewardDirty && s.wallZone == wall
}

func (s *saveSnapshot) relicsCurrent() bool {
	return s.result.relics != nil && s.result.relicErr == nil && !s.relicsDirty
}

func (p *gamePipeline) exportReason() string {
	if p.export.reason != "" {
		return p.export.reason
	}
	switch {
	case p.export.initialSetup:
		return "initial game state"
	case p.export.prestigeOnly:
		return "prestige assessment or transaction receipt"
	case p.export.gildsOnly:
		return "changed gild distribution, target or transfer receipt"
	case p.export.goalsOnly:
		return "collected achievement quest batch"
	case p.export.relicsOnly && p.relic.verifyNeeded || p.relic.step == relicVerifyEquip:
		return "relic equipment receipt"
	case p.export.relicsOnly:
		return "changed relic inventory or Ascension preflight"
	default:
		return "Hero Souls allocation after startup or reset"
	}
}

func (p *gamePipeline) reuseRelicSave(now time.Time) bool {
	if !p.save.relicsCurrent() {
		return false
	}
	fmt.Println("save snapshot: reusing relic inventory")
	if err := p.acceptRelicSave(p.save.result.relics, now); err != nil {
		p.save.relicsDirty = true
		p.relicFailed(err.Error(), now)
	}
	return true
}

func ascensionSaveError(out exportResult) error {
	if out.ascensionErr != nil {
		return out.ascensionErr
	}
	if out.ascension == nil {
		return errors.New("missing Ascension facts")
	}
	return errors.New("missing invested Hero Souls ledger")
}
