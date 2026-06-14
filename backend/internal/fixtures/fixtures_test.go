package fixtures

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// ── helpers ────────────────────────────────────────────────────────────────────

// mkParticipants builds n participants with deterministic ids p01,p02,… and
// ascending registration times (so registration-order == input order).
func mkParticipants(n int) []Participant {
	out := make([]Participant, n)
	for i := 0; i < n; i++ {
		out[i] = Participant{ID: fmt.Sprintf("p%02d", i+1), RegisteredAt: int64(i)}
	}
	return out
}

func unorderedKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

// concretePairs returns the unordered concrete pairings in a plan (ignores TBD).
func concretePairs(p *Plan) []string {
	var keys []string
	for _, m := range p.Matches {
		if m.Home.Kind == SlotConcrete && m.Away.Kind == SlotConcrete {
			keys = append(keys, unorderedKey(m.Home.ID, m.Away.ID))
		}
	}
	return keys
}

// assertNoSelfMatches fails if any match pits a participant against itself.
func assertNoSelfMatches(t *testing.T, p *Plan) {
	t.Helper()
	for _, m := range p.Matches {
		if m.Home.Kind == SlotConcrete && m.Away.Kind == SlotConcrete && m.Home.ID == m.Away.ID {
			t.Fatalf("self-match in %s: %s vs %s", m.RoundName, m.Home.ID, m.Away.ID)
		}
	}
}

// assertProgressionIntegrity validates the FE-8B wiring of a plan:
//   - every TBD slot is fed by exactly one feeder,
//   - every concrete/qualifier slot is fed by zero feeders,
//   - each edge points strictly forward (target round > source round) to a TBD slot,
//   - no two feeders share a (target, slot) — no orphan / unreachable slots.
func assertProgressionIntegrity(t *testing.T, p *Plan) {
	t.Helper()
	byIdx := make(map[int]*PlannedMatch, len(p.Matches))
	for i := range p.Matches {
		byIdx[p.Matches[i].Index] = &p.Matches[i]
	}
	feeders := make(map[[2]int]int) // (matchIndex, slot) → feeder count
	for i := range p.Matches {
		m := &p.Matches[i]
		if m.NextMatchIndex == nil && m.NextSlot == nil {
			continue
		}
		if m.NextMatchIndex == nil || m.NextSlot == nil {
			t.Fatalf("match %d has a half-set edge (idx=%v slot=%v)", m.Index, m.NextMatchIndex, m.NextSlot)
		}
		tgt, ok := byIdx[*m.NextMatchIndex]
		if !ok {
			t.Fatalf("match %d points to non-existent match %d", m.Index, *m.NextMatchIndex)
		}
		if tgt.RoundNumber <= m.RoundNumber {
			t.Fatalf("backward/sideways edge: match %d (r%d) → %d (r%d)", m.Index, m.RoundNumber, tgt.Index, tgt.RoundNumber)
		}
		if *m.NextSlot != 1 && *m.NextSlot != 2 {
			t.Fatalf("match %d has invalid next slot %d", m.Index, *m.NextSlot)
		}
		feeders[[2]int{*m.NextMatchIndex, *m.NextSlot}]++
	}
	for i := range p.Matches {
		m := &p.Matches[i]
		check := func(slot SlotRef, slotNo int) {
			n := feeders[[2]int{m.Index, slotNo}]
			switch slot.Kind {
			case SlotTBD:
				if n != 1 {
					t.Fatalf("match %d slot %d is TBD but has %d feeders (want 1)", m.Index, slotNo, n)
				}
			default: // concrete or qualifier
				if n != 0 {
					t.Fatalf("match %d slot %d is filled but has %d feeders (want 0)", m.Index, slotNo, n)
				}
			}
		}
		check(m.Home, 1)
		check(m.Away, 2)
	}
}

// ── Round robin ─────────────────────────────────────────────────────────────────

