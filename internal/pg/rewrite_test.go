package pg

import (
	"strings"
	"testing"
)

func TestRewriteKeepsOrdinarySQL(t *testing.T) {
	in := "SELECT 'BEGIN IMMEDIATE', 'changes()'\n"
	got, err := Rewrite(in)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("%q", got)
	}
}

func TestRewriteBeginImmediateAndChanges(t *testing.T) {
	in := "BEGIN IMMEDIATE;\n" +
		"UPDATE usage SET n = n + 1 WHERE feature = 'a';\n" +
		"INSERT INTO usage(feature, n) SELECT 'a', 1 WHERE changes() = 0;\n" +
		"COMMIT;\n"
	got, err := Rewrite(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "IMMEDIATE") || strings.Contains(got, "changes()") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "pg_advisory_xact_lock") || !strings.Contains(got, "sqlx_updated") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "RETURNING 1") || !strings.Contains(got, "COMMIT;") {
		t.Fatal(got)
	}
}

func TestRewriteDropsPragma(t *testing.T) {
	got, err := Rewrite("PRAGMA journal_mode=WAL;\nSELECT 1;\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(got), "pragma") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "SELECT 1;") {
		t.Fatal(got)
	}
}

func TestRewriteRefusesChangesWithoutAWrite(t *testing.T) {
	_, err := Rewrite("SELECT changes();\n")
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRewriteChangesAfterCommit(t *testing.T) {
	got, err := Rewrite("BEGIN IMMEDIATE;\nDELETE FROM task WHERE id = 1;\nCOMMIT;\nSELECT changes() AS n;\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "changes()") || strings.Contains(got, "IMMEDIATE") {
		t.Fatal(got)
	}
	del := strings.Index(got, "DELETE FROM task")
	commit := strings.Index(got, "COMMIT;")
	if del < 0 || commit < del || !strings.Contains(got, "sqlx_changed") {
		t.Fatal(got)
	}
}

func TestJSONTextKeepsTheWriteAtTheTop(t *testing.T) {
	got, err := Rewrite("UPDATE item SET name = 'c' WHERE id = 1;\nSELECT changes() AS n;\n")
	if err != nil {
		t.Fatal(err)
	}
	parts, err := statementsOf(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 {
		t.Fatalf("%q", got)
	}
	text := jsonText(parts[0])
	if !strings.HasPrefix(strings.TrimSpace(text), "WITH sqlx_updated") || !strings.Contains(text, "json_agg") {
		t.Fatal(text)
	}
	if strings.Contains(text, "FROM (WITH") || strings.Contains(text, "FROM (\nWITH") {
		t.Fatal(text)
	}
}
