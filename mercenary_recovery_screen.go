package main

import (
	"context"
	"fmt"
	"image"
	"regexp"
	"strconv"
	"strings"
)

type mercenaryDeadCard struct {
	row          image.Point
	traits       mercenaryRecoveryTraits
	revive, bury image.Point
}

type mercenaryRecoveryPrompt struct {
	method  mercenaryRecoveryMethod
	budget  mercenaryRecoveryBudget
	yes, no image.Point
}

func mercenaryRecoveryDialog(screen image.Image) bool {
	return mercenaryRecoveryDialogMethod(screen) != mercenaryRecoverySkip
}

func mercenaryRecoveryDialogMethod(screen image.Image) mercenaryRecoveryMethod {
	if screen == nil {
		return mercenaryRecoverySkip
	}
	b := screen.Bounds()
	buttons := image.Rectangle{Min: mercenaryPoint(b, 325, 503), Max: mercenaryPoint(b, 674, 593)}
	if !mercenaryTemplate(screen, buttons, "mercenaries/recovery-yes.png", image.Pt(104, 56)) ||
		!mercenaryTemplate(screen, buttons, "mercenaries/recovery-no.png", image.Pt(88, 56)) {
		return mercenaryRecoverySkip
	}
	prompt := image.Rectangle{Min: mercenaryPoint(b, 315, 411), Max: mercenaryPoint(b, 685, 464)}
	if mercenaryTemplate(screen, prompt, "mercenaries/bury-prompt.png", image.Pt(792, 60)) {
		return mercenaryRecoveryBury
	}
	if mercenaryTemplate(screen, prompt, "mercenaries/revive-prompt.png", image.Pt(676, 60)) {
		return mercenaryRecoveryRubies
	}
	return mercenaryRecoverySkip
}

var mercenaryDemigodLevel = regexp.MustCompile(`^demigod\s*\+\s*([0-9]{1,4})$`)
var mercenaryTraitBonus = regexp.MustCompile(`^\+([0-9]{1,4}(?:\.[0-9]{1,2})?)% (gold from quests|hero souls from quests|recruitment quest speed|rubies from quests|skills activated by quests)$`)
var mercenaryTraitLives = regexp.MustCompile(`^extra lives: ([0-6])$`)

func parseMercenaryLevel(raw string) (int, bool) {
	text := strings.ToLower(strings.Join(strings.Fields(raw), " "))
	for i, title := range []string{"noob", "rookie", "journeyman", "expert", "master", "grandmaster", "legend", "demigod"} {
		if text == title {
			return i + 1, true
		}
	}
	if match := mercenaryDemigodLevel.FindStringSubmatch(text); match != nil {
		extra, err := strconv.Atoi(match[1])
		return 8 + extra, err == nil
	}
	return 0, false
}

func parseMercenaryTraits(level, bonus string) (mercenaryRecoveryTraits, bool) {
	traits := mercenaryRecoveryTraits{}
	var ok bool
	if traits.Level, ok = parseMercenaryLevel(level); !ok {
		return traits, false
	}
	text := strings.ToLower(strings.Join(strings.Fields(bonus), " "))
	if match := mercenaryTraitLives.FindStringSubmatch(text); match != nil {
		traits.ExtraLives, _ = strconv.Atoi(match[1])
	} else if match := mercenaryTraitBonus.FindStringSubmatch(text); match != nil {
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil || value <= 0 || value > 2000 {
			return traits, false
		}
		if match[2] == "rubies from quests" {
			traits.RubyBonusPercent = value
		}
	} else {
		// Missing or unknown text may conceal extra lives or a ruby bonus.
		return traits, false
	}
	traits.Known = true
	return traits, true
}

