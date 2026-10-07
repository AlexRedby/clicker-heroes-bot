package ancientcalc

import "testing"

func TestActiveRelicPolicyChoosesIncomparableSet(t *testing.T) {
	item := func(uid, slot, typ int, level string) Relic {
		return Relic{UID: uid, Slot: slot, Bonuses: []RelicBonus{{Type: typ, AncientID: relicAncientIDs[typ], Level: level}}}
	}
	s := RelicSnapshot{
		EquipmentSlots: 4,
		AncientLevels:  map[int]string{13: "1", 21: "1", 19: "1"},
		Items: []Relic{
			item(1, 1, 2, "1"), item(2, 2, 2, "1"), item(3, 3, 2, "1"), item(4, 4, 2, "1"),
			item(5, 0, 17, "30"), // Atman
		},
	}
	move, err := PlanRelicEquipment(s)
	if err != nil || move == nil || move.UID != 5 {
		t.Fatalf("policy did not choose incomparable capped utility: %+v %v", move, err)
	}
}

func TestActiveRelicPolicyPreservesConvergedTie(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{13: "2880"}, Items: []Relic{
		{UID: 1, Slot: 1, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "1"}}},
		{UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4},
		{UID: 5, Slot: 0, Bonuses: []RelicBonus{{Type: 17, AncientID: 13, Level: "1000"}}},
	}}
	move, err := PlanRelicEquipment(s)
	if err != nil || move != nil {
		t.Fatalf("converged Atman effect oscillated: %+v %v", move, err)
	}
}

func TestRelicJunkDominanceCertificate(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{19: "1"}, Items: []Relic{
		{UID: 1, Slot: 1, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "10"}}},
		{UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4},
		{UID: 5, Slot: 5, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "10"}}},
	}}
	ok, err := RelicJunkIsDominated(s)
	if err != nil || !ok {
		t.Fatalf("equal junk was not certified: %v %v", ok, err)
	}
	s.Items[4].Bonuses[0].Level = "10.1"
	ok, err = RelicJunkIsDominated(s)
	if err != nil || ok {
		t.Fatalf("tradeoff junk was certified: %v %v", ok, err)
	}
}

func TestActiveRelicPolicyUsefulCapsAndIndividualCooldowns(t *testing.T) {
	for _, tc := range []struct {
		name              string
		typ               int
		levels, outsiders map[int]string
		improve           bool
	}{
		{"Kuma already two monsters", 27, map[int]string{21: "50"}, map[int]string{6: "100"}, false},
		{"Atman already guaranteed", 17, map[int]string{13: "50"}, map[int]string{7: "100"}, false},
		{"Lucky continuous", 10, map[int]string{20: "1440", 25: "210"}, nil, false},
		{"Lucky needs one level", 10, map[int]string{20: "1440", 25: "209"}, nil, true},
		{"Golden continuous", 9, map[int]string{20: "1440", 26: "435"}, nil, false},
		{"Golden needs one level", 9, map[int]string{20: "1440", 26: "434"}, nil, true},
		{"Absent Ancient receives bonus", 17, map[int]string{}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: tc.levels, OutsiderLevels: tc.outsiders, Items: []Relic{
				{UID: 1, Slot: 1, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "1"}}},
				{UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4},
				{UID: 5, Bonuses: []RelicBonus{{Type: tc.typ, AncientID: relicAncientIDs[tc.typ], Level: "1"}}},
			}}
			move, err := PlanRelicEquipment(s)
			if err != nil || (move != nil) != tc.improve {
				t.Fatal("wrong useful-cap decision", move, err)
			}
		})
	}
}

func TestActiveRelicPolicyFillsEmptySlotsAndConverges(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{}, Items: []Relic{
		{UID: 1, Slot: 1, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "1"}}},
		{UID: 2, Bonuses: []RelicBonus{{Type: 17, AncientID: 13, Level: "20"}}},
		{UID: 3, Bonuses: []RelicBonus{{Type: 27, AncientID: 21, Level: "20"}}},
	}}
	moves := 0
	for ; moves < 4; moves++ {
		move, err := PlanRelicEquipment(s)
		if err != nil {
			t.Fatal(err)
		}
		if move == nil {
			break
		}
		if move.ReplaceUID != 0 {
			t.Fatal("dropped a useful equipped item while slots were empty", move)
		}
		for i := range s.Items {
			if s.Items[i].UID == move.UID {
				s.Items[i].Slot = move.Slot
			}
		}
	}
	if moves != 2 {
		t.Fatal("equipment did not settle", moves, s)
	}
}

func TestRelicPolicyRejectsUnsafeProfilesAndExactDiscardDifferences(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{19: "1e1000"}, Items: []Relic{
		{UID: 1, Slot: 1, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "10"}}},
		{UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4},
		{UID: 5, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "10.00000000000000000000000001"}}},
	}}
	if _, err := PlanRelicEquipment(s); err != nil {
		t.Fatal("large valid Ancient level rejected", err)
	}
	if ok, err := RelicJunkIsDominated(s); err != nil || ok {
		t.Fatal("tiny real improvement was destroyed", ok, err)
	}
	s.Items[4].Bonuses[0] = RelicBonus{Type: 99, Level: "1"}
	if _, err := PlanRelicEquipment(s); err == nil {
		t.Fatal("unknown effect accepted")
	}
	if ok, err := RelicJunkIsDominated(s); err == nil || ok {
		t.Fatal("unknown effect destroyed")
	}
	s.Items = s.Items[:4]
	if ok, err := RelicJunkIsDominated(s); err != nil || ok {
		t.Fatal("empty junk requested salvage", ok, err)
	}
	s.AncientLevels[19] = "bad"
	if _, err := PlanRelicEquipment(s); err == nil {
		t.Fatal("invalid profile accepted")
	}
}
