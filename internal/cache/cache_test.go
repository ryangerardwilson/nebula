package cache

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/orm"
)

func TestDirUsesRuntimeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SQLX_RUNTIME", dir)
	if Dir() != dir {
		t.Fatal(Dir())
	}
	if DBPath("tesseract") != filepath.Join(dir, "tesseract.sqlite") {
		t.Fatal(DBPath("tesseract"))
	}
}

func TestLiteralQuotesText(t *testing.T) {
	got, err := literal("a'b")
	if err != nil || got != "'a''b'" {
		t.Fatal(got, err)
	}
	got, err = literal(json.Number("12"))
	if err != nil || got != "12" {
		t.Fatal(got, err)
	}
	got, err = literal(nil)
	if err != nil || got != "NULL" {
		t.Fatal(got, err)
	}
}

func TestInsertRowsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SQLX_RUNTIME", dir)
	path := filepath.Join(dir, "app.sqlite")
	if err := sqliteExec(context.Background(), path, `CREATE TABLE item (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{{"id": json.Number("2"), "name": "a'b"}}
	if err := insertRows(context.Background(), path, "item", []string{"id", "name"}, rows); err != nil {
		t.Fatal(err)
	}
	raw, err := sqliteJSON(context.Background(), path, "SELECT id, name FROM item")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeRows(raw)
	if err != nil || len(decoded) != 1 || decoded[0]["name"] != "a'b" {
		t.Fatal(raw, err)
	}
}

func TestSchemaStamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SQLX_RUNTIME", dir)
	parsed, err := orm.Parse("model item\n  id integer primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	if SchemaCurrent("item", parsed) {
		t.Fatal("stamp existed")
	}
	if err := MarkSchema("item", parsed); err != nil {
		t.Fatal(err)
	}
	if !SchemaCurrent("item", parsed) {
		t.Fatal("stamp was not recorded")
	}
}