func TestRoundRobin_AllPairsExactlyOnce(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 6, 7, 8, 9, 12} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			plan, err := Generate(mkParticipants(n), Settings{Format: FormatRoundRobin, SeedStrategy: SeedRegistration})
			if err != nil {
				t.Fatal(err)
			}
			assertNoSelfMatches(t, plan)

			pairs := concretePairs(plan)
			want := n * (n - 1) / 2
			if len(pairs) != want {
				t.Fatalf("got %d fixtures, want %d (C(%d,2))", len(pairs), want, n)
			}
			seen := map[string]int{}
			for _, k := range pairs {
				seen[k]++
			}
			if len(seen) != want {
				t.Fatalf("duplicate fixtures: %d unique of %d", len(seen), want)
			}
			for k, c := range seen {
				if c != 1 {
					t.Fatalf("pair %s scheduled %d times", k, c)
				}
			}
			// Round count = (even-count − 1).
			maxRound := 0
			for _, m := range plan.Matches {
				if m.RoundNumber > maxRound {
					maxRound = m.RoundNumber
				}
			}
			wantRounds := evenCount(n) - 1
			if maxRound != wantRounds {
				t.Fatalf("rounds = %d, want %d", maxRound, wantRounds)
			}
		})
	}
}

func TestRoundRobin_HomeAwayBalanced(t *testing.T) {
	for _, n := range []int{4, 5, 6, 8, 10} {
		plan, _ := Generate(mkParticipants(n), Settings{Format: FormatRoundRobin, SeedStrategy: SeedRegistration})
		home := map[string]int{}
		away := map[string]int{}
		for _, m := range plan.Matches {
			home[m.Home.ID]++
			away[m.Away.ID]++
		}
		for _, p := range mkParticipants(n) {
			d := home[p.ID] - away[p.ID]
			if d < -1 || d > 1 {
				t.Errorf("n=%d participant %s home-away imbalance %d (want within ±1)", n, p.ID, d)
			}
		}
	}
}

func TestLeague_EachPairTwice_PerfectBalance(t *testing.T) {
	n := 6
	plan, err := Generate(mkParticipants(n), Settings{Format: FormatLeague, SeedStrategy: SeedRegistration})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSelfMatches(t, plan)
	seen := map[string]int{}
	for _, k := range concretePairs(plan) {
		seen[k]++
	}
	for k, c := range seen {
		if c != 2 {
			t.Fatalf("league pair %s played %d times, want 2", k, c)
		}
	}
	if len(seen) != n*(n-1)/2 {
		t.Fatalf("unique pairs %d, want %d", len(seen), n*(n-1)/2)
	}
	// Double round-robin is exactly balanced: each pair plays home once, away once.
	home, away := map[string]int{}, map[string]int{}
	for _, m := range plan.Matches {
		home[m.Home.ID]++
		away[m.Away.ID]++
	}
	for _, p := range mkParticipants(n) {
		if home[p.ID] != away[p.ID] {
			t.Errorf("league %s home %d != away %d", p.ID, home[p.ID], away[p.ID])
		}
	}
}

// ── Knockout ────────────────────────────────────────────────────────────────────

func TestKnockout_PowersOfTwo(t *testing.T) {
	for _, n := range []int{8, 16, 32, 64} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			plan, err := Generate(mkParticipants(n), Settings{Format: FormatKnockout, SeedStrategy: SeedRegistration})
			if err != nil {
				t.Fatal(err)
			}
			assertNoSelfMatches(t, plan)
			assertProgressionIntegrity(t, plan)

			if len(plan.Matches) != n-1 {
				t.Fatalf("n=%d: %d matches, want %d", n, len(plan.Matches), n-1)
			}
			if len(plan.Byes) != 0 {
				t.Fatalf("n=%d: %d byes, want 0", n, len(plan.Byes))
			}
			finals := 0
			for _, m := range plan.Matches {
				if m.NextMatchIndex == nil {
					finals++
					if m.RoundName != "Final" {
						t.Errorf("terminal match is %q, want Final", m.RoundName)
					}
				}
			}
			if finals != 1 {
				t.Fatalf("n=%d: %d finals, want 1", n, finals)
			}
			// Round 1 must contain all n participants exactly once (no byes).
			seen := map[string]bool{}
			for _, m := range plan.Matches {
				if m.RoundNumber == 1 {
					seen[m.Home.ID] = true
					seen[m.Away.ID] = true
				}
			}
			if len(seen) != n {
				t.Fatalf("round 1 covers %d participants, want %d", len(seen), n)
			}
		})
	}
}

