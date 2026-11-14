package pg

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/orm"
)

const backupRoot = "/var/lib/postgres/sqlx-backups"

var dayRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

const ormTable = `CREATE TABLE IF NOT EXISTS sqlx_orm (
  model text PRIMARY KEY,
  body text NOT NULL,
  position integer NOT NULL
)`

func Exists(ctx context.Context, name string) (bool, error) {
	if err := appDB(name); err != nil {
		return false, err
	}
	out, err := scalar(ctx, "postgres", "SELECT datname FROM pg_database WHERE datname = "+orm.QuoteLiteral(name))
	if err != nil {
		return false, err
	}
	return out == name, nil
}

func createDB(ctx context.Context, name string) error {
	if err := appDB(name); err != nil {
		return err
	}
	return execRaw(ctx, "postgres", "CREATE DATABASE "+orm.QuoteIdent(name))
}

func Drop(ctx context.Context, name string) error {
	if err := appDB(name); err != nil {
		return err
	}
	return execRaw(ctx, "postgres", "DROP DATABASE IF EXISTS "+orm.QuoteIdent(name)+" WITH (FORCE)")
}

func Rename(ctx context.Context, oldName, newName string) error {
	if err := appDB(oldName); err != nil {
		return err
	}
	if err := appDB(newName); err != nil {
		return err
	}
	sql := "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = " +
		orm.QuoteLiteral(oldName) + " AND pid <> pg_backend_pid();\nALTER DATABASE " +
		orm.QuoteIdent(oldName) + " RENAME TO " + orm.QuoteIdent(newName)
	return execRaw(ctx, "postgres", sql)
}

func Names(ctx context.Context) ([]string, error) {
	out, err := psql(ctx, "postgres", "SELECT datname FROM pg_database WHERE datistemplate = false AND datname <> 'postgres' ORDER BY 1", []string{"-t", "-A"})
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !orm.ValidName(line) || reserved(line) {
			continue
		}
		names = append(names, line)
	}
	return names, nil
}

func Dump(ctx context.Context, day string, names []string) (string, error) {
	if !dayRE.MatchString(day) {
		return "", fmt.Errorf("backup day must be YYYY-MM-DD")
	}
	for _, name := range names {
		if err := appDB(name); err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString("set -eu\n")
	fmt.Fprintf(&b, "root=%s\n", backupRoot)
	fmt.Fprintf(&b, "final=\"$root/%s\"\n", day)
	b.WriteString("tmp=\"$root/.partial\"\n")
	b.WriteString("rm -rf \"$tmp\"\n")
	b.WriteString("mkdir -p \"$tmp\"\n")
	b.WriteString("chown postgres:postgres \"$root\" \"$tmp\"\n")
	b.WriteString("chmod 700 \"$root\" \"$tmp\"\n")
	for _, name := range names {
		fmt.Fprintf(&b, "runuser -u postgres -- pg_dump -Fc --file=\"$tmp/%s.dump\" --dbname=%s\n", name, name)
	}
	b.WriteString("replaced=\"$final.replaced\"\n")
	b.WriteString("rm -rf \"$replaced\"\n")
	b.WriteString("if [ -d \"$final\" ]; then mv \"$final\" \"$replaced\"; fi\n")
	b.WriteString("mv \"$tmp\" \"$final\"\n")
	b.WriteString("rm -rf \"$replaced\"\n")
	b.WriteString("chmod 700 \"$final\"\n")
	if _, err := sshBash(ctx, b.String()); err != nil {
		return "", err
	}
	return backupRoot + "/" + day, nil
}

func BackupDays(ctx context.Context) ([]string, error) {
	out, err := sshBash(ctx, "if [ -d "+backupRoot+" ]; then ls -1 "+backupRoot+"; fi\n")
	if err != nil {
		return nil, err
	}
	var days []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if dayRE.MatchString(line) {
			days = append(days, line)
		}
	}
	return days, nil
}

func RemoveBackup(ctx context.Context, name string) error {
	if !dayRE.MatchString(name) {
		return fmt.Errorf("backup name refused")
	}
	_, err := sshBash(ctx, "rm -rf "+backupRoot+"/"+name+"\n")
	return err
}
