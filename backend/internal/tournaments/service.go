package tournaments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/notifications"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/rankings"
	"github.com/4yushraman-jpg/playarena/internal/standings"
)

var (
	reCurrency    = regexp.MustCompile(`^[A-Z]{3}$`)
	reNonSlugChar = regexp.MustCompile(`[^a-z0-9]+`)

	// allowedTransitions defines the permitted lifecycle moves.
	// Any status may transition to cancelled.
	// completed → cancelled is allowed for admin corrections.
	// cancelled is a terminal state: no transitions out.
	allowedTransitions = map[db.TournamentStatus][]db.TournamentStatus{
		db.TournamentStatusDraft: {
			db.TournamentStatusRegistrationOpen,
			db.TournamentStatusCancelled,
		},
		db.TournamentStatusRegistrationOpen: {
			db.TournamentStatusRegistrationClosed,
			db.TournamentStatusCancelled,
		},
		db.TournamentStatusRegistrationClosed: {
			db.TournamentStatusOngoing,
			db.TournamentStatusCancelled,
		},
		db.TournamentStatusOngoing: {
			db.TournamentStatusCompleted,
			db.TournamentStatusCancelled,
		},
		db.TournamentStatusCompleted: {
			db.TournamentStatusCancelled,
		},
		db.TournamentStatusCancelled: {}, // terminal
	}
)

// Service implements tournament use-cases.
type Service struct {
	repo         *Repository
	log          *slog.Logger
	notifSvc     *notifications.Service
	rankingsRepo *rankings.Repository // nil is valid: snapshot is skipped
}

// NewService constructs a Service.
func NewService(repo *Repository, log *slog.Logger, notifSvc *notifications.Service, rankingsRepo *rankings.Repository) *Service {
	return &Service{repo: repo, log: log, notifSvc: notifSvc, rankingsRepo: rankingsRepo}
}

// ── tournament CRUD ───────────────────────────────────────────────────────────

// Create registers a new tournament in draft status.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Create(ctx context.Context, orgSlug string, req CreateRequest, actorID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	format, err := parseTournamentFormat(req.Format)
	if err != nil {
		return nil, err
	}

	participantType, err := parseParticipantType(req.ParticipantType)
	if err != nil {
		return nil, err
	}

	currency, err := normalizeCurrency(req.Currency)
	if err != nil {
		return nil, err
	}

	country, err := normalizeCountry(req.Country)
	if err != nil {
		return nil, err
	}

	prizePool, err := parsePrizePool(req.PrizePool)
	if err != nil {
		return nil, err
	}

	regOpensAt, err := parseTimestamp(req.RegistrationOpensAt)
	if err != nil {
		return nil, err
	}
	regClosesAt, err := parseTimestamp(req.RegistrationClosesAt)
	if err != nil {
		return nil, err
	}
	startsAt, err := parseTimestamp(req.StartsAt)
	if err != nil {
		return nil, err
	}
	endsAt, err := parseTimestamp(req.EndsAt)
	if err != nil {
		return nil, err
	}

	if err := validateDateRange(regOpensAt, regClosesAt, startsAt, endsAt); err != nil {
		return nil, err
	}

	baseSlug := generateTournamentSlug(req.Name)
	params := db.CreateTournamentParams{
		OrganizationID:       org.ID,
		Name:                 strings.TrimSpace(req.Name),
		Description:          req.Description,
		Sport:                strings.ToLower(strings.TrimSpace(req.Sport)),
		Format:               format,
		ParticipantType:      participantType,
		BannerUrl:            req.BannerURL,
		PrizePool:            prizePool,
		Currency:             currency,
		MaxParticipants:      req.MaxParticipants,
		MinParticipants:      req.MinParticipants,
		RegistrationOpensAt:  regOpensAt,
		RegistrationClosesAt: regClosesAt,
		StartsAt:             startsAt,
		EndsAt:               endsAt,
		Venue:                req.Venue,
		City:                 req.City,
		Country:              country,
		Rules:                req.Rules,
		CreatedBy:            actorUID,
	}

	var t *db.Tournament
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt == 1 {
			params.Slug = baseSlug
		} else {
			params.Slug = fmt.Sprintf("%s-%d", baseSlug, attempt)
		}
		t, err = s.repo.CreateWithAudit(ctx, createTournamentTxParams{
			createParams: params,
			actorID:      actorUID,
		})
		if errors.Is(err, ErrSlugAlreadyTaken) {
			continue
		}
		break
	}
	if err != nil {
		if errors.Is(err, ErrSlugAlreadyTaken) {
			return nil, ErrSlugGenerationFailed
		}
		return nil, err
	}
	resp := tournamentToResponse(t)
	// A new tournament has no registrations yet; attach explicit zero counts
	// so the response shape matches get/list/update.
	resp.RegistrationCounts = &RegistrationCountsResponse{}
	return resp, nil
}

