package auth_integration_test

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/4yushraman-jpg/playarena/internal/testutil"
)

// testPool is shared across all integration tests in this package.
// It is initialised once by TestMain before any test function runs.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.SetupTestDB(m)
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}
