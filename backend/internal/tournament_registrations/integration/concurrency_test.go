package tournament_registrations_integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/4yushraman-jpg/playarena/internal/platform/pgutil"
	"github.com/4yushraman-jpg/playarena/internal/testutil/fixtures"
)

// TestRegistration_Race_DuplicateRegistration fires 4 goroutines that all attempt
// to register the same team for the same tournament simultaneously using a
// close(chan)-based barrier. Exactly one must succeed (201); the rest must receive
// 409 (already registered). This exercises the UNIQUE constraint + tournament row
// lock that serialize concurrent registration attempts.
func TestRegistration_Race_DuplicateRegistration(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	tournament := fixtures.CreateRegistrationOpenTournament(ctx, t, ts.pool, orgUID)
	team, _ := fixtures.CreateTeamWithMember(ctx, t, ts.pool, orgUID)
	tournamentID := pgutil.UUIDToString(tournament.ID)
	teamID := pgutil.UUIDToString(team.ID)

	url := ts.url + registrationsURL(actor.orgSlug, tournamentID)
	body, _ := json.Marshal(map[string]any{"team_id": teamID})

	const workers = 4
	start := make(chan struct{})
	statuses := make([]int, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+actor.token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses[i] = -1
				return
			}
			statuses[i] = resp.StatusCode
			resp.Body.Close()
		}()
	}

	close(start)
	wg.Wait()

	created := 0
	for _, code := range statuses {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			// expected for all but one
		default:
			t.Errorf("unexpected status %d; expected 201 or 409", code)
		}
	}
	if created != 1 {
		t.Errorf("expected exactly 1 successful registration, got %d; all statuses: %v",
			created, statuses)
	}
}
