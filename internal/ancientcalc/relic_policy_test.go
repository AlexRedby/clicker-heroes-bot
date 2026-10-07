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

func TestActiveRelicPolicyRequiresMeaningfulEffects(t *testing.T) {
	for _, tc := range []struct {
		name                string
		chronos, juggernaut string
		equipped, candidate RelicBonus
		wantMove            bool
	}{
		{"reported zero-effect Chronos replaces zero-effect Juggernaut", "250", "1e30", RelicBonus{Type: 26, AncientID: 29, Level: "6.46"}, RelicBonus{Type: 3, AncientID: 17, Level: "5.54"}, false},
		{"useful early Chronos", "10", "1e30", RelicBonus{Type: 26, AncientID: 29, Level: "6.46"}, RelicBonus{Type: 3, AncientID: 17, Level: "5.54"}, true},
		{"tiny duration must not displace useful damage", "250", "10", RelicBonus{Type: 26, AncientID: 29, Level: "1000"}, RelicBonus{Type: 3, AncientID: 17, Level: "5.54"}, false},
		{"tiny damage improvement", "250", "1000000", RelicBonus{Type: 26, AncientID: 29, Level: "6"}, RelicBonus{Type: 26, AncientID: 29, Level: "6.46"}, false},
		{"useful damage improvement", "250", "10000", RelicBonus{Type: 26, AncientID: 29, Level: "6"}, RelicBonus{Type: 26, AncientID: 29, Level: "1000"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{17: tc.chronos, 29: tc.juggernaut}, Items: []Relic{
				{UID: 1, Slot: 1, Bonuses: []RelicBonus{tc.equipped}}, {UID: 2, Slot: 2, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "100"}}},
				{UID: 3, Slot: 3, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "100"}}},
				{UID: 4, Slot: 4, Bonuses: []RelicBonus{{Type: 2, AncientID: 19, Level: "100"}}}, {UID: 5, Bonuses: []RelicBonus{tc.candidate}},
			}}
			move, err := PlanRelicEquipment(s)
			if err != nil || (move != nil) != tc.wantMove {
				t.Fatalf("move=%+v error=%v want move=%v", move, err, tc.wantMove)
			}
		})
	}
}

func TestActiveRelicPolicyKeepsUsefulFractionalMonsterReduction(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{21: "250"}, Items: []Relic{
		{UID: 1, Slot: 1}, {UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4}, {UID: 5, Bonuses: []RelicBonus{{Type: 27, AncientID: 21, Level: "1"}}},
	}}
	if move, err := PlanRelicEquipment(s); err != nil || move != nil {
		t.Fatalf("tiny reduction near the cap: %+v %v", move, err)
	}
	// Fractional monsters affect the chance of an extra enemy, so a useful
	// fractional reduction must remain eligible rather than being rounded away.
	s.AncientLevels[21] = "1"
	if move, err := PlanRelicEquipment(s); err != nil || move == nil {
		t.Fatalf("useful fractional reduction was lost: %+v %v", move, err)
	}
}

func TestActiveRelicPolicyDoesNotStartInsignificantBatch(t *testing.T) {
	s := RelicSnapshot{EquipmentSlots: 4, AncientLevels: map[int]string{17: "210"}, Items: []Relic{{UID: 1, Slot: 1}, {UID: 2, Slot: 2}, {UID: 3, Slot: 3}, {UID: 4, Slot: 4}}}
	// Four swaps together pass the boss-timer threshold, but each proposed
	// drag must be useful by itself. There is no stored multi-move outfit plan.
	for uid := 5; uid <= 8; uid++ {
		s.Items = append(s.Items, Relic{UID: uid, Bonuses: []RelicBonus{{Type: 3, AncientID: 17, Level: "5.54"}}})
	}
	if move, err := PlanRelicEquipment(s); err != nil || move != nil {
		t.Fatalf("started an individually useless swap: %+v %v", move, err)
	}
}
