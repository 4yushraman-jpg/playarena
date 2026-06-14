package fixtures

import (
	"math/rand"
	"sort"
)

// resolveSeedOrder returns participants in deterministic seed order according to
// the strategy. The result is a pure function of the inputs (including
// settings.RandomSeed for SeedRandom), so it is fully reproducible.
//
// A total order is always established (final tiebreak on ID) so there is never
// any dependence on map iteration or input order beyond what the strategy
// defines.
func resolveSeedOrder(participants []Participant, s Settings) ([]Participant, error) {
	out := make([]Participant, len(participants))
	copy(out, participants)

	switch s.SeedStrategy {
	case SeedRegistration:
		sortByRegistration(out)

	case SeedManual:
		// Seeded participants first (by seed number), then unseeded by
		// registration. ID is the final, always-present tiebreaker.
		sort.SliceStable(out, func(i, j int) bool {
			si, sj := out[i].SeedNumber, out[j].SeedNumber
			switch {
			case si != nil && sj != nil:
				if *si != *sj {
					return *si < *sj
				}
			case si != nil:
				return true // seeded sorts before unseeded
			case sj != nil:
				return false
			}
			return lessByRegistration(out[i], out[j])
		})

	case SeedRandom:
		// Establish a deterministic base order first, then a seeded Fisher–Yates
		// shuffle. Same RandomSeed ⇒ identical permutation, every time.
		sortByRegistration(out)
		rng := rand.New(rand.NewSource(s.RandomSeed))
		for i := len(out) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			out[i], out[j] = out[j], out[i]
		}

	default:
		return nil, ErrUnknownSeedStrategy
	}

	return out, nil
}

func sortByRegistration(p []Participant) {
	sort.SliceStable(p, func(i, j int) bool { return lessByRegistration(p[i], p[j]) })
}

func lessByRegistration(a, b Participant) bool {
	if a.RegisteredAt != b.RegisteredAt {
		return a.RegisteredAt < b.RegisteredAt
	}
	return a.ID < b.ID
}

// seedOrderIDs extracts the resolved id order for explainability.
func seedOrderIDs(p []Participant) []string {
	ids := make([]string, len(p))
	for i := range p {
		ids[i] = p[i].ID
	}
	return ids
}
