package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/engine"
)

type sshBox struct{ host string }

// OpenSSH talks to the same host as sql. It does not open a public port.
func OpenSSH() (Box, error) {
	host := engine.SSHHost()
	if !hostOK(host) {
		return nil, fmt.Errorf("SQLX_PG_SSH is not a host name")
	}
	return sshBox{host: host}, nil
}

func hostOK(host string) bool {
	if host == "" {
		return false
	}
	for _, r := range host {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}

func (b sshBox) Read(ctx context.Context, path string) (string, error) {
	if err := allowed(path); err != nil {
		return "", err
	}
	out, err := b.argv(ctx, nil, "cat", "--", path)
	if err != nil && strings.Contains(err.Error(), "No such file") {
		return "", os.ErrNotExist
	}
	return out, err
}

func (b sshBox) Write(ctx context.Context, path, body string) error {
	if err := allowed(path); err != nil {
		return err
	}
	dir := path[:strings.LastIndex(path, "/")]
	if _, err := b.argv(ctx, nil, "mkdir", "-p", "--", dir); err != nil {
		return err
	}
	if _, err := b.argv(ctx, strings.NewReader(body), "tee", path); err != nil {
		return err
	}
	_, err := b.argv(ctx, nil, "chmod", "600", "--", path)
	return err
}

func (b sshBox) Upload(ctx context.Context, local, remote string) error {
	if err := allowed(remote); err != nil {
		return err
	}
	if _, err := b.argv(ctx, nil, "mkdir", "-p", "--", remote); err != nil {
		return err
	}
	return streamTar(ctx, b.host, local, remote)
}

func (b sshBox) Output(ctx context.Context, script string) (string, error) {
	return b.argv(ctx, strings.NewReader(script), "bash", "-s")
}

func (b sshBox) argv(ctx context.Context, stdin io.Reader, command ...string) (string, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=/tmp/nebula-ssh-%C",
		b.host, "--",
	}
	args = append(args, command...)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("run %s: %s", b.host, short(stderr.String(), err))
	}
	return stdout.String(), nil
}

func streamTar(ctx context.Context, host, local, remote string) error {
	tar := exec.CommandContext(ctx, "tar", "-C", local, "-czf", "-", ".")
	sshCmd := exec.CommandContext(ctx, "ssh", sshArgs(host, "tar", "-xzf", "-", "-C", remote)...)
	pipe, err := tar.StdoutPipe()
	if err != nil {
		return err
	}
	sshCmd.Stdin = pipe
	var stderr bytes.Buffer
	tar.Stderr = &stderr
	sshCmd.Stderr = &stderr
	if err := sshCmd.Start(); err != nil {
		return err
	}
	if err := tar.Start(); err != nil {
		_ = sshCmd.Wait()
		return err
	}
	errTar := tar.Wait()
	errSSH := sshCmd.Wait()
	if errTar != nil || errSSH != nil {
		return fmt.Errorf("upload: %s", short(stderr.String(), errTar))
	}
	return nil
}

func sshArgs(host string, command ...string) []string {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=/tmp/nebula-ssh-%C",
		host, "--",
	}
	return append(args, command...)
}

func allowed(path string) error {
	if err := plainPath(path); err != nil {
		return err
	}
	switch {
	case strings.HasPrefix(path, "/var/lib/nebula/apps/"):
		return nil
	case strings.HasPrefix(path, "/etc/cloudflared/"):
		return nil
	case strings.HasPrefix(path, "/etc/systemd/system/"):
		return nil
	case strings.HasPrefix(path, "/usr/lib/systemd/system/"):
		return nil
	default:
		return fmt.Errorf("refusing path %s", path)
	}
}

func short(stderr string, err error) string {
	msg := strings.TrimSpace(stderr)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	if msg == "" {
		msg = "failed"
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
