package s3x

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// DatabaseBucket holds one sqlite snapshot per application database.
	DatabaseBucket = "sqlx"
	// BackupBucket holds dated copies of those snapshots.
	BackupBucket = "sqlx-backups"
	// RemoteRoot is the object store on the SSH host. It is not a public port.
	RemoteRoot = "/var/lib/nebula/s3x"
)

var (
	bucketRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	keyRE    = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	hostRE   = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)
)

func checkBucket(name string) error {
	if !bucketRE.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("%w: bucket name must be a lowercase s3 name", ErrUsage)
	}
	return nil
}

func checkKey(key string) error {
	if key == "" || len(key) > 1024 || strings.Contains(key, "\\") {
		return fmt.Errorf("%w: object key must be a relative path", ErrUsage)
	}
	for _, part := range strings.Split(key, "/") {
		if !keyRE.MatchString(part) {
			return fmt.Errorf("%w: object key must be a relative path", ErrUsage)
		}
	}
	return nil
}

func checkHost(host string) error {
	if !hostRE.MatchString(host) {
		return fmt.Errorf("S3X_SSH is not a host name")
	}
	return nil
}
