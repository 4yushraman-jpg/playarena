package match_events_integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"

	db "github.com/4yushraman-jpg/playarena/db/sqlc"
)

// TestEvent_Create_Success verifies POST /events returns 201 with expected fields
// for a live match.
func TestEvent_Create_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	resp := ts.postWithHeaders(t, eventsURL(actor.orgSlug, matchID), map[string]any{
		"event_type": "raid_attempt",
		"team_id":    homeTeamID,
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	var ev eventResponse
	decodeBody(t, resp, &ev)
	if ev.ID == "" {
		t.Error("expected event ID in response")
	}
	if ev.EventType != "raid_attempt" {
		t.Errorf("event_type = %q, want raid_attempt", ev.EventType)
	}
	if ev.MatchID != matchID {
		t.Errorf("match_id = %q, want %q", ev.MatchID, matchID)
	}
	if ev.SequenceNumber < 1 {
		t.Errorf("sequence_number = %d, want >= 1", ev.SequenceNumber)
	}
}

// TestEvent_List_Default verifies GET /events returns event list.
func TestEvent_List_Default(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	fixtures.CreateMatchEvent(ctx, t, ts.pool, orgUID, match.ID, db.MatchEventTypeRaidAttempt, []byte(`{}`))
	fixtures.CreateMatchEvent(ctx, t, ts.pool, orgUID, match.ID, db.MatchEventTypeRaidSuccessful, []byte(`{}`))

	resp := ts.get(t, eventsURL(actor.orgSlug, matchID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var list eventListResponse
	decodeBody(t, resp, &list)
	if list.Total < 2 {
		t.Errorf("total = %d, want >= 2", list.Total)
	}
}

// TestEvent_GetByID_Success verifies GET /events/{eventId} returns the event.
func TestEvent_GetByID_Success(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	ev := fixtures.CreateMatchEvent(ctx, t, ts.pool, orgUID, match.ID, db.MatchEventTypeRaidAttempt, []byte(`{}`))
	eventID := pgutil.UUIDToString(ev.ID)

	resp := ts.get(t, eventURL(actor.orgSlug, matchID, eventID), bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	var got eventResponse
	decodeBody(t, resp, &got)
	if got.ID != eventID {
		t.Errorf("id = %q, want %q", got.ID, eventID)
	}
}

// TestEvent_Create_NotLive verifies posting an event to a scheduled (non-live) match
// returns 400.
func TestEvent_Create_NotLive(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, eventsURL(actor.orgSlug, matchID), map[string]any{
		"event_type": "raid_attempt",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnprocessableEntity)
}

// TestEvent_Create_NoAuth verifies POST /events without a token returns 401.
func TestEvent_Create_NoAuth(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.post(t, eventsURL(actor.orgSlug, matchID), map[string]any{
		"event_type": "raid_attempt",
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusUnauthorized)
}

// TestEvent_Create_NoPermission verifies that a same-org viewer cannot create events.
// The viewer is in the same org as the owner so 403 comes from permission denial,
// not from BOLA.
func TestEvent_Create_NoPermission(t *testing.T) {
	ts := buildTestServer(t, testPool)
	ctx := context.Background()

	ownerCtx := setupUserAndOrg(t, ts, "org_owner")
	orgUID := mustUUID(t, ownerCtx.orgID)

	viewerUser := fixtures.CreateActiveUser(ctx, t, ts.pool)
	fixtures.AddUserToOrg(ctx, t, ts.pool, orgUID, viewerUser.ID, "viewer")
	viewerToken := loginAs(t, ts, viewerUser.Email, fixtures.KnownPasswordRaw, ownerCtx.orgID)

	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, eventsURL(ownerCtx.orgSlug, matchID), map[string]any{
		"event_type": "raid_attempt",
	}, bearerHeader(viewerToken))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusForbidden)
}

// TestEvent_SequenceMonotonic verifies that successive POST /events calls produce
// strictly increasing sequence numbers, proving the event log is ordered.
func TestEvent_SequenceMonotonic(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)
	homeTeamID := pgutil.UUIDToString(setup.HomeTeam.ID)

	const n = 4
	seqs := make([]int64, n)
	for i := 0; i < n; i++ {
		resp := ts.postWithHeaders(t, eventsURL(actor.orgSlug, matchID), map[string]any{
			"event_type": "raid_attempt",
			"team_id":    homeTeamID,
		}, bearerHeader(actor.token))
		assertStatus(t, resp, http.StatusCreated)
		var ev eventResponse
		decodeBody(t, resp, &ev)
		resp.Body.Close()
		seqs[i] = ev.SequenceNumber
	}

	for i := 1; i < n; i++ {
		if seqs[i] <= seqs[i-1] {
			t.Errorf("sequence not monotonically increasing: seq[%d]=%d, seq[%d]=%d",
				i-1, seqs[i-1], i, seqs[i])
		}
	}
}

// TestEvent_Create_InvalidEventType verifies an unknown event_type returns 400.
func TestEvent_Create_InvalidEventType(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	resp := ts.postWithHeaders(t, eventsURL(actor.orgSlug, matchID), map[string]any{
		"event_type": "not_a_real_event_type",
	}, bearerHeader(actor.token))
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadRequest)
}

// TestEvent_List_WrongOrg_Invisible verifies events from Org A's match are not
// accessible via Org B's URL.
func TestEvent_List_WrongOrg_Invisible(t *testing.T) {
	ts := buildTestServer(t, testPool)

	orgA := setupUserAndOrg(t, ts, "org_owner")
	orgB := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgAUID := mustUUID(t, orgA.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgAUID)
	match := fixtures.CreateLiveMatch(ctx, t, ts.pool, orgAUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	// Org B actor tries to access Org A's match events via Org B's slug.
	resp := ts.get(t, eventsURL(orgB.orgSlug, matchID), bearerHeader(orgB.token))
	defer resp.Body.Close()
	// 404 (match not found in org B) or empty list.
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusOK {
		t.Errorf("expected 404 or 200 (empty), got %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK {
		var list eventListResponse
		decodeBody(t, resp, &list)
		if list.Total > 0 {
			t.Errorf("Org B should not see Org A's events; got %d events", list.Total)
		}
	}
}
