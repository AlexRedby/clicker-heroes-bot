package bot

import "math"

type mercenaryRecoveryTraits struct {
	Known            bool
	Level            int
	RubyBonusPercent float64
	ExtraLives       int
}

type mercenaryRecoveryBudget struct {
	Known        bool
	Cost, Rubies int64
}

type mercenaryRecoveryMethod uint8

const (
	mercenaryRecoverySkip mercenaryRecoveryMethod = iota
	mercenaryRecoveryInspect
	mercenaryRecoveryRubies
	mercenaryRecoveryExtraLife
	mercenaryRecoveryBury
	mercenaryRecoveryWait
)

type mercenaryRecoveryDecision struct {
	method mercenaryRecoveryMethod
	reason string
}

// These approximate ranges assume frequent quest checks and the bot's ruby-first
// quest ranking. They are not a prediction of the mercenary's hidden lifetime.
// Source: https://ch.jehare.nl/guide/ (Should I revive my mercenary that died?).
func chooseMercenaryRecovery(t mercenaryRecoveryTraits, b mercenaryRecoveryBudget) mercenaryRecoveryDecision {
	if !t.Known || t.Level < 1 || math.IsNaN(t.RubyBonusPercent) || math.IsInf(t.RubyBonusPercent, 0) ||
		t.RubyBonusPercent < 0 || t.RubyBonusPercent > 2000 || t.ExtraLives < 0 || t.ExtraLives > 6 {
		return mercenaryRecoveryDecision{mercenaryRecoverySkip, "dead mercenary traits are unreadable"}
	}
	minimum, maximum := 2, 8
	if t.RubyBonusPercent > 0 {
		for _, band := range []struct {
			bonus float64
			min   int
			max   int
		}{{10, 2, 9}, {20, 1, 10}, {50, 1, 11}, {200, 1, 13}, {500, 1, 15}, {2000, 1, 19}} {
			if t.RubyBonusPercent <= band.bonus {
				minimum, maximum = band.min, band.max
				break
			}
		}
	}
	if t.ExtraLives > 0 {
		minimum = 1
		maximum = max(maximum, [...]int{0, 10, 12, 13, 14, 15, 16}[t.ExtraLives])
	}
	if t.Level < minimum || t.Level > maximum {
		if t.ExtraLives > 0 {
			return mercenaryRecoveryDecision{mercenaryRecoveryExtraLife, "use a remaining life beyond the paid revive range"}
		}
		return mercenaryRecoveryDecision{mercenaryRecoveryBury, "free replacement is preferred to a paid revive"}
	}
	if !b.Known {
		return mercenaryRecoveryDecision{mercenaryRecoveryInspect, "read the displayed revive price before spending"}
	}
	if b.Cost <= 0 || b.Rubies < 0 {
		return mercenaryRecoveryDecision{mercenaryRecoverySkip, "revive price or ruby balance is unreadable"}
	}
	if b.Cost <= b.Rubies {
		return mercenaryRecoveryDecision{mercenaryRecoveryRubies, "affordable revive within the paid level range"}
	}
	if t.ExtraLives > 0 {
		return mercenaryRecoveryDecision{mercenaryRecoveryExtraLife, "use a remaining life when the paid revive is unaffordable"}
	}
	return mercenaryRecoveryDecision{mercenaryRecoveryWait, "keep this mercenary until its worthwhile revive is affordable"}
}
