package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/agent"
)

func TestHelpAndVersion(t *testing.T) {
	var out, err bytes.Buffer
	app := App{Out: &out, Err: &err}
	if code := app.Run(nil); code != 0 {
		t.Fatal(code)
	}
	text := out.String()
	if strings.Contains(text, "\033") {
		t.Fatal("help on a pipe was colored")
	}
	for _, want := range []string{
		"features:",
		"sql app query \"select 1\"",
		"sql app inspect",
		"sql app schema",
		"sql app migrate ~/app/models",
		"model <name>",
		"sql backup",
		"sql create database app",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %s\n%s", want, text)
		}
	}
	if strings.Contains(text, "\ncommands:\n") {
		t.Fatal(text)
	}
	out.Reset()
	if code := app.Run([]string{"version"}); code != 0 || strings.TrimSpace(out.String()) != Version {
		t.Fatalf("version %q %d", out.String(), code)
	}
}

func TestDatabaseCommands(t *testing.T) {
	var out, err bytes.Buffer
	var queried, migrated string
	app := App{
		Out: &out,
		Err: &err,
		Query: func(db, sql string) error {
			queried = db + " " + sql
			return nil
		},
		Migrate: func(db, path string) error {
			migrated = db + " " + path
			return nil
		},
		Inspect: func(db string) (string, error) {
			return "database " + db + "\n", nil
		},
		Schema: func(db string) (string, error) {
			return "model " + db + "\n", nil
		},
	}
	if code := app.Run([]string{"app", "query", "select", "1"}); code != 0 || queried != "app select 1" {
		t.Fatal(code, queried, err.String())
	}
	var jsonned string
	app.QueryJSON = func(db, sql string) error {
		jsonned = db + " " + sql
		return nil
	}
	if code := app.Run([]string{"app", "query", "--json", "select", "1"}); code != 0 || jsonned != "app select 1" {
		t.Fatal(code, jsonned, err.String())
	}
	if code := app.Run([]string{"app", "migrate", "/tmp/models"}); code != 0 || migrated != "app /tmp/models" {
		t.Fatal(code, migrated, err.String())
	}
	out.Reset()
	if code := app.Run([]string{"app", "inspect"}); code != 0 || !strings.Contains(out.String(), "database app") {
		t.Fatal(out.String(), code)
	}
	out.Reset()
	if code := app.Run([]string{"app", "schema"}); code != 0 || !strings.Contains(out.String(), "model app") {
		t.Fatal(out.String(), code)
	}
}

func TestUsage(t *testing.T) {
	var out, err bytes.Buffer
	app := App{Out: &out, Err: &err}
	if code := app.Run([]string{"create", "database"}); code != 2 {
		t.Fatalf("create %d %s", code, err.String())
	}
	err.Reset()
	if code := app.Run([]string{"app", "query"}); code != 2 {
		t.Fatalf("query %d %s", code, err.String())
	}
	err.Reset()
	if code := app.Run([]string{"nope"}); code != 2 {
		t.Fatalf("unknown %d %s", code, err.String())
	}
	err.Reset()
	app.Create = func(name string) error {
		return agent.ErrUsage
	}
	if code := app.Run([]string{"create", "database", "bad-name"}); code != 2 {
		t.Fatalf("bad name %d", code)
	}
	app.Backup = func() error { return errString("sync failed") }
	if code := app.Run([]string{"backup"}); code != 1 {
		t.Fatalf("backup %d", code)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestExecAndQueryReadAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "note.sql")
	if err := os.WriteFile(file, []byte("delete from item\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, err bytes.Buffer
	var executed, queried, jsonned string
	app := App{
		Out: &out,
		Err: &err,
		Exec: func(db, sql string) error {
			executed = db + " " + sql
			return nil
		},
		Query: func(db, sql string) error {
			queried = db + " " + sql
			return nil
		},
		QueryJSON: func(db, sql string) error {
			jsonned = db + " " + sql
			return nil
		},
	}
	if code := app.Run([]string{"app", "exec", "--file", file}); code != 0 || executed != "app delete from item\n" {
		t.Fatal(code, executed, err.String())
	}
	if code := app.Run([]string{"app", "query", "--file", file}); code != 0 || queried != "app delete from item\n" {
		t.Fatal(code, queried, err.String())
	}
	if code := app.Run([]string{"app", "query", "--json", "--file", file}); code != 0 || jsonned != "app delete from item\n" {
		t.Fatal(code, jsonned, err.String())
	}
	if code := app.Run([]string{"app", "exec", "--file"}); code != 2 {
		t.Fatalf("exec file usage %d %s", code, err.String())
	}
}