// List returns a paginated page of non-cancelled tournaments for an organization.
// No ownership check: any authenticated user may list any org's tournaments.
func (s *Service) List(ctx context.Context, orgSlug string, params ListParams) (*ListResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if params.Limit <= 0 || params.Limit > MaxListLimit {
		params.Limit = DefaultListLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	ts, err := s.repo.List(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	total, err := s.repo.Count(ctx, org.ID, params)
	if err != nil {
		return nil, err
	}

	resp := make([]Response, len(ts))
	ptrs := make([]*Response, len(ts))
	for i := range ts {
		resp[i] = *tournamentToResponse(&ts[i])
		ptrs[i] = &resp[i]
	}
	if err := s.attachRegistrationCounts(ctx, ptrs...); err != nil {
		return nil, err
	}
	return &ListResponse{
		Tournaments: resp,
		Total:       total,
		Limit:       int(params.Limit),
		Offset:      int(params.Offset),
	}, nil
}

// GetByID retrieves a single tournament by UUID within an organization.
// No ownership check: any authenticated user may read tournament details.
//
// Cancelled tournaments (soft-deleted via DELETE) are intentionally returned.
// Status "cancelled" in the response signals that the tournament no longer
// runs. Records are retained so that future registration and match references
// remain resolvable.
func (s *Service) GetByID(ctx context.Context, orgSlug, tournamentID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}

	t, err := s.repo.GetByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}
	resp := tournamentToResponse(t)
	if err := s.attachRegistrationCounts(ctx, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Update applies a partial update to a tournament.
// Status changes are validated against the allowed transition table.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Update(ctx context.Context, orgSlug, tournamentID string, req UpdateRequest, actorID, actorOrgID string) (*Response, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}

	current, err := s.repo.GetByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return nil, errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(tournamentToResponse(current))
	if err != nil {
		return nil, err
	}

	// Start with current state; apply non-nil request fields over it.
	params := db.UpdateTournamentParams{
		ID:                   current.ID,
		OrganizationID:       current.OrganizationID,
		Name:                 current.Name,
		Description:          current.Description,
		Sport:                current.Sport,
		Format:               current.Format,
		ParticipantType:      current.ParticipantType,
		BannerUrl:            current.BannerUrl,
		PrizePool:            current.PrizePool,
		Currency:             current.Currency,
		MaxParticipants:      current.MaxParticipants,
		MinParticipants:      current.MinParticipants,
		RegistrationOpensAt:  current.RegistrationOpensAt,
		RegistrationClosesAt: current.RegistrationClosesAt,
		StartsAt:             current.StartsAt,
		EndsAt:               current.EndsAt,
		Venue:                current.Venue,
		City:                 current.City,
		Country:              current.Country,
		Rules:                current.Rules,
		Status:               current.Status,
	}

	if req.Name != nil {
		params.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		params.Description = req.Description
	}
	if req.Sport != nil {
		params.Sport = strings.ToLower(strings.TrimSpace(*req.Sport))
	}
	if req.Format != nil {
		f, err := parseTournamentFormat(*req.Format)
		if err != nil {
			return nil, err
		}
		params.Format = f
	}
	if req.ParticipantType != nil {
		pt, err := parseParticipantType(req.ParticipantType)
		if err != nil {
			return nil, err
		}
		params.ParticipantType = pt
	}
	if req.BannerURL != nil {
		params.BannerUrl = req.BannerURL
	}
	if req.PrizePool != nil {
		pp, err := parsePrizePool(req.PrizePool)
		if err != nil {
			return nil, err
		}
		params.PrizePool = pp
	}
	if req.Currency != nil {
		c, err := normalizeCurrency(req.Currency)
		if err != nil {
			return nil, err
		}
		params.Currency = c
	}
	if req.MaxParticipants != nil {
		params.MaxParticipants = req.MaxParticipants
	}
	if req.MinParticipants != nil {
		params.MinParticipants = req.MinParticipants
	}
	if req.RegistrationOpensAt != nil {
		ts, err := parseTimestamp(req.RegistrationOpensAt)
		if err != nil {
			return nil, err
		}
		params.RegistrationOpensAt = ts
	}
	if req.RegistrationClosesAt != nil {
		ts, err := parseTimestamp(req.RegistrationClosesAt)
		if err != nil {
			return nil, err
		}
		params.RegistrationClosesAt = ts
	}
	if req.StartsAt != nil {
		ts, err := parseTimestamp(req.StartsAt)
		if err != nil {
			return nil, err
		}
		params.StartsAt = ts
	}
	if req.EndsAt != nil {
		ts, err := parseTimestamp(req.EndsAt)
		if err != nil {
			return nil, err
		}
		params.EndsAt = ts
	}
	if req.Venue != nil {
		params.Venue = req.Venue
	}
	if req.City != nil {
		params.City = req.City
	}
	if req.Country != nil {
		c, err := normalizeCountry(req.Country)
		if err != nil {
			return nil, err
		}
		params.Country = c
	}
	if req.Rules != nil {
		params.Rules = req.Rules
	}
	if req.Status != nil {
		newStatus, err := parseTournamentStatus(*req.Status)
		if err != nil {
			return nil, err
		}
		if err := validateStatusTransition(current.Status, newStatus); err != nil {
			return nil, err
		}
		params.Status = newStatus
	}

	if err := validateDateRange(
		params.RegistrationOpensAt, params.RegistrationClosesAt,
		params.StartsAt, params.EndsAt,
	); err != nil {
		return nil, err
	}

	updated, err := s.repo.UpdateWithAudit(ctx, updateTournamentTxParams{
		updateParams:   params,
		actorID:        actorUID,
		oldData:        oldData,
		previousStatus: current.Status,
	})
	if err != nil {
		return nil, err
	}

	if updated.Status == db.TournamentStatusCompleted && s.rankingsRepo != nil {
		s.snapshotTournamentStats(ctx, updated, org.ID)
	}

	// Synchronous post-commit drain.
	s.notifSvc.DrainOutbox(ctx, org.ID, s.log)

	// Clients write this response straight into their tournament-detail cache,
	// so it must carry the same registration_counts shape as GetByID.
	resp := tournamentToResponse(updated)
	if err := s.attachRegistrationCounts(ctx, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Delete soft-cancels the tournament (status → cancelled).
// Records are retained permanently for future registration and match references.
// BOLA guard: actorOrgID must match the target org or be empty (platform admin).
func (s *Service) Delete(ctx context.Context, orgSlug, tournamentID string, actorID, actorOrgID string) error {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return err
	}

	if err := assertOrgOwnership(actorOrgID, pgutil.UUIDToString(org.ID)); err != nil {
		return err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return ErrTournamentNotFound
	}

	current, err := s.repo.GetByID(ctx, tid, org.ID)
	if err != nil {
		return err
	}

	actorUID, err := pgutil.ParseUUID(actorID)
	if err != nil {
		return errors.New("invalid actor user id")
	}

	oldData, err := json.Marshal(tournamentToResponse(current))
	if err != nil {
		return err
	}

	if err := s.repo.CancelWithAudit(ctx, cancelTournamentTxParams{
		id:             current.ID,
		orgID:          current.OrganizationID,
		actorID:        actorUID,
		oldData:        oldData,
		previousStatus: current.Status,
	}); err != nil {
		return err
	}

	// Synchronous post-commit drain.
	s.notifSvc.DrainOutbox(ctx, org.ID, s.log)

	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// attachRegistrationCounts populates RegistrationCounts on each response using
// a single grouped query. Tournaments with no registrations get explicit zeros.
func (s *Service) attachRegistrationCounts(ctx context.Context, resps ...*Response) error {
	if len(resps) == 0 {
		return nil
	}

	ids := make([]pgtype.UUID, 0, len(resps))
	for _, r := range resps {
		uid, err := pgutil.ParseUUID(r.ID)
		if err != nil {
			continue
		}
		ids = append(ids, uid)
	}

	rows, err := s.repo.CountRegistrationsByStatus(ctx, ids)
	if err != nil {
		return err
	}

	counts := make(map[string]*RegistrationCountsResponse, len(resps))
	for _, row := range rows {
		id := pgutil.UUIDToString(row.TournamentID)
		c := counts[id]
		if c == nil {
			c = &RegistrationCountsResponse{}
			counts[id] = c
		}
		switch row.Status {
		case db.RegistrationStatusPending:
			c.Pending = row.Count
		case db.RegistrationStatusApproved:
			c.Approved = row.Count
		case db.RegistrationStatusRejected:
			c.Rejected = row.Count
		case db.RegistrationStatusWithdrawn:
			c.Withdrawn = row.Count
		case db.RegistrationStatusDisqualified:
			c.Disqualified = row.Count
		}
	}

	for _, r := range resps {
		c := counts[r.ID]
		if c == nil {
			c = &RegistrationCountsResponse{}
		}
		c.Active = c.Pending + c.Approved
		c.Total = c.Pending + c.Approved + c.Rejected + c.Withdrawn + c.Disqualified
		r.RegistrationCounts = c
	}
	return nil
}

// resolveParticipantNames batch-resolves team and player display names for a
// set of participant IDs. A participant ID appears in exactly one of the two
// tables, so both lookups are merged into a single map.
func (s *Service) resolveParticipantNames(ctx context.Context, participantIDs []string) (map[string]string, error) {
	names := make(map[string]string, len(participantIDs))
	if len(participantIDs) == 0 {
		return names, nil
	}

	ids := make([]pgtype.UUID, 0, len(participantIDs))
	for _, id := range participantIDs {
		uid, err := pgutil.ParseUUID(id)
		if err != nil {
			continue
		}
		ids = append(ids, uid)
	}

	teams, err := s.repo.GetTeamNamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range teams {
		names[pgutil.UUIDToString(t.ID)] = t.Name
	}

	players, err := s.repo.GetPlayerNamesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, p := range players {
		names[pgutil.UUIDToString(p.ID)] = p.DisplayName
	}
	return names, nil
}

// snapshotTournamentStats upserts final standings into the rankings tables.
// Called synchronously after a tournament transitions to completed.
// Errors are logged but do not fail the PATCH response.
func (s *Service) snapshotTournamentStats(ctx context.Context, t *db.Tournament, orgID pgtype.UUID) {
	rawMatches, err := s.repo.GetCompletedMatchesForStandings(ctx, t.ID, orgID)
	if err != nil {
		s.log.ErrorContext(ctx, "rankings.snapshot: fetch matches failed",
			slog.String("tournament_id", pgutil.UUIDToString(t.ID)),
			slog.Any("error", err),
		)
		return
	}

	rawRegs, err := s.repo.GetRegistrationsForStandings(ctx, t.ID)
	if err != nil {
		s.log.ErrorContext(ctx, "rankings.snapshot: fetch registrations failed",
			slog.String("tournament_id", pgutil.UUIDToString(t.ID)),
			slog.Any("error", err),
		)
		return
	}

	settings := parseStandingsSettings(t.Settings)

	matches := make([]standings.CompletedMatch, 0, len(rawMatches))
	for _, m := range rawMatches {
		cm := standings.CompletedMatch{
			HomeParticipantID: participantID(m.HomeTeamID, m.HomePlayerID),
			AwayParticipantID: participantID(m.AwayTeamID, m.AwayPlayerID),
			HomeScore:         int(m.HomeScore),
			AwayScore:         int(m.AwayScore),
			WinnerID:          participantID(m.WinnerTeamID, m.WinnerPlayerID),
			IsWalkover:        m.IsWalkover,
		}
		if cm.HomeParticipantID == "" || cm.AwayParticipantID == "" {
			continue
		}
		matches = append(matches, cm)
	}

	regs := make([]standings.RegistrationInfo, 0, len(rawRegs))
	for _, reg := range rawRegs {
		pid := participantID(reg.TeamID, reg.PlayerID)
		if pid == "" {
			continue
		}
		regs = append(regs, standings.RegistrationInfo{
			ParticipantID: pid,
			SeedNumber:    reg.SeedNumber,
			RegisteredAt:  reg.RegisteredAt.Time.UTC(),
		})
	}

	rows := standings.Compute(matches, regs, settings)

	statsRows := make([]rankings.StatsRow, len(rows))
	for i, row := range rows {
		statsRows[i] = rankings.StatsRow{
			ParticipantID: row.ParticipantID,
			Position:      row.Position,
			Played:        row.Played,
			Wins:          row.Wins,
			Draws:         row.Draws,
			Losses:        row.Losses,
			Points:        row.Points,
			ScoreFor:      row.ScoreFor,
			ScoreAgainst:  row.ScoreAgainst,
		}
	}

	var snapErr error
	if t.ParticipantType == db.ParticipantTypeIndividual {
		snapErr = s.rankingsRepo.SnapshotPlayerStats(ctx, orgID, t.ID, statsRows)
	} else {
		snapErr = s.rankingsRepo.SnapshotTeamStats(ctx, orgID, t.ID, statsRows)
	}
	if snapErr != nil {
		s.log.ErrorContext(ctx, "rankings.snapshot: upsert failed",
			slog.String("tournament_id", pgutil.UUIDToString(t.ID)),
			slog.Any("error", snapErr),
		)
	}
}

func assertOrgOwnership(actorOrgID, targetOrgID string) error {
	if actorOrgID == "" {
		return nil
	}
	if actorOrgID != targetOrgID {
		return ErrForbidden
	}
	return nil
}

func generateTournamentSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = reNonSlugChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 90 {
		s = s[:90]
		s = strings.TrimRight(s, "-")
	}
	if len(s) < 3 {
		s = "tournament-" + s
	}
	if len(s) < 3 {
		s = "tournament-x"
	}
	return s
}

func parseTournamentFormat(s string) (db.TournamentFormat, error) {
	f := db.TournamentFormat(strings.ToLower(strings.TrimSpace(s)))
	switch f {
	case db.TournamentFormatLeague, db.TournamentFormatKnockout,
		db.TournamentFormatGroupKnockout, db.TournamentFormatRoundRobin,
		db.TournamentFormatDoubleElimination:
		return f, nil
	}
	return "", ErrInvalidFormat
}

func parseParticipantType(s *string) (db.ParticipantType, error) {
	if s == nil || *s == "" {
		return db.ParticipantTypeTeam, nil
	}
	pt := db.ParticipantType(strings.ToLower(strings.TrimSpace(*s)))
	switch pt {
	case db.ParticipantTypeTeam, db.ParticipantTypeIndividual:
		return pt, nil
	}
	return "", ErrInvalidParticipantType
}

func parseTournamentStatus(s string) (db.TournamentStatus, error) {
	st := db.TournamentStatus(strings.ToLower(strings.TrimSpace(s)))
	switch st {
	case db.TournamentStatusDraft, db.TournamentStatusRegistrationOpen,
		db.TournamentStatusRegistrationClosed, db.TournamentStatusOngoing,
		db.TournamentStatusCompleted, db.TournamentStatusCancelled:
		return st, nil
	}
	return "", ErrInvalidStatus
}

func validateStatusTransition(from, to db.TournamentStatus) error {
	targets, ok := allowedTransitions[from]
	if !ok {
		return ErrInvalidStatusTransition
	}
	for _, t := range targets {
		if t == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s → %s", ErrInvalidStatusTransition, from, to)
}

func normalizeCurrency(s *string) (string, error) {
	if s == nil || *s == "" {
		return DefaultCurrency, nil
	}
	c := strings.ToUpper(strings.TrimSpace(*s))
	if !reCurrency.MatchString(c) {
		return "", ErrInvalidCurrency
	}
	return c, nil
}

func normalizeCountry(s *string) (*string, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	code := strings.ToUpper(strings.TrimSpace(*s))
	if len(code) != 2 {
		return nil, ErrInvalidCountry
	}
	return &code, nil
}

func parsePrizePool(s *string) (pgtype.Numeric, error) {
	if s == nil || *s == "" {
		return pgtype.Numeric{}, nil
	}
	var n pgtype.Numeric
	if err := n.Scan(*s); err != nil {
		return pgtype.Numeric{}, ErrInvalidPrizePool
	}
	return n, nil
}

func numericToString(n pgtype.Numeric) *string {
	if !n.Valid {
		return nil
	}
	v, err := n.Value()
	if err != nil || v == nil {
		return nil
	}
	s := fmt.Sprintf("%v", v)
	return &s
}

func parseTimestamp(s *string) (pgtype.Timestamptz, error) {
	if s == nil || *s == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidTimestamp
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

func timestampToString(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.UTC().Format(time.RFC3339)
	return &s
}

// validateDateRange enforces:
//
//	registration_opens_at  < registration_closes_at  (if both set)
//	registration_closes_at <= starts_at              (if both set)
//	starts_at              <= ends_at                (if both set)
func validateDateRange(regOpens, regCloses, startsAt, endsAt pgtype.Timestamptz) error {
	if regOpens.Valid && regCloses.Valid {
		if !regOpens.Time.Before(regCloses.Time) {
			return ErrInvalidDateRange
		}
	}
	if regCloses.Valid && startsAt.Valid {
		if regCloses.Time.After(startsAt.Time) {
			return ErrInvalidDateRange
		}
	}
	if startsAt.Valid && endsAt.Valid {
		if startsAt.Time.After(endsAt.Time) {
			return ErrInvalidDateRange
		}
	}
	return nil
}

// GetStandings derives the current standings table for a tournament.
//
// Source of truth for scores is matches.home_score / matches.away_score —
// the columns snapshotted at match completion.  This method never reads
// match_events.
//
// All approved registrants appear in the result regardless of whether they
// have played any matches yet (position reflects current standing with 0s).
func (s *Service) GetStandings(ctx context.Context, orgSlug, tournamentID string) (*StandingsResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, orgSlug)
	if err != nil {
		return nil, err
	}

	tid, err := pgutil.ParseUUID(tournamentID)
	if err != nil {
		return nil, ErrTournamentNotFound
	}

	tournament, err := s.repo.GetByID(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	// Fetch completed matches (org-scoped) and approved registrations.
	rawMatches, err := s.repo.GetCompletedMatchesForStandings(ctx, tid, org.ID)
	if err != nil {
		return nil, err
	}

	rawRegs, err := s.repo.GetRegistrationsForStandings(ctx, tid)
	if err != nil {
		return nil, err
	}

	settings := parseStandingsSettings(tournament.Settings)

	// Convert DB rows to standings engine input types.
	matches := make([]standings.CompletedMatch, 0, len(rawMatches))
	for _, m := range rawMatches {
		cm := standings.CompletedMatch{
			HomeParticipantID: participantID(m.HomeTeamID, m.HomePlayerID),
			AwayParticipantID: participantID(m.AwayTeamID, m.AwayPlayerID),
			HomeScore:         int(m.HomeScore),
			AwayScore:         int(m.AwayScore),
			WinnerID:          participantID(m.WinnerTeamID, m.WinnerPlayerID),
			IsWalkover:        m.IsWalkover,
		}
		if cm.HomeParticipantID == "" || cm.AwayParticipantID == "" {
			continue // skip TBD bracket matches
		}
		matches = append(matches, cm)
	}

	regs := make([]standings.RegistrationInfo, 0, len(rawRegs))
	for _, r := range rawRegs {
		pid := participantID(r.TeamID, r.PlayerID)
		if pid == "" {
			continue
		}
		var seed *int16
		if r.SeedNumber != nil {
			seed = r.SeedNumber
		}
		regs = append(regs, standings.RegistrationInfo{
			ParticipantID: pid,
			SeedNumber:    seed,
			RegisteredAt:  r.RegisteredAt.Time.UTC(),
		})
	}

	rows := standings.Compute(matches, regs, settings)

	participantIDs := make([]string, len(rows))
	for i, row := range rows {
		participantIDs[i] = row.ParticipantID
	}
	names, err := s.resolveParticipantNames(ctx, participantIDs)
	if err != nil {
		return nil, err
	}

	standingsResp := make([]StandingsRowResponse, len(rows))
	for i, row := range rows {
		standingsResp[i] = StandingsRowResponse{
			Position:        row.Position,
			ParticipantID:   row.ParticipantID,
			ParticipantName: names[row.ParticipantID],
			Played:          row.Played,
			Wins:            row.Wins,
			Losses:          row.Losses,
			Draws:           row.Draws,
			Points:          row.Points,
			ScoreFor:        row.ScoreFor,
			ScoreAgainst:    row.ScoreAgainst,
			ScoreDifference: row.ScoreDifference,
		}
	}

	return &StandingsResponse{
		TournamentID:   pgutil.UUIDToString(tournament.ID),
		TournamentName: tournament.Name,
		Format:         string(tournament.Format),
		Status:         string(tournament.Status),
		PointSystem: PointSystemResponse{
			WinPoints:       settings.WinPoints,
			DrawPoints:      settings.DrawPoints,
			LossPoints:      settings.LossPoints,
			CloseMargin:     settings.CloseMargin,
			CloseLossPoints: settings.CloseLossPoints,
		},
		Standings: standingsResp,
	}, nil
}

// participantID returns the UUID string for the first valid pgtype.UUID provided.
// Used to normalise team-vs-player participant references into a single string.
func participantID(primary, fallback pgtype.UUID) string {
	if primary.Valid {
		return pgutil.UUIDToString(primary)
	}
	if fallback.Valid {
		return pgutil.UUIDToString(fallback)
	}
	return ""
}

// standingsSettingsJSON is the subset of tournaments.settings used for standings.
type standingsSettingsJSON struct {
	WinPoints       *int `json:"win_points"`
	DrawPoints      *int `json:"draw_points"`
	LossPoints      *int `json:"loss_points"`
	CloseMargin     *int `json:"close_margin"`
	CloseLossPoints *int `json:"close_loss_points"`
}

// parseStandingsSettings unmarshals the point system from tournaments.settings
// JSONB.  Fields absent from the JSON use the standings.DefaultSettings values.
func parseStandingsSettings(raw []byte) standings.Settings {
	s := standings.DefaultSettings()
	if len(raw) == 0 {
		return s
	}
	var js standingsSettingsJSON
	if err := json.Unmarshal(raw, &js); err != nil {
		return s
	}
	if js.WinPoints != nil {
		s.WinPoints = *js.WinPoints
	}
	if js.DrawPoints != nil {
		s.DrawPoints = *js.DrawPoints
	}
	if js.LossPoints != nil {
		s.LossPoints = *js.LossPoints
	}
	if js.CloseMargin != nil {
		s.CloseMargin = *js.CloseMargin
	}
	if js.CloseLossPoints != nil {
		s.CloseLossPoints = *js.CloseLossPoints
	}
	return s
}

func tournamentToResponse(t *db.Tournament) *Response {
	var createdBy *string
	if t.CreatedBy.Valid {
		uid := pgutil.UUIDToString(t.CreatedBy)
		createdBy = &uid
	}
	return &Response{
		ID:                   pgutil.UUIDToString(t.ID),
		OrganizationID:       pgutil.UUIDToString(t.OrganizationID),
		Name:                 t.Name,
		Slug:                 t.Slug,
		Description:          t.Description,
		Sport:                t.Sport,
		Format:               string(t.Format),
		ParticipantType:      string(t.ParticipantType),
		Status:               string(t.Status),
		BannerURL:            t.BannerUrl,
		PrizePool:            numericToString(t.PrizePool),
		Currency:             t.Currency,
		MaxParticipants:      t.MaxParticipants,
		MinParticipants:      t.MinParticipants,
		RegistrationOpensAt:  timestampToString(t.RegistrationOpensAt),
		RegistrationClosesAt: timestampToString(t.RegistrationClosesAt),
		StartsAt:             timestampToString(t.StartsAt),
		EndsAt:               timestampToString(t.EndsAt),
		Venue:                t.Venue,
		City:                 t.City,
		Country:              t.Country,
		Rules:                t.Rules,
		CreatedBy:            createdBy,
		CreatedAt:            t.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:            t.UpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
