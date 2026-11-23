package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ryangerardwilson/nebula/internal/store"
	"github.com/ryangerardwilson/nebula/s3"
)

// testKeeper keeps state in a temporary directory and objects in a local s3
// root, the fake s3. Spawn records the names it would start.
func testKeeper(t *testing.T) (*Keeper, string, *[]string) {
	t.Helper()
	root := t.TempDir()
	objects := filepath.Join(root, "s3")
	t.Setenv("S3_ROOT", objects)
	spawned := &[]string{}
	k := &Keeper{
		Dir:     filepath.Join(root, "state"),
		Now:     time.Now,
		Spawn:   func(name string) { *spawned = append(*spawned, name) },
		Put:     s3.PutMakingBucket,
		Backoff: []time.Duration{time.Millisecond, time.Millisecond},
		Idle:    20 * time.Millisecond,
		Poll:    2 * time.Millisecond,
		Timeout: 10 * time.Second,
	}
	return k, root, spawned
}

func appDB(t *testing.T, root string, rows int) string {
	t.Helper()
	path := filepath.Join(root, "runtime", "app.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	sql := "CREATE TABLE IF NOT EXISTS schema_migration (version INTEGER PRIMARY KEY);\n" +
		"INSERT OR IGNORE INTO schema_migration VALUES (1);\n" +
		"CREATE TABLE IF NOT EXISTS note (id INTEGER PRIMARY KEY, body TEXT NOT NULL);\n"
	for i := 0; i < rows; i++ {
		sql += "INSERT INTO note(body) VALUES ('row');\n"
	}
	if err := store.Exec(context.Background(), path, sql); err != nil {
		t.Fatal(err)
	}
	if !isFile(markerPath(path)) {
		if err := writeMarker(path, "restored"); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func keptNotes(t *testing.T, root string) int {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "kept.sqlite")
	if err := s3.Get(context.Background(), KeepBucket, "app.sqlite", dst); err != nil {
		t.Fatal(err)
	}
	raw, err := store.QueryJSON(context.Background(), dst, "SELECT COUNT(*) AS n FROM note")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := store.Decode(raw, &rows); err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	return rows[0].N
}

func TestMarkIsLocalAndWakesOneWorker(t *testing.T) {
	k, root, spawned := testKeeper(t)
	path := appDB(t, root, 1)
	var puts atomic.Int32
	k.Put = func(context.Context, string, string, string) error {
		puts.Add(1)
		return errors.New("mark must not upload")
	}
	start := time.Now()
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("mark took %v", took)
	}
	if puts.Load() != 0 || len(*spawned) != 1 || (*spawned)[0] != "app" {
		t.Fatalf("puts %d spawned %v", puts.Load(), *spawned)
	}
	rec, err := k.load("app")
	if err != nil || rec.Marked != 1 || rec.Kept != 0 || rec.State() != "waiting" || rec.DirtySince.IsZero() || rec.Path != path {
		t.Fatalf("%+v %v", rec, err)
	}
	lock, ok, err := k.tryLock("app.run.lock")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer lock.Close()
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if len(*spawned) != 1 {
		t.Fatalf("a second worker started while one runs: %v", *spawned)
	}
}

func TestWorkerUploadsAConsistentSnapshotAndCoalesces(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 2)
	if err := s3.MakeBucket(context.Background(), KeepBucket); err != nil {
		t.Fatal(err)
	}
	var puts atomic.Int32
	put := k.Put
	k.Put = func(ctx context.Context, local, bucket, key string) error {
		puts.Add(1)
		if local == path {
			t.Errorf("uploaded the live file")
		}
		time.Sleep(30 * time.Millisecond)
		return put(ctx, local, bucket, key)
	}
	for i := 0; i < 5; i++ {
		if err := k.Mark("app", path); err != nil {
			t.Fatal(err)
		}
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := puts.Load(); n != 1 {
		t.Fatalf("five marks made %d uploads", n)
	}
	if n := keptNotes(t, root); n != 2 {
		t.Fatalf("kept %d notes", n)
	}
	rec, _ := k.load("app")
	if rec.State() != "ok" || rec.Kept != 5 || !rec.DirtySince.IsZero() || rec.KeptAt.IsZero() || rec.Source == "" {
		t.Fatalf("%+v", rec)
	}
	if matches, _ := filepath.Glob(filepath.Join(k.Dir, "snapshots", "*")); len(matches) != 0 {
		t.Fatalf("snapshot left: %v", matches)
	}
}

func TestASlowUploadDoesNotHoldUpMarks(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 1)
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	put := k.Put
	k.Put = func(ctx context.Context, local, bucket, key string) error {
		entered <- struct{}{}
		<-release
		return put(ctx, local, bucket, key)
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- k.Run("app") }()
	<-entered
	start := time.Now()
	if err := store.Exec(context.Background(), path, "INSERT INTO note(body) VALUES ('during the upload');"); err != nil {
		t.Fatal(err)
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 200*time.Millisecond {
		t.Fatalf("commit and mark waited on the upload: %v", took)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := keptNotes(t, root); n != 2 {
		t.Fatalf("the write during the upload was not kept: %d", n)
	}
	if rec, _ := k.load("app"); rec.State() != "ok" || rec.Kept != 2 {
		t.Fatalf("%+v", rec)
	}
}

func TestFailuresBackOffAndStayVisible(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 1)
	var puts atomic.Int32
	k.Put = func(context.Context, string, string, string) error {
		puts.Add(1)
		return errors.New("s3 del-vc2: Connection refused")
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := puts.Load(); n != 3 {
		t.Fatalf("tries %d, want the first and two retries", n)
	}
	rec, _ := k.load("app")
	if rec.State() != "failing" || !strings.Contains(rec.Error, "Connection refused") || rec.DirtySince.IsZero() || rec.Failures != 3 {
		t.Fatalf("%+v", rec)
	}
	k.Put = s3.PutMakingBucket
	left, err := k.Flush(time.Second)
	if err != nil || len(left) != 0 {
		t.Fatal(left, err)
	}
	if rec, _ := k.load("app"); rec.State() != "ok" || rec.Error != "" || rec.Failures != 0 {
		t.Fatalf("after flush %+v", rec)
	}
}

func TestAnEmptyDatabaseNeverReplacesTheBackup(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 3)
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(context.Background(), path, "DELETE FROM note;"); err != nil {
		t.Fatal(err)
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := keptNotes(t, root); n != 3 {
		t.Fatalf("the empty file replaced the backup: %d", n)
	}
	if rec, _ := k.load("app"); rec.State() != "failing" || !strings.Contains(rec.Error, "no data") {
		t.Fatalf("%+v", rec)
	}
}

func TestAFileThatSkippedRestoreWaitsForTheOperator(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 3)
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	// A reboot clears the runtime file and its marker. An app that makes a
	// new file without restoring it must not replace the backup.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(markerPath(path)); err != nil {
		t.Fatal(err)
	}
	if err := store.Exec(context.Background(), path, "CREATE TABLE note (id INTEGER PRIMARY KEY, body TEXT NOT NULL); INSERT INTO note(body) VALUES ('stray');"); err != nil {
		t.Fatal(err)
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := keptNotes(t, root); n != 3 {
		t.Fatalf("an unrestored file replaced the backup: %d", n)
	}
	rec, _ := k.load("app")
	if rec.State() != "failing" || !strings.Contains(rec.Error, "nebula keep adopt app "+path) {
		t.Fatalf("%+v", rec)
	}
	if err := k.Adopt("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := keptNotes(t, root); n != 1 {
		t.Fatalf("the adopted file was not kept: %d", n)
	}
}

func TestAdoptRefusesAnEmptyFile(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := filepath.Join(root, "runtime", "empty.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := k.Adopt("empty", path); err == nil {
		t.Fatal("adopted an empty file")
	}
	if isFile(markerPath(path)) {
		t.Fatal("marker written for an empty file")
	}
}

func TestACoveredCopyIsOkWhateverAnOldErrorSays(t *testing.T) {
	rec := Record{Marked: 4, Kept: 4, Error: "s3 del-vc2: Connection refused"}
	if rec.State() != "ok" {
		t.Fatalf("state %s", rec.State())
	}
	rec.Marked = 5
	if rec.State() != "failing" {
		t.Fatalf("state %s", rec.State())
	}
}

func TestRestoreBringsBackAMissingFile(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 4)
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(markerPath(path))
	how, err := k.Restore("app", path)
	if err != nil || how != "restored" {
		t.Fatal(how, err)
	}
	if body, _ := os.ReadFile(markerPath(path)); !strings.HasPrefix(string(body), "restored ") {
		t.Fatalf("marker %q", body)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal(info, err)
	}
	if how, err := k.Restore("app", path); err != nil || how != "present" {
		t.Fatal(how, err)
	}
	if err := store.Exec(context.Background(), path, "INSERT INTO note(body) VALUES ('after restore');"); err != nil {
		t.Fatal(err)
	}
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	if n := keptNotes(t, root); n != 5 {
		t.Fatalf("kept %d", n)
	}
}

