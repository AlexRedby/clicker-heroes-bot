package bot

import (
	"fmt"
	"image"
	"time"

	"clicker-heroes-bot/internal/vision"
)

type navigationStep uint8

const (
	navigationFocus navigationStep = iota
	navigationSettings
	navigationQuantity
	navigationAscension
	navigationJunk
	navigationQuest
	navigationRecovery
	navigationGildChest
	navigationGildReward
	navigationGildRoster
	navigationHeroes
)

func (s navigationStep) String() string {
	return [...]string{"restore game focus", "close Settings", "cancel Ancient quantity", "cancel Ascension", "cancel junk salvage", "close quest selection", "cancel Mercenary recovery", "open earned gild chest", "close gild reward", "close gild roster", "return to Heroes"}[s]
}

type gameNavigation struct {
	window, message string
	nextAction      time.Time
}

func (n *gameNavigation) report(message string) {
	if message != n.message {
		fmt.Println("navigation:", message)
		n.message = message
	}
}

func (a gameAction) restoresFocus() bool {
	return a.kind == handleExport && a.export.step == exportRestoreGame || a.kind == navigateGame && a.navigation == navigationFocus
}

// Recovery uses only verified free close/cancel controls, never confirmation.
func navigationControl(f gameFrame, step navigationStep) (image.Point, bool, error) {
	c := f.context
	var region image.Rectangle
	switch step {
	case navigationSettings:
		if c.saveMenu {
			return saveControl(f.image, 1)
		}
	case navigationQuantity:
		if c.ancientDialog {
			region = image.Rect(863, 225, 887, 249)
		}
	case navigationAscension:
		if c.ascension {
			return ascensionActionPoint(f.image, cancelAscension)
		}
	case navigationJunk:
		if c.relicJunk {
			return relicJunkControl(f.image, false)
		}
	case navigationQuest:
		if c.questDialog {
			return mercenaryPoint(c.bounds, 811, 53), true, nil
		}
	case navigationRecovery:
		if c.mercenaryDialog {
			return mercenaryPoint(c.bounds, 594, 548), true, nil
		}
	case navigationGildChest:
		if c.modal == gildChestModal {
			return gildActionPoint(f)
		}
	case navigationGildReward:
		if c.modal == gildRewardModal {
			region = image.Rect(982, 116, 1006, 140)
		}
	case navigationGildRoster:
		if c.modal == gildRosterModal {
			region = image.Rect(1137, 27, 1161, 51)
		}
	case navigationHeroes:
		if c.known && !c.heroes && !c.saveMenu && !c.ancientDialog && !c.ascension && !c.relicJunk && !c.questDialog && !c.mercenaryDialog && c.modal == noGildModal {
			return ancientTabPoint(f.image, true), true, nil
		}
	}
	if region.Empty() {
		return image.Point{}, false, nil
	}
	found, err := vision.MatchControl(f.image, region, "gilds/close.png")
	r := vision.Rect(f.image, region)
	return r.Min.Add(r.Size().Div(2)), found, err
}

func (p *gamePipeline) navigationStable(a gameAction) bool {
	if a.frame.generation != p.generation || a.frame.layout != p.layout || a.frame.context != p.frame.context {
		return false
	}
	if a.navigation == navigationFocus {
		return p.frame.context.window == "!outside-game" && p.navigation.window != "" && p.input.focus != nil
	}
	point, found, err := navigationControl(p.frame, a.navigation)
	return err == nil && found && point == a.point
}

