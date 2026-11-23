package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/internal/orm"
	"github.com/ryangerardwilson/nebula/internal/pg"
	"github.com/ryangerardwilson/nebula/internal/store"
	"github.com/ryangerardwilson/nebula/s3"
)

func createObject(ctx context.Context, name string) error {
	if err := os.MkdirAll(cache.Dir(), 0o700); err != nil {
		return err
	}
	if cache.Has(name) {
		return cache.Publish(ctx, name)
	}
	if _, err := cache.Open(ctx, name); err == nil {
		return nil
	} else if !missingDB(err) {
		return err
	}
	path := cache.DBPath(name)
	sql := "PRAGMA journal_mode=WAL;\n" + ormTable
	if err := store.Exec(ctx, path, sql); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return cache.Publish(ctx, name)
}

func commitObject(ctx context.Context, db, path, sql string) error {
	if remoteCommit() {
		return cache.Publish(ctx, db)
	}
	pre, err := snapshotFile(ctx, path)
	if err != nil {
		return err
	}
	defer os.Remove(pre)
	if err := store.Exec(ctx, path, sql); err != nil {
		return err
	}
	if localCommit() {
		return nil
	}
	if err := cache.Publish(ctx, db); err != nil {
		if rerr := restoreSnapshot(pre, path); rerr != nil {
			return fmt.Errorf("%s (restore failed: %s)", err.Error(), rerr.Error())
		}
		return err
	}
	return nil
}

func objectNames() ([]string, error) {
	set := map[string]struct{}{}
	add := func(name string) {
		if orm.ValidName(name) {
			set[name] = struct{}{}
		}
	}
	local, err := cache.Names()
	if err != nil {
		return nil, err
	}
	for _, name := range local {
		add(name)
	}
	keys, err := s3.List(context.Background(), s3.DatabaseBucket, "")
	if err != nil && !missingObject(err) {
		if len(set) > 0 && s3.Unreachable(err) {
			return nameList(set), nil
		}
		if !s3.Local() && len(set) == 0 {
			return postgresNames(set)
		}
		if len(set) == 0 {
			return nil, err
		}
	}
	for _, object := range keys {
		name, ok := strings.CutSuffix(object.Key, ".sqlite")
		if ok && !strings.Contains(name, "/") {
			add(name)
		}
	}
	if s3.Local() {
		return nameList(set), nil
	}
	return postgresNames(set)
}

func postgresNames(set map[string]struct{}) ([]string, error) {
	names, err := pg.Names(context.Background())
	if err != nil {
		if len(set) > 0 && pg.Unreachable(err) {
			return nameList(set), nil
		}
		if len(set) > 0 {
			return nameList(set), nil
		}
		return nil, err
	}
	for _, name := range names {
		if orm.ValidName(name) {
			set[name] = struct{}{}
		}
	}
	return nameList(set), nil
}

func dropObject(name string) error {
	ctx := context.Background()
	err := s3.Delete(ctx, s3.DatabaseBucket, name+".sqlite")
	if err != nil && !missingObject(err) {
		return err
	}
	if err := cache.Remove(name); err != nil {
		return err
	}
	if err := os.Remove(layout.ORMPath(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if s3.Local() {
		return nil
	}
	exists, err := pg.Exists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return pg.Drop(ctx, name)
}

func renameObject(oldName, newName string) error {
	ctx := context.Background()
	destKey := newName + ".sqlite"
	ok, err := s3.Exists(ctx, s3.DatabaseBucket, destKey)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	srcKey := oldName + ".sqlite"
	ok, err = s3.Exists(ctx, s3.DatabaseBucket, srcKey)
	if err != nil {
		return err
	}
	if ok {
		if err := s3.Copy(ctx, s3.DatabaseBucket, srcKey, s3.DatabaseBucket, destKey); err != nil {
			return err
		}
		if err := s3.Delete(ctx, s3.DatabaseBucket, srcKey); err != nil {
			return err
		}
	}
	if err := cache.Rename(oldName, newName); err != nil {
		return err
	}
	if err := renameORM(oldName, newName); err != nil {
		return err
	}
	if s3.Local() {
		return nil
	}
	oldExists, err := pg.Exists(ctx, oldName)
	if err != nil || !oldExists {
		return err
	}
	newExists, err := pg.Exists(ctx, newName)
	if err != nil || newExists {
		return err
	}
	return pg.Rename(ctx, oldName, newName)
}

func renameORM(oldName, newName string) error {
	dst := layout.ORMPath(newName)
	if _, err := os.Stat(dst); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	src := layout.ORMPath(oldName)
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(src, dst)
}

func snapshotFile(ctx context.Context, path string) (string, error) {
	tmp, err := os.CreateTemp("", "sqlx-pre-*.sqlite")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	if err := store.BackupFile(ctx, path, name); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func restoreSnapshot(pre, path string) error {
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	_ = os.Remove(path)
	in, err := os.Open(pre)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func missingDB(err error) bool {
	return err != nil && strings.Contains(err.Error(), "does not exist")
}

func missingObject(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

func nameList(set map[string]struct{}) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
