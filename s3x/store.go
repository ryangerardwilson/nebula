// Package s3x stores objects for the nebula s3 command.
// S3X_ROOT selects a local directory and does not open SSH.
// Without it, objects live on the SSH host under RemoteRoot.
package s3x

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/engine"
)

// ErrUsage is a command-shape error. ErrNotFound is a missing bucket or object.
// ErrNotEmpty is a bucket that still has objects.
var (
	ErrUsage    = errors.New("usage")
	ErrNotFound = errors.New("not found")
	ErrNotEmpty = errors.New("bucket is not empty")
)

// Object is one stored file.
type Object struct {
	Key  string
	Size int64
}

func localRoot() string { return os.Getenv("S3X_ROOT") }

func sshHostName() (string, error) {
	host := os.Getenv("S3X_SSH")
	if host == "" {
		host = engine.SSHHost()
	}
	if err := checkHost(host); err != nil {
		return "", err
	}
	return host, nil
}

// MakeBucket creates a bucket. An existing bucket is left in place.
func MakeBucket(ctx context.Context, bucket string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return os.MkdirAll(filepath.Join(root, bucket), 0o700)
	}
	return makeBucketSSH(ctx, bucket)
}

// RemoveBucket deletes an empty bucket.
func RemoveBucket(ctx context.Context, bucket string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return removeBucketLocal(root, bucket)
	}
	return removeBucketSSH(ctx, bucket)
}

// BucketExists reports whether the bucket directory is present.
func BucketExists(ctx context.Context, bucket string) (bool, error) {
	if err := checkBucket(bucket); err != nil {
		return false, err
	}
	if root := localRoot(); root != "" {
		info, err := os.Stat(filepath.Join(root, bucket))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return info.IsDir(), nil
	}
	return bucketExistsSSH(ctx, bucket)
}

// ListBuckets returns bucket names in order.
func ListBuckets(ctx context.Context) ([]string, error) {
	if root := localRoot(); root != "" {
		return listBucketsLocal(root)
	}
	return listBucketsSSH(ctx)
}

// Put stores the file at key. The bucket must already exist.
func Put(ctx context.Context, local, bucket, key string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if err := checkKey(key); err != nil {
		return err
	}
	info, err := os.Stat(local)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a file", ErrUsage, local)
	}
	if root := localRoot(); root != "" {
		return putLocal(root, local, bucket, key)
	}
	return putSSH(ctx, local, bucket, key)
}

// Get copies an object to local, replacing a file that is already there.
func Get(ctx context.Context, bucket, key, local string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if err := checkKey(key); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return getLocal(root, bucket, key, local)
	}
	return getSSH(ctx, bucket, key, local)
}

// Delete removes an object.
func Delete(ctx context.Context, bucket, key string) error {
	if err := checkBucket(bucket); err != nil {
		return err
	}
	if err := checkKey(key); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return deleteLocal(root, bucket, key)
	}
	return deleteSSH(ctx, bucket, key)
}

// Exists reports whether the object is present. A missing bucket is not an error.
func Exists(ctx context.Context, bucket, key string) (bool, error) {
	if err := checkBucket(bucket); err != nil {
		return false, err
	}
	if err := checkKey(key); err != nil {
		return false, err
	}
	if root := localRoot(); root != "" {
		path, err := localPath(root, bucket, key)
		if err != nil {
			return false, err
		}
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return info.Mode().IsRegular(), nil
	}
	return existsSSH(ctx, bucket, key)
}

// List returns objects in a bucket. prefix restricts the keys. A missing bucket
// returns ErrNotFound.
func List(ctx context.Context, bucket, prefix string) ([]Object, error) {
	if err := checkBucket(bucket); err != nil {
		return nil, err
	}
	if prefix != "" {
		if err := checkKey(prefix); err != nil {
			return nil, err
		}
	}
	if root := localRoot(); root != "" {
		return listLocal(root, bucket, prefix)
	}
	return listSSH(ctx, bucket, prefix)
}

// Copy copies one object onto another. An existing destination is replaced.
func Copy(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := checkPair(srcBucket, srcKey, dstBucket, dstKey); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return copyLocal(root, srcBucket, srcKey, dstBucket, dstKey, false)
	}
	return copySSH(ctx, srcBucket, srcKey, dstBucket, dstKey, false)
}

// Move moves one object onto another. An existing destination is replaced.
func Move(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := checkPair(srcBucket, srcKey, dstBucket, dstKey); err != nil {
		return err
	}
	if root := localRoot(); root != "" {
		return copyLocal(root, srcBucket, srcKey, dstBucket, dstKey, true)
	}
	return copySSH(ctx, srcBucket, srcKey, dstBucket, dstKey, true)
}

func checkPair(srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := checkBucket(srcBucket); err != nil {
		return err
	}
	if err := checkBucket(dstBucket); err != nil {
		return err
	}
	if err := checkKey(srcKey); err != nil {
		return err
	}
	return checkKey(dstKey)
}

func localPath(root, bucket, key string) (string, error) {
	if err := checkBucket(bucket); err != nil {
		return "", err
	}
	base := filepath.Join(root, bucket)
	if key == "" {
		return inside(root, base)
	}
	if err := checkKey(key); err != nil {
		return "", err
	}
	return inside(root, filepath.Join(base, filepath.FromSlash(key)))
}

func inside(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: path escapes the store", ErrUsage)
	}
	return path, nil
}

func listBucketsLocal(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && checkBucket(entry.Name()) == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func removeBucketLocal(root, bucket string) error {
	dir, err := localPath(root, bucket, "")
	if err != nil {
		return err
	}
	objects, err := listLocal(root, bucket, "")
	if err != nil {
		return err
	}
	if len(objects) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEmpty, bucket)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}

func putLocal(root, src, bucket, key string) error {
	if _, err := os.Stat(filepath.Join(root, bucket)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
		}
		return err
	}
	dest, err := localPath(root, bucket, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	return copyFile(src, dest)
}

func getLocal(root, bucket, key, dest string) error {
	src, err := localPath(root, bucket, key)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	return copyFile(src, dest)
}

func deleteLocal(root, bucket, key string) error {
	path, err := localPath(root, bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
		}
		return err
	}
	return nil
}

func listLocal(root, bucket, prefix string) ([]Object, error) {
	dir, err := localPath(root, bucket, "")
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		return nil, fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var objects []Object
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		if strings.Contains(entry.Name(), ".tmp.") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		objects = append(objects, Object{Key: key, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func copyLocal(root, srcBucket, srcKey, dstBucket, dstKey string, move bool) error {
	src, err := localPath(root, srcBucket, srcKey)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, dstBucket)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bucket %s: %w", dstBucket, ErrNotFound)
		}
		return err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("s3://%s/%s: %w", srcBucket, srcKey, ErrNotFound)
		}
		return err
	}
	dest, err := localPath(root, dstBucket, dstKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}
	if move {
		if err := os.Rename(src, dest); err == nil {
			return os.Chmod(dest, 0o600)
		}
	}
	if err := copyFile(src, dest); err != nil {
		return err
	}
	if move {
		return os.Remove(src)
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return err
	}
	ok = true
	return nil
}
