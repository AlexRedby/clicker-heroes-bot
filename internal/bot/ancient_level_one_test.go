package bot

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"

	"clicker-heroes-bot/internal/ancientcalc"

	xdraw "golang.org/x/image/draw"
)

func TestNewlySummonedAncientLevelOne(t *testing.T) {
	requireAncientOCR(t)
	original := loadTestImage(t, "../../testdata/ancient-summoned-level-one.png")
	for _, width := range []int{1280, 1920, 2560} {
		screen := image.NewRGBA(image.Rect(0, 0, width, width*9/16))
		xdraw.CatmullRom.Scale(screen, screen.Bounds(), original, original.Bounds(), draw.Src, nil)
		frame := gameFrame{id: 1, image: screen, context: gameContext{known: true, ancients: true, bounds: screen.Bounds()}}
		out, err := readAncientObservation(context.Background(), frame)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.rows) != 3 || out.rows[0].name != "Argaiv" || out.rows[1].name != "Atman" || out.rows[2].name != "Berserker" {
			t.Fatalf("width %d: visible newly summoned rows missing: %+v", width, out.rows)
		}
		for _, row := range out.rows {
			if row.level != "1" {
				t.Errorf("width %d: level 1 not read: %+v", width, row)
			}
		}
		for _, row := range out.rows {
			plan := ancientPlan{Plan: ancientcalc.Plan{Souls: "1080000", Reserve: "0", Rows: []ancientcalc.Purchase{{Name: row.name, Current: "1", Quantity: "100", Cost: "5150"}}}}
			p := ancientPlanner{plan: &plan, active: true, started: true, selected: -1, done: map[int]bool{}}
			now := time.Now()
			p.observe(out, nil, now)
			action, ok := p.action(frame, now)
			if !ok || p.blocked || action.ancient.step != openAncientQuantity || action.point != row.point {
				t.Errorf("width %d: %s scrolled instead of buying: %+v, %s", width, row.name, action, p.failure)
			}
		}
		point := out.rows[0].point
		region := ancientLevelRegion(screen, point)
		// A level 11 shares the level-1 prefix; the complete crop must reject its extra digit.
		digit := image.Rect(region.Min.X+width*108/2560, region.Min.Y, region.Min.X+width*123/2560, region.Max.Y)
		draw.Draw(screen, digit.Add(image.Pt(width*22/2560, 0)), screen, digit.Min, draw.Src)
		if one, err := ancientLevelOne(screen, point); one || err != nil {
			t.Fatalf("width %d: level-1 prefix accepted as the whole level: %t, %v", width, one, err)
		}
		draw.Draw(screen, digit.Union(digit.Add(image.Pt(width*22/2560, 0))), image.NewUniform(color.Black), image.Point{}, draw.Src)
		if one, err := ancientLevelOne(screen, point); one || err != nil {
			t.Fatalf("width %d: label without a digit accepted: %t, %v", width, one, err)
		}
		draw.Draw(screen, region, image.NewUniform(color.Black), image.Point{}, draw.Src)
		if one, err := ancientLevelOne(screen, point); one || err != nil {
			t.Fatalf("width %d: blank level accepted: %t, %v", width, one, err)
		}
	}
}
