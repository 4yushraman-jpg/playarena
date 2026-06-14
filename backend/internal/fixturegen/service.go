package fixturegen

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log/slog"
	"strings"
	"time"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/fixtures"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
)

// Service implements fixture generation use-cases.
type Service struct {
	repo *Repository
	log  *slog.Logger
}

func NewService(repo *Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Generate previews (dry_run) or persists a full fixture set for a tournament.
// The same request body (incl. random_seed) yields a byte-identical bracket, so
// the preview the director approves is exactly what gets persisted.
func (s *Service) Generate(ctx context.Context, orgSlug, tournamentID string, req GenerateRequest, actorID, actorOrgID string) (*GenerateResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrInvalidTournamentID
	}

	strategy, err := parseStrategy(req.SeedStrategy)
	if err != nil {
		return nil, err
	}

	// Resolve (and, for an un-seeded random preview, mint) the random seed so the
	// bracket is reproducible from it.
	var randomSeed int64
	if strategy == fixtures.SeedRandom {
		if req.RandomSeed != nil {
			randomSeed = *req.RandomSeed
		} else {
			randomSeed = newRandomSeed()
		}
	}

	legs := derefInt(req.Legs)
	groups := derefInt(req.Groups)
	qpg := derefInt(req.QualifiersPerGroup)

	buildPlan := func(participants []fixtures.Participant, rowFormat db.TournamentFormat) (*fixtures.Plan, error) {
		f, err := s.resolveFormat(req.Format, rowFormat)
		if err != nil {
			return nil, err
		}
		plan, err := fixtures.Generate(participants, fixtures.Settings{
			Format:             f,
			SeedStrategy:       strategy,
			RandomSeed:         randomSeed,
			Legs:               legs,
			Groups:             groups,
			QualifiersPerGroup: qpg,
		})
		if err != nil {
			return nil, mapEngineError(err)
		}
		return plan, nil
	}

	// ── Preview (no persistence) ──────────────────────────────────────────────
	if req.DryRun {
		t, err := s.repo.GetTournamentByID(ctx, tid, org.ID)
		if err != nil {
			return nil, err
		}
		participants, err := s.repo.ListApprovedParticipants(ctx, tid, t.ParticipantType)
		if err != nil {
			return nil, err
		}
		if len(participants) < 2 {
			return nil, ErrTooFewParticipants
		}
		plan, err := buildPlan(participants, t.Format)
		if err != nil {
			return nil, err
		}
		return buildResponse(plan, true, strategy, randomSeed, nil), nil
	}

	// ── Confirmed generation (atomic) ─────────────────────────────────────────
	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}
	expected := 0
	if req.ExpectedParticipants != nil {
		expected = *req.ExpectedParticipants
	}

	res, err := s.repo.Generate(ctx, GenerateParams{
		TournamentID:         tid,
		OrgID:                org.ID,
		ActorID:              actorUID,
		BuildPlan:            buildPlan,
		ExpectedParticipants: expected,
		GenerationMeta: func(plan *fixtures.Plan, at time.Time) map[string]any {
			return generationMeta(plan, actorID, strategy, randomSeed, legs, groups, qpg, at)
		},
	})
	if err != nil {
		return nil, err
	}

	at := res.GeneratedAt.Format(time.RFC3339)
	resp := buildResponse(res.Plan, false, strategy, randomSeed, &at)
	resp.Generated = res.Count
	return resp, nil
}

// ResolveQualifiers fills the knockout qualifier slots of a group_knockout
// tournament from the completed group standings (atomic, idempotent).
func (s *Service) ResolveQualifiers(ctx context.Context, orgSlug, tournamentID, actorID, actorOrgID string) (*ResolveQualifiersResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}
	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrInvalidTournamentID
	}
	n, err := s.repo.ResolveQualifiers(ctx, ResolveParams{TournamentID: tid, OrgID: org.ID})
	if err != nil {
		return nil, err
	}
	return &ResolveQualifiersResponse{SlotsFilled: n}, nil
}

// GetGenerationInfo returns the stored explainability record for a tournament.
func (s *Service) GetGenerationInfo(ctx context.Context, orgSlug, tournamentID string) (*GenerationInfo, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}
	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrInvalidTournamentID
	}
	t, err := s.repo.GetTournamentByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}
	info := GenerationInfoFromSettings(t.Settings)
	return &info, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────────

func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil // platform admin
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

func (s *Service) resolveFormat(override *string, rowFormat db.TournamentFormat) (fixtures.Format, error) {
	if override != nil && *override != "" {
		return parseFormat(*override)
	}
	return parseFormat(string(rowFormat))
}

