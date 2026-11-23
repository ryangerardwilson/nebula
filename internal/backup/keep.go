package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ryangerardwilson/nebula/internal/store"
	"github.com/ryangerardwilson/nebula/s3"
)

// Keep owns one fact per application database: whether its local sqlite
// file has a durable copy at s3://backups/<name>.sqlite. The application
// keeps its schema, its rules, and its one write path; after a commit it
// says "this file changed" with Mark, and at startup it asks Restore for a
// missing file. Mark never touches the network. A detached worker, one per
// database, uploads a consistent snapshot afterwards and records how far
// the copy lags.

// KeepBucket holds the copy of each application database.
const KeepBucket = "backups"

var keepName = regexp.MustCompile(`^[a-z0-9_][a-z0-9_.-]{0,63}$`)

// ErrKeepUsage is a bad database name or path.
var ErrKeepUsage = errors.New("usage")

// Keeper is the backup component. The zero value is not usable; NewKeeper
// gives the production one.
type Keeper struct {
	Dir     string // state directory: one record and one lock per database
	Now     func() time.Time
	Spawn   func(name string) // starts a detached worker for name
	Put     func(ctx context.Context, local, bucket, key string) error
	Backoff []time.Duration // wait before each retry; after the last, the worker exits
	Idle    time.Duration   // a clean worker waits this long for another mark
	Poll    time.Duration
	Timeout time.Duration // bound on one snapshot and upload
}

// NewKeeper is the keeper the nebula command uses.
func NewKeeper() *Keeper {
	return &Keeper{
		Dir:     KeepDir(),
		Now:     time.Now,
		Spawn:   spawnKeepWorker,
		Put:     s3.PutMakingBucket,
		Backoff: []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute},
		Idle:    3 * time.Second,
		Poll:    100 * time.Millisecond,
		Timeout: 60 * time.Second,
	}
}

// KeepDir is the state directory. NEBULA_KEEP_DIR overrides it. A test
// binary that sets neither gets a directory of its own, so no test reaches
// the operator's keep records.
func KeepDir() string {
	if v := os.Getenv("NEBULA_KEEP_DIR"); v != "" {
		return v
	}
	if testing.Testing() {
		return filepath.Join(os.TempDir(), "nebula-keep-test-"+strconv.Itoa(os.Getpid()))
	}
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "nebula", "keep")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "state", "nebula", "keep")
}

// Record is the backup status of one database. Marked counts the marks;
// Kept is the mark the last good copy covers. The copy lags while
// Marked > Kept.
type Record struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Marked     int64     `json:"marked"`
	Kept       int64     `json:"kept"`
	DirtySince time.Time `json:"dirty_since,omitzero"` // first mark the copy does not cover; zero when it covers all
	KeptAt     time.Time `json:"kept_at,omitzero"`     // last good upload
	KeptSize   int64     `json:"kept_size,omitempty"`
	Source     string    `json:"source,omitempty"` // size and mtime of the file at that upload; sweep compares it
	Error      string    `json:"error,omitempty"`  // last failed upload; empty after a good one
	ErrorAt    time.Time `json:"error_at,omitzero"`
	Failures   int       `json:"failures,omitempty"`
}

// State is ok, waiting, or failing. It is derived, never stored. A copy
// that covers every mark is ok whatever an older failure left in Error.
func (r Record) State() string {
	switch {
	case r.Marked <= r.Kept:
		return "ok"
	case r.Error != "":
		return "failing"
	default:
		return "waiting"
	}
}

func checkKeep(name, path string) error {
	return checkKeepPath(name, path, false)
}

func checkKeepPath(name, path string, needPath bool) error {
	if needPath && path == "" {
		return fmt.Errorf("%w: a database path is required", ErrKeepUsage)
	}
	if !keepName.MatchString(name) {
		return fmt.Errorf("%w: database name must be lowercase letters, digits, _ . -", ErrKeepUsage)
	}
	if path != "" && (!filepath.IsAbs(path) || !strings.HasSuffix(path, ".sqlite")) {
		return fmt.Errorf("%w: database path must be an absolute .sqlite path", ErrKeepUsage)
	}
	return nil
}

