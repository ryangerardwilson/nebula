package engine

import "os"

// Mode is sqlite, postgres, or s3.
// SQLX_ENGINE=s3 opts into s3. A set SQLX_ROOT, or SQLX_ENGINE=sqlite, keeps
// the sqlite tree under that root and never opens Postgres, even when the
// login environment sets SQLX_ENGINE=postgres. Tests rely on that.
// Production, with no root, talks to Postgres on the SSH host.
// Applications keep their own sqlite and back it up with s3. They do not call sql.
func Mode() string {
	engine := os.Getenv("SQLX_ENGINE")
	switch {
	case engine == "s3":
		return "s3"
	case engine == "sqlite" || os.Getenv("SQLX_ROOT") != "":
		return "sqlite"
	default:
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
