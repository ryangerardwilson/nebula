package s3

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Stat returns the modification time and size of one object. It does not
// read the body. A missing object returns ErrNotFound.
func Stat(ctx context.Context, bucket, key string) (time.Time, int64, error) {
	if err := checkBucket(bucket); err != nil {
		return time.Time{}, 0, err
	}
	if err := checkKey(key); err != nil {
		return time.Time{}, 0, err
	}
	if root := localRoot(); root != "" {
		return statLocal(root, bucket, key)
	}
	return statSSH(ctx, bucket, key)
}

func statLocal(root, bucket, key string) (time.Time, int64, error) {
	path, err := localPath(root, bucket, key)
	if err != nil {
		return time.Time{}, 0, err
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) || err == nil && !info.Mode().IsRegular() {
		return time.Time{}, 0, fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
	}
	if err != nil {
		return time.Time{}, 0, err
	}
	return info.ModTime(), info.Size(), nil
}

// statSSH runs one remote stat on the object path. The format has no space
// because the remote login shell splits the joined command.
func statSSH(ctx context.Context, bucket, key string) (time.Time, int64, error) {
	host, err := sshHostName()
	if err != nil {
		return time.Time{}, 0, err
	}
	cmd, err := sshCommand(ctx, host, pickObject(bucket, key)+" && stat -c %Y,%s -- \"$src\"")
	if err != nil {
		return time.Time{}, 0, err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return time.Time{}, 0, ctx.Err()
		}
		if exitCode(err) == missingExit {
			return time.Time{}, 0, fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
		}
		return time.Time{}, 0, fmt.Errorf("s3 %s: %s", host, sshText(stderr.String(), err))
	}
	return parseStat(stdout.String())
}

func parseStat(out string) (time.Time, int64, error) {
	secText, sizeText, found := strings.Cut(strings.TrimSpace(out), ",")
	if !found {
		return time.Time{}, 0, fmt.Errorf("s3 stat: unexpected output %q", strings.TrimSpace(out))
	}
	sec, err := strconv.ParseInt(secText, 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	size, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil {
		return time.Time{}, 0, err
	}
	return time.Unix(sec, 0), size, nil
}