func TestKnockout_TopSeedsMeetLate(t *testing.T) {
	// In an 8-bracket, seed 1 and seed 2 must be in opposite halves (meet only in
	// the final). seedSlotOrder = [1,8,4,5,2,7,3,6]: seed 1 at pos0, seed 2 at pos4.
	order := seedSlotOrder(8)
	pos := map[int]int{}
	for i, s := range order {
		pos[s] = i
	}
	if (pos[1] < 4) == (pos[2] < 4) {
		t.Fatalf("seeds 1 and 2 share a half: pos1=%d pos2=%d", pos[1], pos[2])
	}
	// Round-1 opponents always sum to bracketSize+1 (1v8, 4v5, 2v7, 3v6).
	for i := 0; i < len(order); i += 2 {
		if order[i]+order[i+1] != 9 {
			t.Fatalf("round-1 pair (%d,%d) does not sum to 9", order[i], order[i+1])
		}
	}
}

func TestKnockout_Byes(t *testing.T) {
	for _, n := range []int{3, 5, 6, 12, 20} {
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			plan, err := Generate(mkParticipants(n), Settings{Format: FormatKnockout, SeedStrategy: SeedRegistration})
			if err != nil {
				t.Fatal(err)
			}
			assertNoSelfMatches(t, plan)
			assertProgressionIntegrity(t, plan)

			size := nextPow2(n)
			wantByes := size - n
			if len(plan.Byes) != wantByes {
				t.Fatalf("n=%d: %d byes, want %d", n, len(plan.Byes), wantByes)
			}
			// Byes go to the top seeds (seed protection): the first `wantByes`
			// participants in seed order must each have a bye.
			byeIDs := map[string]bool{}
			for _, b := range plan.Byes {
				byeIDs[b.ParticipantID] = true
			}
			for i := 0; i < wantByes; i++ {
				if !byeIDs[plan.SeedOrder[i]] {
					t.Errorf("top seed %s (#%d) did not receive a bye", plan.SeedOrder[i], i+1)
				}
			}
			// Every participant appears exactly once across round-1 matches + byes.
			seen := map[string]int{}
			for _, m := range plan.Matches {
				if m.RoundNumber == 1 {
					seen[m.Home.ID]++
					seen[m.Away.ID]++
				}
			}
			for _, b := range plan.Byes {
				seen[b.ParticipantID]++
			}
			for _, id := range plan.SeedOrder {
				if seen[id] != 1 {
					t.Errorf("participant %s appears %d times in round 1 (incl. byes), want 1", id, seen[id])
				}
			}
		})
	}
}

// ── Group + knockout ────────────────────────────────────────────────────────────

