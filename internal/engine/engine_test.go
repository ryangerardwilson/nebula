package engine

import "testing"

func TestEngineFollowsRootAndOverride(t *testing.T) {
	t.Setenv("SQLX_ENGINE", "")
	t.Setenv("SQLX_ROOT", t.TempDir())
	if Postgres() {
		t.Fatal("SQLX_ROOT should stay on sqlite")
	}
	t.Setenv("SQLX_ROOT", "")
	if !Postgres() || Object() {
		t.Fatal("an unset root should use postgres")
	}
	t.Setenv("SQLX_ENGINE", "s3")
	if !Object() || Postgres() {
		t.Fatal("SQLX_ENGINE=s3 should force s3")
	}
	t.Setenv("SQLX_ENGINE", "sqlite")
	t.Setenv("SQLX_ROOT", "")
	if Postgres() {
		t.Fatal("SQLX_ENGINE=sqlite should force sqlite")
	}
	t.Setenv("SQLX_ENGINE", "postgres")
	t.Setenv("SQLX_ROOT", t.TempDir())
	if Postgres() {
		t.Fatal("SQLX_ROOT should outrank SQLX_ENGINE=postgres")
	}
	t.Setenv("SQLX_ROOT", "")
	if !Postgres() {
		t.Fatal("SQLX_ENGINE=postgres should use postgres")
	}
	if SSHHost() != "del-vc2" {
		t.Fatal(SSHHost())
	}
	t.Setenv("SQLX_PG_SSH", "db.example")
	if SSHHost() != "db.example" {
		t.Fatal(SSHHost())
	}
}
