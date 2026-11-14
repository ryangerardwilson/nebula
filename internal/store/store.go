package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/layout"
)

func QueryCSV(ctx context.Context, dbPath, sql string) (string, error) {
	text, err := run(ctx, dbPath, sql, "-csv", "-header")
	return strings.ReplaceAll(text, "\r\n", "\n"), err
}

func QueryJSON(ctx context.Context, dbPath, sql string) (string, error) {
	return run(ctx, dbPath, sql, "-json")
}

func Exec(ctx context.Context, dbPath, sql string) error {
	_, err := run(ctx, dbPath, sql)
	return err
}

func BackupFile(ctx context.Context, src, dst string) error {
	quoted := "'" + strings.ReplaceAll(dst, "'", "''") + "'"
	return Exec(ctx, src, ".backup "+quoted)
}

func Decode(raw string, dest any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("sqlite json: %w", err)
	}
	return nil
}

func run(ctx context.Context, dbPath, sql string, flags ...string) (string, error) {
	args := []string{"-batch", "-cmd", ".timeout 5000", "-cmd", ".bail on"}
	args = append(args, flags...)
	args = append(args, dbPath)
	cmd := exec.CommandContext(ctx, layout.Bin("SQLX_SQLITE", "/usr/bin/sqlite3"), args...)
	// A course body is larger than one argument, so the statement is stdin.
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
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil && !os.IsNotExist(err) {
		return stdout.String(), err
	}
	return stdout.String(), nil
}
