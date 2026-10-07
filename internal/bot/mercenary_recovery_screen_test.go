package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"os"
	"testing"
	"time"
)

func TestMercenaryRecoverySpecificNativeDialogs(t *testing.T) {
	for _, divisor := range []int{1, 2, 4} {
		for _, test := range []struct {
			path string
			want mercenaryRecoveryMethod
		}{{"../../testdata/mercenary-bury.png", mercenaryRecoveryBury}, {"../../testdata/mercenary-revive.png", mercenaryRecoveryRubies},
			{"../../testdata/mercenary-paid-hire.png", mercenaryRecoverySkip}, {"../../testdata/mercenary-dead.png", mercenaryRecoverySkip},
			{"../../testdata/mercenary-after-bury.png", mercenaryRecoverySkip}, {"../../testdata/mercenary-recruitment.png", mercenaryRecoverySkip}} {
			im := mercenaryScaled(t, test.path, divisor)
			if got := mercenaryRecoveryDialogMethod(im); got != test.want {
				t.Fatalf("%s scale 1/%d method=%d want=%d", test.path, divisor, got, test.want)
			}
		}
	}
}

func TestMercenaryRecoveryNativePriceTraitsAndRebuiltRoster(t *testing.T) {
	if os.Getenv("REQUIRE_OCR_TESTS") == "" {
		t.Skip("set REQUIRE_OCR_TESTS=1")
	}
	for _, divisor := range []int{1, 2} {
		dead := mercenaryScaled(t, "../../testdata/mercenary-dead.png", divisor)
		row := mercenaryRows(dead)[1]
		card, err := readMercenaryDeadCard(context.Background(), dead, row)
		if err != nil || card.traits != (mercenaryRecoveryTraits{Known: true, Level: 10, RubyBonusPercent: 8}) || card.revive == (image.Point{}) || card.bury == (image.Point{}) {
			t.Fatalf("scale 1/%d dead card=%+v err=%v", divisor, card, err)
		}
		if _, err := readMercenaryDeadCard(context.Background(), dead, mercenaryRows(dead)[0]); err == nil {
			t.Fatal("living mercenary accepted by dead-traits reader")
		}
		prompt, err := readMercenaryRecoveryPrompt(context.Background(), mercenaryScaled(t, "../../testdata/mercenary-revive.png", divisor))
		if err != nil || prompt == nil || prompt.budget != (mercenaryRecoveryBudget{Known: true, Cost: 680, Rubies: 4039}) {
			t.Fatalf("scale 1/%d revive prompt=%+v err=%v", divisor, prompt, err)
		}
		rebuilt, err := readMercenaryObservation(context.Background(), gameFrame{image: mercenaryScaled(t, "../../testdata/mercenary-after-bury.png", divisor)})
		if err != nil || !rebuilt.readable || len(rebuilt.dead) != 0 || len(rebuilt.collect) != 2 || len(rebuilt.running) != 1 || !rebuilt.bottom {
			t.Fatalf("scale 1/%d post-burial roster=%+v err=%v", divisor, rebuilt, err)
		}
		quest, err := readMercenaryObservation(context.Background(), gameFrame{image: mercenaryScaled(t, "../../testdata/mercenary-recruitment.png", divisor)})
		if err != nil || quest.quests[0].reward != "recruitment" || quest.quests[0].duration != 8*time.Hour || chooseMercenaryQuest(quest.quests) != 0 {
			t.Fatalf("scale 1/%d free recruitment=%+v err=%v", divisor, quest.quests, err)
		}
	}
}

func TestMercenaryRecoveryConfirmationRejectsChangedPriceAndWallet(t *testing.T) {
	im := loadTestImage(t, "../../testdata/mercenary-revive.png")
	c := gameContext{known: true, mercenaries: true, mercenaryDialog: true, bounds: im.Bounds()}
	frame := gameFrame{image: im, context: c}
	a := gameAction{frame: frame, mercenary: mercenaryCommand{step: confirmMercenaryRecovery, recovery: mercenaryRecoveryRubies}}
	if !mercenaryActionStable(a, frame) {
		t.Fatal("unchanged native paid prompt rejected")
	}
	for _, region := range []image.Rectangle{
		{Min: mercenaryPoint(im.Bounds(), 318, 416), Max: mercenaryPoint(im.Bounds(), 683, 460)},
		{Min: mercenaryPoint(im.Bounds(), 39, 303), Max: mercenaryPoint(im.Bounds(), 145, 334)},
	} {
		changed := image.NewRGBA(im.Bounds())
		draw.Draw(changed, changed.Bounds(), im, im.Bounds().Min, draw.Src)
		draw.Draw(changed, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
		frame.image = changed
		if mercenaryActionStable(a, frame) {
			t.Fatalf("changed confirmation region accepted: %v", region)
		}
	}
	frame.image = loadTestImage(t, "../../testdata/mercenary-paid-hire.png")
	if mercenaryActionStable(a, frame) {
		t.Fatal("paid Hire accepted as revive")
	}
}

func TestMercenaryRecoveryTraitsRequireKnownBonus(t *testing.T) {
	for _, test := range []struct {
		level, bonus string
		want         mercenaryRecoveryTraits
	}{{"Demigod + 2", "+8% rubies from quests", mercenaryRecoveryTraits{Known: true, Level: 10, RubyBonusPercent: 8}},
		{"Rookie", "+7.5% gold from\r\nquests", mercenaryRecoveryTraits{Known: true, Level: 2}},
		{"Noob", "+20% recruitment quest speed", mercenaryRecoveryTraits{Known: true, Level: 1}},
		{"Demigod + 12", "Extra lives: 2", mercenaryRecoveryTraits{Known: true, Level: 20, ExtraLives: 2}}} {
		got, ok := parseMercenaryTraits(test.level, test.bonus)
		if !ok || got != test.want {
			t.Fatalf("level=%q bonus=%q got=%+v ok=%t", test.level, test.bonus, got, ok)
		}
	}
	for _, test := range [][2]string{{"Demigod +", "+8% rubies from quests"}, {"Demigod", ""}, {"Demigod", "+8% unknown bonus"},
		{"Demigod", "Extra lives: ?"}, {"Demigod", "Extra lives: 7"}, {"Demigod", "+8000% rubies from quests"}, {"Demigod + 1", "+0% rubies from quests"}} {
		if got, ok := parseMercenaryTraits(test[0], test[1]); ok || got.Known {
			t.Fatalf("unreadable traits accepted: %v -> %+v", test, got)
		}
	}
}
