// Package fixtures is a pure, deterministic tournament-fixture generation
// engine. It has NO database access, NO I/O, NO global state, and NO hidden
// randomness — randomness is confined to an explicit, caller-supplied seed.
//
// Given a format, a set of participants, a seeding strategy, and (for
// group_knockout) a group configuration, it produces a Plan: a flat list of
// PlannedMatches that together describe the fixture graph, the knockout bracket
// graph, and the progression graph (next-match wiring, mirroring the FE-8B
// linkage model). The same inputs always produce byte-identical output, so a
// tournament director can reproduce and audit any bracket exactly.
package fixtures

import "errors"

// Format is the structural tournament format. Mirrors the DB tournament_format
// enum, restricted to the formats FE-8C supports (double_elimination is FE-8D).
type Format string

const (
	FormatRoundRobin    Format = "round_robin"
	FormatLeague        Format = "league"
	FormatKnockout      Format = "knockout"
	FormatGroupKnockout Format = "group_knockout"
)

// SeedStrategy determines how participants are ordered before placement. The
// ordering is the single source of all structure, so it is fully explainable.
type SeedStrategy string

const (
	// SeedManual orders by the organiser-assigned seed_number (ascending),
	// falling back to registration time then id for any unseeded participants.
	SeedManual SeedStrategy = "manual"
	// SeedRandom shuffles deterministically from RandomSeed (Fisher–Yates over a
	// fixed PRNG). The same RandomSeed always yields the same order.
	SeedRandom SeedStrategy = "random"
	// SeedRegistration orders by registration time (then id) — first-come bracket.
	SeedRegistration SeedStrategy = "registration"
)

// Participant is one approved registrant. ID is the team or player UUID (the
// engine is agnostic to which — the caller maps it to the right column).
type Participant struct {
	ID           string
	SeedNumber   *int  // organiser-assigned seed, nil when unseeded
	RegisteredAt int64 // unix nanos; deterministic tiebreaker
}

// Settings is the full generation configuration.
type Settings struct {
	Format       Format
	SeedStrategy SeedStrategy
	// RandomSeed is used (and must be stored) when SeedStrategy == SeedRandom.
	RandomSeed int64
	// Legs is the number of times every pair meets in round_robin/league.
	// Defaults: round_robin → 1, league → 2 (home/away). Knockout ignores it.
	Legs int
	// Groups and QualifiersPerGroup configure group_knockout.
	Groups             int
	QualifiersPerGroup int
}

// SlotKind classifies what fills a match slot at generation time.
type SlotKind int

const (
	// SlotConcrete: a known participant (ID set).
	SlotConcrete SlotKind = iota
	// SlotTBD: filled later by FE-8B winner propagation from a feeder match.
	SlotTBD
	// SlotQualifier: filled later by group-stage qualifier resolution
	// (Group + Rank identify the group standings position).
	SlotQualifier
)

// SlotRef describes one side of a planned match.
type SlotRef struct {
	Kind  SlotKind
	ID    string // SlotConcrete
	Group string // SlotQualifier — group label, e.g. "A"
	Rank  int    // SlotQualifier — 1-based finishing rank within the group
}

func concrete(id string) SlotRef { return SlotRef{Kind: SlotConcrete, ID: id} }
func tbd() SlotRef               { return SlotRef{Kind: SlotTBD} }
func qualifier(group string, rank int) SlotRef {
	return SlotRef{Kind: SlotQualifier, Group: group, Rank: rank}
}

// PlannedMatch is one match in the plan. Index is its position in Plan.Matches
// and is the stable handle used by NextMatchIndex for progression wiring.
type PlannedMatch struct {
	Index       int
	RoundNumber int
	RoundName   string
	MatchNumber int    // 1-based display order across the whole plan
	GroupLabel  string // "" unless a group-stage match

	Home SlotRef
	Away SlotRef

	// Progression edge (FE-8B): the plan index of the match this winner advances
	// into, and the slot there (1 = home, 2 = away). Nil for finals, round-robin,
	// league, and group-stage matches (which have no single successor).
	NextMatchIndex *int
	NextSlot       *int
}

// Bye records a participant that advanced a round without playing (knockout
// seed protection). No match row is emitted for a bye — the participant is
// placed directly into its next-round slot — so byes are reported here purely
// for explainability.
type Bye struct {
	ParticipantID string
	IntoRound     int
}

// Plan is the complete, deterministic output of generation.
type Plan struct {
	Matches   []PlannedMatch
	Byes      []Bye
	SeedOrder []string // resolved participant order (ids), the root of all structure
	Settings  Settings
	Warnings  []string
}

// Engine errors. All are sentinel values so callers can map them to API errors.
var (
	ErrTooFewParticipants   = errors.New("at least 2 participants are required to generate fixtures")
	ErrUnknownFormat        = errors.New("unknown tournament format")
	ErrUnknownSeedStrategy  = errors.New("unknown seed strategy")
	ErrInvalidGroups        = errors.New("invalid group configuration")
	ErrInvalidQualifiers    = errors.New("invalid qualifiers-per-group configuration")
	ErrDuplicateParticipant = errors.New("duplicate participant id in input")
)
