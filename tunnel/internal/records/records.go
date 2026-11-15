// Package records stores tunnel routes in the c3po sqlite file.
// A path outside that production record stays a file, which is what tests use.
// Writes publish the sqlite file with nebula s3. Reads do not.
package records

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const databaseName = "c3po"

const schemaSQL = "" +
	"CREATE TABLE IF NOT EXISTS record (\n" +
	"  id integer primary key,\n" +
	"  app text not null,\n" +
	"  kind text not null,\n" +
	"  item_key text not null,\n" +
	"  body text not null\n" +
	");\n" +
	"CREATE TABLE IF NOT EXISTS meta (\n" +
	"  id integer primary key,\n" +
	"  key text not null,\n" +
	"  value text\n" +
	");\n"

// Read returns the production record, or the file when path is not that record.
func Read(app, kind, key, path string) ([]byte, error) {
	if !useDB(path) {
		return os.ReadFile(path)
	}
	db, err := open()
	if err != nil {
		return nil, err
	}
	return readDB(db, app, kind, key, path)
}

// Write stores a production record. Other paths stay files.
func Write(app, kind, key, path string, body []byte, mode os.FileMode) error {
	if !useDB(path) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		return os.WriteFile(path, body, mode)
	}
	db, err := open()
	if err != nil {
		return err
	}
	return put(db, app, kind, key, body)
}

// Get reads one record. ok is false when the key is absent.
func Get(app, kind, key string) ([]byte, bool, error) {
	db, err := open()
	if err != nil {
		return nil, false, err
	}
	return get(db, app, kind, key)
}

// Put writes one record and publishes the sqlite file.
func Put(app, kind, key string, body []byte) error {
	db, err := open()
	if err != nil {
		return err
	}
	return put(db, app, kind, key, body)
}

// Delete removes one record. A missing key is not an error.
func Delete(app, kind, key string) error {
	db, err := open()
	if err != nil {
		return err
	}
	sql := "DELETE FROM record WHERE app = " + quote(app) + " AND kind = " + quote(kind) + " AND item_key = " + quote(key)
	return execSQL(db, sql)
}

func readDB(db, app, kind, key, path string) ([]byte, error) {
	body, ok, err := get(db, app, kind, key)
	if err != nil || ok {
		return body, err
	}
	return importOnce(db, app, kind, key, path)
}

func importOnce(db, app, kind, key, path string) ([]byte, error) {
	flag, done, err := meta(db, "tunnel_routes_imported")
	if err != nil {
		return nil, err
	}
	if done && flag == "1" {
		return nil, os.ErrNotExist
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = setMeta(db, "tunnel_routes_imported", "1")
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	if err := put(db, app, kind, key, disk); err != nil {
		return nil, err
	}
	if err := setMeta(db, "tunnel_routes_imported", "1"); err != nil {
		return nil, err
	}
	return disk, nil
}

func useDB(path string) bool {
	if os.Getenv("C3PO_SQLX") == "off" {
		return false
	}
	path = filepath.Clean(path)
	if os.Getenv("SQLX_ROOT") != "" && strings.Contains(path, string(os.PathSeparator)+"c3po"+string(os.PathSeparator)) {
		return true
	}
	home, err := loginHome()
	if err != nil {
		return false
	}
	data := filepath.Join(home, ".local", "share", "c3po")
	if rel, err := filepath.Rel(data, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return true
	}
	routes := filepath.Join(home, ".config", "c3po", "tunnel", "routes.json")
	return path == filepath.Clean(routes)
}

func open() (string, error) {
	db, err := databaseFile()
	if err != nil {
		return "", err
	}
	if err := prepare(db); err != nil {
		return "", err
	}
	return db, nil
}

func databaseFile() (string, error) {
	if root := os.Getenv("SQLX_ROOT"); root != "" {
		return filepath.Join(root, "data", databaseName+".sqlite"), nil
	}
	if v := os.Getenv("SQLX_RUNTIME"); v != "" {
		return filepath.Join(v, databaseName+".sqlite"), nil
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join("/dev/shm", fmt.Sprintf("%s-%d", databaseName, os.Getuid()))
	} else {
		base = filepath.Join(base, databaseName)
	}
	return filepath.Join(base, databaseName+".sqlite"), nil
}

func loginHome() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	if current.HomeDir == "" {
		return "", errors.New("login home is empty")
	}
	return current.HomeDir, nil
}

func prepare(path string) error {
	if root := os.Getenv("SQLX_ROOT"); root != "" {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		_ = os.Chmod(root, 0o700)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = copyLegacy(path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = restoreObject(path)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			return err
		}
	}
	if err := sqliteExec(path, schemaSQL); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func copyLegacy(path string) error {
	if os.Getenv("SQLX_ROOT") != "" {
		return nil
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return nil
	}
	src := filepath.Join(base, "sqlx", databaseName+".sqlite")
	if src == path {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return nil
	}
	return os.WriteFile(path, data, 0o600)
}