// Mark records that the file changed and wakes its worker. It returns in
// milliseconds and never touches the network.
func (k *Keeper) Mark(name, path string) error {
	if err := checkKeepPath(name, path, true); err != nil {
		return err
	}
	err := k.update(name, func(r *Record) {
		r.Path = path
		r.Marked++
		if r.DirtySince.IsZero() {
			r.DirtySince = k.stamp()
		}
	})
	if err != nil {
		return err
	}
	k.wake(name)
	return nil
}

func (k *Keeper) wake(name string) {
	if k.Spawn == nil {
		return
	}
	lock, ok, err := k.tryLock(name + ".run.lock")
	if err != nil || !ok {
		return // a worker is running and sees this mark before it exits
	}
	_ = lock.Close()
	k.Spawn(name)
}

// Run is the detached worker for one database. It uploads until no mark has
// arrived for Idle, or until the retries run out. A mark that landed after
// its last look found the lock held, so it looks once more after letting go.
func (k *Keeper) Run(name string) error {
	if err := checkKeep(name, ""); err != nil {
		return err
	}
	for {
		lock, ok, err := k.tryLock(name + ".run.lock")
		if err != nil || !ok {
			return err
		}
		gaveUp := k.drain(name)
		_ = lock.Close()
		rec, err := k.load(name)
		if err != nil || gaveUp || rec.Marked <= rec.Kept {
			return err
		}
	}
}

// drain uploads while the copy lags and reports whether it stopped because
// the retries ran out.
func (k *Keeper) drain(name string) bool {
	quiet := time.Now()
	for {
		rec, err := k.load(name)
		if err != nil {
			return true
		}
		if rec.Marked <= rec.Kept {
			if time.Since(quiet) >= k.Idle {
				return false
			}
			time.Sleep(k.Poll)
			continue
		}
		if err := k.keepOnce(rec); err != nil {
			failures := k.fail(name, err)
			if failures > len(k.Backoff) {
				return true
			}
			time.Sleep(k.Backoff[failures-1])
			continue
		}
		quiet = time.Now()
	}
}

// keepOnce snapshots the file and uploads the snapshot. It records the mark
// the upload covers.
func (k *Keeper) keepOnce(rec Record) error {
	started := k.stamp()
	covered := rec.Marked
	size, source, err := k.publish(rec)
	if err != nil {
		return err
	}
	return k.update(rec.Name, func(r *Record) {
		if covered > r.Kept {
			r.Kept = covered
		}
		r.DirtySince = time.Time{}
		if r.Marked > r.Kept {
			r.DirtySince = started
		}
		r.KeptAt = k.stamp()
		r.KeptSize = size
		r.Source = source
		r.Error = ""
		r.ErrorAt = time.Time{}
		r.Failures = 0
	})
}

func (k *Keeper) fail(name string, cause error) int {
	failures := 0
	_ = k.update(name, func(r *Record) {
		r.Error = oneLine(cause.Error())
		r.ErrorAt = k.stamp()
		r.Failures++
		failures = r.Failures
	})
	return failures
}

// publish holds the file's publish lock, the one c3po's own publisher takes,
// from the snapshot until the upload returns, so an older publisher and this
// worker never upload the same object at once.
func (k *Keeper) publish(rec Record) (int64, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), k.Timeout)
	defer cancel()
	if !isFile(rec.Path) {
		return 0, "", fmt.Errorf("%s is missing", rec.Path)
	}
	lock, err := flock(rec.Path+".publish.lock", false)
	if err != nil {
		return 0, "", err
	}
	defer lock.Close()
	source := fileStamp(rec.Path)
	snap, err := k.snapshot(ctx, rec)
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(snap)
	if err := k.mayReplace(ctx, rec, snap); err != nil {
		return 0, "", err
	}
	if err := k.put(ctx, snap, rec.Name+".sqlite"); err != nil {
		return 0, "", err
	}
	info, err := os.Stat(snap)
	if err != nil {
		return 0, "", err
	}
	return info.Size(), source, nil
}

// snapshot writes a consistent copy with the sqlite backup API. A snapshot
// left by a killed worker is removed first: this worker holds the run lock.
func (k *Keeper) snapshot(ctx context.Context, rec Record) (string, error) {
	dir := filepath.Join(k.Dir, "snapshots")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stale, _ := filepath.Glob(filepath.Join(dir, rec.Name+".*.snapshot"))
	for _, path := range stale {
		_ = os.Remove(path)
	}
	snap := filepath.Join(dir, rec.Name+"."+strconv.Itoa(os.Getpid())+".snapshot")
	if err := store.BackupFile(ctx, rec.Path, snap); err != nil {
		_ = os.Remove(snap)
		return "", fmt.Errorf("snapshot %s: %w", rec.Path, err)
	}
	return snap, nil
}

