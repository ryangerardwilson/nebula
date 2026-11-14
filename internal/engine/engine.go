package engine

import "os"

// Mode is sqlite, postgres, or s3.
// SQLX_ENGINE forces one of those.
// A set SQLX_ROOT with no engine keeps the sqlite tree used by tests.
// Production, with neither set, backs the runtime sqlite up as an s3 object.
func Mode() string {
	switch os.Getenv("SQLX_ENGINE") {
	case "postgres", "sqlite", "s3":
		return os.Getenv("SQLX_ENGINE")
	default:
		if os.Getenv("SQLX_ROOT") != "" {
			return "sqlite"
		}
		return "s3"
	}
}

// Postgres reports whether this command talks to Postgres.
func Postgres() bool { return Mode() == "postgres" }

// Object reports whether the runtime sqlite is backed up as an s3 object.
func Object() bool { return Mode() == "s3" }

// Runtime reports that day-to-day reads use the login runtime sqlite.
func Runtime() bool { return Postgres() || Object() }

func SSHHost() string {
	if v := os.Getenv("SQLX_PG_SSH"); v != "" {
		return v
	}
	return "del-vc2"
}
