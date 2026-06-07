// Package fixtures provides typed test-fixture helpers for domain entities.
//
// This file extends the auth fixtures with entity builders for every domain:
// organizations, players, teams, team memberships, tournaments, tournament
// registrations, and matches.
//
// Isolation model: every fixture is UUID-keyed so concurrent tests do not share
// rows. Callers register t.Cleanup via the returned helpers; CASCADE on the
// organizations table handles child-entity teardown automatically.
//
// Direct SQL vs. service layer: fixtures that do not exercise business rules
// (e.g. inserting a completed match with known scores) bypass service layers
// entirely via direct SQL INSERT. This avoids the multi-step setup required by
// the business rule stack when those rules are not the subject of the test.
package fixtures

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// ── organizations ─────────────────────────────────────────────────────────────

// CreateOrg inserts an active organization with a random name and slug.
// Registers t.Cleanup to delete the organization (CASCADE removes all children).
// Returns the db.Organization row.
func CreateOrg(ctx context.Context, t testing.TB, pool *pgxpool.Pool) db.Organization {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	short := fmt.Sprintf("%x", uid.Bytes[0:4])
	name := "Test Org " + short
	slug := "test-org-" + short

	queries := db.New(pool)
	org, err := queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name: name,
		Slug: slug,
		Type: db.OrgTypeClub,
	})
	if err != nil {
		t.Fatalf("fixtures.CreateOrg: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM organizations WHERE id = $1", org.ID)
	})

	return org
}

// CreateOrgForUser creates an organization and grants the given user a role
// within it. roleSlug must be one of the system roles (e.g. "org_owner",
// "org_admin", "viewer"). This is the primary setup helper for domain tests
// that need a user + org + role triple before performing authenticated requests.
//
// Returns the db.Organization row. Cleanup cascades via t.Cleanup.
func CreateOrgForUser(ctx context.Context, t testing.TB, pool *pgxpool.Pool, userID pgtype.UUID, roleSlug string) db.Organization {
	t.Helper()

	org := CreateOrg(ctx, t, pool)

	var roleID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT id FROM roles WHERE slug = $1 LIMIT 1",
		roleSlug,
	).Scan(&roleID); err != nil {
		t.Fatalf("fixtures.CreateOrgForUser lookup role %q: %v", roleSlug, err)
	}

	queries := db.New(pool)
	if err := queries.GrantRoleToUserInOrg(ctx, db.GrantRoleToUserInOrgParams{
		UserID:         userID,
		OrganizationID: org.ID,
		RoleID:         roleID,
		GrantedBy:      userID,
	}); err != nil {
		t.Fatalf("fixtures.CreateOrgForUser grant role: %v", err)
	}

	return org
}

// ── players ───────────────────────────────────────────────────────────────────

