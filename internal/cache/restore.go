package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/orm"
	"github.com/ryangerardwilson/nebula/internal/pg"
)

const pageSize = 5000

func restore(ctx context.Context, name, path string) error {
	part := path + ".part"
	_ = os.Remove(part)
	defer func() { _ = os.Remove(part) }()
	if err := skeleton(ctx, part); err != nil {
		return err
	}
	if err := copyModels(ctx, name, part); err != nil {
		return err
	}
	if err := copyCatalog(ctx, name, part); err != nil {
		return err
	}
	if err := os.Chmod(part, 0o600); err != nil {
		return err
	}
	return os.Rename(part, path)
}

func skeleton(ctx context.Context, path string) error {
	return sqliteExec(ctx, path, `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA temp_store=MEMORY;
CREATE TABLE sqlx_orm (
  model TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  position INTEGER NOT NULL
);
CREATE TABLE sqlx_once (
  id TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
);
`)
}

func copyModels(ctx context.Context, name, path string) error {
	models, err := pg.LiveModels(ctx, name)
	if err != nil {
		return err
	}
	for _, model := range models {
		if err := sqliteExec(ctx, path, orm.CreateSQL(model)+";"); err != nil {
			return err
		}
		if err := copyTable(ctx, name, path, model.Name, columnNames(model)); err != nil {
			return err
		}
	}
	return nil
}

func copyCatalog(ctx context.Context, name, path string) error {
	if err := copyTable(ctx, name, path, "sqlx_orm", []string{"model", "body", "position"}); err != nil {
		return err
	}
	return copyOptional(ctx, name, path, "sqlx_once", []string{"id", "applied_at"})
}

func copyOptional(ctx context.Context, name, path, table string, cols []string) error {
	err := copyTable(ctx, name, path, table, cols)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		return nil
	}
	return err
}

func copyTable(ctx context.Context, name, path, table string, cols []string) error {
	quoted := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = orm.QuoteIdent(col)
	}
	list := strings.Join(quoted, ", ")
	from := orm.QuoteIdent(table)
	offset := 0
	for {
		query := fmt.Sprintf("SELECT %s FROM %s ORDER BY 1, ctid LIMIT %d OFFSET %d", list, from, pageSize, offset)
		raw, err := pg.QueryJSON(ctx, name, query)
		if err != nil {
			return err
		}
		rows, err := decodeRows(raw)
		if err != nil {
			return err
		}
		if err := insertRows(ctx, path, table, cols, rows); err != nil {
			return err
		}
		if len(rows) < pageSize {
			return nil
		}
		offset += pageSize
	}
}

func columnNames(model orm.Model) []string {
	names := make([]string, len(model.Columns))
	for i, column := range model.Columns {
		names[i] = column.Name
	}
	return names
}

func decodeRows(raw string) ([]map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var rows []map[string]any
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("backup json: %w", err)
	}
	return rows, nil
}

func insertRows(ctx context.Context, path, table string, cols []string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	quoted := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = orm.QuoteIdent(col)
	}
	var b strings.Builder
	b.WriteString("BEGIN IMMEDIATE;\n")
	prefix := "INSERT INTO " + orm.QuoteIdent(table) + "(" + strings.Join(quoted, ", ") + ") VALUES ("
	for _, row := range rows {
		b.WriteString(prefix)
		for i, col := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			text, err := literal(row[col])
			if err != nil {
				return err
			}
			b.WriteString(text)
		}
		b.WriteString(");\n")
	}
	b.WriteString("COMMIT;\n")
	return sqliteExec(ctx, path, b.String())
}

func literal(value any) (string, error) {
	if value == nil {
		return "NULL", nil
	}
	switch v := value.(type) {
	case string:
		return orm.QuoteLiteral(v), nil
	case json.Number:
		return v.String(), nil
	case bool:
		if v {
			return "1", nil
		}
		return "0", nil
	default:
		return "", fmt.Errorf("unsupported backup value %T", value)
	}
}
