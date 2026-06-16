package public

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/standings"
)

// Service implements the public (anonymous) read use-cases. Every method begins
// by resolving the tournament through the visibility gate; only on success does
// it read further data scoped to that tournament. No principal, no org scope.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Tournament returns the public overview, or ErrNotFound if not publicly eligible.
func (s *Service) Tournament(ctx context.Context, orgSlug, tournamentSlug string) (*PublicTournament, error) {
	t, err := s.repo.GetTournament(ctx, orgSlug, tournamentSlug)
	if err != nil {
		return nil, err
	}
	return &PublicTournament{
		Name:                 t.Name,
		Slug:                 t.Slug,
		Sport:                t.Sport,
		Format:               string(t.Format),
		ParticipantType:      string(t.ParticipantType),
		Status:               string(t.Status),
		Visibility:           string(t.Visibility),
		BannerURL:            t.BannerUrl,
		Description:          t.Description,
		PrizePool:            numericToString(t.PrizePool),
		Currency:             t.Currency,
		MaxParticipants:      t.MaxParticipants,
		RegistrationOpensAt:  tsPtr(t.RegistrationOpensAt),
		RegistrationClosesAt: tsPtr(t.RegistrationClosesAt),
		StartsAt:             tsPtr(t.StartsAt),
		EndsAt:               tsPtr(t.EndsAt),
		Venue:                t.Venue,
		City:                 t.City,
		Country:              t.Country,
		Rules:                t.Rules,
		Organization:         OrganizationRef{Name: t.OrganizationName, Slug: t.OrganizationSlug},
	}, nil
}

// Matches returns all whitelisted matches with names resolved.
func (s *Service) Matches(ctx context.Context, orgSlug, tournamentSlug string) (*PublicMatchesResponse, error) {
	t, err := s.repo.GetTournament(ctx, orgSlug, tournamentSlug)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListMatches(ctx, t.ID, t.OrganizationID)
	if err != nil {
		return nil, err
	}

	names, err := s.resolveMatchNames(ctx, rows)
	if err != nil {
		return nil, err
	}

	out := make([]PublicMatch, len(rows))
	for i, m := range rows {
		out[i] = PublicMatch{
			ID:            uuidString(m.ID),
			RoundNumber:   m.RoundNumber,
			RoundName:     m.RoundName,
			MatchNumber:   m.MatchNumber,
			GroupLabel:    m.GroupLabel,
			Home:          slot(names, m.HomeTeamID, m.HomePlayerID),
			Away:          slot(names, m.AwayTeamID, m.AwayPlayerID),
			ScheduledAt:   tsPtr(m.ScheduledAt),
			Venue:         m.Venue,
			Status:        string(m.Status),
			HomeScore:     m.HomeScore,
			AwayScore:     m.AwayScore,
			IsWalkover:    m.IsWalkover,
			WinnerName:    names[participantID(m.WinnerTeamID, m.WinnerPlayerID)],
			NextMatchID:   uuidStringPtr(m.NextMatchID),
			NextMatchSlot: m.NextMatchSlot,
		}
	}
	return &PublicMatchesResponse{Matches: out}, nil
}

