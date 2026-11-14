package layout

import (
	"os"
	"path/filepath"
	"strconv"
)

func Root() string {
	if v := os.Getenv("SQLX_ROOT"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".sqlx")
}

func DataDir() string   { return filepath.Join(Root(), "data") }
func ORMDir() string    { return filepath.Join(Root(), "orm") }
func BackupDir() string { return filepath.Join(Root(), "backups") }

func DBPath(name string) string  { return filepath.Join(DataDir(), name+".sqlite") }
func ORMPath(name string) string { return filepath.Join(ORMDir(), name) }

func Ensure() error {
	for _, dir := range []string{Root(), DataDir(), ORMDir(), BackupDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func Bin(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

func KeepDays() int {
	if v := os.Getenv("SQLX_KEEP_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 {
			return n
		}
	}
	return 10
}