func readMercenaryDeadCard(ctx context.Context, screen image.Image, row image.Point) (mercenaryDeadCard, error) {
	card := mercenaryDeadCard{row: row}
	if screen == nil || !mercenaryDead(screen, row) {
		return card, fmt.Errorf("mercenary card is not visibly dead")
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	levelRegion := image.Rect(b.Min.X+w*118/1000, row.Y-h*39/1000, b.Min.X+w*220/1000, row.Y-h*5/1000)
	bonusRegion := image.Rect(b.Min.X+w*92/1000, row.Y+h*20/1000, b.Min.X+w*220/1000, row.Y+h*60/1000)
	level, err := readGameText(ctx, screen, levelRegion, max(2, 4096/w), 7, -170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+ ")
	if err != nil {
		return card, err
	}
	bonus, err := readGameText(ctx, screen, bonusRegion, max(2, 2560/w), 6, -170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+%.: ")
	if err != nil {
		return card, err
	}
	// The verified 1280px ruby label reads "rubles" when OCR confuses i/l.
	// Correct only that exact known label; unknown traits remain untrusted.
	bonus = strings.Replace(bonus, "rubles from quests", "rubies from quests", 1)
	var ok bool
	if card.traits, ok = parseMercenaryTraits(level, bonus); !ok {
		return card, fmt.Errorf("unreadable dead mercenary traits: level=%q bonus=%q", level, bonus)
	}
	card.revive = mercenaryDeadControl(screen, row, mercenaryRecoveryRubies)
	card.bury = mercenaryDeadControl(screen, row, mercenaryRecoveryBury)
	return card, nil
}

func mercenaryDeadControl(screen image.Image, row image.Point, method mercenaryRecoveryMethod) image.Point {
	if screen == nil {
		return image.Point{}
	}
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	name, x, size := "revive", 269, image.Pt(125, 88)
	if method == mercenaryRecoveryBury {
		name, x, size = "bury", 359, image.Pt(117, 88)
	}
	region := image.Rect(b.Min.X+w*(x-40)/1000, row.Y+h*5/1000, b.Min.X+w*(x+40)/1000, row.Y+h*80/1000)
	if mercenaryTemplate(screen, region, "mercenaries/"+name+".png", size) {
		return image.Pt(b.Min.X+w*x/1000, row.Y+h*38/1000)
	}
	return image.Point{}
}

var mercenaryRevivePrice = regexp.MustCompile(`^Spend ([0-9]{1,15}) Rubies on Reviving Mercenary\?$`)
var mercenaryRubyWallet = regexp.MustCompile(`^([0-9]{1,15}) Rubies$`)

func readMercenaryRecoveryPrompt(ctx context.Context, screen image.Image) (*mercenaryRecoveryPrompt, error) {
	method := mercenaryRecoveryDialogMethod(screen)
	if method == mercenaryRecoverySkip {
		return nil, fmt.Errorf("unrecognized mercenary recovery dialog")
	}
	b := screen.Bounds()
	prompt := &mercenaryRecoveryPrompt{method: method, yes: mercenaryPoint(b, 406, 548), no: mercenaryPoint(b, 594, 548)}
	if method == mercenaryRecoveryBury {
		return prompt, nil
	}
	region := image.Rectangle{Min: mercenaryPoint(b, 318, 416), Max: mercenaryPoint(b, 683, 460)}
	raw, err := readGameText(ctx, screen, region, max(2, 4096/b.Dx()), 7, -170, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789? ")
	if err != nil {
		return prompt, err
	}
	match := mercenaryRevivePrice.FindStringSubmatch(strings.Join(strings.Fields(raw), " "))
	if match == nil {
		return prompt, fmt.Errorf("unreadable native revive price %q", raw)
	}
	prompt.budget.Cost, err = strconv.ParseInt(match[1], 10, 64)
	if err != nil || prompt.budget.Cost <= 0 {
		return prompt, fmt.Errorf("invalid native revive price %q", raw)
	}
	region = image.Rectangle{Min: mercenaryPoint(b, 39, 303), Max: mercenaryPoint(b, 145, 334)}
	raw, err = readGameText(ctx, screen, region, max(2, 4096/b.Dx()), 7, -80, "0123456789Rubies ")
	if err != nil {
		return prompt, err
	}
	match = mercenaryRubyWallet.FindStringSubmatch(strings.Join(strings.Fields(raw), " "))
	if match == nil {
		return prompt, fmt.Errorf("unreadable native ruby balance %q", raw)
	}
	prompt.budget.Rubies, err = strconv.ParseInt(match[1], 10, 64)
	prompt.budget.Known = err == nil
	return prompt, err
}
