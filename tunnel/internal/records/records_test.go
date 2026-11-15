package records

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileModeStaysAFile(t *testing.T) {
	t.Setenv("SQLX_ROOT", "")
	path := filepath.Join(t.TempDir(), "routes.json")
	body := []byte("{\"zone\":\"ryangerardwilson.com\"}\n")
	if err := Write("tunnel", "routes", "routes", path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read("tunnel", "routes", "routes", path)
	if err != nil || string(got) != string(body) {
		t.Fatalf("%s %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestProductionRecordRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	path := filepath.Join(root, "config", "c3po", "tunnel", "routes.json")
	body := []byte("{\"name\":\"o'clock\"}\n")
	if err := Write("tunnel", "routes", "routes", path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("production record wrote a file")
	}
	got, err := Read("tunnel", "routes", "routes", path)
	if err != nil || string(got) != string(body) {
		t.Fatalf("%s %v", got, err)
	}
	db := filepath.Join(root, "data", "c3po.sqlite")
	info, err := os.Stat(db)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("db %v %v", info, err)
	}
}

func TestImportRoutesFileOnce(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SQLX_ROOT", root)
	path := filepath.Join(root, "config", "c3po", "tunnel", "routes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	disk := []byte("{\"from\":\"disk\"}\n")
	if err := os.WriteFile(path, disk, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read("tunnel", "routes", "routes", path)
	if err != nil || string(got) != string(disk) {
		t.Fatalf("import %s %v", got, err)
	}
	if err := os.WriteFile(path, []byte("later\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := databaseFile()
	if err != nil {
		t.Fatal(err)
	}
	if err := execSQL(db, "DELETE FROM record WHERE app = 'tunnel' AND kind = 'routes' AND item_key = 'routes'"); err != nil {
		t.Fatal(err)
	}
	_, err = Read("tunnel", "routes", "routes", path)
	if !os.IsNotExist(err) {
		t.Fatalf("second import %v", err)
	}
	if strings.Contains(string(disk), "later") {
		t.Fatal("fixture changed")
	}
}