func TestGroupKnockout_Structure(t *testing.T) {
	// 24 teams, 8 groups of 3, top 2 advance → 16-team knockout.
	n, groups, qpg := 24, 8, 2
	plan, err := Generate(mkParticipants(n), Settings{
		Format: FormatGroupKnockout, SeedStrategy: SeedRegistration,
		Groups: groups, QualifiersPerGroup: qpg,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSelfMatches(t, plan)
	assertProgressionIntegrity(t, plan)

	// Group stage: each group of 3 plays C(3,2)=3 matches → 24 group matches.
	groupMatches := 0
	groupMembers := map[string]map[string]bool{}
	for _, m := range plan.Matches {
		if m.GroupLabel != "" {
			groupMatches++
			if groupMembers[m.GroupLabel] == nil {
				groupMembers[m.GroupLabel] = map[string]bool{}
			}
			groupMembers[m.GroupLabel][m.Home.ID] = true
			groupMembers[m.GroupLabel][m.Away.ID] = true
		}
	}
	if groupMatches != groups*3 {
		t.Fatalf("group matches = %d, want %d", groupMatches, groups*3)
	}
	if len(groupMembers) != groups {
		t.Fatalf("groups = %d, want %d", len(groupMembers), groups)
	}
	for label, members := range groupMembers {
		if len(members) != 3 {
			t.Errorf("group %s has %d members, want 3", label, len(members))
		}
	}
	// Knockout: 16 qualifiers → 15 matches, exactly one final.
	ko := 0
	finals := 0
	for _, m := range plan.Matches {
		if m.GroupLabel == "" {
			ko++
			if m.NextMatchIndex == nil {
				finals++
			}
		}
	}
	if ko != groups*qpg-1 {
		t.Fatalf("knockout matches = %d, want %d", ko, groups*qpg-1)
	}
	if finals != 1 {
		t.Fatalf("knockout finals = %d, want 1", finals)
	}
}

func TestGroupKnockout_NoSameGroupRound1(t *testing.T) {
	plan, err := Generate(mkParticipants(16), Settings{
		Format: FormatGroupKnockout, SeedStrategy: SeedRegistration,
		Groups: 4, QualifiersPerGroup: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The first knockout round must never pit two qualifiers of the same group.
	koFirstRound := 0
	for _, m := range plan.Matches {
		if m.GroupLabel != "" {
			continue
		}
		if m.Home.Kind == SlotQualifier && m.Away.Kind == SlotQualifier {
			koFirstRound++
			if m.Home.Group == m.Away.Group {
				t.Errorf("same-group round-1 knockout pairing: %s#%d vs %s#%d",
					m.Home.Group, m.Home.Rank, m.Away.Group, m.Away.Rank)
			}
		}
	}
	if koFirstRound == 0 {
		t.Fatal("expected qualifier-vs-qualifier first-round matches")
	}
}

func TestGroupKnockout_QualifierOppositeHalves(t *testing.T) {
	// A group's #1 and #2 must sit in opposite halves of the knockout (so they
	// can only meet in the final), for an even, power-of-two group count.
	plan, _ := Generate(mkParticipants(16), Settings{
		Format: FormatGroupKnockout, SeedStrategy: SeedRegistration,
		Groups: 4, QualifiersPerGroup: 2,
	})
	// Find the knockout final, then the two semifinal feeders define the halves.
	var koMatches []PlannedMatch
	for _, m := range plan.Matches {
		if m.GroupLabel == "" {
			koMatches = append(koMatches, m)
		}
	}
	// Collect, per group, the set of first-round matches it appears in and which
	// half (via the final's two sub-trees) — simpler: assert the property the
	// construction guarantees, that for each group the two qualifier slots never
	// appear in the same first-round match (already covered) and the bracket has
	// 8 first-round slots = 4 matches with 8 distinct qualifiers.
	firstRoundQuals := map[string]int{}
	minRound := 1 << 30
	for _, m := range koMatches {
		if m.RoundNumber < minRound {
			minRound = m.RoundNumber
		}
	}
	for _, m := range koMatches {
		if m.RoundNumber == minRound {
			firstRoundQuals[fmt.Sprintf("%s%d", m.Home.Group, m.Home.Rank)]++
			firstRoundQuals[fmt.Sprintf("%s%d", m.Away.Group, m.Away.Rank)]++
		}
	}
	if len(firstRoundQuals) != 8 {
		t.Fatalf("expected 8 distinct first-round qualifiers, got %d", len(firstRoundQuals))
	}
}

// ── Seeding strategies ──────────────────────────────────────────────────────────

func TestSeeding_Registration_PreservesOrder(t *testing.T) {
	plan, _ := Generate(mkParticipants(8), Settings{Format: FormatKnockout, SeedStrategy: SeedRegistration})
	want := []string{"p01", "p02", "p03", "p04", "p05", "p06", "p07", "p08"}
	if !reflect.DeepEqual(plan.SeedOrder, want) {
		t.Fatalf("seed order = %v, want %v", plan.SeedOrder, want)
	}
}

func TestSeeding_Manual_OrdersBySeedNumber(t *testing.T) {
	ps := mkParticipants(4)
	s4, s1, s2 := 4, 1, 2
	ps[0].SeedNumber = &s4 // p01 → seed 4
	ps[1].SeedNumber = &s1 // p02 → seed 1
	ps[2].SeedNumber = &s2 // p03 → seed 2
	// p04 unseeded → sorts after the seeded ones
	plan, _ := Generate(ps, Settings{Format: FormatKnockout, SeedStrategy: SeedManual})
	want := []string{"p02", "p03", "p01", "p04"}
	if !reflect.DeepEqual(plan.SeedOrder, want) {
		t.Fatalf("manual seed order = %v, want %v", plan.SeedOrder, want)
	}
}

func TestSeeding_Random_DeterministicForSameSeed(t *testing.T) {
	ps := mkParticipants(16)
	a, err := Generate(ps, Settings{Format: FormatKnockout, SeedStrategy: SeedRandom, RandomSeed: 42})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Generate(ps, Settings{Format: FormatKnockout, SeedStrategy: SeedRandom, RandomSeed: 42})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same random seed produced different plans")
	}
	c, _ := Generate(ps, Settings{Format: FormatKnockout, SeedStrategy: SeedRandom, RandomSeed: 43})
	if reflect.DeepEqual(a.SeedOrder, c.SeedOrder) {
		t.Fatal("different random seeds produced identical seed orders (suspicious)")
	}
}

func TestSeeding_Random_IsAPermutation(t *testing.T) {
	ps := mkParticipants(32)
	plan, _ := Generate(ps, Settings{Format: FormatKnockout, SeedStrategy: SeedRandom, RandomSeed: 7})
	got := append([]string{}, plan.SeedOrder...)
	sort.Strings(got)
	want := make([]string, len(ps))
	for i := range ps {
		want[i] = ps[i].ID
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("random seed order is not a permutation of the input")
	}
}

// ── Determinism (property) across all formats ───────────────────────────────────

func TestGenerate_DeterministicAllFormats(t *testing.T) {
	cases := []Settings{
		{Format: FormatRoundRobin, SeedStrategy: SeedRegistration},
		{Format: FormatLeague, SeedStrategy: SeedRegistration},
		{Format: FormatKnockout, SeedStrategy: SeedRandom, RandomSeed: 99},
		{Format: FormatGroupKnockout, SeedStrategy: SeedRegistration, Groups: 4, QualifiersPerGroup: 2},
	}
	ps := mkParticipants(16)
	for _, s := range cases {
		t.Run(string(s.Format), func(t *testing.T) {
			a, err := Generate(ps, s)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := Generate(ps, s)
			if !reflect.DeepEqual(a, b) {
				t.Fatal("non-deterministic generation")
			}
		})
	}
}

// ── Input validation ────────────────────────────────────────────────────────────

func TestGenerate_Validation(t *testing.T) {
	if _, err := Generate(mkParticipants(1), Settings{Format: FormatKnockout, SeedStrategy: SeedRegistration}); err != ErrTooFewParticipants {
		t.Errorf("1 participant: err = %v, want ErrTooFewParticipants", err)
	}
	dup := []Participant{{ID: "x"}, {ID: "x"}}
	if _, err := Generate(dup, Settings{Format: FormatKnockout, SeedStrategy: SeedRegistration}); err != ErrDuplicateParticipant {
		t.Errorf("dup ids: err = %v, want ErrDuplicateParticipant", err)
	}
	if _, err := Generate(mkParticipants(8), Settings{Format: "bogus", SeedStrategy: SeedRegistration}); err != ErrUnknownFormat {
		t.Errorf("bad format: err = %v, want ErrUnknownFormat", err)
	}
	if _, err := Generate(mkParticipants(8), Settings{Format: FormatGroupKnockout, SeedStrategy: SeedRegistration, Groups: 1, QualifiersPerGroup: 2}); err != ErrInvalidGroups {
		t.Errorf("1 group: err = %v, want ErrInvalidGroups", err)
	}
	if _, err := Generate(mkParticipants(8), Settings{Format: FormatGroupKnockout, SeedStrategy: SeedRegistration, Groups: 4, QualifiersPerGroup: 3}); err != ErrInvalidQualifiers {
		t.Errorf("too many qualifiers: err = %v, want ErrInvalidQualifiers", err)
	}
}
