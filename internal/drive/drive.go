package drive

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/layout"
)

func Registered(list, dir string) bool {
	want := "local   : " + filepath.Clean(dir)
	for _, line := range strings.Split(list, "\n") {
		if strings.TrimRight(line, "\r") == want {
			return true
		}
	}
	return false
}

func Publish(ctx context.Context, dir string) error {
	dir = filepath.Clean(dir)
	gdrive := layout.Bin("SQLX_GDRIVE", "gdrive")
	preset := layout.Bin("SQLX_GDRIVE_PRESET", "1")
	drivePath := layout.Bin("SQLX_DRIVE_PATH", "sqlx")
	list, err := run(ctx, gdrive, preset, "list", "registrations")
	if err != nil {
		return fmt.Errorf("drive: %w", err)
	}
	if !Registered(list, dir) {
		if _, err := run(ctx, gdrive, preset, "register", dir, "as", drivePath); err != nil {
			return fmt.Errorf("drive: %w", err)
		}
	}
	if _, err := run(ctx, gdrive, "sync", "run"); err != nil {
		return fmt.Errorf("drive: %w", err)
	}
	return nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
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
	return stdout.String(), nil
}