func parseFormat(s string) (fixtures.Format, error) {
	switch fixtures.Format(strings.ToLower(strings.TrimSpace(s))) {
	case fixtures.FormatRoundRobin:
		return fixtures.FormatRoundRobin, nil
	case fixtures.FormatLeague:
		return fixtures.FormatLeague, nil
	case fixtures.FormatKnockout:
		return fixtures.FormatKnockout, nil
	case fixtures.FormatGroupKnockout:
		return fixtures.FormatGroupKnockout, nil
	default:
		return "", ErrInvalidFormat // double_elimination and unknowns rejected (FE-8D)
	}
}

func parseStrategy(s string) (fixtures.SeedStrategy, error) {
	switch fixtures.SeedStrategy(strings.ToLower(strings.TrimSpace(s))) {
	case fixtures.SeedManual:
		return fixtures.SeedManual, nil
	case fixtures.SeedRandom:
		return fixtures.SeedRandom, nil
	case fixtures.SeedRegistration:
		return fixtures.SeedRegistration, nil
	default:
		return "", ErrInvalidSeedStrategy
	}
}

// mapEngineError translates pure-engine sentinels to service errors.
func mapEngineError(err error) error {
	switch {
	case errors.Is(err, fixtures.ErrTooFewParticipants):
		return ErrTooFewParticipants
	case errors.Is(err, fixtures.ErrUnknownFormat):
		return ErrInvalidFormat
	case errors.Is(err, fixtures.ErrUnknownSeedStrategy):
		return ErrInvalidSeedStrategy
	case errors.Is(err, fixtures.ErrInvalidGroups), errors.Is(err, fixtures.ErrInvalidQualifiers):
		return ErrInvalidGroupConfig
	default:
		return err
	}
}

func buildResponse(plan *fixtures.Plan, dryRun bool, strategy fixtures.SeedStrategy, randomSeed int64, generatedAt *string) *GenerateResponse {
	numberByIndex := make(map[int]int, len(plan.Matches))
	for _, m := range plan.Matches {
		numberByIndex[m.Index] = m.MatchNumber
	}

	matches := make([]PreviewMatch, len(plan.Matches))
	for i, m := range plan.Matches {
		pm := PreviewMatch{
			RoundNumber: m.RoundNumber,
			RoundName:   m.RoundName,
			MatchNumber: m.MatchNumber,
			GroupLabel:  m.GroupLabel,
			Home:        toPreviewSlot(m.Home),
			Away:        toPreviewSlot(m.Away),
		}
		if m.NextMatchIndex != nil {
			n := numberByIndex[*m.NextMatchIndex]
			pm.NextMatchNumber = &n
			pm.NextMatchSlot = m.NextSlot
		}
		matches[i] = pm
	}

	var byes []PreviewBye
	for _, b := range plan.Byes {
		byes = append(byes, PreviewBye{ParticipantID: b.ParticipantID, IntoRound: b.IntoRound})
	}

	resp := &GenerateResponse{
		DryRun:       dryRun,
		Format:       string(plan.Settings.Format),
		SeedStrategy: string(strategy),
		SeedOrder:    plan.SeedOrder,
		Matches:      matches,
		Byes:         byes,
		Warnings:     plan.Warnings,
		GeneratedAt:  generatedAt,
	}
	if strategy == fixtures.SeedRandom {
		rs := randomSeed
		resp.RandomSeed = &rs
	}
	return resp
}

func toPreviewSlot(s fixtures.SlotRef) PreviewSlot {
	switch s.Kind {
	case fixtures.SlotConcrete:
		return PreviewSlot{Kind: "participant", ParticipantID: s.ID}
	case fixtures.SlotQualifier:
		return PreviewSlot{Kind: "qualifier", Group: s.Group, Rank: s.Rank}
	default:
		return PreviewSlot{Kind: "tbd"}
	}
}

func generationMeta(plan *fixtures.Plan, actorID string, strategy fixtures.SeedStrategy, randomSeed int64, legs, groups, qpg int, at time.Time) map[string]any {
	m := map[string]any{
		"generated_by":  actorID,
		"format":        string(plan.Settings.Format),
		"seed_strategy": string(strategy),
		"generated_at":  at.Format(time.RFC3339),
		"match_count":   len(plan.Matches),
		"seed_order":    plan.SeedOrder,
	}
	if strategy == fixtures.SeedRandom {
		m["random_seed"] = randomSeed
	}
	if legs > 0 {
		m["legs"] = legs
	}
	if groups > 0 {
		m["groups"] = groups
	}
	if qpg > 0 {
		m["qualifiers_per_group"] = qpg
	}
	return m
}

func newRandomSeed() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UnixNano() & ((1 << 52) - 1)
	}
	// Mask to 52 bits so the value round-trips exactly through a JSON number
	// (JavaScript safe-integer range), keeping random brackets reproducible from
	// the seed the client echoes back on confirm.
	return int64(binary.BigEndian.Uint64(b[:]) & ((1 << 52) - 1))
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
