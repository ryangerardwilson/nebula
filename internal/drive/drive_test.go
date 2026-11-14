package drive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisteredMatchesTheLocalLine(t *testing.T) {
	dir := "/home/ryan/.sqlx/backups"
	list := "edit_id : 4\nlocal   : " + dir + "\ndrive   : deusvult_backup/sqlx\n"
	if !Registered(list, dir) {
		t.Fatal("registered directory was missed")
	}
	if Registered(list, dir+"-other") {
		t.Fatal("prefix matched a different directory")
	}
}

func TestPublishRegistersThenSyncs(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "log")
	script := filepath.Join(t.TempDir(), "gdrive")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + shellQuote(logPath) + "\nexit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SQLX_GDRIVE", script)
	t.Setenv("SQLX_GDRIVE_PRESET", "1")
	t.Setenv("SQLX_DRIVE_PATH", "sqlx")
	dir := t.TempDir()
	if err := Publish(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(log)
	if !strings.Contains(text, "1 list registrations") || !strings.Contains(text, "1 register "+dir+" as sqlx") || !strings.Contains(text, "sync run") {
		t.Fatal(text)
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
