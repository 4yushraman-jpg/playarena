package fixturegen

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
	"github.com/4yushraman-jpg/playarena/internal/fixtures"
	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/standings"
)

// Repository provides data access for fixture generation. The two mutating
// operations (Generate, ResolveQualifiers) run inside a single transaction each
// so generation is strictly all-or-nothing.
type Repository struct {
	queries *db.Queries
	pool    *pgxpool.Pool
}

func NewRepository(queries *db.Queries, pool *pgxpool.Pool) *Repository {
	return &Repository{queries: queries, pool: pool}
}

// ── reads (used by dry-run preview, no lock) ───────────────────────────────────

func (r *Repository) GetOrgBySlug(ctx context.Context, slug string) (*db.Organization, error) {
	org, err := r.queries.GetOrganizationBySlug(ctx, slug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (r *Repository) GetTournamentByID(ctx context.Context, id, orgID pgtype.UUID) (*db.Tournament, error) {
	t, err := r.queries.GetTournamentByID(ctx, db.GetTournamentByIDParams{ID: id, OrganizationID: orgID})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTournamentNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (r *Repository) ListApprovedParticipants(ctx context.Context, tournamentID pgtype.UUID, pt db.ParticipantType) ([]fixtures.Participant, error) {
	rows, err := r.queries.ListApprovedRegistrationsForStandings(ctx, tournamentID)
	if err != nil {
		return nil, err
	}
	return toParticipants(rows, pt), nil
}

// GenerationInfoFromSettings extracts the stored generation audit record.
func GenerationInfoFromSettings(settings []byte) GenerationInfo {
	var info GenerationInfo
	if len(settings) == 0 {
		return info
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(settings, &m); err != nil {
		return info
	}
	raw, ok := m["generation"]
	if !ok {
		return info
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return info
	}
	info.Generated = true
	return info
}

// ── generation transaction ─────────────────────────────────────────────────────

type GenerateParams struct {
	TournamentID pgtype.UUID
	OrgID        pgtype.UUID
	ActorID      pgtype.UUID
	// BuildPlan turns the in-transaction participant snapshot into a Plan via the
	// pure engine. Kept as a callback so the engine stays out of the repository
	// and the participant set is read inside the transaction (consistent snapshot).
	BuildPlan func(participants []fixtures.Participant, format db.TournamentFormat) (*fixtures.Plan, error)
	// GenerationMeta builds the explainability object stored under settings.generation.
	GenerationMeta func(plan *fixtures.Plan, generatedAt time.Time) map[string]any
	// ExpectedParticipants is the count the preview was built from; a mismatch
	// inside the transaction means registrations changed (late registration).
	ExpectedParticipants int
}

type GenerateResult struct {
	Plan        *fixtures.Plan
	GeneratedAt time.Time
	Count       int
}

// Generate atomically: locks the tournament, re-validates registration_closed +
// no existing fixtures, reads the approved participants, builds the plan, inserts
// every match (successors first so self-referential next_match_id is always
// satisfiable), and records the generation audit in settings. Any failure rolls
// everything back — generation is all-or-nothing.
func (r *Repository) Generate(ctx context.Context, p GenerateParams) (*GenerateResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := r.queries.WithTx(tx)

	row, err := qtx.LockTournamentForGeneration(ctx, db.LockTournamentForGenerationParams{
		ID:             p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTournamentNotFound
		}
		return nil, err
	}
	if row.Status != db.TournamentStatusRegistrationClosed {
		return nil, ErrNotRegistrationClosed
	}
	cnt, err := qtx.CountMatchesForTournament(ctx, db.CountMatchesForTournamentParams{
		TournamentID:   p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, ErrFixturesExist
	}

	regRows, err := qtx.ListApprovedRegistrationsForStandings(ctx, p.TournamentID)
	if err != nil {
		return nil, err
	}
	participants := toParticipants(regRows, row.ParticipantType)
	if len(participants) < 2 {
		return nil, ErrTooFewParticipants
	}
	if p.ExpectedParticipants > 0 && len(participants) != p.ExpectedParticipants {
		return nil, ErrRegistrationsChanged
	}

	plan, err := p.BuildPlan(participants, row.Format)
	if err != nil {
		return nil, err
	}

	if err := r.insertPlan(ctx, qtx, p, row.ParticipantType, plan); err != nil {
		return nil, err
	}

	generatedAt := time.Now().UTC()
	merged, err := mergeSettings(row.Settings, p.GenerationMeta(plan, generatedAt))
	if err != nil {
		return nil, err
	}
	if err := qtx.UpdateTournamentSettings(ctx, db.UpdateTournamentSettingsParams{
		ID:             p.TournamentID,
		OrganizationID: p.OrgID,
		Settings:       merged,
	}); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &GenerateResult{Plan: plan, GeneratedAt: generatedAt, Count: len(plan.Matches)}, nil
}

// insertPlan inserts matches in descending round order so that a feeder's
// next_match_id (which always points to a later round) references an
// already-inserted row, satisfying the self-FK without a second pass.
func (r *Repository) insertPlan(ctx context.Context, qtx *db.Queries, p GenerateParams, pt db.ParticipantType, plan *fixtures.Plan) error {
	order := make([]int, len(plan.Matches))
	for i := range plan.Matches {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return plan.Matches[order[a]].RoundNumber > plan.Matches[order[b]].RoundNumber
	})

	uuidByIndex := make(map[int]pgtype.UUID, len(plan.Matches))
	for _, oi := range order {
		pm := plan.Matches[oi]
		params := db.InsertGeneratedMatchParams{
			TournamentID:   p.TournamentID,
			OrganizationID: p.OrgID,
			RoundNumber:    int16ptr(pm.RoundNumber),
			RoundName:      strptr(pm.RoundName),
			MatchNumber:    int16ptr(pm.MatchNumber),
			GroupLabel:     groupLabelPtr(pm.GroupLabel),
			Metadata:       qualifierMetadata(pm),
		}
		setSlotColumns(&params, pt, pm.Home, pm.Away)
		if pm.NextMatchIndex != nil {
			params.NextMatchID = uuidByIndex[*pm.NextMatchIndex]
			params.NextMatchSlot = int16ptr(*pm.NextSlot)
		}
		m, err := qtx.InsertGeneratedMatch(ctx, params)
		if err != nil {
			return err
		}
		uuidByIndex[pm.Index] = m.ID
	}
	return nil
}

// ── resolve-qualifiers transaction ─────────────────────────────────────────────

type ResolveParams struct {
	TournamentID pgtype.UUID
	OrgID        pgtype.UUID
}

// ResolveQualifiers atomically fills knockout qualifier slots from the completed
// group standings. Requires the tournament to be ongoing and the group stage to
// be fully concluded. Idempotent: slots already filled are left untouched.
func (r *Repository) ResolveQualifiers(ctx context.Context, p ResolveParams) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	qtx := r.queries.WithTx(tx)

	row, err := qtx.LockTournamentForGeneration(ctx, db.LockTournamentForGenerationParams{
		ID:             p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, ErrTournamentNotFound
		}
		return 0, err
	}
	if row.Format != db.TournamentFormatGroupKnockout {
		return 0, ErrNotGroupKnockout
	}
	if row.Status != db.TournamentStatusOngoing {
		return 0, ErrTournamentNotOngoing
	}

	unfinished, err := qtx.CountUnfinishedGroupMatches(ctx, db.CountUnfinishedGroupMatchesParams{
		TournamentID:   p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		return 0, err
	}
	if unfinished > 0 {
		return 0, ErrGroupStageIncomplete
	}

	qualMatches, err := qtx.ListKnockoutQualifierMatches(ctx, db.ListKnockoutQualifierMatchesParams{
		TournamentID:   p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		return 0, err
	}
	if len(qualMatches) == 0 {
		return 0, ErrNoQualifierMatches
	}

	groupRows, err := qtx.ListGroupMatchesForStandings(ctx, db.ListGroupMatchesForStandingsParams{
		TournamentID:   p.TournamentID,
		OrganizationID: p.OrgID,
	})
	if err != nil {
		return 0, err
	}
	regRows, err := qtx.ListApprovedRegistrationsForStandings(ctx, p.TournamentID)
	if err != nil {
		return 0, err
	}
	ranked := computeGroupStandings(groupRows, regRows, row.ParticipantType)

	filled := 0
	for _, qm := range qualMatches {
		var meta qualifierMeta
		if err := json.Unmarshal(qm.Metadata, &meta); err != nil || meta.Qualifiers == nil {
			continue
		}
		params := db.SetMatchParticipantsParams{
			ID:             qm.ID,
			OrganizationID: p.OrgID,
			HomeTeamID:     qm.HomeTeamID,
			AwayTeamID:     qm.AwayTeamID,
			HomePlayerID:   qm.HomePlayerID,
			AwayPlayerID:   qm.AwayPlayerID,
		}
		changed := false
		if q := meta.Qualifiers.Home; q != nil && slotEmpty(qm.HomeTeamID, qm.HomePlayerID) {
			id, err := resolveQualifier(ranked, q.Group, q.Rank)
			if err != nil {
				return 0, err
			}
			setOneSlot(&params, row.ParticipantType, true, id)
			changed = true
		}
		if q := meta.Qualifiers.Away; q != nil && slotEmpty(qm.AwayTeamID, qm.AwayPlayerID) {
			id, err := resolveQualifier(ranked, q.Group, q.Rank)
			if err != nil {
				return 0, err
			}
			setOneSlot(&params, row.ParticipantType, false, id)
			changed = true
		}
		if !changed {
			continue
		}
		n, err := qtx.SetMatchParticipants(ctx, params)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			if meta.Qualifiers.Home != nil {
				filled++
			}
			if meta.Qualifiers.Away != nil {
				filled++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return filled, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────────

func toParticipants(rows []db.ListApprovedRegistrationsForStandingsRow, pt db.ParticipantType) []fixtures.Participant {
	out := make([]fixtures.Participant, 0, len(rows))
	for _, r := range rows {
		id := r.TeamID
		if pt == db.ParticipantTypeIndividual {
			id = r.PlayerID
		}
		if !id.Valid {
			continue
		}
		var seed *int
		if r.SeedNumber != nil {
			v := int(*r.SeedNumber)
			seed = &v
		}
		out = append(out, fixtures.Participant{
			ID:           pgutil.UUIDToString(id),
			SeedNumber:   seed,
			RegisteredAt: r.RegisteredAt.Time.UnixNano(),
		})
	}
	return out
}

// qualifierMeta is the JSON shape stored in matches.metadata for slots filled by
// group-stage qualification.
type qualifierMeta struct {
	Qualifiers *struct {
		Home *qualRef `json:"home,omitempty"`
		Away *qualRef `json:"away,omitempty"`
	} `json:"qualifiers,omitempty"`
}

type qualRef struct {
	Group string `json:"group"`
	Rank  int    `json:"rank"`
}

func qualifierMetadata(pm fixtures.PlannedMatch) []byte {
	var home, away *qualRef
	if pm.Home.Kind == fixtures.SlotQualifier {
		home = &qualRef{Group: pm.Home.Group, Rank: pm.Home.Rank}
	}
	if pm.Away.Kind == fixtures.SlotQualifier {
		away = &qualRef{Group: pm.Away.Group, Rank: pm.Away.Rank}
	}
	if home == nil && away == nil {
		return []byte("{}")
	}
	m := qualifierMeta{}
	m.Qualifiers = &struct {
		Home *qualRef `json:"home,omitempty"`
		Away *qualRef `json:"away,omitempty"`
	}{Home: home, Away: away}
	b, _ := json.Marshal(m)
	return b
}

// computeGroupStandings returns, per group label, the participant ids ordered by
// finishing position (index 0 = winner).
func computeGroupStandings(groupRows []db.ListGroupMatchesForStandingsRow, regRows []db.ListApprovedRegistrationsForStandingsRow, pt db.ParticipantType) map[string][]string {
	// Partition matches and collect members per group.
	matchesByGroup := map[string][]standings.CompletedMatch{}
	membersByGroup := map[string]map[string]bool{}
	for _, gr := range groupRows {
		label := ""
		if gr.GroupLabel != nil {
			label = *gr.GroupLabel
		}
		home := participantID(gr.HomeTeamID, gr.HomePlayerID)
		away := participantID(gr.AwayTeamID, gr.AwayPlayerID)
		if home == "" || away == "" {
			continue
		}
		matchesByGroup[label] = append(matchesByGroup[label], standings.CompletedMatch{
			HomeParticipantID: home,
			AwayParticipantID: away,
			HomeScore:         int(gr.HomeScore),
			AwayScore:         int(gr.AwayScore),
			WinnerID:          participantID(gr.WinnerTeamID, gr.WinnerPlayerID),
			IsWalkover:        gr.IsWalkover,
		})
		if membersByGroup[label] == nil {
			membersByGroup[label] = map[string]bool{}
		}
		membersByGroup[label][home] = true
		membersByGroup[label][away] = true
	}

	// Registration metadata for tiebreakers, keyed by participant id.
	regByID := map[string]standings.RegistrationInfo{}
	for _, r := range regRows {
		id := participantID(r.TeamID, r.PlayerID)
		if id == "" {
			continue
		}
		var seed *int16
		if r.SeedNumber != nil {
			seed = r.SeedNumber
		}
		regByID[id] = standings.RegistrationInfo{
			ParticipantID: id,
			SeedNumber:    seed,
			RegisteredAt:  r.RegisteredAt.Time.UTC(),
		}
	}

	out := map[string][]string{}
	for label, members := range membersByGroup {
		regs := make([]standings.RegistrationInfo, 0, len(members))
		for id := range members {
			if ri, ok := regByID[id]; ok {
				regs = append(regs, ri)
			} else {
				regs = append(regs, standings.RegistrationInfo{ParticipantID: id})
			}
		}
		rows := standings.Compute(matchesByGroup[label], regs, standings.DefaultSettings())
		ids := make([]string, len(rows))
		for i, rrow := range rows {
			ids[i] = rrow.ParticipantID
		}
		out[label] = ids
	}
	return out
}

func resolveQualifier(ranked map[string][]string, group string, rank int) (pgtype.UUID, error) {
	ids, ok := ranked[group]
	if !ok || rank < 1 || rank > len(ids) {
		return pgtype.UUID{}, ErrQualifierUnavailable
	}
	uid, err := pgutil.ParseUUID(ids[rank-1])
	if err != nil {
		return pgtype.UUID{}, ErrQualifierUnavailable
	}
	return uid, nil
}

func setSlotColumns(params *db.InsertGeneratedMatchParams, pt db.ParticipantType, home, away fixtures.SlotRef) {
	if home.Kind == fixtures.SlotConcrete {
		uid, _ := pgutil.ParseUUID(home.ID)
		if pt == db.ParticipantTypeIndividual {
			params.HomePlayerID = uid
		} else {
			params.HomeTeamID = uid
		}
	}
	if away.Kind == fixtures.SlotConcrete {
		uid, _ := pgutil.ParseUUID(away.ID)
		if pt == db.ParticipantTypeIndividual {
			params.AwayPlayerID = uid
		} else {
			params.AwayTeamID = uid
		}
	}
}

func setOneSlot(params *db.SetMatchParticipantsParams, pt db.ParticipantType, home bool, id pgtype.UUID) {
	switch {
	case home && pt == db.ParticipantTypeIndividual:
		params.HomePlayerID = id
	case home:
		params.HomeTeamID = id
	case pt == db.ParticipantTypeIndividual:
		params.AwayPlayerID = id
	default:
		params.AwayTeamID = id
	}
}

func slotEmpty(team, player pgtype.UUID) bool { return !team.Valid && !player.Valid }

func participantID(team, player pgtype.UUID) string {
	if team.Valid {
		return pgutil.UUIDToString(team)
	}
	if player.Valid {
		return pgutil.UUIDToString(player)
	}
	return ""
}

func mergeSettings(existing []byte, generation map[string]any) ([]byte, error) {
	m := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &m); err != nil {
			return nil, err
		}
	}
	m["generation"] = generation
	return json.Marshal(m)
}

func int16ptr(v int) *int16 { x := int16(v); return &x }
func strptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func groupLabelPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
