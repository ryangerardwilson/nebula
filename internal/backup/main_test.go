package backup

import (
	"os"
	"testing"
)

// TestMain keeps unit tests off the operator's production database. The
// login environment sets SQLX_ENGINE=postgres, which outranks SQLX_ROOT, so a
// test that only sets SQLX_ROOT would otherwise reach Postgres on del-vc2.
// SQLX_PG_TEST=1 keeps the real host for the opt-in remote test.
func TestMain(m *testing.M) {
	_ = os.Unsetenv("SQLX_ENGINE")
	_ = os.Unsetenv("SQLX_COMMIT")
	if os.Getenv("SQLX_PG_TEST") != "1" {
		_ = os.Setenv("SQLX_PG_SSH", "nebula-test.invalid")
	}
	os.Exit(m.Run())
}