// CreatePlayer inserts an active player in the given organization.
// The display name is randomised to avoid collisions across parallel tests.
// No t.Cleanup is registered — the org cleanup (CASCADE) handles it.
func CreatePlayer(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Player {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	displayName := fmt.Sprintf("player_%x", uid.Bytes[0:4])

	queries := db.New(pool)
	p, err := queries.CreatePlayer(ctx, db.CreatePlayerParams{
		OrganizationID: orgID,
		DisplayName:    displayName,
	})
	if err != nil {
		t.Fatalf("fixtures.CreatePlayer: %v", err)
	}
	return p
}

// CreateInactivePlayer inserts a player then sets status = 'inactive'.
func CreateInactivePlayer(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Player {
	t.Helper()
	p := CreatePlayer(ctx, t, pool, orgID)
	if _, err := pool.Exec(ctx,
		"UPDATE players SET status = 'inactive' WHERE id = $1",
		p.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateInactivePlayer: %v", err)
	}
	p.Status = db.PlayerStatusInactive
	return p
}

// ── teams ─────────────────────────────────────────────────────────────────────

// CreateTeam inserts an active team in the given organization with a random
// name and slug.
func CreateTeam(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Team {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	short := fmt.Sprintf("%x", uid.Bytes[0:4])
	name := "Team " + short
	slug := "team-" + short

	queries := db.New(pool)
	team, err := queries.CreateTeam(ctx, db.CreateTeamParams{
		OrganizationID: orgID,
		Name:           name,
		Slug:           slug,
	})
	if err != nil {
		t.Fatalf("fixtures.CreateTeam: %v", err)
	}
	return team
}

// CreateDisbandedTeam inserts a team then sets status = 'disbanded'.
func CreateDisbandedTeam(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Team {
	t.Helper()
	team := CreateTeam(ctx, t, pool, orgID)
	if _, err := pool.Exec(ctx,
		"UPDATE teams SET status = 'disbanded' WHERE id = $1",
		team.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateDisbandedTeam: %v", err)
	}
	team.Status = db.TeamStatusDisbanded
	return team
}

// AddPlayerToTeam inserts an active team membership row. No uniqueness check is
// performed here — that is exercised by service-layer tests.
func AddPlayerToTeam(ctx context.Context, t testing.TB, pool *pgxpool.Pool, teamID, playerID, orgID pgtype.UUID) db.TeamMembership {
	t.Helper()
	queries := db.New(pool)
	m, err := queries.CreateMembership(ctx, db.CreateMembershipParams{
		TeamID:         teamID,
		PlayerID:       playerID,
		OrganizationID: orgID,
		Role:           db.MembershipRolePlayer,
	})
	if err != nil {
		t.Fatalf("fixtures.AddPlayerToTeam: %v", err)
	}
	return m
}

// CreateTeamWithMember creates a team with one active player member. Returns
// both the team and the player. Useful as a precondition for tournament
// registration tests (which require a non-empty team).
func CreateTeamWithMember(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) (db.Team, db.Player) {
	t.Helper()
	team := CreateTeam(ctx, t, pool, orgID)
	player := CreatePlayer(ctx, t, pool, orgID)
	AddPlayerToTeam(ctx, t, pool, team.ID, player.ID, orgID)
	return team, player
}

// ── tournaments ───────────────────────────────────────────────────────────────

// CreateTournament inserts a tournament in the given status. Timestamps are set
// relative to the requested status so that business-rule checks (registration
// window, started_at, ended_at) pass for the given state.
//
// For status = registration_open the registration window is: opens 1 hour ago,
// closes 1 hour from now. For all other statuses timestamps are set to sensible
// past/future values.
func CreateTournament(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID, status db.TournamentStatus) db.Tournament {
	t.Helper()

	uid := newUUID(ctx, t, pool)
	short := fmt.Sprintf("%x", uid.Bytes[0:4])
	name := "Tournament " + short
	slug := "tourn-" + short

	now := time.Now().UTC()

	regOpens := pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	regCloses := pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true}
	startsAt := pgtype.Timestamptz{Time: now.Add(2 * time.Hour), Valid: true}
	endsAt := pgtype.Timestamptz{Time: now.Add(72 * time.Hour), Valid: true}

	// Adjust timestamps for statuses that imply the tournament has already started.
	if status == db.TournamentStatusOngoing ||
		status == db.TournamentStatusCompleted ||
		status == db.TournamentStatusCancelled {
		regOpens = pgtype.Timestamptz{Time: now.Add(-48 * time.Hour), Valid: true}
		regCloses = pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true}
		startsAt = pgtype.Timestamptz{Time: now.Add(-12 * time.Hour), Valid: true}
		endsAt = pgtype.Timestamptz{Time: now.Add(12 * time.Hour), Valid: true}
	}
	if status == db.TournamentStatusCompleted {
		endsAt = pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	}
	if status == db.TournamentStatusDraft ||
		status == db.TournamentStatusRegistrationClosed {
		regOpens = pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true}
		regCloses = pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true}
	}

	queries := db.New(pool)
	tourn, err := queries.CreateTournament(ctx, db.CreateTournamentParams{
		OrganizationID:       orgID,
		Name:                 name,
		Slug:                 slug,
		Sport:                "kabaddi",
		Format:               db.TournamentFormatLeague,
		ParticipantType:      db.ParticipantTypeTeam,
		Currency:             "INR",
		RegistrationOpensAt:  regOpens,
		RegistrationClosesAt: regCloses,
		StartsAt:             startsAt,
		EndsAt:               endsAt,
		CreatedBy:            pgtype.UUID{Valid: false},
	})
	if err != nil {
		t.Fatalf("fixtures.CreateTournament: %v", err)
	}

	// Advance status beyond draft using direct SQL if needed.
	if status != db.TournamentStatusDraft {
		if _, err := pool.Exec(ctx,
			"UPDATE tournaments SET status = $1 WHERE id = $2",
			string(status), tourn.ID,
		); err != nil {
			t.Fatalf("fixtures.CreateTournament set status: %v", err)
		}
		tourn.Status = status
	}

	return tourn
}

