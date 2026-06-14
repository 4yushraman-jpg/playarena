package fixtures

import "fmt"

// buildRoundRobin generates a round-robin (legs=1) or multi-leg league
// (legs=2 → home/away double round-robin) using the circle method.
//
// Correctness guarantees (proven by tests):
//   - every unordered pair meets exactly `legs` times (no missing, no duplicate),
//   - no participant ever plays itself,
//   - odd participant counts are handled by a rotating bye (one rest per round),
//   - home/away is balanced: a greedy orientation keeps each participant's
//     home-minus-away within 1 for a single leg; even legs are exact mirrors,
//     so an even number of legs is perfectly balanced.
func buildRoundRobin(seeded []Participant, legs int) []PlannedMatch {
	ids := make([]string, len(seeded))
	for i := range seeded {
		ids[i] = seeded[i].ID
	}

	pairs := circlePairs(len(seeded))
	roundsPerLeg := evenCount(len(seeded)) - 1

	// Orient leg 0 with a balanced (Eulerian) orientation so every participant's
	// home/away split differs by at most 1.
	orient := orientBalanced(len(seeded))
	type oriented struct{ home, away, round int }
	leg0 := make([]oriented, 0, len(pairs))
	for _, p := range pairs {
		h, a := homeAway(orient, p.a, p.b)
		leg0 = append(leg0, oriented{home: h, away: a, round: p.round})
	}

	matches := make([]PlannedMatch, 0, len(pairs)*legs)
	for leg := 0; leg < legs; leg++ {
		for _, o := range leg0 {
			h, a := o.home, o.away
			if leg%2 == 1 { // odd legs mirror leg 0 → exact home/away reversal
				h, a = a, h
			}
			round := o.round + leg*roundsPerLeg
			matches = append(matches, PlannedMatch{
				Index:       len(matches),
				RoundNumber: round,
				RoundName:   fmt.Sprintf("Round %d", round),
				Home:        concrete(ids[h]),
				Away:        concrete(ids[a]),
			})
		}
	}
	return matches
}

// pair is one unordered circle-method pairing in a given round.
type pair struct{ a, b, round int }

// circlePairs returns every pairing for a single round-robin over n participants
// using the circle method. Indices are into the seeded slice; a padding bye is
// used internally for odd n and its pairings are dropped.
func circlePairs(n int) []pair {
	const byeVal = -1
	arr := make([]int, 0, n+1)
	for i := 0; i < n; i++ {
		arr = append(arr, i)
	}
	if n%2 == 1 {
		arr = append(arr, byeVal)
	}
	m := len(arr)
	half := m / 2

	cur := make([]int, m)
	copy(cur, arr)

	out := make([]pair, 0, (m-1)*half)
	for r := 1; r <= m-1; r++ {
		for i := 0; i < half; i++ {
			a, b := cur[i], cur[m-1-i]
			if a == byeVal || b == byeVal {
				continue
			}
			out = append(out, pair{a: a, b: b, round: r})
		}
		rotateCircle(cur)
	}
	return out
}

// rotateCircle fixes cur[0] and rotates the remaining elements one step
// (the standard circle-method rotation): the last element moves to position 1.
func rotateCircle(cur []int) {
	m := len(cur)
	if m < 3 {
		return
	}
	last := cur[m-1]
	copy(cur[2:], cur[1:m-1])
	cur[1] = last
}

func evenCount(n int) int {
	if n%2 == 1 {
		return n + 1
	}
	return n
}