func TestRestoreWithNoBackupStartsFresh(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := filepath.Join(root, "runtime", "new.sqlite")
	how, err := k.Restore("new", path)
	if err != nil || how != "fresh" {
		t.Fatal(how, err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatal(info, err)
	}
	if body, _ := os.ReadFile(markerPath(path)); !strings.HasPrefix(string(body), "fresh ") {
		t.Fatalf("marker %q", body)
	}
}

func TestAnUnreachableBackupMakesNoFile(t *testing.T) {
	k, root, _ := testKeeper(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("S3_ROOT", blocked)
	path := filepath.Join(root, "runtime", "app.sqlite")
	if _, err := k.Restore("app", path); err == nil {
		t.Fatal("restore succeeded without its backup")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("restore made a file: %v", err)
	}
}
func TestSweepMarksAFileThatChangedWithoutAMark(t *testing.T) {
	k, root, spawned := testKeeper(t)
	path := appDB(t, root, 1)
	if err := k.Mark("app", path); err != nil {
		t.Fatal(err)
	}
	if err := k.Run("app"); err != nil {
		t.Fatal(err)
	}
	*spawned = nil
	if err := k.Sweep(); err != nil || len(*spawned) != 0 {
		t.Fatalf("sweep woke a clean copy: %v %v", *spawned, err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := store.Exec(context.Background(), path, "INSERT INTO note(body) VALUES ('missed mark');"); err != nil {
		t.Fatal(err)
	}
	if err := k.Sweep(); err != nil {
		t.Fatal(err)
	}
	if len(*spawned) != 1 {
		t.Fatalf("sweep did not wake the worker: %v", *spawned)
	}
	if rec, _ := k.load("app"); rec.State() != "waiting" {
		t.Fatalf("%+v", rec)
	}
}

func TestKeepCommandStatusAndUsage(t *testing.T) {
	k, root, _ := testKeeper(t)
	path := appDB(t, root, 1)
	var out, errw bytes.Buffer
	if code := k.main([]string{"mark", "app", path}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	out.Reset()
	if code := k.main([]string{"status"}, &out, &errw); code != 0 || !strings.HasPrefix(out.String(), "app waiting") {
		t.Fatalf("%d %q %q", code, out.String(), errw.String())
	}
	out.Reset()
	if code := k.main([]string{"status", "--json"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["state"] != "waiting" || rows[0]["name"] != "app" {
		t.Fatalf("%q %v", out.String(), err)
	}
	out.Reset()
	if code := k.main([]string{"flush", "--timeout", "5s"}, &out, &errw); code != 0 {
		t.Fatalf("%q %q", out.String(), errw.String())
	}
	for _, bad := range [][]string{{"adopt", "app"}, {"mark", "App", path}, {"mark", "app", "relative.sqlite"}, {"mark", "app"}, {"status", "--yaml"}, {"nope"}} {
		errw.Reset()
		if code := k.main(bad, &out, &errw); code != 2 {
			t.Fatalf("%v exited %d %q", bad, code, errw.String())
		}
	}
	out.Reset()
	if code := k.main(nil, &out, &errw); code != 0 || !strings.Contains(out.String(), "nebula keep mark c3po") {
		t.Fatal(out.String())
	}
}
