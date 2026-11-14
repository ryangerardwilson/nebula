package pg

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
)

func TestRemoteCopyAndUsage(t *testing.T) {
	if os.Getenv("SQLX_PG_TEST") != "1" {
		t.Skip("set SQLX_PG_TEST=1 to use the remote postgres")
	}
	t.Setenv("SQLX_ENGINE", "postgres")
	t.Setenv("SQLX_ROOT", t.TempDir())
	ctx := context.Background()
	const name = "sqlxprobe"
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Drop(context.Background(), name) })
	_ = Drop(ctx, name)
	if err := sqliteExec(ctx, layout.DBPath(name), probeSQL); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(layout.DBPath(name)); !os.IsNotExist(err) {
		t.Fatal("sqlite file was not retired")
	}
	raw, err := QueryJSON(ctx, name, "SELECT name FROM item ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var names []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0].Name != "a'b" || names[1].Name != "line\ntwo" {
		t.Fatalf("%#v", names)
	}
	if err := Exec(ctx, name, "INSERT INTO item(name) VALUES ('c')"); err != nil {
		t.Fatal(err)
	}
	models, err := LiveModels(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	item, ok := modelNamed(models, "item")
	if !ok || !item.Columns[0].PrimaryKey || item.Columns[0].NotNull || item.Columns[0].Type != "integer" {
		t.Fatalf("%#v", item)
	}
	script := "BEGIN IMMEDIATE;\n" +
		"UPDATE usage SET n = n + 1 WHERE feature = 'help' AND actor = 'human' AND day = '2026-11-13';\n" +
		"INSERT INTO usage(feature, actor, day, n, ok, last) SELECT 'help', 'human', '2026-11-13', 1, 1, 5 WHERE changes() = 0;\n" +
		"COMMIT;\n"
	if err := Exec(ctx, name, script); err != nil {
		t.Fatal(err)
	}
	if err := Exec(ctx, name, script); err != nil {
		t.Fatal(err)
	}
	got, err := scalar(ctx, name, "SELECT n FROM usage WHERE feature = 'help'")
	if err != nil {
		t.Fatal(err)
	}
	if got != "2" {
		t.Fatal(got)
	}
	count, err := scalar(ctx, name, "SELECT count(*) FROM usage")
	if err != nil || count != "1" {
		t.Fatal(count, err)
	}
	changed, err := QueryJSON(ctx, name, "UPDATE item SET name = name WHERE id = 1;\nSELECT changes() AS n;")
	if err != nil || changed != `[{"n":1}]` {
		t.Fatalf("%q %v", changed, err)
	}
	missed, err := QueryJSON(ctx, name, "BEGIN IMMEDIATE;\nDELETE FROM item WHERE name = 'missing';\nCOMMIT;\nSELECT changes() AS n;")
	if err != nil || missed != `[{"n":0}]` {
		t.Fatalf("%q %v", missed, err)
	}
}

func modelNamed(models []orm.Model, name string) (orm.Model, bool) {
	for _, model := range models {
		if model.Name == name {
			return model, true
		}
	}
	return orm.Model{}, false
}

func TestProbeFixtureBuilds(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil && !strings.Contains(probeSQL, "model usage") {
		t.Fatal(err)
	}
}

const probeSQL = `
CREATE TABLE item (id integer primary key, name text not null);
INSERT INTO item(name) VALUES ('a''b');
INSERT INTO item(name) VALUES ('line
two');
CREATE TABLE usage (
  id integer primary key,
  feature text not null,
  actor text not null,
  day text not null,
  n integer not null,
  ok integer not null,
  last integer not null
);
CREATE TABLE sqlx_orm (
  model text primary key,
  body text not null,
  position integer not null
);
INSERT INTO sqlx_orm(model, body, position) VALUES ('usage', 'model usage
  id integer primary key
  feature text not null
  actor text not null
  day text not null
  n integer not null
  ok integer not null
  last integer not null
  index usage_feature_actor_day unique feature actor day
', 0);
`
