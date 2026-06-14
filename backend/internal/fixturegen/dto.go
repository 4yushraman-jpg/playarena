package fixturegen

// GenerateRequest is the payload for
// POST /api/v1/organizations/{slug}/tournaments/{id}/fixtures/generate.
//
// dry_run = true returns the full preview WITHOUT persisting anything, so the
// wizard can show the exact structure before the director confirms. Confirming
// re-sends the same body (including random_seed) so the persisted bracket is
// byte-identical to the preview.
type GenerateRequest struct {
	// Format overrides the tournament's format for this generation (optional).
	Format       *string `json:"format"`
	SeedStrategy string  `json:"seed_strategy" validate:"required,oneof=manual random registration"`
	// RandomSeed is required for reproducible random seeding. When omitted on a
	// random preview, the server picks one and returns it for the director to
	// confirm with.
	RandomSeed         *int64 `json:"random_seed"`
	Legs               *int   `json:"legs"`
	Groups             *int   `json:"groups"`
	QualifiersPerGroup *int   `json:"qualifiers_per_group"`
	// ExpectedParticipants, when set on a confirm, guards against the approved set
	// changing since the preview (late registration). The wizard sends the count
	// it previewed; a mismatch forces a re-preview (ErrRegistrationsChanged).
	ExpectedParticipants *int `json:"expected_participants"`
	DryRun               bool `json:"dry_run"`
}

// PreviewSlot describes one side of a previewed match.
type PreviewSlot struct {
	// Kind: "participant" (known), "tbd" (filled by progression), or
	// "qualifier" (filled by group-stage resolution).
	Kind          string `json:"kind"`
	ParticipantID string `json:"participant_id,omitempty"`
	Group         string `json:"group,omitempty"`
	Rank          int    `json:"rank,omitempty"`
}

// PreviewMatch is one planned match in a generation preview/result.
type PreviewMatch struct {
	RoundNumber     int         `json:"round_number"`
	RoundName       string      `json:"round_name"`
	MatchNumber     int         `json:"match_number"`
	GroupLabel      string      `json:"group_label,omitempty"`
	Home            PreviewSlot `json:"home"`
	Away            PreviewSlot `json:"away"`
	NextMatchNumber *int        `json:"next_match_number,omitempty"`
	NextMatchSlot   *int        `json:"next_match_slot,omitempty"`
}

// PreviewBye reports a participant that advances without a round-1 match.
type PreviewBye struct {
	ParticipantID string `json:"participant_id"`
	IntoRound     int    `json:"into_round"`
}

// GenerateResponse is returned for both preview (dry_run) and confirmed
// generation. For a preview, Generated is 0 and GeneratedAt is nil.
type GenerateResponse struct {
	DryRun       bool           `json:"dry_run"`
	Generated    int            `json:"generated"`
	Format       string         `json:"format"`
	SeedStrategy string         `json:"seed_strategy"`
	RandomSeed   *int64         `json:"random_seed,omitempty"`
	SeedOrder    []string       `json:"seed_order"`
	Matches      []PreviewMatch `json:"matches"`
	Byes         []PreviewBye   `json:"byes,omitempty"`
	Warnings     []string       `json:"warnings,omitempty"`
	GeneratedAt  *string        `json:"generated_at,omitempty"`
}

// GenerationInfo is the explainability record (Deliverable 8), read back from
// the tournament's stored settings so a director can audit/reproduce a bracket.
type GenerationInfo struct {
	Generated          bool     `json:"generated"`
	GeneratedBy        string   `json:"generated_by,omitempty"`
	Format             string   `json:"format,omitempty"`
	SeedStrategy       string   `json:"seed_strategy,omitempty"`
	RandomSeed         *int64   `json:"random_seed,omitempty"`
	Legs               int      `json:"legs,omitempty"`
	Groups             int      `json:"groups,omitempty"`
	QualifiersPerGroup int      `json:"qualifiers_per_group,omitempty"`
	GeneratedAt        string   `json:"generated_at,omitempty"`
	MatchCount         int      `json:"match_count,omitempty"`
	SeedOrder          []string `json:"seed_order,omitempty"`
}

// ResolveQualifiersResponse reports how many knockout slots were filled.
type ResolveQualifiersResponse struct {
	SlotsFilled int `json:"slots_filled"`
}
