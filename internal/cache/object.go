package cache

import (
	"context"
	"fmt"
	"os"

	"github.com/ryangerardwilson/nebula/internal/pg"
	"github.com/ryangerardwilson/nebula/internal/store"
	"github.com/ryangerardwilson/nebula/s3"
)

// Publish uploads a consistent snapshot of the runtime sqlite to s3.
func Publish(ctx context.Context, name string) error {
	path := DBPath(name)
	if !isFile(path) {
		return fmt.Errorf("database %s has no runtime copy", name)
	}
	tmp, err := os.CreateTemp("", "sqlx-object-*.sqlite")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		return err
	}
	defer os.Remove(tmpName)
	if err := os.Remove(tmpName); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := store.BackupFile(ctx, path, tmpName); err != nil {
		return err
	}
	return s3.PutMakingBucket(ctx, tmpName, s3.DatabaseBucket, name+".sqlite")
}

func openObject(ctx context.Context, name, path string) (string, error) {
	key := name + ".sqlite"
	ok, err := s3.Exists(ctx, s3.DatabaseBucket, key)
	if err != nil {
		return "", err
	}
	if ok {
		return installObject(ctx, path, key)
	}
	// A local object root is the test store. It must not dial Postgres.
	if s3.Local() {
		return "", fmt.Errorf("database %s does not exist", name)
	}
	remote, err := pg.Exists(ctx, name)
	if err != nil {
		return "", err
	}
	if !remote {
		return "", fmt.Errorf("database %s does not exist", name)
	}
	if err := restore(ctx, name, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := Publish(ctx, name); err != nil {
		_ = Remove(name)
		return "", err
	}
	return path, nil
}

func installObject(ctx context.Context, path, key string) (string, error) {
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	tmp := path + ".download"
	if err := s3.Get(ctx, s3.DatabaseBucket, key, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}
