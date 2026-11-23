package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/internal/agent"
	"github.com/ryangerardwilson/nebula/s3"
)

func TestObjectBackupKeepsTenDays(t *testing.T) {
	t.Setenv("SQLX_ENGINE", "s3")
	t.Setenv("SQLX_ROOT", "")
	t.Setenv("S3_ROOT", t.TempDir())
	t.Setenv("SQLX_RUNTIME", t.TempDir())
	ctx := context.Background()
	if err := agent.CreateDatabase(ctx, "app"); err != nil {
		t.Fatal(err)
	}
	if err := s3.MakeBucket(ctx, s3.BackupBucket); err != nil {
		t.Fatal(err)
	}
	if err := s3.Put(ctx, mustFile(t, "old"), s3.BackupBucket, "2020-01-01/app.sqlite"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "backup s3://sqlx-backups/") || !strings.Contains(out.String(), "pruned 1") {
		t.Fatal(out.String())
	}
	objects, err := s3.List(ctx, s3.BackupBucket, "2020-01-01")
	if err != nil || len(objects) != 0 {
		t.Fatal(objects, err)
	}
}

func mustFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "body")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
