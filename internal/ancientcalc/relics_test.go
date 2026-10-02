package ancientcalc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// A small numeric projection, not a raw player save.
const relicInventoryJSON = `{"equipmentSlots":4,"items":{"7":{"uid":7,"level":16,"rarity":4,"imageId":5,"upgradeCount":1,"bonusType1":2,"bonusType2":0,"bonusType3":0,"bonusType4":0,"bonus1Level":"6.09164336965949","bonus2Level":"0","bonus3Level":"0","bonus4Level":"0"}},"slots":{"1":7}}`

func encodeRelicSave(inventory string) []byte {
	text := strings.TrimSuffix(testSaveJSON, "}") + `,"items":` + inventory + `}`
	return []byte(base64.StdEncoding.EncodeToString([]byte(text)))
}

func TestReadRelics(t *testing.T) {
	snapshot, err := ReadRelics(context.Background(), encodeRelicSave(relicInventoryJSON))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.EquipmentSlots != 4 || snapshot.Ascensions != 3 || len(snapshot.Items) != 1 || snapshot.Items[0].Slot != 1 || snapshot.Items[0].UID != 7 || snapshot.Items[0].Bonuses[0].Level != "6.09164336965949" {
		t.Fatalf("bad relic projection: %+v", snapshot)
	}
	decimalCount := strings.Replace(strings.TrimSuffix(testSaveJSON, "}"), `"numWorldResets":"3"`, `"numWorldResets":3.0`, 1) + `,"items":` + relicInventoryJSON + `}`
	if snapshot, err := ReadRelics(nil, []byte(base64.StdEncoding.EncodeToString([]byte(decimalCount)))); err != nil || snapshot.Ascensions != 3 {
		t.Fatalf("integral decimal reset count: %+v, %v", snapshot, err)
	}
	// Neither lifetime counters nor core balances can manufacture current items.
	empty := `{"equipmentSlots":4,"items":{},"slots":{},"salvagePoints":999,"_currentUids":{"items":100}}`
	snapshot, err = ReadRelics(nil, encodeRelicSave(empty))
	if err != nil || len(snapshot.Items) != 0 {
		t.Fatalf("empty inventory: %+v, %v", snapshot, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadRelics(ctx, encodeRelicSave(relicInventoryJSON)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestRelicsRejectIncompleteInventory(t *testing.T) {
	for _, inventory := range []string{
		`null`, `{}`, `{"equipmentSlots":4,"items":{},"slots":null}`,
		strings.Replace(relicInventoryJSON, `"equipmentSlots":4`, `"equipmentSlots":5`, 1),
		strings.Replace(relicInventoryJSON, `"slots":{"1":7}`, `"slots":{"1":7,"2":7}`, 1),
		strings.Replace(relicInventoryJSON, `"slots":{"1":7}`, `"slots":{"1":8}`, 1),
		strings.Replace(relicInventoryJSON, `"slots":{"1":7}`, `"slots":{"01":7}`, 1),
		strings.Replace(relicInventoryJSON, `"uid":7`, `"uid":8`, 1),
		strings.Replace(relicInventoryJSON, `"level":16`, `"level":-1`, 1),
		strings.Replace(relicInventoryJSON, `"upgradeCount":1,`, ``, 1),
		strings.Replace(relicInventoryJSON, `"bonusType2":0`, `"bonusType2":2`, 1),
		strings.Replace(relicInventoryJSON, `"bonusType1":2`, `"bonusType1":0`, 1),
		strings.Replace(relicInventoryJSON, `"bonusType1":2`, `"bonusType1":null`, 1),
		strings.Replace(relicInventoryJSON, `"6.09164336965949"`, `"1e99999"`, 1),
	} {
		if _, err := ReadRelics(context.Background(), encodeRelicSave(inventory)); err == nil {
			t.Fatalf("accepted malformed inventory: %s", inventory)
		}
	}
	if _, err := ReadRelics(nil, []byte(base64.StdEncoding.EncodeToString([]byte(testSaveJSON)))); err == nil {
		t.Fatal("save with no relic data accepted as empty")
	}
	raw, err := base64.StdEncoding.DecodeString(string(encodeRelicSave(relicInventoryJSON)))
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"transcendent":true,`, "", 1))
	if _, err := ReadRelics(nil, []byte(base64.StdEncoding.EncodeToString(raw))); err == nil {
		t.Fatal("missing transcendence state accepted for relic policy")
	}
}

func TestRelicNumbersPreserveTinyDifferences(t *testing.T) {
	a, ok := relicNumber("0.10000000000000000000000000000000000000000000000000000000000000000000000000000001")
	b, okB := relicNumber("0.1")
	if !ok || !okB || a.Cmp(b) <= 0 {
		t.Fatal("fractional relic bonuses lost precision")
	}
	for _, value := range []string{"", "-1", "NaN", "1e309", "1e-309", "1/2"} {
		if _, ok := relicNumber(value); ok {
			t.Fatalf("unsafe relic number accepted: %q", value)
		}
	}
}

func TestRelicDominanceRequiresNoActiveLoss(t *testing.T) {
	bonus := func(kind int, level string) RelicBonus { return RelicBonus{Type: kind, Level: level} }
	for _, test := range []struct {
		name               string
		candidate, current []RelicBonus
		want               bool
	}{
		{"better same effect", []RelicBonus{bonus(2, "30")}, []RelicBonus{bonus(2, "16")}, true},
		{"equal decimals", []RelicBonus{bonus(2, "0.10")}, []RelicBonus{bonus(2, "0.1")}, false},
		{"mixed tradeoff", []RelicBonus{bonus(26, "10")}, []RelicBonus{bonus(2, "1")}, false},
		{"zero effect", []RelicBonus{bonus(2, "0")}, nil, false},
		{"active over idle", []RelicBonus{bonus(2, "1")}, []RelicBonus{bonus(1, "100")}, true},
		{"tiny loss plus gain", []RelicBonus{bonus(2, "0.1"), bonus(26, "1")}, []RelicBonus{bonus(2, "0.10000000000000000000000000000000000000000000000000000000000000000000000000000001")}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := relicDominates(Relic{Bonuses: test.candidate}, Relic{Bonuses: test.current}); got != test.want {
				t.Fatalf("dominance=%v, want %v", got, test.want)
			}
		})
	}
}

func TestRelicPreviewRemainsManualAndUnknown(t *testing.T) {
	var inventory map[string]any
	if err := json.Unmarshal([]byte(relicInventoryJSON), &inventory); err != nil {
		t.Fatal(err)
	}
	items := inventory["items"].(map[string]any)
	current := items["7"].(map[string]any)
	candidate := map[string]any{}
	for key, value := range current {
		candidate[key] = value
	}
	candidate["uid"], candidate["bonus1Level"] = 8, "30"
	items["8"] = candidate
	check := func() RelicPreview {
		t.Helper()
		data, err := json.Marshal(inventory)
		if err != nil {
			t.Fatal(err)
		}
		out, err := PreviewRelics(nil, encodeRelicSave(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		if out.Readiness != "unknown" || len(out.Snapshot.Items) != 2 {
			t.Fatalf("snapshot granted readiness or lost unassigned item: %+v", out)
		}
		return out
	}
	// An item without a slot is preserved, with a manual suggestion only.
	if out := check(); out.Suggestion == nil || out.Suggestion.UID != 8 || out.Suggestion.Slot != 1 || out.Suggestion.ReplaceUID != 7 {
		t.Fatalf("manual dominance suggestion: %+v", out)
	}
	candidate["bonusType1"] = 23 // Absent from the official asset.
	if out := check(); out.Suggestion != nil || len(out.UnsupportedTypes) != 1 || out.UnsupportedTypes[0] != 23 {
		t.Fatalf("unknown effect allowed a suggestion: %+v", out)
	}
	candidate["bonusType1"] = 6 // Iris needs restart support first.
	if out := check(); out.Suggestion != nil || len(out.UnsupportedTypes) != 1 {
		t.Fatalf("unsupported starting zone allowed a suggestion: %+v", out)
	}
	candidate["bonusType1"] = 2
	inventory["slots"].(map[string]any)["5"] = 8
	if out := check(); out.Suggestion != nil || out.Snapshot.Items[1].Slot != 5 {
		t.Fatalf("unproven slot range allowed a suggestion or lost the item: %+v", out)
	}
	delete(inventory["slots"].(map[string]any), "5")
	inventory["slots"].(map[string]any)["2"] = 8
	if out := check(); out.Suggestion != nil {
		t.Fatalf("already equipped candidate moved out of its slot: %+v", out)
	}
}
