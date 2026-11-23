package s3

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSSH puts an ssh on PATH that runs its command through sh -c, the way
// the remote login shell re-parses the joined words. It maps the remote
// object roots into a temporary directory and logs one line per call.
func fakeSSH(t *testing.T) (remote, logPath string) {
	t.Helper()
	dir := t.TempDir()
	remote = filepath.Join(dir, "remote")
	logPath = filepath.Join(dir, "ssh.log")
	script := `#!/bin/sh
while [ $# -gt 0 ] && [ "$1" != "--" ]; do shift; done
shift
printf '%s\n' "$*" >> "$FAKE_SSH_LOG"
cmd=$(printf '%s' "$*" | sed "s#/var/lib/nebula/#$FAKE_REMOTE/#g")
exec sh -c "$cmd"
`
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_REMOTE", remote)
	t.Setenv("FAKE_SSH_LOG", logPath)
	t.Setenv("S3_ROOT", "")
	t.Setenv("S3_SSH", "fake-host")
	return remote, logPath
}

func sshCalls(t *testing.T, logPath string) []string {
	t.Helper()
	body, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPutOverSSHIsOneCommand(t *testing.T) {
	remote, logPath := fakeSSH(t)
	if err := os.MkdirAll(filepath.Join(remote, "s3", "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "c3po.sqlite")
	writeFile(t, src, "snapshot")
	if err := Put(context.Background(), src, "backups", "c3po.sqlite"); err != nil {
		t.Fatal(err)
	}
	if calls := sshCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("ssh ran %d times: %q", len(calls), calls)
	}
	body, err := os.ReadFile(filepath.Join(remote, "s3", "backups", "c3po.sqlite"))
	if err != nil || string(body) != "snapshot" {
		t.Fatalf("%q %v", body, err)
	}
	info, err := os.Stat(filepath.Join(remote, "s3", "backups", "c3po.sqlite"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
}

func TestPutOverSSHRefusesAMissingBucket(t *testing.T) {
	_, logPath := fakeSSH(t)
	src := filepath.Join(t.TempDir(), "a.sqlite")
	writeFile(t, src, "a")
	err := Put(context.Background(), src, "backups", "a.sqlite")
	if !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if calls := sshCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("ssh ran %d times: %q", len(calls), calls)
	}
}
func TestListOverSSHSurvivesTheRemoteShell(t *testing.T) {
	remote, logPath := fakeSSH(t)
	writeFile(t, filepath.Join(remote, "s3", "backups", "c3po.sqlite"), "12345")
	writeFile(t, filepath.Join(remote, "s3", "backups", "day", "a.sqlite"), "12")
	writeFile(t, filepath.Join(remote, "s3", "backups", "half.sqlite.tmp.x1"), "1")
	objects, err := List(context.Background(), "backups", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[0] != (Object{Key: "c3po.sqlite", Size: 5}) || objects[1] != (Object{Key: "day/a.sqlite", Size: 2}) {
		t.Fatalf("%+v", objects)
	}
	if calls := sshCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("ssh ran %d times: %q", len(calls), calls)
	}
	names, err := ListBuckets(context.Background())
	if err != nil || len(names) != 1 || names[0] != "backups" {
		t.Fatalf("%q %v", names, err)
	}
}
func TestCopyOverSSHIsOneCommand(t *testing.T) {
	remote, logPath := fakeSSH(t)
	writeFile(t, filepath.Join(remote, "s3", "sqlx", "a.sqlite"), "a")
	if err := os.MkdirAll(filepath.Join(remote, "s3", "sqlx-backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Copy(context.Background(), "sqlx", "a.sqlite", "sqlx-backups", "2026-11-22/a.sqlite"); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(remote, "s3", "sqlx-backups", "2026-11-22", "a.sqlite")); err != nil || string(body) != "a" {
		t.Fatalf("%q %v", body, err)
	}
	if calls := sshCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("ssh ran %d times: %q", len(calls), calls)
	}
}

func TestMakeBucketOverSSHIsOneCommand(t *testing.T) {
	remote, logPath := fakeSSH(t)
	if err := MakeBucket(context.Background(), "backups"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(remote, "s3", "backups"))
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("%v %v", info, err)
	}
	if calls := sshCalls(t, logPath); len(calls) != 1 {
		t.Fatalf("ssh ran %d times: %q", len(calls), calls)
	}
}