func (p *gamePipeline) planNavigation(now time.Time) (recovering bool) {
	defer func() {
		if !recovering {
			delete(p.queue, navigateGame)
		}
	}()
	c := p.frame.context
	n := &p.navigation
	if p.frame.id == 0 {
		return true
	}
	if !c.heroes {
		p.startupDeadline = time.Time{}
	}
	if p.gild.active && c.known && c.modal == noGildModal && (!p.gild.deadline.IsZero() && now.After(p.gild.deadline) || p.gild.attempts >= 3) {
		p.gild = gildCollector{nextCheck: now.Add(time.Minute)}
	}
	step := navigationHeroes
	needed := true
	switch {
	case !c.known:
		if p.export.active && (p.export.step == exportRestoreGame || p.export.step == exportCloseMenu) {
			return false
		}
		if c.window != "!outside-game" || n.window == "" || p.input.focus == nil {
			if n.message == "" {
				n.report("screen not recognized; waiting for a recognizable game frame")
			}
			return true
		}
		step = navigationFocus
	case c.saveMenu:
		if p.options.export != nil && p.export.requested && (p.startup == startupSave || !p.startupCheck && p.startup == noStartup || p.export.active) {
			return false
		}
		step = navigationSettings
	case c.ancientDialog:
		if p.ancient.active && !p.ancient.blocked && (p.ancient.deadline.IsZero() || now.Before(p.ancient.deadline)) {
			return false
		}
		if p.ancient.active {
			p.ancient.fail("quantity dialog transition timed out; cancelling without purchase acknowledgement")
		}
		step = navigationQuantity
	case c.ascension || c.relicJunk:
		if c.relics && p.relic.active && !p.relic.failed {
			if !p.relic.failed && now.Before(p.relic.deadline) && (p.relic.step == relicOpenSalvage || p.relic.step == relicConfirmSalvage || p.relic.step == relicWaitSalvage) {
				return false
			}
			p.relicFailed("junk salvage transition interrupted; cancelling", now)
		}
		if p.ascension.active && (p.ascension.deadline.IsZero() || now.Before(p.ascension.deadline)) {
			return false
		}
		if p.ascension.active {
			p.ascension.interrupt()
			p.ascension.nextCheck = now.Add(time.Minute)
		}
		step = navigationAscension
		if c.relicJunk {
			step = navigationJunk
		}
	case c.questDialog || c.mercenaryDialog:
		if p.options.mercenaries && p.mercenary.active {
			return false
		}
		step = navigationQuest
		if c.mercenaryDialog {
			step = navigationRecovery
		}
	case c.modal != noGildModal:
		if p.options.gilds && (p.gild.active || c.modal == gildChestModal || c.modal == gildRewardModal) && (p.gild.deadline.IsZero() || now.Before(p.gild.deadline)) && p.gild.attempts < 3 {
			return false
		}
		p.gild = gildCollector{nextCheck: now.Add(time.Minute)}
		step = navigationGildRoster
		if c.modal == gildRewardModal {
			step = navigationGildReward
		} else if c.modal == gildChestModal {
			step = navigationGildChest
		}
	case c.heroes:
		needed = false
	case c.outsiders && p.outsiderJobFrame != 0:
		needed = false
	default:
		needed = (p.options.heroes || p.options.progression || p.startupCheck || p.startup != noStartup || p.ancient.blocked && c.ancients) && !p.ancient.active && !p.relic.active && !p.mercenary.active && !p.export.active && !(p.export.requested && !p.startupCheck && (p.startup == noStartup || p.startup == startupSave)) && !p.ascension.active && !(c.ancients && p.ancient.plan != nil && !p.ancient.finished && !p.ancient.blocked)
	}
	if !needed {
		n.message = ""
		return false
	}
	// One shared paced retry, acknowledged by the next recognized screen.
	p.queue = make(map[actionKind]gameAction)
	if now.Before(n.nextAction) {
		return true
	}
	a := gameAction{kind: navigateGame, frame: p.frame, navigation: step}
	if step != navigationFocus {
		point, found, err := navigationControl(p.frame, step)
		if err != nil || !found {
			n.report(fmt.Sprintf("%s control not recognized; observing", step))
			return true
		}
		a.point = point
	}
	p.enqueue(a, now)
	return true
}