// mayReplace refuses a snapshot that would replace a good backup with
// nothing: an empty database, or one that did not come from Restore.
func (k *Keeper) mayReplace(ctx context.Context, rec Record, snap string) error {
	held, err := holdsData(ctx, snap)
	if err != nil {
		return err
	}
	if !held {
		return errors.New("refused: the database holds no data, so it does not replace the backup")
	}
	if isFile(markerPath(rec.Path)) {
		return nil
	}
	return fmt.Errorf("refused: the file did not come from nebula keep restore; if it is current, run nebula keep adopt %s %s once", rec.Name, rec.Path)
}

func (k *Keeper) put(ctx context.Context, local, key string) error {
	return k.Put(ctx, local, KeepBucket, key)
}

// Adopt vouches for a runtime file that did not come from Restore, such as
// one an application restored itself before keep existed. The file must
// hold data. It is the operator's call, never automatic: an empty or
// unrestored file must not replace a good backup by default.
func (k *Keeper) Adopt(name, path string) error {
	if err := checkKeepPath(name, path, true); err != nil {
		return err
	}
	if !isFile(path) {
		return fmt.Errorf("%s is missing", path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), k.Timeout)
	defer cancel()
	held, err := holdsData(ctx, path)
	if err != nil {
		return err
	}
	if !held {
		return errors.New("refused: the database holds no data")
	}
	if err := writeMarker(path, "adopted"); err != nil {
		return err
	}
	return k.Mark(name, path)
}

// Restore brings a missing file back from its copy. A present file is left
// alone and the network is not touched. An absent copy makes an empty file
// and says so in the marker. An unreachable copy fails and makes no file.
func (k *Keeper) Restore(name, path string) (string, error) {
	if err := checkKeepPath(name, path, true); err != nil {
		return "", err
	}
	if isFile(path) {
		return "present", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), k.Timeout)
	defer cancel()
	tmp := path + ".restore"
	_ = os.Remove(tmp)
	defer os.Remove(tmp)
	how := "restored"
	err := s3.Get(ctx, KeepBucket, name+".sqlite", tmp)
	switch {
	case errors.Is(err, s3.ErrNotFound):
		how = "fresh"
		err = os.WriteFile(tmp, nil, 0o600)
	case err != nil:
		return "", fmt.Errorf("restore s3://%s/%s.sqlite: %w", KeepBucket, name, err)
	}
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return "", err
	}
	// The marker is written before the file appears, so a crash between the
	// two never leaves a restored file keep refuses to upload.
	if err := writeMarker(path, how); err != nil {
		return "", err
	}
	if err := os.Link(tmp, path); err != nil {
		if os.IsExist(err) {
			return "present", nil // another process restored it meanwhile
		}
		_ = os.Remove(markerPath(path))
		return "", err
	}
	return how, k.update(name, func(r *Record) {
		r.Path = path
		r.Kept = r.Marked
		r.DirtySince = time.Time{}
		r.Error = ""
		r.ErrorAt = time.Time{}
		r.Failures = 0
		r.Source = fileStamp(path)
	})
}

// Flush uploads every database whose copy lags, waiting up to timeout. It
// returns the names still lagging.
func (k *Keeper) Flush(timeout time.Duration) ([]string, error) {
	recs, err := k.Records()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	var left []string
	for _, rec := range recs {
		if rec.Marked <= rec.Kept {
			continue
		}
		if !k.flushOne(rec.Name, deadline) {
			left = append(left, rec.Name)
		}
	}
	return left, nil
}

func (k *Keeper) flushOne(name string, deadline time.Time) bool {
	for time.Now().Before(deadline) {
		lock, ok, err := k.tryLock(name + ".run.lock")
		if err != nil {
			return false
		}
		if !ok {
			time.Sleep(k.Poll) // a worker is uploading; wait for it
			continue
		}
		rec, err := k.load(name)
		if err == nil && rec.Marked > rec.Kept {
			err = k.keepOnce(rec)
			if err != nil {
				k.fail(name, err)
			}
		}
		_ = lock.Close()
		if rec, err := k.load(name); err == nil && rec.Marked <= rec.Kept {
			return true
		}
		if err != nil {
			return false
		}
		time.Sleep(k.Poll)
	}
	return false
}

