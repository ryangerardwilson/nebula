package pg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/orm"
)

type liveRow struct {
	Model   string `json:"model"`
	Name    string `json:"name"`
	Format  string `json:"format"`
	NotNull int    `json:"notnull"`
	PK      int    `json:"pk"`
	Comment string `json:"comment"`
}

func LiveModels(ctx context.Context, db string) ([]orm.Model, error) {
	raw, err := QueryJSON(ctx, db, liveSQL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var rows []liveRow
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, fmt.Errorf("postgres json: %w", err)
	}
	var models []orm.Model
	index := map[string]int{}
	for _, row := range rows {
		if strings.HasPrefix(row.Model, "sqlx_") {
			continue
		}
		at, ok := index[row.Model]
		if !ok {
			models = append(models, orm.Model{Name: row.Model})
			at = len(models) - 1
			index[row.Model] = at
		}
		kind, err := ormType(row.Format)
		if err != nil {
			return nil, fmt.Errorf("column %s.%s: %w", row.Model, row.Name, err)
		}
		notNull := row.NotNull == 1
		if row.PK == 1 {
			notNull = row.Comment == "sqlx_notnull"
		}
		models[at].Columns = append(models[at].Columns, orm.Column{
			Name:       row.Name,
			Type:       kind,
			PrimaryKey: row.PK == 1,
			NotNull:    notNull,
		})
	}
	return models, nil
}

func ColumnsSQL(table string) string {
	return `SELECT a.attname AS name
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relname = ` + orm.QuoteLiteral(table) + `
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`
}

const liveSQL = `SELECT c.relname AS model,
  a.attname AS name,
  pg_catalog.format_type(a.atttypid, a.atttypmod) AS format,
  CASE WHEN a.attnotnull THEN 1 ELSE 0 END AS notnull,
  CASE WHEN EXISTS (
    SELECT 1 FROM pg_index i
    WHERE i.indrelid = c.oid AND i.indisprimary AND a.attnum = ANY (i.indkey)
  ) THEN 1 ELSE 0 END AS pk,
  COALESCE(col_description(c.oid, a.attnum), '') AS comment
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE n.nspname = 'public' AND c.relkind = 'r'
ORDER BY c.relname, a.attnum`

func ormType(format string) (string, error) {
	switch format {
	case "bigint", "integer", "smallint":
		return "integer", nil
	case "text":
		return "text", nil
	case "double precision", "real":
		return "real", nil
	case "bytea":
		return "blob", nil
	default:
		if strings.HasPrefix(format, "character varying") {
			return "text", nil
		}
		return "", fmt.Errorf("type %s", format)
	}
}
