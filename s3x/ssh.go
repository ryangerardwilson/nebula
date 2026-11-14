package s3x

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Unreachable reports that the SSH host did not accept a connection.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 255 {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "timed out") || strings.Contains(msg, "Could not resolve") || strings.Contains(msg, "Connection refused")
}

func remotePath(bucket, key string) string {
	path := RemoteRoot + "/" + bucket
	if key != "" {
		path += "/" + key
	}
	return path
}

func makeBucketSSH(ctx context.Context, bucket string) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, "mkdir", "-p", "--", remotePath(bucket, ""))
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, "chmod", "700", RemoteRoot, remotePath(bucket, ""))
	return err
}

func bucketExistsSSH(ctx context.Context, bucket string) (bool, error) {
	host, err := sshHostName()
	if err != nil {
		return false, err
	}
	err = sshStatus(ctx, host, "test", "-d", remotePath(bucket, ""))
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

func listBucketsSSH(ctx context.Context) ([]string, error) {
	host, err := sshHostName()
	if err != nil {
		return nil, err
	}
	out, err := ssh(ctx, host, nil, "find", RemoteRoot, "-mindepth", "1", "-maxdepth", "1", "-type", "d", "-printf", "%f\\n")
	if err != nil {
		if strings.Contains(err.Error(), "No such file") {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && checkBucket(line) == nil {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names, nil
}

func putSSH(ctx context.Context, local, bucket, key string) error {
	ok, err := bucketExistsSSH(ctx, bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
	}
	host, err := sshHostName()
	if err != nil {
		return err
	}
	dest := remotePath(bucket, key)
	parent := remoteDir(dest)
	tmp := dest + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 36)
	if _, err := ssh(ctx, host, nil, "mkdir", "-p", "--", parent); err != nil {
		return err
	}
	file, err := os.Open(local)
	if err != nil {
		return err
	}
	defer file.Close()
	script := `cat > "$1" && chmod 600 "$1" && mv "$1" "$2"`
	_, err = ssh(ctx, host, file, "bash", "-c", script, "bash", tmp, dest)
	return err
}

func getSSH(ctx context.Context, bucket, key, local string) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parentDir(local), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parentDir(local), ".get-*")
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
	cmd, err := sshCommand(ctx, host, "cat", "--", remotePath(bucket, key))
	if err != nil {
		_ = tmp.Close()
		return err
	}
	cmd.Stdout = tmp
	if err := cmd.Run(); err != nil {
		_ = tmp.Close()
		if exitCode(err) == 1 {
			return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
		}
		return fmt.Errorf("s3 %s: %s", host, err.Error())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, local); err != nil {
		return err
	}
	ok = true
	return nil
}

func deleteSSH(ctx context.Context, bucket, key string) error {
	ok, err := existsSSH(ctx, bucket, key)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
	}
	host, err := sshHostName()
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, "rm", "-f", "--", remotePath(bucket, key))
	return err
}

func existsSSH(ctx context.Context, bucket, key string) (bool, error) {
	host, err := sshHostName()
	if err != nil {
		return false, err
	}
	err = sshStatus(ctx, host, "test", "-f", remotePath(bucket, key))
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

func listSSH(ctx context.Context, bucket, prefix string) ([]Object, error) {
	ok, err := bucketExistsSSH(ctx, bucket)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
	}
	host, err := sshHostName()
	if err != nil {
		return nil, err
	}
	out, err := ssh(ctx, host, nil, "find", remotePath(bucket, ""), "-type", "f", "-printf", "%s\\t%P\\n")
	if err != nil {
		return nil, err
	}
	var objects []Object
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		sizeText, key, found := strings.Cut(line, "\t")
		if !found || strings.Contains(key, ".tmp.") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil {
			return nil, err
		}
		objects = append(objects, Object{Key: key, Size: size})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func copySSH(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string, move bool) error {
	ok, err := bucketExistsSSH(ctx, dstBucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %s: %w", dstBucket, ErrNotFound)
	}
	ok, err = existsSSH(ctx, srcBucket, srcKey)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("s3://%s/%s: %w", srcBucket, srcKey, ErrNotFound)
	}
	host, err := sshHostName()
	if err != nil {
		return err
	}
	src := remotePath(srcBucket, srcKey)
	dest := remotePath(dstBucket, dstKey)
	if _, err := ssh(ctx, host, nil, "mkdir", "-p", "--", remoteDir(dest)); err != nil {
		return err
	}
	if move {
		_, err = ssh(ctx, host, nil, "mv", "-f", "--", src, dest)
		return err
	}
	_, err = ssh(ctx, host, nil, "cp", "-f", "--", src, dest)
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, "chmod", "600", dest)
	return err
}

func removeBucketSSH(ctx context.Context, bucket string) error {
	objects, err := listSSH(ctx, bucket, "")
	if err != nil {
		return err
	}
	if len(objects) > 0 {
		return fmt.Errorf("%w: %s", ErrNotEmpty, bucket)
	}
	host, err := sshHostName()
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, "rm", "-rf", "--", remotePath(bucket, ""))
	return err
}

func remoteDir(path string) string {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return path
	}
	return path[:i]
}

func parentDir(path string) string {
	dir := strings.TrimSpace(path)
	if i := strings.LastIndex(dir, "/"); i > 0 {
		return dir[:i]
	}
	return "."
}

func ssh(ctx context.Context, host string, stdin io.Reader, command ...string) (string, error) {
	cmd, err := sshCommand(ctx, host, command...)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("s3 %s: %s", host, sshText(stderr.String(), err))
	}
	return stdout.String(), nil
}

func sshStatus(ctx context.Context, host string, command ...string) error {
	cmd, err := sshCommand(ctx, host, command...)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func sshCommand(ctx context.Context, host string, command ...string) (*exec.Cmd, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=/tmp/nebula-ssh-%C",
		host, "--",
	}
	args = append(args, command...)
	return exec.CommandContext(ctx, "ssh", args...), nil
}

func sshText(stderr string, err error) string {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		return err.Error()
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func exitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}
