package s3

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

// Each SSH call below sends one shell script, so one operation costs one
// round trip to the host. A script exits missingExit when the bucket or the
// object is absent; that maps to ErrNotFound.
const missingExit = 3

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
	return rootPath(RemoteRoot, bucket, key)
}

func rootPath(root, bucket, key string) string {
	path := root + "/" + bucket
	if key != "" {
		path += "/" + key
	}
	return path
}

// shq quotes one word for the remote login shell. Bucket and key names are
// already restricted, so this is a guard, not an escape hatch.
func shq(word string) string {
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// bucketTest succeeds when the bucket directory exists.
func bucketTest(bucket string) string {
	return "test -d " + shq(remotePath(bucket, ""))
}

// needBucket exits missingExit unless the bucket exists.
func needBucket(bucket string) string {
	return "{ " + bucketTest(bucket) + " || exit " + strconv.Itoa(missingExit) + "; }"
}

// pickObject sets src to the object's path and exits missingExit when it is absent.
func pickObject(bucket, key string) string {
	return "if test -f " + shq(remotePath(bucket, key)) + "; then src=" + shq(remotePath(bucket, key)) +
		"; else exit " + strconv.Itoa(missingExit) + "; fi"
}

// prepare creates the new root, the bucket, and dir with mode 700.
func prepare(bucket, dir string) string {
	return "umask 077 && mkdir -p " + shq(dir) + " && chmod 700 " + shq(RemoteRoot) + " " + shq(remotePath(bucket, ""))
}

func makeBucketSSH(ctx context.Context, bucket string) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	_, err = ssh(ctx, host, nil, prepare(bucket, remotePath(bucket, "")))
	return err
}

func bucketExistsSSH(ctx context.Context, bucket string) (bool, error) {
	host, err := sshHostName()
	if err != nil {
		return false, err
	}
	err = sshStatus(ctx, host, bucketTest(bucket))
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
	script := "if test -d " + shq(RemoteRoot) + "; then find " + shq(RemoteRoot) + " -mindepth 1 -maxdepth 1 -type d -printf '%f\\n'; fi"
	out, err := ssh(ctx, host, nil, script)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !seen[line] && checkBucket(line) == nil {
			seen[line] = true
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names, nil
}

// putSSH checks the bucket, writes a temporary file, and renames it into
// place in one remote command. The body is stdin.
func putSSH(ctx context.Context, local, bucket, key string) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	file, err := os.Open(local)
	if err != nil {
		return err
	}
	defer file.Close()
	dest := remotePath(bucket, key)
	tmp := dest + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 36)
	script := needBucket(bucket) + " && " + prepare(bucket, remoteDir(dest)) +
		" && cat > " + shq(tmp) + " && chmod 600 " + shq(tmp) + " && mv -f -- " + shq(tmp) + " " + shq(dest)
	_, err = ssh(ctx, host, file, script)
	if exitCode(err) == missingExit {
		return fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
	}
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
	cmd, err := sshCommand(ctx, host, pickObject(bucket, key)+" && cat -- \"$src\"")
	if err != nil {
		_ = tmp.Close()
		return err
	}
	var stderr bytes.Buffer
	cmd.Stdout = tmp
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = tmp.Close()
		if exitCode(err) == missingExit {
			return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
		}
		return fmt.Errorf("s3 %s: %s", host, sshText(stderr.String(), err))
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

// deleteSSH removes the object.
func deleteSSH(ctx context.Context, bucket, key string) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	script := pickObject(bucket, key) + " && rm -f -- " + shq(remotePath(bucket, key))
	_, err = ssh(ctx, host, nil, script)
	if exitCode(err) == missingExit {
		return fmt.Errorf("s3://%s/%s: %w", bucket, key, ErrNotFound)
	}
	return err
}

func existsSSH(ctx context.Context, bucket, key string) (bool, error) {
	host, err := sshHostName()
	if err != nil {
		return false, err
	}
	err = sshStatus(ctx, host, "test -f "+shq(remotePath(bucket, key)))
	if err == nil {
		return true, nil
	}
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, err
}

// listSSH lists one bucket in one command. The format is single-quoted so
// the remote shell keeps its escapes.
func listSSH(ctx context.Context, bucket, prefix string) ([]Object, error) {
	host, err := sshHostName()
	if err != nil {
		return nil, err
	}
	script := needBucket(bucket) +
		"; if test -d " + shq(remotePath(bucket, "")) + "; then find " + shq(remotePath(bucket, "")) + " -type f -printf '%s %P\\n'; fi"
	out, err := ssh(ctx, host, nil, script)
	if exitCode(err) == missingExit {
		return nil, fmt.Errorf("bucket %s: %w", bucket, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return parseList(out, prefix)
}

func parseList(out, prefix string) ([]Object, error) {
	found := map[string]Object{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), " ", 2)
		if len(fields) != 2 {
			continue
		}
		sizeText, key := fields[0], fields[1]
		if strings.Contains(key, ".tmp.") || checkKey(key) != nil {
			continue
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		size, err := strconv.ParseInt(sizeText, 10, 64)
		if err != nil {
			return nil, err
		}
		found[key] = Object{Key: key, Size: size}
	}
	objects := make([]Object, 0, len(found))
	for _, object := range found {
		objects = append(objects, object)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

// copySSH copies or moves an object in one command.
func copySSH(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string, move bool) error {
	host, err := sshHostName()
	if err != nil {
		return err
	}
	dest := remotePath(dstBucket, dstKey)
	script := needBucket(dstBucket) + " && " + pickObject(srcBucket, srcKey) + " && " + prepare(dstBucket, remoteDir(dest))
	if move {
		script += " && mv -f -- \"$src\" " + shq(dest)
	} else {
		script += " && cp -f -- \"$src\" " + shq(dest)
	}
	script += " && chmod 600 " + shq(dest)
	_, err = ssh(ctx, host, nil, script)
	if exitCode(err) == missingExit {
		return fmt.Errorf("s3://%s/%s or bucket %s: %w", srcBucket, srcKey, dstBucket, ErrNotFound)
	}
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
	_, err = ssh(ctx, host, nil, "rm -rf -- "+shq(remotePath(bucket, "")))
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

// ssh runs one script on the host. The returned error keeps the exit status,
// so a caller can tell missingExit from a failed connection.
func ssh(ctx context.Context, host string, stdin io.Reader, script string) (string, error) {
	cmd, err := sshCommand(ctx, host, script)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), &sshError{host: host, text: sshText(stderr.String(), err), err: err}
	}
	return stdout.String(), nil
}

// sshError is a failed remote command. Unwrap keeps the exit status.
type sshError struct {
	host string
	text string
	err  error
}

func (e *sshError) Error() string { return "s3 " + e.host + ": " + e.text }

func (e *sshError) Unwrap() error { return e.err }

func sshStatus(ctx context.Context, host, script string) error {
	cmd, err := sshCommand(ctx, host, script)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd.Run()
}

func sshCommand(ctx context.Context, host, script string) (*exec.Cmd, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=/tmp/nebula-ssh-%C",
		host, "--", script,
	}
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
