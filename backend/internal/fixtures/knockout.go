package fixtures

import "fmt"

// leaf is one round-1 bracket position: either a real entrant (ref) or a bye.
type leaf struct {
	ref SlotRef
	bye bool
}

// buildKnockout generates a seeded single-elimination bracket over the seeded
// participants. Top seeds receive byes when the field is not a power of two
// (seed protection), and every later-round match is pre-wired with its
// NextMatchIndex/NextSlot using the FE-8B progression model.
func buildKnockout(seeded []Participant) ([]PlannedMatch, []Bye) {
	n := len(seeded)
	size := nextPow2(n)
	order := seedSlotOrder(size) // label per leaf position, in seed-slot order

	leaves := make([]leaf, size)
	for pos, label := range order {
		if label <= n {
			leaves[pos] = leaf{ref: concrete(seeded[label-1].ID)}
		} else {
			leaves[pos] = leaf{bye: true}
		}
	}
	return buildBracket(leaves, 0)
}

// buildBracket is the pure bracket constructor shared by knockout and the
// knockout phase of group_knockout. leaves are bracketSize (a power of two)
// round-1 positions in seed-slot order; roundOffset shifts round numbers so a
// knockout phase can follow a group stage.
//
// Returns matches with LOCAL indices (0-based within this bracket) and Byes.
// Callers that embed the bracket in a larger plan offset Index/NextMatchIndex.
//
// Invariants (proven by tests): exactly size-1-byes matches; every non-final
// match has exactly one outgoing edge; every successor slot is fed by exactly
// one feeder or one concrete/bye placement; no orphan or unreachable match.
func buildBracket(leaves []leaf, roundOffset int) ([]PlannedMatch, []Bye) {
	size := len(leaves)
	rounds := log2(size)

	matches := make([]PlannedMatch, 0, size-1)
	var byes []Bye

	// ── Round 1: classify each pair as a real match or a bye promotion ────────
	r1nodes := size / 2
	prev := make([]contrib, r1nodes)
	for j := 0; j < r1nodes; j++ {
		la, lb := leaves[2*j], leaves[2*j+1]
		switch {
		case !la.bye && !lb.bye:
			idx := len(matches)
			matches = append(matches, PlannedMatch{
				Index:       idx,
				RoundNumber: roundOffset + 1,
				RoundName:   knockoutRoundName(size),
				Home:        la.ref,
				Away:        lb.ref,
			})
			prev[j] = contrib{matchIdx: idx}
		case la.bye && !lb.bye:
			prev[j] = contrib{ref: lb.ref, concrete: true}
			if lb.ref.Kind == SlotConcrete {
				byes = append(byes, Bye{ParticipantID: lb.ref.ID, IntoRound: roundOffset + 2})
			}
		case !la.bye && lb.bye:
			prev[j] = contrib{ref: la.ref, concrete: true}
			if la.ref.Kind == SlotConcrete {
				byes = append(byes, Bye{ParticipantID: la.ref.ID, IntoRound: roundOffset + 2})
			}
		default:
			// Both byes: impossible because byes < size/2 (n > size/2) and the
			// seed-slot order never pairs two phantom labels. Guarded for safety.
			prev[j] = contrib{ref: tbd()}
		}
	}

	// ── Rounds 2..R: every node is a real match fed by its two children ───────
	for r := 2; r <= rounds; r++ {
		nodes := size / pow2(r)
		cur := make([]contrib, nodes)
		playersInRound := size / pow2(r-1)
		for j := 0; j < nodes; j++ {
			idx := len(matches)
			m := PlannedMatch{
				Index:       idx,
				RoundNumber: roundOffset + r,
				RoundName:   knockoutRoundName(playersInRound),
				Home:        tbd(),
				Away:        tbd(),
			}
			// Wire child 2j → home (slot 1), child 2j+1 → away (slot 2).
			wireChild(&matches, &m.Home, prev[2*j], idx, 1)
			wireChild(&matches, &m.Away, prev[2*j+1], idx, 2)
			matches = append(matches, m)
			cur[j] = contrib{matchIdx: idx}
		}
		prev = cur
	}

	return matches, byes
}

// contrib describes what a bracket node hands up to its parent slot: either a
// concrete/qualifier ref (a bye placement) or a feeding match (by index).
type contrib struct {
	ref      SlotRef
	concrete bool
	matchIdx int
}

// wireChild fills one parent slot from a child contribution: a concrete/qualifier
// child becomes the slot value directly (a bye placement); a feeding child leaves
// the slot TBD and gets its NextMatchIndex/NextSlot pointed at the parent.
func wireChild(matches *[]PlannedMatch, slot *SlotRef, c contrib, parentIdx, parentSlot int) {
	if c.concrete {
		*slot = c.ref
		return
	}
	*slot = tbd()
	parent := parentIdx
	ps := parentSlot
	(*matches)[c.matchIdx].NextMatchIndex = &parent
	(*matches)[c.matchIdx].NextSlot = &ps
}

// knockoutRoundName names a round by how many entrants begin it.
func knockoutRoundName(playersInRound int) string {
	switch playersInRound {
	case 2:
		return "Final"
	case 4:
		return "Semi-final"
	case 8:
		return "Quarter-final"
	default:
		return fmt.Sprintf("Round of %d", playersInRound)
	}
}

// seedSlotOrder returns, for a power-of-two bracket, the seed label occupying
// each leaf position so that top seeds meet as late as possible (1 vs N, etc.).
func seedSlotOrder(size int) []int {
	slots := []int{1}
	for len(slots) < size {
		sum := len(slots)*2 + 1
		next := make([]int, 0, len(slots)*2)
		for _, s := range slots {
			next = append(next, s, sum-s)
		}
		slots = next
	}
	return slots
}

func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

func pow2(k int) int { return 1 << k }

func log2(n int) int {
	k := 0
	for (1 << k) < n {
		k++
	}
	return k
}