// CreateOngoingTournament is a convenience wrapper for the most common
// precondition: a tournament in ongoing status with a capacity of 10.
func CreateOngoingTournament(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Tournament {
	t.Helper()
	tourn := CreateTournament(ctx, t, pool, orgID, db.TournamentStatusOngoing)
	maxP := int16(10)
	if _, err := pool.Exec(ctx,
		"UPDATE tournaments SET max_participants = $1 WHERE id = $2",
		maxP, tourn.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateOngoingTournament set capacity: %v", err)
	}
	tourn.MaxParticipants = &maxP
	return tourn
}

// CreateRegistrationOpenTournament creates a tournament in registration_open
// status with an open registration window and max_participants = 10.
func CreateRegistrationOpenTournament(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) db.Tournament {
	t.Helper()
	return CreateTournament(ctx, t, pool, orgID, db.TournamentStatusRegistrationOpen)
}

// ── tournament registrations ──────────────────────────────────────────────────

// CreateApprovedRegistration inserts an approved registration directly via SQL,
// bypassing all business rules. Use only when setting up match preconditions
// where the registration itself is not the subject of the test.
func CreateApprovedRegistration(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, tournamentID, teamID pgtype.UUID) db.TournamentRegistration {
	t.Helper()

	uid := newUUID(ctx, t, pool)

	var reg db.TournamentRegistration
	err := pool.QueryRow(ctx, `
		INSERT INTO tournament_registrations (id, tournament_id, organization_id, team_id, status, registered_at)
		VALUES ($1, $2, $3, $4, 'approved', NOW())
		RETURNING id, tournament_id, organization_id, team_id, player_id, seed_number, status,
		          registered_by, registered_at, approved_by, approved_at, notes, metadata, created_at, updated_at`,
		uid, tournamentID, orgID, teamID,
	).Scan(
		&reg.ID, &reg.TournamentID, &reg.OrganizationID, &reg.TeamID, &reg.PlayerID,
		&reg.SeedNumber, &reg.Status, &reg.RegisteredBy, &reg.RegisteredAt,
		&reg.ApprovedBy, &reg.ApprovedAt, &reg.Notes, &reg.Metadata, &reg.CreatedAt, &reg.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("fixtures.CreateApprovedRegistration: %v", err)
	}
	return reg
}

// ── matches ───────────────────────────────────────────────────────────────────

// CreateScheduledMatch inserts a scheduled match directly via SQL.
func CreateScheduledMatch(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, tournamentID, homeTeamID, awayTeamID pgtype.UUID) db.Match {
	t.Helper()

	queries := db.New(pool)
	m, err := queries.CreateMatch(ctx, db.CreateMatchParams{
		TournamentID:   tournamentID,
		OrganizationID: orgID,
		HomeTeamID:     homeTeamID,
		AwayTeamID:     awayTeamID,
		ScheduledAt:    pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		Status:         db.MatchStatusScheduled,
	})
	if err != nil {
		t.Fatalf("fixtures.CreateScheduledMatch: %v", err)
	}
	return m
}

// CreateLiveMatch inserts a match and sets its status to live.
func CreateLiveMatch(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, tournamentID, homeTeamID, awayTeamID pgtype.UUID) db.Match {
	t.Helper()
	m := CreateScheduledMatch(ctx, t, pool, orgID, tournamentID, homeTeamID, awayTeamID)
	if _, err := pool.Exec(ctx,
		"UPDATE matches SET status = 'live', started_at = NOW() WHERE id = $1",
		m.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateLiveMatch: %v", err)
	}
	m.Status = db.MatchStatusLive
	return m
}

// CreateCompletedMatch inserts a match and sets it to completed with the given
// home/away scores and winner. The winner must be one of the participant team IDs.
func CreateCompletedMatch(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, tournamentID, homeTeamID, awayTeamID pgtype.UUID, homeScore, awayScore int32, winnerTeamID pgtype.UUID) db.Match {
	t.Helper()
	m := CreateScheduledMatch(ctx, t, pool, orgID, tournamentID, homeTeamID, awayTeamID)
	if _, err := pool.Exec(ctx, `
		UPDATE matches
		SET    status = 'completed', started_at = NOW() - INTERVAL '1 hour',
		       ended_at = NOW(), home_score = $2, away_score = $3,
		       winner_team_id = $4
		WHERE  id = $1`,
		m.ID, homeScore, awayScore, winnerTeamID,
	); err != nil {
		t.Fatalf("fixtures.CreateCompletedMatch: %v", err)
	}
	m.Status = db.MatchStatusCompleted
	m.HomeScore = homeScore
	m.AwayScore = awayScore
	m.WinnerTeamID = winnerTeamID
	return m
}

// CreateMatchEvent inserts a match event directly via SQL. The sequence number
// is computed as MAX(sequence_number)+1 for the match, so callers may call this
// sequentially without a lock (suitable for fixture setup, not concurrency tests).
func CreateMatchEvent(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, matchID pgtype.UUID, eventType db.MatchEventType, payload []byte) db.MatchEvent {
	t.Helper()

	if payload == nil {
		payload = []byte("{}")
	}

	var ev db.MatchEvent
	err := pool.QueryRow(ctx, `
		INSERT INTO match_events (match_id, organization_id, sequence_number, event_type, payload, recorded_at)
		VALUES ($1, $2,
		        COALESCE((SELECT MAX(sequence_number) FROM match_events WHERE match_id = $1), 0) + 1,
		        $3, $4, NOW())
		RETURNING id, match_id, organization_id, sequence_number, event_type, team_id, player_id,
		          period, clock_seconds, payload, recorded_by, recorded_at,
		          cancels_event_id, created_at`,
		matchID, orgID, string(eventType), payload,
	).Scan(
		&ev.ID, &ev.MatchID, &ev.OrganizationID, &ev.SequenceNumber, &ev.EventType,
		&ev.TeamID, &ev.PlayerID, &ev.Period, &ev.ClockSeconds, &ev.Payload,
		&ev.RecordedBy, &ev.RecordedAt, &ev.CancelsEventID, &ev.CreatedAt,
	)
	if err != nil {
		t.Fatalf("fixtures.CreateMatchEvent: %v", err)
	}
	return ev
}

// AddUserToOrg grants a role to an existing user in an existing organization.
// Use when a test needs a second user in the same org without creating a new one,
// e.g. to exercise within-org permission denial instead of BOLA.
func AddUserToOrg(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID, userID pgtype.UUID, roleSlug string) {
	t.Helper()

	var roleID pgtype.UUID
	if err := pool.QueryRow(ctx,
		"SELECT id FROM roles WHERE slug = $1 LIMIT 1",
		roleSlug,
	).Scan(&roleID); err != nil {
		t.Fatalf("fixtures.AddUserToOrg lookup role %q: %v", roleSlug, err)
	}

	queries := db.New(pool)
	if err := queries.GrantRoleToUserInOrg(ctx, db.GrantRoleToUserInOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
		RoleID:         roleID,
		GrantedBy:      userID,
	}); err != nil {
		t.Fatalf("fixtures.AddUserToOrg grant role: %v", err)
	}
}

