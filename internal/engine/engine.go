package engine

import "os"

// Mode is sqlite, postgres, or s3.
// SQLX_ENGINE forces one of those.
// A set SQLX_ROOT with no engine keeps the sqlite tree used by tests.
// Production, with neither set, talks to Postgres on the SSH host.
// Applications keep their own sqlite and back it up with s3. They do not call sql.
func Mode() string {
	switch os.Getenv("SQLX_ENGINE") {
	case "postgres", "sqlite", "s3":
		return os.Getenv("SQLX_ENGINE")
	default:
		if os.Getenv("SQLX_ROOT") != "" {
			return "sqlite"
		}
		return "postgres"
	}
}

// Postgres reports whether this command talks to Postgres.
func Postgres() bool { return Mode() == "postgres" }

// Object reports whether sql itself snapshots a sqlite file to s3.
// That mode is opt-in. Applications do not use it.
func Object() bool { return Mode() == "s3" }

// Runtime reports that sql is serving a login-runtime sqlite copy.
func Runtime() bool { return Postgres() || Object() }

func SSHHost() string {
	if v := os.Getenv("SQLX_PG_SSH"); v != "" {
		return v
	}
	return "del-vc2"
}
