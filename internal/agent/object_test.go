package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/store"
	"github.com/ryangerardwilson/nebula/s3"
)

func TestObjectBackupSurvivesALostRuntimeCopy(t *testing.T) {
	t.Setenv("SQLX_ENGINE", "s3")
	t.Setenv("SQLX_ROOT", "")
	t.Setenv("S3_ROOT", t.TempDir())
	t.Setenv("SQLX_RUNTIME", t.TempDir())
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "model item\n  id integer primary key\n  name text not null\n"
	if err := os.WriteFile(filepath.Join(dir, "item.model"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, "app", dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Exec(ctx, "app", "insert into item (name) values ('kept')"); err != nil {
		t.Fatal(err)
	}
	if err := cache.Remove("app"); err != nil {
		t.Fatal(err)
	}
	var query bytes.Buffer
	if err := Query(ctx, "app", "select name from item", &query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query.String(), "kept") {
		t.Fatal(query.String())
	}
}

func TestLocalCommitDoesNotPublishUntilTheBackup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ENGINE", "s3")
	t.Setenv("S3_ROOT", root)
	t.Setenv("SQLX_RUNTIME", t.TempDir())
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "model item\n  id integer primary key\n  name text not null\n"
	if err := os.WriteFile(filepath.Join(dir, "item.model"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, "app", dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SQLX_COMMIT", "local")
	if err := Exec(ctx, "app", "insert into item (name) values ('local')"); err != nil {
		t.Fatal(err)
	}
	snap := filepath.Join(t.TempDir(), "snap.sqlite")
	if err := s3.Get(ctx, s3.DatabaseBucket, "app.sqlite", snap); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryCSV(ctx, snap, "select name from item")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "local") {
		t.Fatalf("backup includes a local-only write: %q", got)
	}
	t.Setenv("SQLX_COMMIT", "remote")
	if err := Exec(ctx, "app", "insert into item (name) values ('ignored')"); err != nil {
		t.Fatal(err)
	}
	if err := s3.Get(ctx, s3.DatabaseBucket, "app.sqlite", snap); err != nil {
		t.Fatal(err)
	}
	got, err = store.QueryCSV(ctx, snap, "select name from item")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "local") || strings.Contains(got, "ignored") {
		t.Fatalf("remote backup %q", got)
	}
}

func TestFailedPublishRestoresTheRuntimeCopy(t *testing.T) {
	t.Setenv("SQLX_ENGINE", "s3")
	t.Setenv("S3_ROOT", t.TempDir())
	t.Setenv("SQLX_RUNTIME", t.TempDir())
	t.Setenv("SQLX_COMMIT", "")
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "model item\n  id integer primary key\n  name text not null\n"
	if err := os.WriteFile(filepath.Join(dir, "item.model"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, "app", dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("S3_ROOT", blocked)
	err := Exec(ctx, "app", "insert into item (name) values ('lost')")
	if err == nil {
		t.Fatal("publish succeeded")
	}
	got, qerr := store.QueryCSV(ctx, cache.DBPath("app"), "select name from item")
	if qerr != nil {
		t.Fatal(qerr)
	}
	if strings.Contains(got, "lost") {
		t.Fatalf("runtime kept a write the backup refused: %q", got)
	}
}
