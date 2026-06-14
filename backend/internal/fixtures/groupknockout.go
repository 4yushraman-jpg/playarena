package fixtures

import "fmt"

// buildGroupKnockout generates a group stage (round-robin within each group)
// followed by a knockout phase seeded from group qualifiers.
//
// Algorithm (deterministic, documented):
//  1. Snake/serpentine distribution spreads seed strength across groups: seeds
//     are dealt A,B,…,G,G,…,B,A,A,B,… so no group is stacked with top seeds.
//  2. Each group plays a single round-robin (circle method, greedy home/away).
//  3. The knockout field is the top `QualifiersPerGroup` of every group. These
//     qualifier SLOTS (not concrete participants — results are unknown at
//     generation) are ordered RANK-MAJOR: all group winners (in group order),
//     then all runners-up, etc. Standard seed-slot placement over that order
//     pairs round 1 as winner-vs-runner-up of DIFFERENT groups (for an even
//     group count, no same-group round-1 rematch is possible), and places a
//     group's own qualifiers in opposite bracket halves.
//  4. The knockout is wired with FE-8B progression edges. Qualifier slots are
//     filled later by resolve-qualifiers once the group stage is complete.
func buildGroupKnockout(seeded []Participant, s Settings) ([]PlannedMatch, []string, error) {
	groups := distributeSnake(seeded, s.Groups)

	var matches []PlannedMatch
	var warnings []string
	maxGroupRounds := 0

	// ── Group stage ───────────────────────────────────────────────────────────
	for g, members := range groups {
		label := groupLabel(g)
		gm := buildGroupStage(members, label)
		for i := range gm {
			gm[i].Index = len(matches)
			if gm[i].RoundNumber > maxGroupRounds {
				maxGroupRounds = gm[i].RoundNumber
			}
			matches = append(matches, gm[i])
		}
	}

	// ── Qualifier ordering (RANK-MAJOR) ───────────────────────────────────────
	qualifiers := make([]SlotRef, 0, s.Groups*s.QualifiersPerGroup)
	for rank := 1; rank <= s.QualifiersPerGroup; rank++ {
		for g := 0; g < s.Groups; g++ {
			qualifiers = append(qualifiers, qualifier(groupLabel(g), rank))
		}
	}

	// ── Knockout phase over qualifier slots ───────────────────────────────────
	size := nextPow2(len(qualifiers))
	order := seedSlotOrder(size)
	leaves := make([]leaf, size)
	for pos, label := range order {
		if label <= len(qualifiers) {
			leaves[pos] = leaf{ref: qualifiers[label-1]}
		} else {
			leaves[pos] = leaf{bye: true}
		}
	}
	koMatches, _ := buildBracket(leaves, maxGroupRounds)

	base := len(matches)
	for i := range koMatches {
		koMatches[i].Index += base
		if koMatches[i].NextMatchIndex != nil {
			shifted := *koMatches[i].NextMatchIndex + base
			koMatches[i].NextMatchIndex = &shifted
		}
		matches = append(matches, koMatches[i])
	}

	if size != len(qualifiers) {
		warnings = append(warnings, fmt.Sprintf(
			"knockout field of %d qualifiers is not a power of two; %d bye(s) applied to top qualifier seeds",
			len(qualifiers), size-len(qualifiers)))
	}
	if s.Groups%2 == 1 {
		warnings = append(warnings,
			"odd group count: one round-1 knockout pairing may be a same-group rematch")
	}

	return matches, warnings, nil
}

// distributeSnake deals seeded participants into g groups serpentine-style.
func distributeSnake(seeded []Participant, g int) [][]Participant {
	groups := make([][]Participant, g)
	for i, p := range seeded {
		cycle := i / g
		pos := i % g
		gi := pos
		if cycle%2 == 1 { // reverse direction every other pass
			gi = g - 1 - pos
		}
		groups[gi] = append(groups[gi], p)
	}
	return groups
}

// buildGroupStage builds one group's single round-robin with greedy home/away
// orientation and a group label. Indices are local (fixed up by the caller).
func buildGroupStage(members []Participant, label string) []PlannedMatch {
	ids := make([]string, len(members))
	for i := range members {
		ids[i] = members[i].ID
	}
	pairs := circlePairs(len(members))
	orient := orientBalanced(len(members))

	out := make([]PlannedMatch, 0, len(pairs))
	for _, p := range pairs {
		h, a := homeAway(orient, p.a, p.b)
		out = append(out, PlannedMatch{
			RoundNumber: p.round,
			RoundName:   fmt.Sprintf("Group %s · Round %d", label, p.round),
			GroupLabel:  label,
			Home:        concrete(ids[h]),
			Away:        concrete(ids[a]),
		})
	}
	return out
}

// groupLabel maps a 0-based group index to a label: A..Z, then G27, G28, …
func groupLabel(i int) string {
	if i < 26 {
		return string(rune('A' + i))
	}
	return fmt.Sprintf("G%d", i+1)
}