// Sweep is the safety net for a missed mark. A file that changed since its
// last upload is marked; a lagging copy with no worker gets one.
func (k *Keeper) Sweep() error {
	recs, err := k.Records()
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if !isFile(rec.Path) {
			continue
		}
		if rec.Marked > rec.Kept || fileStamp(rec.Path) != rec.Source {
			if err := k.Mark(rec.Name, rec.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

// Records lists every database keep knows, by name.
func (k *Keeper) Records() ([]Record, error) {
	entries, err := os.ReadDir(k.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []Record
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || entry.IsDir() || checkKeep(name, "") != nil {
			continue
		}
		rec, err := k.load(name)
		if err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Name < recs[j].Name })
	return recs, nil
}

func (k *Keeper) recordPath(name string) string {
	return filepath.Join(k.Dir, name+".json")
}

func (k *Keeper) load(name string) (Record, error) {
	raw, err := os.ReadFile(k.recordPath(name))
	if os.IsNotExist(err) {
		return Record{Name: name}, nil
	}
	if err != nil {
		return Record{}, err
	}
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, fmt.Errorf("keep record %s: %w", name, err)
	}
	rec.Name = name
	return rec, nil
}

// update is the one write path to a record: read, change, and replace it
// under the record's lock.
func (k *Keeper) update(name string, change func(*Record)) error {
	if err := os.MkdirAll(k.Dir, 0o700); err != nil {
		return err
	}
	lock, err := flock(filepath.Join(k.Dir, name+".lock"), false)
	if err != nil {
		return err
	}
	defer lock.Close()
	rec, err := k.load(name)
	if err != nil {
		return err
	}
	change(&rec)
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	tmp := k.recordPath(name) + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, k.recordPath(name))
}

func (k *Keeper) stamp() time.Time {
	return k.Now().UTC().Truncate(time.Second)
}

func (k *Keeper) tryLock(name string) (*os.File, bool, error) {
	if err := os.MkdirAll(k.Dir, 0o700); err != nil {
		return nil, false, err
	}
	file, err := flock(filepath.Join(k.Dir, name), true)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return file, true, nil
}

// flock opens path and takes an exclusive lock, without waiting when
// nonblocking. Closing the file releases it.
func flock(path string, nonblocking bool) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	how := syscall.LOCK_EX
	if nonblocking {
		how |= syscall.LOCK_NB
	}
	if err := syscall.Flock(int(file.Fd()), how); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

// holdsData reports whether any table other than a schema ledger has a row.
func holdsData(ctx context.Context, path string) (bool, error) {
	raw, err := store.QueryJSON(ctx, path, "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT IN ('schema_migration', 'app_once', 'sqlx_orm')")
	if err != nil {
		return false, err
	}
	var tables []struct {
		Name string `json:"name"`
	}
	if err := store.Decode(raw, &tables); err != nil {
		return false, err
	}
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table.Name, `"`, `""`) + `"`
		raw, err := store.QueryJSON(ctx, path, "SELECT EXISTS (SELECT 1 FROM "+quoted+") AS n")
		if err != nil {
			return false, err
		}
		var rows []struct {
			N int `json:"n"`
		}
		if err := store.Decode(raw, &rows); err != nil {
			return false, err
		}
		if len(rows) == 1 && rows[0].N == 1 {
			return true, nil
		}
	}
	return false, nil
}

// markerPath sits beside the runtime file, so it lives and dies with it.
func markerPath(path string) string {
	return path + ".keep"
}

func writeMarker(path, how string) error {
	body := how + " " + time.Now().UTC().Format(time.RFC3339) + "\n"
	return os.WriteFile(markerPath(path), []byte(body), 0o600)
}

func fileStamp(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(info.Size(), 10) + ":" + strconv.FormatInt(info.ModTime().UnixNano(), 10)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 300 {
		text = text[:300]
	}
	return text
}

// spawnKeepWorker starts "nebula keep run <name>" in its own session, so it
// outlives the command that marked the file.
// A test binary starts none: os.Executable is the test binary.
func spawnKeepWorker(name string) {
	if testing.Testing() {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "keep", "run", name)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if null, err := os.Open(os.DevNull); err == nil {
		cmd.Stdin = null
		cmd.Stdout = null
		cmd.Stderr = null
	}
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release()
}
