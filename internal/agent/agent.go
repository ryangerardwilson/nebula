package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/engine"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
	"github.com/ryangerardwilson/nebula/internal/pg"
	"github.com/ryangerardwilson/nebula/internal/store"
)

var ErrUsage = errors.New("usage")

const ormTable = `CREATE TABLE IF NOT EXISTS sqlx_orm (
  model TEXT PRIMARY KEY,
  body TEXT NOT NULL,
  position INTEGER NOT NULL
)`

func CreateDatabase(ctx context.Context, name string) error {
	if err := checkDB(name); err != nil {
		return err
	}
	if engine.Object() {
		return createObject(ctx, name)
	}
	if engine.Postgres() {
		if cache.Has(name) {
			return nil
		}
		if err := pg.Ensure(ctx, name); err != nil {
			return err
		}
		_, err := cache.Open(ctx, name)
		return err
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	path := layout.DBPath(name)
	sql := "PRAGMA journal_mode=WAL;\n" + ormTable
	if _, err := os.Stat(path); err == nil {
		if err := store.Exec(ctx, path, sql); err != nil {
			return err
		}
		return os.Chmod(path, 0o600)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := store.Exec(ctx, path, sql); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func RenameDatabase(oldName, newName string) error {
	if err := checkDB(oldName); err != nil {
		return err
	}
	if err := checkDB(newName); err != nil {
		return err
	}
	if oldName == newName {
		return nil
	}
	if engine.Object() {
		return renameObject(oldName, newName)
	}
	if engine.Postgres() {
		if cache.Has(newName) && !cache.Has(oldName) {
			return nil
		}
		return renameRemote(oldName, newName)
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	newPath := layout.DBPath(newName)
	if _, err := os.Stat(newPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	oldPath := layout.DBPath(oldName)
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := oldPath + suffix
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Rename(src, newPath+suffix); err != nil {
			return err
		}
	}
	dstORM := layout.ORMPath(newName)
	if _, err := os.Stat(dstORM); os.IsNotExist(err) {
		srcORM := layout.ORMPath(oldName)
		if _, err := os.Stat(srcORM); err == nil {
			if err := os.Rename(srcORM, dstORM); err != nil {
				return err
			}
		}
	}
	return nil
}

func DropDatabase(name string) error {
	if err := checkDB(name); err != nil {
		return err
	}
	if engine.Object() {
		return dropObject(name)
	}
	if engine.Postgres() {
		return dropRemote(name)
	}
	path := layout.DBPath(name)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(layout.ORMPath(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Names() ([]string, error) {
	if engine.Object() {
		return objectNames()
	}
	if engine.Postgres() {
		names, err := cache.Names()
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			return names, nil
		}
		return pg.Names(context.Background())
	}
	entries, err := os.ReadDir(layout.DataDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(entry.Name(), ".sqlite")
		if !ok || !orm.ValidName(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func ListDatabases() (string, error) {
	names, err := Names()
	if err != nil || len(names) == 0 {
		return "", err
	}
	return strings.Join(names, "\n") + "\n", nil
}

func Query(ctx context.Context, db, sql string, out io.Writer) error {
	path, err := existing(db)
	if err != nil {
		return err
	}
	if err := checkData(sql, "sql <database> query <sql>"); err != nil {
		return err
	}
	if err := requireMatch(ctx, path); err != nil {
		return err
	}
	text, err := store.QueryCSV(ctx, path, sql)
	if text != "" {
		if _, werr := io.WriteString(out, text); werr != nil {
			return werr
		}
		if !strings.HasSuffix(text, "\n") {
			if _, werr := io.WriteString(out, "\n"); werr != nil {
				return werr
			}
		}
	}
	return err
}

func QueryJSON(ctx context.Context, db, sql string, out io.Writer) error {
	path, err := existing(db)
	if err != nil {
		return err
	}
	if err := checkData(sql, "sql <database> query --json <sql>"); err != nil {
		return err
	}
	if err := requireMatch(ctx, path); err != nil {
		return err
	}
	text, err := store.QueryJSON(ctx, path, sql)
	if text != "" {
		if _, werr := io.WriteString(out, text); werr != nil {
			return werr
		}
		if !strings.HasSuffix(text, "\n") {
			if _, werr := io.WriteString(out, "\n"); werr != nil {
				return werr
			}
		}
	}
	return err
}

func Exec(ctx context.Context, db, sql string) error {
	if localCommit() && engine.Runtime() {
		return execRuntime(ctx, db, sql)
	}
	path, err := existing(db)
	if err != nil {
		return err
	}
	if err := checkData(sql, "sql <database> exec <sql>"); err != nil {
		return err
	}
	if err := requireMatch(ctx, path); err != nil {
		return err
	}
	return commit(ctx, db, path, sql)
}

// localCommit writes the runtime sqlite and does not open the backup.
// remoteCommit sends the statement to the backup and does not write the runtime copy again.
func localCommit() bool { return os.Getenv("SQLX_COMMIT") == "local" }

func remoteCommit() bool { return os.Getenv("SQLX_COMMIT") == "remote" }

func execRuntime(ctx context.Context, db, sql string) error {
	if err := checkDB(db); err != nil {
		return err
	}
	if err := checkData(sql, "sql <database> exec <sql>"); err != nil {
		return err
	}
	if !cache.Has(db) {
		return fmt.Errorf("database %s has no runtime copy", db)
	}
	path := cache.DBPath(db)
	if err := requireMatch(ctx, path); err != nil {
		return err
	}
	return store.Exec(ctx, path, sql)
}

func commit(ctx context.Context, db, path, sql string) error {
	if engine.Object() {
		return commitObject(ctx, db, path, sql)
	}
	offline := false
	if engine.Postgres() && !localCommit() {
		err := cache.Backup(ctx, db, sql)
		if err != nil && (!pg.Unreachable(err) || remoteCommit()) {
			return err
		}
		offline = err != nil
	}
	if remoteCommit() {
		return nil
	}
	if err := store.Exec(ctx, path, sql); err != nil {
		if engine.Postgres() && !offline {
			fmt.Fprintf(os.Stderr, "sql %s dropped its memory copy and will restore it from the backup: %s\n", db, err)
			_ = cache.Remove(db)
			return nil
		}
		return err
	}
	return nil
}

func Columns(ctx context.Context, db, table string) (string, error) {
	if err := checkDB(table); err != nil {
		return "", err
	}
	path, err := existing(db)
	if err != nil {
		return "", err
	}
	query := "SELECT name FROM pragma_table_info(" + orm.QuoteLiteral(table) + ") ORDER BY cid"
	raw, err := store.QueryJSON(ctx, path, query)
	if err != nil {
		return "", err
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := store.Decode(raw, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = row.Name
	}
	return strings.Join(names, "\n") + "\n", nil
}

const onceTable = `CREATE TABLE IF NOT EXISTS sqlx_once (
  id TEXT PRIMARY KEY,
  applied_at TEXT NOT NULL
)`

func Once(ctx context.Context, db, id, sql string, out io.Writer) error {
	if !orm.ValidName(id) {
		return fmt.Errorf("%w: once id must be letters, digits, and underscores", ErrUsage)
	}
	if strings.TrimSpace(sql) == "" {
		return fmt.Errorf("%w: sql <database> once <id> <sql>", ErrUsage)
	}
	path, err := existing(db)
	if err != nil {
		return err
	}
	if err := store.Exec(ctx, path, onceTable); err != nil {
		return err
	}
	applied, err := onceApplied(ctx, path, id)
	if err != nil {
		return err
	}
	if applied {
		fmt.Fprint(out, "done\n")
		return nil
	}
	if err := commit(ctx, db, path, sql); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	insert := "INSERT INTO sqlx_once(id, applied_at) VALUES (" + orm.QuoteLiteral(id) + ", " + orm.QuoteLiteral(stamp) + ")"
	if err := commit(ctx, db, path, insert); err != nil {
		return err
	}
	fmt.Fprint(out, "applied\n")
	return nil
}

func onceApplied(ctx context.Context, path, id string) (bool, error) {
	raw, err := store.QueryJSON(ctx, path, "SELECT id FROM sqlx_once WHERE id = "+orm.QuoteLiteral(id))
	if err != nil {
		return false, err
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := store.Decode(raw, &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func checkData(sql, usage string) error {
	if err := orm.CheckDataSQL(sql); err != nil {
		if err.Error() == "query needs sql" {
			return fmt.Errorf("%w: %s", ErrUsage, usage)
		}
		return err
	}
	return nil
}

func Schema(ctx context.Context, db string) (string, error) {
	path, err := existing(db)
	if err != nil {
		return "", err
	}
	models, err := loadStored(ctx, path)
	if err != nil {
		return "", err
	}
	return orm.Format(models), nil
}

func Inspect(ctx context.Context, db string) (string, error) {
	path, err := existing(db)
	if err != nil {
		return "", err
	}
	if err := requireMatch(ctx, path); err != nil {
		return "", err
	}
	models, err := loadStored(ctx, path)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "database %s\norm match\n\nmodels\nmodel\tcolumns\trows\n", db)
	for _, model := range models {
		raw, err := store.QueryCSV(ctx, path, "SELECT count(*) FROM "+orm.QuoteIdent(model.Name))
		if err != nil {
			return "", err
		}
		rows, err := lastNumber(raw)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s\t%s\t%d\n", model.Name, orm.ColumnsText(model), rows)
	}
	return b.String(), nil
}

func Migrate(ctx context.Context, db, path string, out io.Writer) error {
	dbPath, err := existing(db)
	if err != nil {
		return err
	}
	models, err := ReadORM(path)
	if err != nil {
		return err
	}
	if err := store.Exec(ctx, dbPath, ormTable); err != nil {
		return err
	}
	live, err := loadLive(ctx, dbPath)
	if err != nil {
		return err
	}
	plan, err := orm.Compare(live, models)
	if err != nil {
		return err
	}
	var script strings.Builder
	script.WriteString("BEGIN;\n")
	for _, model := range plan.Create {
		script.WriteString(createStatement(model))
		script.WriteString(";\n")
	}
	for _, add := range plan.Add {
		script.WriteString(addColumnStatement(add.Table, add.Column))
		script.WriteString(";\n")
	}
	haveIndex, err := indexNames(ctx, dbPath)
	if err != nil {
		return err
	}
	var indexed []string
	for _, model := range models {
		for _, index := range model.Indexes {
			if !haveIndex[index.Name] {
				indexed = append(indexed, index.Name)
			}
			script.WriteString(orm.CreateIndexSQL(model.Name, index))
			script.WriteString(";\n")
		}
	}
	script.WriteString("DELETE FROM sqlx_orm;\n")
	for i, model := range models {
		fmt.Fprintf(&script, "INSERT INTO sqlx_orm(model, body, position) VALUES (%s, %s, %d);\n",
			orm.QuoteLiteral(model.Name), orm.QuoteLiteral(model.Canonical()), i)
	}
	script.WriteString("COMMIT;\n")
	if err := store.Exec(ctx, dbPath, script.String()); err != nil {
		return err
	}
	if engine.Postgres() && !cache.SchemaCurrent(db, models) {
		if err := cache.BackupSchema(ctx, db, models); err != nil {
			return err
		}
		if err := cache.MarkSchema(db, models); err != nil {
			return err
		}
	}
	if err := writeORMFile(db, models); err != nil {
		return err
	}
	if engine.Object() && !localCommit() {
		if err := cache.Publish(ctx, db); err != nil {
			return err
		}
	}
	for _, model := range plan.Create {
		fmt.Fprintf(out, "created %s\n", model.Name)
	}
	for _, add := range plan.Add {
		fmt.Fprintf(out, "added %s.%s\n", add.Table, add.Column.Name)
	}
	for _, name := range indexed {
		fmt.Fprintf(out, "indexed %s\n", name)
	}
	if plan.Empty() && len(indexed) == 0 {
		fmt.Fprintf(out, "unchanged\n")
	}
	return nil
}

func ReadORM(path string) ([]orm.Model, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return orm.Parse(string(body))
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".model") {
			continue
		}
		files = append(files, filepath.Join(path, entry.Name()))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("orm path has no models")
	}
	var b strings.Builder
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		b.Write(body)
		b.WriteByte('\n')
	}
	return orm.Parse(b.String())
}

func requireMatch(ctx context.Context, path string) error {
	live, err := loadLive(ctx, path)
	if err != nil {
		return err
	}
	stored, err := loadStored(ctx, path)
	if err != nil {
		return err
	}
	plan, err := orm.Compare(live, stored)
	if err != nil {
		return err
	}
	return orm.Drift(plan)
}

func loadStored(ctx context.Context, path string) ([]orm.Model, error) {
	raw, err := store.QueryJSON(ctx, path, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'sqlx_orm'")
	if err != nil {
		return nil, err
	}
	var present []struct {
		Name string `json:"name"`
	}
	if err := store.Decode(raw, &present); err != nil {
		return nil, err
	}
	if len(present) == 0 {
		return nil, fmt.Errorf("database has no orm")
	}
	body, err := store.QueryJSON(ctx, path, "SELECT model, body, position FROM sqlx_orm ORDER BY position, model")
	if err != nil {
		return nil, err
	}
	return modelsFromBody(body)
}

func modelsFromBody(body string) ([]orm.Model, error) {
	var rows []struct {
		Model string `json:"model"`
		Body  string `json:"body"`
	}
	if err := store.Decode(body, &rows); err != nil {
		return nil, err
	}
	var models []orm.Model
	for _, row := range rows {
		parsed, err := orm.Parse(row.Body)
		if err != nil {
			return nil, err
		}
		if len(parsed) != 1 || parsed[0].Name != row.Model {
			return nil, fmt.Errorf("stored orm for %s is unreadable", row.Model)
		}
		models = append(models, parsed[0])
	}
	return models, nil
}

func indexNames(ctx context.Context, path string) (map[string]bool, error) {
	query := "SELECT name FROM sqlite_master WHERE type = 'index'"
	raw, err := store.QueryJSON(ctx, path, query)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Name string `json:"name"`
	}
	if err := store.Decode(raw, &rows); err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, row := range rows {
		names[row.Name] = true
	}
	return names, nil
}

func loadLive(ctx context.Context, path string) ([]orm.Model, error) {
	raw, err := store.QueryJSON(ctx, path, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'sqlx_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	var tables []struct {
		Name string `json:"name"`
	}
	if err := store.Decode(raw, &tables); err != nil {
		return nil, err
	}
	var models []orm.Model
	for _, table := range tables {
		info, err := store.QueryJSON(ctx, path, "SELECT * FROM pragma_table_info("+orm.QuoteLiteral(table.Name)+")")
		if err != nil {
			return nil, err
		}
		var cols []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			NotNull int    `json:"notnull"`
			PK      int    `json:"pk"`
		}
		if err := store.Decode(info, &cols); err != nil {
			return nil, err
		}
		model := orm.Model{Name: table.Name}
		for _, col := range cols {
			model.Columns = append(model.Columns, orm.Column{
				Name:       col.Name,
				Type:       strings.ToLower(col.Type),
				PrimaryKey: col.PK > 0,
				NotNull:    col.NotNull == 1,
			})
		}
		models = append(models, model)
	}
	return models, nil
}

func writeORMFile(name string, models []orm.Model) error {
	if err := layout.Ensure(); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(layout.ORMDir(), ".orm-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := io.WriteString(tmp, orm.Format(models)); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, layout.ORMPath(name)); err != nil {
		return err
	}
	ok = true
	return nil
}

func existing(name string) (string, error) {
	if err := checkDB(name); err != nil {
		return "", err
	}
	if engine.Runtime() {
		return cache.Open(context.Background(), name)
	}
	path := layout.DBPath(name)
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && info.IsDir()) {
		return "", fmt.Errorf("database %s does not exist", name)
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

func checkDB(name string) error {
	if !orm.ValidName(name) {
		return fmt.Errorf("%w: database name must be letters, digits, and underscores", ErrUsage)
	}
	return nil
}

func createStatement(model orm.Model) string {
	return orm.CreateSQL(model)
}

func addColumnStatement(table string, column orm.Column) string {
	return orm.AddColumnSQL(table, column)
}

func lastNumber(csv string) (int64, error) {
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("unexpected row count")
	}
	return strconv.ParseInt(strings.TrimSpace(lines[len(lines)-1]), 10, 64)
}

func dropRemote(name string) error {
	if err := pg.Drop(context.Background(), name); err != nil {
		return err
	}
	if err := cache.Remove(name); err != nil {
		return err
	}
	if err := os.Remove(layout.ORMPath(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func renameRemote(oldName, newName string) error {
	ctx := context.Background()
	newExists, err := pg.Exists(ctx, newName)
	if err != nil {
		return err
	}
	if newExists {
		return nil
	}
	oldExists, err := pg.Exists(ctx, oldName)
	if err != nil {
		return err
	}
	if !oldExists {
		return nil
	}
	if err := pg.Rename(ctx, oldName, newName); err != nil {
		return err
	}
	if err := cache.Rename(oldName, newName); err != nil {
		return err
	}
	dst := layout.ORMPath(newName)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	src := layout.ORMPath(oldName)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(src, dst)
}
