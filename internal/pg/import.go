package pg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
)

func Ensure(ctx context.Context, name string) error {
	if err := appDB(name); err != nil {
		return err
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	ok, err := Exists(ctx, name)
	if err != nil {
		return err
	}
	if ok {
		return Exec(ctx, name, ormTable)
	}
	path := layout.DBPath(name)
	info, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !info.IsDir() {
		return importSQLite(ctx, name, path)
	}
	if err := createDB(ctx, name); err != nil {
		return err
	}
	return Exec(ctx, name, ormTable)
}

func importSQLite(ctx context.Context, name, path string) error {
	unlock, err := lockDB(name)
	if err != nil {
		return err
	}
	defer unlock()
	if err := createDB(ctx, name); err != nil {
		return err
	}
	kept := false
	defer func() {
		if !kept {
			_ = Drop(ctx, name)
		}
	}()
	fmt.Fprintf(os.Stderr, "import %s\n", name)
	if err := sqliteExec(ctx, path, "PRAGMA busy_timeout=5000; PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		return err
	}
	before, err := fileSignature(path)
	if err != nil {
		return err
	}
	if err := copySQLite(ctx, name, path); err != nil {
		return err
	}
	after, err := fileSignature(path)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("database %s changed during copy", name)
	}
	kept = true
	return retireSQLite(path, name)
}

func copySQLite(ctx context.Context, name, path string) error {
	tables, err := sqliteTables(ctx, path)
	if err != nil {
		return err
	}
	indexes, err := sqliteIndexes(ctx, path)
	if err != nil {
		return err
	}
	var sequences []columnRef
	for _, table := range tables {
		cols, err := sqliteColumns(ctx, path, table)
		if err != nil {
			return err
		}
		if err := execRaw(ctx, name, createTableSQL(table, cols)); err != nil {
			return err
		}
		if err := copyTable(ctx, name, path, table, cols); err != nil {
			return err
		}
		for _, col := range cols {
			if col.PK && strings.EqualFold(col.Type, "integer") {
				sequences = append(sequences, columnRef{table, col.Name})
			}
		}
	}
	if err := createIndexes(ctx, name, indexes); err != nil {
		return err
	}
	if err := resetSequences(ctx, name, sequences); err != nil {
		return err
	}
	return verifyCounts(ctx, name, path, tables)
}

type columnRef struct {
	table string
	name  string
}

