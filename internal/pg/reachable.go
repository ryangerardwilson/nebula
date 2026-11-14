package pg

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type unreachableError struct {
	host string
}

func (e unreachableError) Error() string {
	return "sql backup " + e.host + " is unreachable"
}

// Unreachable reports that the backup host did not accept a connection.
func Unreachable(err error) bool {
	return errors.As(err, new(unreachableError))
}

func ensureReachable(alias string) error {
	if backupDown() {
		return unreachableError{host: alias}
	}
	endpoint, err := sshEndpoint(alias)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", endpoint, 1200*time.Millisecond)
	if err != nil {
		markBackupDown()
		return unreachableError{host: alias}
	}
	_ = conn.Close()
	clearBackupDown()
	return nil
}

func backupDown() bool {
	info, err := os.Stat(downStampPath())
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < 45*time.Second
}

func markBackupDown() {
	_ = os.WriteFile(downStampPath(), []byte("down\n"), 0o600)
}

func clearBackupDown() {
	_ = os.Remove(downStampPath())
}

func downStampPath() string {
	return filepath.Join(os.TempDir(), "sqlx-backup-down-"+strconv.Itoa(os.Getuid()))
}

var sshEndpoints = map[string]string{}

func sshEndpoint(alias string) (string, error) {
	if endpoint, ok := sshEndpoints[alias]; ok {
		return endpoint, nil
	}
	out, err := exec.Command("ssh", "-G", alias).Output()
	if err != nil {
		return "", fmt.Errorf("ssh config for %s: %w", alias, err)
	}
	host := ""
	port := "22"
	for _, line := range strings.Split(string(out), "\n") {
		key, value, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		switch strings.ToLower(key) {
		case "hostname":
			host = value
		case "port":
			port = value
		}
	}
	if host == "" {
		host = alias
	}
	endpoint := net.JoinHostPort(host, port)
	sshEndpoints[alias] = endpoint
	return endpoint, nil
}