// Standings computes and returns the public standings table.
func (s *Service) Standings(ctx context.Context, orgSlug, tournamentSlug string) (*PublicStandingsResponse, error) {
	t, err := s.repo.GetTournament(ctx, orgSlug, tournamentSlug)
	if err != nil {
		return nil, err
	}

	rawMatches, err := s.repo.CompletedMatches(ctx, t.ID, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	rawRegs, err := s.repo.StandingsRegistrations(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	settingsJSON, err := s.repo.TournamentSettings(ctx, t.ID, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	settings := parsePointSystem(settingsJSON)

	matches := make([]standings.CompletedMatch, 0, len(rawMatches))
	for _, m := range rawMatches {
		home := participantID(m.HomeTeamID, m.HomePlayerID)
		away := participantID(m.AwayTeamID, m.AwayPlayerID)
		if home == "" || away == "" {
			continue
		}
		matches = append(matches, standings.CompletedMatch{
			HomeParticipantID: home,
			AwayParticipantID: away,
			HomeScore:         int(m.HomeScore),
			AwayScore:         int(m.AwayScore),
			WinnerID:          participantID(m.WinnerTeamID, m.WinnerPlayerID),
			IsWalkover:        m.IsWalkover,
		})
	}

	regs := make([]standings.RegistrationInfo, 0, len(rawRegs))
	for _, r := range rawRegs {
		pid := participantID(r.TeamID, r.PlayerID)
		if pid == "" {
			continue
		}
		regs = append(regs, standings.RegistrationInfo{
			ParticipantID: pid,
			SeedNumber:    r.SeedNumber,
			RegisteredAt:  r.RegisteredAt.Time.UTC(),
			Disqualified:  r.Status == db.RegistrationStatusDisqualified,
		})
	}

	rows := standings.Compute(matches, regs, settings)

	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ParticipantID
	}
	names, err := s.resolveNames(ctx, t.ParticipantType, ids)
	if err != nil {
		return nil, err
	}

	resp := make([]PublicStandingsRow, len(rows))
	for i, row := range rows {
		resp[i] = PublicStandingsRow{
			Position:        row.Position,
			ParticipantName: names[row.ParticipantID],
			Played:          row.Played,
			Wins:            row.Wins,
			Losses:          row.Losses,
			Draws:           row.Draws,
			Points:          row.Points,
			ScoreFor:        row.ScoreFor,
			ScoreAgainst:    row.ScoreAgainst,
			ScoreDifference: row.ScoreDifference,
			Disqualified:    row.Disqualified,
		}
	}

	return &PublicStandingsResponse{
		Format: string(t.Format),
		Status: string(t.Status),
		PointSystem: PublicPointSystem{
			WinPoints:       settings.WinPoints,
			DrawPoints:      settings.DrawPoints,
			LossPoints:      settings.LossPoints,
			CloseMargin:     settings.CloseMargin,
			CloseLossPoints: settings.CloseLossPoints,
		},
		Standings: resp,
	}, nil
}

// Match returns a single public match with its timeline.
func (s *Service) Match(ctx context.Context, orgSlug, tournamentSlug, matchID string) (*PublicMatchDetail, error) {
	t, err := s.repo.GetTournament(ctx, orgSlug, tournamentSlug)
	if err != nil {
		return nil, err
	}
	mid, err := pgutil.ParseUUID(matchID)
	if err != nil {
		return nil, ErrNotFound
	}
	m, err := s.repo.GetMatch(ctx, mid, t.ID, t.OrganizationID)
	if err != nil {
		return nil, err
	}

	// Resolve participant + winner names.
	teamIDs := validUUIDs(m.HomeTeamID, m.AwayTeamID, m.WinnerTeamID)
	playerIDs := validUUIDs(m.HomePlayerID, m.AwayPlayerID, m.WinnerPlayerID)
	names, err := s.lookupNames(ctx, teamIDs, playerIDs)
	if err != nil {
		return nil, err
	}

	events, err := s.repo.ListMatchEvents(ctx, mid, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	timeline, err := s.buildTimeline(ctx, events)
	if err != nil {
		return nil, err
	}

	return &PublicMatchDetail{
		ID:          uuidString(m.ID),
		RoundName:   m.RoundName,
		GroupLabel:  m.GroupLabel,
		Home:        slot(names, m.HomeTeamID, m.HomePlayerID),
		Away:        slot(names, m.AwayTeamID, m.AwayPlayerID),
		ScheduledAt: tsPtr(m.ScheduledAt),
		StartedAt:   tsPtr(m.StartedAt),
		EndedAt:     tsPtr(m.EndedAt),
		Venue:       m.Venue,
		Status:      string(m.Status),
		HomeScore:   m.HomeScore,
		AwayScore:   m.AwayScore,
		IsWalkover:  m.IsWalkover,
		WinnerName:  names[participantID(m.WinnerTeamID, m.WinnerPlayerID)],
		Timeline:    timeline,
	}, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────────

func (s *Service) buildTimeline(ctx context.Context, events []db.ListPublicMatchEventsRow) ([]PublicMatchEvent, error) {
	var teamIDs, playerIDs []pgtype.UUID
	for _, e := range events {
		if e.TeamID.Valid {
			teamIDs = append(teamIDs, e.TeamID)
		}
		if e.PlayerID.Valid {
			playerIDs = append(playerIDs, e.PlayerID)
		}
	}
	names, err := s.lookupNames(ctx, teamIDs, playerIDs)
	if err != nil {
		return nil, err
	}
	out := make([]PublicMatchEvent, len(events))
	for i, e := range events {
		actor := names[uuidString(e.TeamID)]
		if actor == "" {
			actor = names[uuidString(e.PlayerID)]
		}
		out[i] = PublicMatchEvent{
			Sequence:     e.SequenceNumber,
			EventType:    string(e.EventType),
			Period:       e.Period,
			ClockSeconds: e.ClockSeconds,
			ActorName:    actor,
			RecordedAt:   tsPtr(e.RecordedAt),
		}
	}
	return out, nil
}

// resolveMatchNames batch-resolves every team/player name referenced by a match list.
func (s *Service) resolveMatchNames(ctx context.Context, rows []db.ListPublicMatchesRow) (map[string]string, error) {
	var teamIDs, playerIDs []pgtype.UUID
	add := func(u pgtype.UUID, team bool) {
		if !u.Valid {
			return
		}
		if team {
			teamIDs = append(teamIDs, u)
		} else {
			playerIDs = append(playerIDs, u)
		}
	}
	for _, m := range rows {
		add(m.HomeTeamID, true)
		add(m.AwayTeamID, true)
		add(m.WinnerTeamID, true)
		add(m.HomePlayerID, false)
		add(m.AwayPlayerID, false)
		add(m.WinnerPlayerID, false)
	}
	return s.lookupNames(ctx, teamIDs, playerIDs)
}

// resolveNames resolves a flat list of participant ids by the tournament's type.
func (s *Service) resolveNames(ctx context.Context, pt db.ParticipantType, ids []string) (map[string]string, error) {
	uuids := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if u, err := pgutil.ParseUUID(id); err == nil {
			uuids = append(uuids, u)
		}
	}
	if pt == db.ParticipantTypeIndividual {
		return s.repo.PlayerNames(ctx, uuids)
	}
	return s.repo.TeamNames(ctx, uuids)
}

// lookupNames resolves both team and player names into a single id→name map.
func (s *Service) lookupNames(ctx context.Context, teamIDs, playerIDs []pgtype.UUID) (map[string]string, error) {
	out := map[string]string{}
	if len(teamIDs) > 0 {
		tm, err := s.repo.TeamNames(ctx, teamIDs)
		if err != nil {
			return nil, err
		}
		for k, v := range tm {
			out[k] = v
		}
	}
	if len(playerIDs) > 0 {
		pm, err := s.repo.PlayerNames(ctx, playerIDs)
		if err != nil {
			return nil, err
		}
		for k, v := range pm {
			out[k] = v
		}
	}
	return out, nil
}

func slot(names map[string]string, teamID, playerID pgtype.UUID) PublicSlot {
	if teamID.Valid {
		return PublicSlot{Kind: "participant", Name: names[uuidString(teamID)]}
	}
	if playerID.Valid {
		return PublicSlot{Kind: "participant", Name: names[uuidString(playerID)]}
	}
	return PublicSlot{Kind: "tbd", Name: "TBD"}
}

func participantID(team, player pgtype.UUID) string {
	if team.Valid {
		return uuidString(team)
	}
	if player.Valid {
		return uuidString(player)
	}
	return ""
}

// validUUIDs returns only the valid (non-null) UUIDs from the inputs.
func validUUIDs(in ...pgtype.UUID) []pgtype.UUID {
	var out []pgtype.UUID
	for _, u := range in {
		if u.Valid {
			out = append(out, u)
		}
	}
	return out
}

// pointSystemJSON mirrors the standings point-system subset of tournaments.settings.
type pointSystemJSON struct {
	WinPoints       *int `json:"win_points"`
	DrawPoints      *int `json:"draw_points"`
	LossPoints      *int `json:"loss_points"`
	CloseMargin     *int `json:"close_margin"`
	CloseLossPoints *int `json:"close_loss_points"`
}

func parsePointSystem(raw []byte) standings.Settings {
	s := standings.DefaultSettings()
	if len(raw) == 0 {
		return s
	}
	var js pointSystemJSON
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
