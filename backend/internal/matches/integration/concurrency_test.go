package matches_integration_test

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

// TestMatch_Race_ConcurrentStatusTransition fires 6 goroutines that simultaneously
// attempt to transition the same match from scheduled→live. Exactly one must succeed
// (200); the rest must receive 422 (ErrMatchNotUpdatable — CAS failure). This
// exercises the FOR UPDATE row lock inside UpdateMatchTx that serialises state
// transitions.
func TestMatch_Race_ConcurrentStatusTransition(t *testing.T) {
	ts := buildTestServer(t, testPool)
	actor := setupUserAndOrg(t, ts, "org_owner")

	ctx := context.Background()
	orgUID := mustUUID(t, actor.orgID)
	setup := fixtures.CreateOngoingTournamentWithTeams(ctx, t, ts.pool, orgUID)
	match := fixtures.CreateScheduledMatch(ctx, t, ts.pool, orgUID,
		setup.Tournament.ID, setup.HomeTeam.ID, setup.AwayTeam.ID)
	matchID := pgutil.UUIDToString(match.ID)

	url := ts.url + matchURL(actor.orgSlug, matchID)
	body, _ := json.Marshal(map[string]any{"status": "live"})

	const workers = 6
	start := make(chan struct{})
	statuses := make([]int, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
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

	successes := 0
	for _, code := range statuses {
		switch code {
		case http.StatusOK:
			successes++
		case http.StatusUnprocessableEntity:
			// CAS failure: ErrMatchNotUpdatable
		default:
			t.Errorf("unexpected status %d; expected 200 or 422", code)
		}
	}
	if successes != 1 {
		t.Errorf("expected exactly 1 successful transition, got %d; all statuses: %v",
			successes, statuses)
	}
}
