package fixtures

// Generate is the single entry point. It validates input, resolves the seed
// order, dispatches to the format builder, and assigns global match numbers.
// It is pure and deterministic: identical inputs always yield identical Plans.
func Generate(participants []Participant, s Settings) (*Plan, error) {
	if len(participants) < 2 {
		return nil, ErrTooFewParticipants
	}
	if err := assertUniqueIDs(participants); err != nil {
		return nil, err
	}

	seeded, err := resolveSeedOrder(participants, s)
	if err != nil {
		return nil, err
	}

	plan := &Plan{
		Settings:  s,
		SeedOrder: seedOrderIDs(seeded),
	}

	switch s.Format {
	case FormatRoundRobin:
		legs := s.Legs
		if legs <= 0 {
			legs = 1
		}
		plan.Matches = buildRoundRobin(seeded, legs)
	case FormatLeague:
		legs := s.Legs
		if legs <= 0 {
			legs = 2 // league defaults to double round-robin (home + away)
		}
		plan.Matches = buildRoundRobin(seeded, legs)
	case FormatKnockout:
		matches, byes := buildKnockout(seeded)
		plan.Matches = matches
		plan.Byes = byes
	case FormatGroupKnockout:
		if err := validateGroupConfig(len(seeded), s); err != nil {
			return nil, err
		}
		matches, warnings, err := buildGroupKnockout(seeded, s)
		if err != nil {
			return nil, err
		}
		plan.Matches = matches
		plan.Warnings = append(plan.Warnings, warnings...)
	default:
		return nil, ErrUnknownFormat
	}

	assignMatchNumbers(plan.Matches)
	return plan, nil
}

func assertUniqueIDs(p []Participant) error {
	seen := make(map[string]struct{}, len(p))
	for i := range p {
		if _, ok := seen[p[i].ID]; ok {
			return ErrDuplicateParticipant
		}
		seen[p[i].ID] = struct{}{}
	}
	return nil
}

// assignMatchNumbers stamps a stable 1-based display order. Matches are already
// emitted in round order by each builder; numbering follows that order so the
// fixture list reads top-to-bottom by round.
func assignMatchNumbers(matches []PlannedMatch) {
	for i := range matches {
		matches[i].MatchNumber = i + 1
	}
}

func validateGroupConfig(n int, s Settings) error {
	if s.Groups < 2 || s.Groups > n {
		return ErrInvalidGroups
	}
	if s.QualifiersPerGroup < 1 {
		return ErrInvalidQualifiers
	}
	// Every group must be able to supply the requested number of qualifiers, and
	// the knockout field (groups × qualifiers) must be at least 2.
	smallestGroup := n / s.Groups // floor — the smallest group size after even split
	if s.QualifiersPerGroup > smallestGroup {
		return ErrInvalidQualifiers
	}
	if s.Groups*s.QualifiersPerGroup < 2 {
		return ErrInvalidQualifiers
	}
	return nil
}
