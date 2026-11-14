package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ryangerardwilson/nebula/internal/drive"
	"github.com/ryangerardwilson/nebula/internal/engine"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
	"github.com/ryangerardwilson/nebula/internal/pg"
	"github.com/ryangerardwilson/nebula/internal/store"
)

var dayName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func DayName(day time.Time) string {
	return day.In(day.Location()).Format("2006-01-02")
}

func Expired(name string, today time.Time, keep int) (expired bool, match bool) {
	if keep < 1 || !dayName.MatchString(name) {
		return false, false
	}
	day, err := time.ParseInLocation("2006-01-02", name, today.Location())
	if err != nil {
		return false, false
	}
	start := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	cutoff := start.AddDate(0, 0, -(keep - 1))
	return day.Before(cutoff), true
}

func Prune(dir string, today time.Time, keep int) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, entry := range entries {
		expired, ok := Expired(entry.Name(), today, keep)
		if !ok || !expired {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func Run(ctx context.Context, out io.Writer) error {
	if engine.Postgres() {
		return runRemote(ctx, out)
	}
	if engine.Object() {
		return runObject(ctx, out)
	}
	if err := layout.Ensure(); err != nil {
		return err
	}
	now := time.Now()
	dir := layout.BackupDir()
	final := filepath.Join(dir, DayName(now))
	tmp, err := os.MkdirTemp(dir, ".snapshot-")
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := writeSnapshot(ctx, tmp, DayName(now)); err != nil {
		return err
	}
	replaced := final + ".replaced"
	_ = os.RemoveAll(replaced)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, replaced); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Rename(replaced, final)
		return err
	}
	_ = os.RemoveAll(replaced)
	ok = true
	pruned, err := Prune(dir, now, layout.KeepDays())
	if err != nil {
		return err
	}
	if err := drive.Publish(ctx, dir); err != nil {
		fmt.Fprintf(out, "backup %s\n", final)
		return err
	}
	fmt.Fprintf(out, "backup %s\npruned %d\n", final, pruned)
	return nil
}

func writeSnapshot(ctx context.Context, dir, day string) error {
	dataDir := filepath.Join(dir, "data")
	ormDir := filepath.Join(dir, "orm")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(ormDir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(layout.DataDir())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(entry.Name(), ".sqlite")
		if !ok || !orm.ValidName(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		dst := filepath.Join(dataDir, name+".sqlite")
		if err := store.BackupFile(ctx, layout.DBPath(name), dst); err != nil {
			return err
		}
		if err := os.Chmod(dst, 0o600); err != nil {
			return err
		}
		body, err := os.ReadFile(layout.ORMPath(name))
		if err == nil {
			if err := os.WriteFile(filepath.Join(ormDir, name), body, 0o600); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	var manifest strings.Builder
	fmt.Fprintf(&manifest, "day %s\n", day)
	for _, name := range names {
		fmt.Fprintf(&manifest, "%s\n", name)
	}
	return os.WriteFile(filepath.Join(dir, "snapshot.txt"), []byte(manifest.String()), 0o600)
}

func runRemote(ctx context.Context, out io.Writer) error {
	names, err := pg.Names(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	path, err := pg.Dump(ctx, DayName(now), names)
	if err != nil {
		return err
	}
	days, err := pg.BackupDays(ctx)
	if err != nil {
		return err
	}
	pruned := 0
	for _, name := range days {
		expired, ok := Expired(name, now, layout.KeepDays())
		if !ok || !expired {
			continue
		}
		if err := pg.RemoveBackup(ctx, name); err != nil {
			return err
		}
		pruned++
	}
	fmt.Fprintf(out, "backup %s\npruned %d\n", path, pruned)
	return nil
}