// CreateRegistrationOpenTournamentWithCapacity creates a registration_open tournament
// with the specified max_participants cap. Use for capacity enforcement tests.
func CreateRegistrationOpenTournamentWithCapacity(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID, capacity int16) db.Tournament {
	t.Helper()
	tourn := CreateTournament(ctx, t, pool, orgID, db.TournamentStatusRegistrationOpen)
	if _, err := pool.Exec(ctx,
		"UPDATE tournaments SET max_participants = $1 WHERE id = $2",
		capacity, tourn.ID,
	); err != nil {
		t.Fatalf("fixtures.CreateRegistrationOpenTournamentWithCapacity set capacity: %v", err)
	}
	tourn.MaxParticipants = &capacity
	return tourn
}

// ── match setup helpers ───────────────────────────────────────────────────────

// MatchSetup bundles a tournament + two registered teams + a match into one
// call. This is the standard precondition for most match and match_event tests.
type MatchSetup struct {
	Tournament db.Tournament
	HomeTeam   db.Team
	AwayTeam   db.Team
}

// CreateOngoingTournamentWithTeams creates an ongoing tournament and two teams
// with approved registrations. No match is created — callers create their own
// match using CreateScheduledMatch or CreateLiveMatch.
func CreateOngoingTournamentWithTeams(ctx context.Context, t testing.TB, pool *pgxpool.Pool, orgID pgtype.UUID) MatchSetup {
	t.Helper()

	tourn := CreateOngoingTournament(ctx, t, pool, orgID)
	homeTeam, _ := CreateTeamWithMember(ctx, t, pool, orgID)
	awayTeam, _ := CreateTeamWithMember(ctx, t, pool, orgID)

	CreateApprovedRegistration(ctx, t, pool, orgID, tourn.ID, homeTeam.ID)
	CreateApprovedRegistration(ctx, t, pool, orgID, tourn.ID, awayTeam.ID)

	return MatchSetup{
		Tournament: tourn,
		HomeTeam:   homeTeam,
		AwayTeam:   awayTeam,
	}
}