func copyTable(ctx context.Context, db, path, table string, cols []column) error {
	var body bytes.Buffer
	fmt.Fprintf(&body, "COPY %s (", orm.QuoteIdent(table))
	for i, col := range cols {
		if i > 0 {
			body.WriteString(", ")
		}
		body.WriteString(orm.QuoteIdent(col.Name))
	}
	body.WriteString(") FROM STDIN WITH (FORMAT csv, NULL '\\N');\n")
	last := int64(0)
	for {
		raw, err := sqliteJSON(ctx, path, pageSQL(table, last))
		if err != nil {
			return err
		}
		if strings.TrimSpace(raw) == "" {
			break
		}
		rows, err := decodeRows(raw)
		if err != nil {
			return fmt.Errorf("import %s %s: %w", db, table, err)
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			id, err := rowID(row["sqlx_rowid"])
			if err != nil || id <= last {
				return fmt.Errorf("import %s %s: row id did not advance", db, table)
			}
			last = id
			line, err := csvRow(table, cols, row)
			if err != nil {
				return err
			}
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	body.WriteString("\\.\n")
	if err := execRaw(ctx, db, "BEGIN;\n"+body.String()+"COMMIT;\n"); err != nil {
		return fmt.Errorf("import %s %s: %w", db, table, err)
	}
	return nil
}

func createTableSQL(table string, cols []column) string {
	body := tableSQL(table, cols)
	var b strings.Builder
	b.WriteString(body)
	for _, col := range cols {
		if col.PK && col.NotNull {
			fmt.Fprintf(&b, ";\nCOMMENT ON COLUMN %s.%s IS 'sqlx_notnull'", orm.QuoteIdent(table), orm.QuoteIdent(col.Name))
		}
	}
	return b.String()
}

func pageSQL(table string, last int64) string {
	return "SELECT rowid AS sqlx_rowid, * FROM " + orm.QuoteIdent(table) +
		" WHERE rowid > " + strconv.FormatInt(last, 10) + " ORDER BY rowid LIMIT 2000"
}

func csvRow(table string, cols []column, row map[string]any) (string, error) {
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		field, err := csvField(row[col.Name])
		if err != nil {
			return "", fmt.Errorf("import %s.%s: %s", table, col.Name, err.Error())
		}
		b.WriteString(field)
	}
	return b.String(), nil
}

func csvField(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return `\N`, nil
	case string:
		if !utf8Valid(v) {
			return "", fmt.Errorf("not utf-8")
		}
		return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`, nil
	case json.Number:
		return v.String(), nil
	default:
		return "", fmt.Errorf("unsupported value")
	}
}

func utf8Valid(s string) bool {
	return strings.ToValidUTF8(s, "") == s
}

func decodeRows(raw string) ([]map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var rows []map[string]any
	if err := dec.Decode(&rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func rowID(value any) (int64, error) {
	switch v := value.(type) {
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("row id")
	}
}

func sqliteColumns(ctx context.Context, path, table string) ([]column, error) {
	raw, err := sqliteJSON(ctx, path, "SELECT name, type, \"notnull\", dflt_value, pk FROM pragma_table_info("+orm.QuoteLiteral(table)+") ORDER BY cid")
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name    string  `json:"name"`
		Type    string  `json:"type"`
		NotNull int     `json:"notnull"`
		Default *string `json:"dflt_value"`
		PK      int     `json:"pk"`
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &rows); err != nil {
			return nil, err
		}
	}
	out := make([]column, 0, len(rows))
	for _, row := range rows {
		kind := strings.ToLower(row.Type)
		switch kind {
		case "integer", "text", "real", "blob":
		default:
			return nil, fmt.Errorf("import %s.%s has type %s", table, row.Name, row.Type)
		}
		col := column{Name: row.Name, Type: kind, NotNull: row.NotNull == 1, PK: row.PK == 1}
		if row.Default != nil {
			col.Default = safeDefault(*row.Default)
		}
		out = append(out, col)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("import %s has no columns", table)
	}
	return out, nil
}

func safeDefault(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") {
		return ""
	}
	if value[0] != '\'' && strings.Trim(value, "-0123456789") == "" && value != "-" {
		return value
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value
	}
	return ""
}

func sqliteIndexes(ctx context.Context, path string) (string, error) {
	raw, err := sqliteJSON(ctx, path, "SELECT body FROM sqlx_orm ORDER BY position, model")
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return "", nil
		}
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var rows []struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, row := range rows {
		models, err := orm.Parse(row.Body)
		if err != nil {
			return "", err
		}
		if len(models) != 1 {
			return "", fmt.Errorf("stored orm is unreadable")
		}
		for _, index := range models[0].Indexes {
			fmt.Fprintf(&b, "%s;\n", orm.CreateIndexSQL(models[0].Name, index))
		}
	}
	return b.String(), nil
}

func createIndexes(ctx context.Context, db, sql string) error {
	if strings.TrimSpace(sql) == "" {
		return nil
	}
	return execRaw(ctx, db, sql)
}

func resetSequences(ctx context.Context, db string, cols []columnRef) error {
	if len(cols) == 0 {
		return nil
	}
	var b strings.Builder
	for _, col := range cols {
		fmt.Fprintf(&b, "SELECT setval(pg_get_serial_sequence(%s, %s), GREATEST(COALESCE((SELECT MAX(%s) FROM %s), 1), 1), (SELECT COUNT(*) > 0 FROM %s));\n",
			orm.QuoteLiteral(col.table), orm.QuoteLiteral(col.name),
			orm.QuoteIdent(col.name), orm.QuoteIdent(col.table), orm.QuoteIdent(col.table))
	}
	return execRaw(ctx, db, b.String())
}

func verifyCounts(ctx context.Context, db, path string, tables []string) error {
	for _, table := range tables {
		local, err := sqliteCount(ctx, path, table)
		if err != nil {
			return err
		}
		remote, err := scalar(ctx, db, "SELECT count(*) FROM "+orm.QuoteIdent(table))
		if err != nil {
			return err
		}
		if remote != strconv.FormatInt(local, 10) {
			return fmt.Errorf("import %s %s count %s, local %d", db, table, remote, local)
		}
	}
	return nil
}

func sqliteTables(ctx context.Context, path string) ([]string, error) {
	out, err := sqliteText(ctx, path, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func sqliteCount(ctx context.Context, path, table string) (int64, error) {
	out, err := sqliteText(ctx, path, "SELECT count(*) FROM "+orm.QuoteIdent(table))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

func sqliteJSON(ctx context.Context, path, sql string) (string, error) {
	return sqliteRun(ctx, path, sql, "-json")
}

func sqliteText(ctx context.Context, path, sql string) (string, error) {
	return sqliteRun(ctx, path, sql)
}

func sqliteExec(ctx context.Context, path, sql string) error {
	_, err := sqliteRun(ctx, path, sql)
	return err
}

func sqliteRun(ctx context.Context, path, sql string, flags ...string) (string, error) {
	args := append([]string{"-batch"}, flags...)
	args = append(args, path)
	cmd := exec.CommandContext(ctx, layout.Bin("SQLX_SQLITE", "/usr/bin/sqlite3"), args...)
	cmd.Stdin = strings.NewReader(sql)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func fileSignature(path string) (string, error) {
	var b strings.Builder
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if os.IsNotExist(err) {
			fmt.Fprintf(&b, "%s absent\n", suffix)
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s %d %d\n", suffix, info.Size(), info.ModTime().UnixNano())
	}
	return b.String(), nil
}

func retireSQLite(path, name string) error {
	dir := filepath.Join(layout.Root(), "retired")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dest := filepath.Join(dir, name+".sqlite")
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(dir, name+"."+time.Now().UTC().Format("20060102T150405Z")+".sqlite")
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := path + suffix
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		target := dest + suffix
		if err := os.Rename(src, target); err != nil {
			return err
		}
		if err := os.Chmod(target, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func lockDB(name string) (func(), error) {
	dir := filepath.Join(layout.Root(), "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
