package agent

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/store"
)

func TestMigrateQueryAndInspect(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := "model item\n  id integer primary key\n  name text not null\n"
	if err := os.WriteFile(filepath.Join(dir, "item.model"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("skip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Migrate(ctx, "app", dir, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "created item") {
		t.Fatal(out.String())
	}
	var query bytes.Buffer
	if err := Query(ctx, "app", "insert into item (id, name) values (1, 'a')", &query); err != nil {
		t.Fatal(err)
	}
	if err := Query(ctx, "app", "drop table item", &query); err == nil {
		t.Fatal("schema sql was accepted")
	}
	text, err := Inspect(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "orm match") || !strings.Contains(text, "item\tid integer primary key, name text not null\t1\n") {
		t.Fatal(text)
	}
	schema, err := Schema(ctx, "app")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(schema, "model item\n  id integer primary key\n") {
		t.Fatal(schema)
	}
	out.Reset()
	if err := Migrate(ctx, "app", dir, &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "unchanged" {
		t.Fatal(out.String())
	}
	if err := store.Exec(ctx, layout.DBPath("app"), "create table rogue (id integer)"); err != nil {
		t.Fatal(err)
	}
	if err := Query(ctx, "app", "select 1", &query); err == nil || !strings.Contains(err.Error(), "rogue") {
		t.Fatal(err)
	}
}

func TestMigrateRefusesARemovedModel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "schema")
	if err := os.WriteFile(file, []byte("model item\n  id integer primary key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, "app", file, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("model other\n  id integer primary key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Migrate(ctx, "app", file, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "item") {
		t.Fatal(err)
	}
}

func TestJSONExecRenameOnceAndIndex(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "item.model")
	body := "model item\n  id integer primary key\n  name text\n  index item_name unique name\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Migrate(ctx, "app", file, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "created item") || !strings.Contains(out.String(), "indexed item_name") {
		t.Fatal(out.String())
	}
	if err := Exec(ctx, "app", "insert into item (id, name) values (1, 'a')"); err != nil {
		t.Fatal(err)
	}
	if err := Exec(ctx, "app", "drop table item"); err == nil {
		t.Fatal("schema sql was accepted")
	}
	var query bytes.Buffer
	if err := QueryJSON(ctx, "app", "select name, n from item", &query); err == nil {
		t.Fatal("missing column was accepted")
	}
	body = "model item\n  id integer primary key\n  name text\n  n integer not null default 0\n  index item_name unique name\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Migrate(ctx, "app", file, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "added item.n") {
		t.Fatal(out.String())
	}
	query.Reset()
	if err := QueryJSON(ctx, "app", "select id, name, n from item", &query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query.String(), "a") {
		t.Fatal(query.String())
	}
	cols, err := Columns(ctx, "app", "missing")
	if err != nil || cols != "" {
		t.Fatalf("missing table %q %v", cols, err)
	}
	out.Reset()
	if err := Once(ctx, "app", "note", "select 1", &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "applied" {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := Once(ctx, "app", "note", "select 1", &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "done" {
		t.Fatal(out.String())
	}
	if err := RenameDatabase("app", "next"); err != nil {
		t.Fatal(err)
	}
	names, err := Names()
	if err != nil || len(names) != 1 || names[0] != "next" {
		t.Fatal(names, err)
	}
	if err := RenameDatabase("gone", "next"); err != nil {
		t.Fatal(err)
	}
	if err := DropDatabase("next"); err != nil {
		t.Fatal(err)
	}
	names, err = Names()
	if err != nil || len(names) != 0 {
		t.Fatal(names, err)
	}
}

func TestExecAcceptsAStatementLargerThanOneArgument(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	ctx := context.Background()
	if err := CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "note.model")
	body := "model note\n  id integer primary key\n  body text\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, "app", file, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("a", 200_000)
	statement := "insert into note (id, body) values (1, '" + payload + "')"
	if err := Exec(ctx, "app", statement); err != nil {
		t.Fatal(err)
	}
	var query bytes.Buffer
	if err := QueryJSON(ctx, "app", "select length(body) as n from note", &query); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query.String(), "200000") {
		t.Fatal(query.String())
	}
}

func TestNamesReadsTheMemoryCopy(t *testing.T) {
	t.Setenv("SQLX_ENGINE", "postgres")
	dir := t.TempDir()
	t.Setenv("SQLX_RUNTIME", dir)
	if err := os.WriteFile(filepath.Join(dir, "crusade.sqlite"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := Names()
	if err != nil || len(names) != 1 || names[0] != "crusade" {
		t.Fatal(names, err)
	}
}

func TestBadDatabaseName(t *testing.T) {
	err := CreateDatabase(context.Background(), "app-db")
	if err == nil || !errorsUsage(err) {
		t.Fatal(err)
	}
}

func TestLocalCommitWritesTheRuntimeCopy(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	t.Setenv("SQLX_ENGINE", "")
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
	var out bytes.Buffer
	if err := Migrate(ctx, "app", dir, &out); err != nil {
		t.Fatal(err)
	}
	src := layout.DBPath("app")
	if err := store.Exec(ctx, src, "PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	runtime := t.TempDir()
	t.Setenv("SQLX_RUNTIME", runtime)
	t.Setenv("SQLX_ENGINE", "postgres")
	t.Setenv("SQLX_COMMIT", "local")
	t.Setenv("SQLX_PG_SSH", "bad host")
	if err := os.WriteFile(cache.DBPath("app"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Exec(ctx, "app", "INSERT INTO item(name) VALUES ('z')"); err != nil {
		t.Fatal(err)
	}
	got, err := store.QueryCSV(ctx, cache.DBPath("app"), "SELECT name FROM item")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "z") {
		t.Fatalf("runtime copy %q", got)
	}
}

func TestLocalCommitDoesNotDialWhenTheRuntimeCopyIsMissing(t *testing.T) {
	t.Setenv("SQLX_RUNTIME", t.TempDir())
	t.Setenv("SQLX_ENGINE", "postgres")
	t.Setenv("SQLX_COMMIT", "local")
	t.Setenv("SQLX_PG_SSH", "bad host")
	err := Exec(context.Background(), "app", "INSERT INTO item(name) VALUES ('z')")
	if err == nil || !strings.Contains(err.Error(), "no runtime copy") {
		t.Fatal(err)
	}
}

func errorsUsage(err error) bool {
	return err != nil && strings.Contains(err.Error(), "letters, digits, and underscores")
}
