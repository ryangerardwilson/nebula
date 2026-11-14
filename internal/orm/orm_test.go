package orm

import (
	"strings"
	"testing"
)

func TestParseRoundTrip(t *testing.T) {
	text := "model item\n  id integer primary key\n  name text not null\n\nmodel note\n  id integer primary key\n  body text\n"
	models, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].Name != "item" || !models[0].Columns[0].PrimaryKey || !models[0].Columns[1].NotNull {
		t.Fatalf("%#v", models)
	}
	again, err := Parse(Format(models))
	if err != nil {
		t.Fatal(err)
	}
	if Format(again) != Format(models) {
		t.Fatalf("%s", Format(again))
	}
}

func TestParseRejectsTwoPrimaryKeys(t *testing.T) {
	_, err := Parse("model item\n  id integer primary key\n  other integer primary key\n")
	if err == nil || !strings.Contains(err.Error(), "two primary keys") {
		t.Fatal(err)
	}
}

func TestCompareAddsANullableColumn(t *testing.T) {
	live, err := Parse("model item\n  id integer primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse("model item\n  id integer primary key\n  name text\n")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Compare(live, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Add) != 1 || plan.Add[0].Column.Name != "name" {
		t.Fatalf("%#v", plan)
	}
}

func TestParseIndexAndDefaultRoundTrip(t *testing.T) {
	text := "model item\n  id integer primary key\n  name text not null default 'a'\n  n integer not null default 0\n  index item_name unique name\n"
	models, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Columns[1].Default != "'a'" || models[0].Columns[2].Default != "0" {
		t.Fatalf("%#v", models)
	}
	if len(models[0].Indexes) != 1 || !models[0].Indexes[0].Unique || models[0].Indexes[0].Columns[0] != "name" {
		t.Fatalf("%#v", models[0].Indexes)
	}
	again, err := Parse(Format(models))
	if err != nil {
		t.Fatal(err)
	}
	if Format(again) != Format(models) {
		t.Fatalf("%s", Format(again))
	}
	_, err = Parse("model item\n  id integer primary key\n  index item_id unique missing\n")
	if err == nil || !strings.Contains(err.Error(), "unknown column") {
		t.Fatal(err)
	}
}

func TestCompareAddsNotNullWithDefault(t *testing.T) {
	live, err := Parse("model item\n  id integer primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse("model item\n  id integer primary key\n  n integer not null default 0\n")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Compare(live, want)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Add) != 1 || plan.Add[0].Column.Default != "0" || !strings.Contains(AddColumnSQL("item", plan.Add[0].Column), "DEFAULT 0") {
		t.Fatalf("%#v", plan)
	}
}

func TestCompareRefusesNotNullOnAnExistingModel(t *testing.T) {
	live, err := Parse("model item\n  id integer primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse("model item\n  id integer primary key\n  name text not null\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compare(live, want)
	if err == nil || !strings.Contains(err.Error(), "nullable") {
		t.Fatal(err)
	}
}

func TestCompareRefusesATableOutsideTheORM(t *testing.T) {
	live, err := Parse("model rogue\n  id integer\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compare(live, nil)
	if err == nil || !strings.Contains(err.Error(), "rogue") {
		t.Fatal(err)
	}
}

func TestCompareRefusesATypeChange(t *testing.T) {
	live, err := Parse("model item\n  id integer primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	want, err := Parse("model item\n  id text primary key\n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = Compare(live, want)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatal(err)
	}
}

func TestDataSQLStaysInsideTheORM(t *testing.T) {
	if err := CheckDataSQL("select 'drop table item'"); err != nil {
		t.Fatal(err)
	}
	if err := CheckDataSQL("insert into item (id) values (1); select id from item"); err != nil {
		t.Fatal(err)
	}
	if err := CheckDataSQL("select 1; drop table item"); err != ErrSchemaSQL {
		t.Fatal(err)
	}
	if err := CheckDataSQL("create table item (id integer)"); err != ErrSchemaSQL {
		t.Fatal(err)
	}
	if err := CheckDataSQL("   "); err == nil {
		t.Fatal("empty sql was accepted")
	}
}
