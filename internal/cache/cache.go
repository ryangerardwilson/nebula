package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/ryangerardwilson/nebula/internal/engine"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
	"github.com/ryangerardwilson/nebula/internal/pg"
)

// Dir is the memory working copy. It lives on a tmpfs that a shutdown,
// reboot, or power loss clears, so the next start restores from the s3 backup.
func Dir() string {
	if v := os.Getenv("SQLX_RUNTIME"); v != "" {
		return v
	}
	if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
		return filepath.Join(v, "sqlx")
	}
	return filepath.Join("/dev/shm", fmt.Sprintf("sqlx-%d", os.Getuid()))
}

func DBPath(name string) string {
	return filepath.Join(Dir(), name+".sqlite")
}

func Has(name string) bool {
	return isFile(DBPath(name))
}

// Names lists databases already in the memory copy. A present copy is read
// without opening SSH to Postgres.
func Names() ([]string, error) {
	entries, err := os.ReadDir(Dir())
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
		if !ok || !orm.ValidName(name) || !isFile(DBPath(name)) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func Open(ctx context.Context, name string) (string, error) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(Dir(), 0o700); err != nil {
		return "", err
	}
	unlock, err := lock(name)
	if err != nil {
		return "", err
	}
	defer unlock()
	path := DBPath(name)
	if isFile(path) {
		return path, nil
	}
	if engine.Object() {
		return openObject(ctx, name, path)
	}
	ok, err := pg.Exists(ctx, name)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("database %s does not exist", name)
	}
	if err := restore(ctx, name, path); err != nil {
		return "", err
	}
	return path, nil
}

func Backup(ctx context.Context, name, sql string) error {
	return pg.Exec(ctx, name, sql)
}

func BackupSchema(ctx context.Context, name string, models []orm.Model) error {
	live, err := pg.LiveModels(ctx, name)
	if err != nil {
		return err
	}
	plan, err := orm.Compare(live, models)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("BEGIN;\n")
	for _, model := range plan.Create {
		writeStmt(&b, pg.CreateSQL(model))
	}
	for _, add := range plan.Add {
		writeStmt(&b, pg.AddColumnSQL(add.Table, add.Column))
	}
	for _, model := range models {
		for _, index := range model.Indexes {
			writeStmt(&b, orm.CreateIndexSQL(model.Name, index))
		}
	}
	b.WriteString("DELETE FROM sqlx_orm;\n")
	for i, model := range models {
		fmt.Fprintf(&b, "INSERT INTO sqlx_orm(model, body, position) VALUES (%s, %s, %d);\n",
			orm.QuoteLiteral(model.Name), orm.QuoteLiteral(model.Canonical()), i)
	}
	b.WriteString("COMMIT;\n")
	return pg.Exec(ctx, name, b.String())
}

func SchemaCurrent(name string, models []orm.Model) bool {
	body, err := os.ReadFile(marker(name))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == schemaID(models)
}

func MarkSchema(name string, models []orm.Model) error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(marker(name), []byte(schemaID(models)+"\n"), 0o600)
}

func Remove(name string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(DBPath(name) + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(marker(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Rename(oldName, newName string) error {
	if oldName == newName || !Has(oldName) || Has(newName) {
		return nil
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		src := DBPath(oldName) + suffix
		if !isFile(src) {
			continue
		}
		if err := os.Rename(src, DBPath(newName)+suffix); err != nil {
			return err
		}
	}
	if isFile(marker(oldName)) {
		return os.Rename(marker(oldName), marker(newName))
	}
	return nil
}

func marker(name string) string {
	return filepath.Join(Dir(), name+".schema")
}

func schemaID(models []orm.Model) string {
	sum := sha256.Sum256([]byte(orm.Format(models)))
	return hex.EncodeToString(sum[:])
}

func writeStmt(b *strings.Builder, sql string) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return
	}
	b.WriteString(sql)
	if !strings.HasSuffix(sql, ";") {
		b.WriteByte(';')
	}
	b.WriteByte('\n')
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func lock(name string) (func(), error) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(Dir(), name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
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

func sqliteJSON(ctx context.Context, path, sql string) (string, error) {
	return sqliteRun(ctx, path, sql, "-json")
}

func sqliteExec(ctx context.Context, path, sql string) error {
	_, err := sqliteRun(ctx, path, sql)
	return err
}

func sqliteRun(ctx context.Context, path, sql string, flags ...string) (string, error) {
	args := []string{"-batch", "-cmd", ".timeout 5000", "-cmd", ".bail on"}
	args = append(args, flags...)
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
		return "", fmt.Errorf("%s", msg)
	}
	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return stdout.String(), nil
}
