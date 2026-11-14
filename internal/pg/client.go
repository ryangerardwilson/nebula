package pg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/engine"
	"github.com/ryangerardwilson/nebula/internal/orm"
)

var hostRE = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)

func Run(ctx context.Context, db, sql string, flags ...string) (string, error) {
	jsonMode := false
	csvMode := false
	for _, flag := range flags {
		switch flag {
		case "-json":
			jsonMode = true
		case "-csv":
			csvMode = true
		}
	}
	if jsonMode {
		return QueryJSON(ctx, db, sql)
	}
	if csvMode {
		return QueryCSV(ctx, db, sql)
	}
	if err := Exec(ctx, db, sql); err != nil {
		return "", err
	}
	return "", nil
}

func Exec(ctx context.Context, db, sql string) error {
	rewritten, err := Rewrite(sql)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rewritten) == "" {
		return nil
	}
	return execRaw(ctx, db, rewritten)
}

func QueryCSV(ctx context.Context, db, sql string) (string, error) {
	rewritten, err := Rewrite(sql)
	if err != nil {
		return "", err
	}
	parts, err := statementsOf(rewritten)
	if err != nil {
		return "", err
	}
	if len(parts) == 1 && !isQuery(parts[0]) {
		return "", Exec(ctx, db, rewritten)
	}
	out, err := psql(ctx, db, rewritten, []string{"--csv"})
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(out, "\r\n", "\n"), nil
}

func QueryJSON(ctx context.Context, db, sql string) (string, error) {
	script, execScript, err := jsonScript(sql)
	if err != nil {
		return "", err
	}
	if execScript {
		return "", Exec(ctx, db, sql)
	}
	if script == "" {
		return "", nil
	}
	out, err := psql(ctx, db, script, []string{"-t", "-A"})
	if err != nil {
		return "", err
	}
	return compactJSON(pickJSON(out))
}

func jsonScript(sql string) (string, bool, error) {
	rewritten, err := Rewrite(sql)
	if err != nil {
		return "", false, err
	}
	parts, err := statementsOf(rewritten)
	if err != nil {
		return "", false, err
	}
	if len(parts) == 0 {
		return "", false, nil
	}
	if len(parts) == 1 && !isQuery(parts[0]) {
		return "", true, nil
	}
	if len(parts) == 1 {
		return jsonText(parts[0]), false, nil
	}
	if !strings.Contains(rewritten, "sqlx_updated") {
		return "", false, fmt.Errorf("query needs one select")
	}
	script, err := jsonParts(parts)
	return script, false, err
}

func jsonParts(parts []string) (string, error) {
	result := -1
	for i, part := range parts {
		if isLockSelect(part) || isSide(part) {
			continue
		}
		if !isResult(part) || result >= 0 {
			return "", fmt.Errorf("query needs one select")
		}
		result = i
	}
	if result < 0 {
		return "", fmt.Errorf("query needs one select")
	}
	parts[result] = jsonText(parts[result])
	return strings.Join(parts, ";\n") + ";\n", nil
}

func isResult(sql string) bool {
	_, tail, ok := cutTail(sql)
	if ok {
		return isQuery(tail)
	}
	return isQuery(sql) && !isLockSelect(sql)
}

func isSide(sql string) bool {
	switch firstWord(sql) {
	case "begin", "commit", "rollback", "end", "savepoint", "release", "insert", "update", "delete":
		return true
	default:
		return false
	}
}

func isLockSelect(sql string) bool {
	return firstWord(sql) == "select" && strings.Contains(sql, "pg_advisory_xact_lock")
}

func jsonText(sql string) string {
	cte, tail, ok := cutTail(sql)
	if !ok {
		return wrapJSON(sql)
	}
	return cte + "\nSELECT coalesce((SELECT json_agg(v.j ORDER BY v.n) FROM (\n" +
		"SELECT row_to_json(q) AS j, row_number() OVER () AS n FROM (" +
		tail + ") q\n) v), '[]'::json)::text"
}

func cutTail(sql string) (string, string, bool) {
	const mark = " AS sqlx_changed\n)\n"
	i := strings.Index(sql, mark)
	if i < 0 {
		return "", "", false
	}
	return sql[:i+len(" AS sqlx_changed\n)")], strings.TrimSpace(sql[i+len(mark):]), true
}

func pickJSON(out string) string {
	var kept string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") || strings.HasPrefix(line, "{") {
			kept = line
		}
	}
	return kept
}

func compactJSON(out string) (string, error) {
	out = strings.TrimSpace(out)
	if out == "" || out == "[]" {
		return "", nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(out)); err != nil {
		return "", fmt.Errorf("postgres json: %w", err)
	}
	if compact.String() == "[]" {
		return "", nil
	}
	return compact.String(), nil
}

func wrapJSON(sql string) string {
	return "SELECT coalesce((SELECT json_agg(v.j ORDER BY v.n) FROM (\n" +
		"SELECT row_to_json(q) AS j, row_number() OVER () AS n FROM (" +
		strings.TrimSpace(sql) + ") q\n) v), '[]'::json)::text"
}

func statementsOf(sql string) ([]string, error) {
	if strings.TrimSpace(sql) == "" {
		return nil, nil
	}
	parts, err := orm.SplitSQL(sql)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

func isQuery(sql string) bool {
	switch firstWord(sql) {
	case "select", "with", "values", "table":
		return true
	default:
		return false
	}
}

func scalar(ctx context.Context, db, sql string) (string, error) {
	out, err := psql(ctx, db, sql, []string{"-t", "-A"})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func execRaw(ctx context.Context, db, sql string) error {
	_, err := psql(ctx, db, sql, nil)
	return err
}

func psql(ctx context.Context, db, sql string, extra []string) (string, error) {
	if !orm.ValidName(db) {
		return "", fmt.Errorf("database name must be letters, digits, and underscores")
	}
	host, err := sshHost()
	if err != nil {
		return "", err
	}
	if err := ensureReachable(host); err != nil {
		return "", err
	}
	if !strings.HasSuffix(sql, "\n") {
		sql += "\n"
	}
	// SSH joins the remote command for a shell, so a spaced PGOPTIONS value would split.
	sql = "SET client_min_messages TO warning;\n" + sql
	args := append(sshBase(host),
		"runuser", "-u", "postgres", "--",
		"psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "--no-psqlrc", "-P", "pager=off",
		"-d", db,
	)
	args = append(args, extra...)
	return runSSH(ctx, args, sql)
}

func sshBash(ctx context.Context, script string) (string, error) {
	host, err := sshHost()
	if err != nil {
		return "", err
	}
	if err := ensureReachable(host); err != nil {
		return "", err
	}
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	return runSSH(ctx, append(sshBase(host), "bash", "-s"), script)
}

func sshBase(host string) []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ControlMaster=auto",
		"-o", "ControlPersist=600",
		"-o", "ControlPath=/tmp/sqlx-ssh-%C",
		host, "--",
	}
}

func sshHost() (string, error) {
	host := engine.SSHHost()
	if !hostRE.MatchString(host) {
		return "", fmt.Errorf("SQLX_PG_SSH is not a host name")
	}
	return host, nil
}

func runSSH(ctx context.Context, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), commandError(stderr.String(), err)
	}
	return stdout.String(), nil
}

func commandError(stderr string, err error) error {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ERROR:") || strings.HasPrefix(line, "FATAL:") || strings.HasPrefix(line, "PANIC:") {
			if len(line) > 300 {
				line = line[:300]
			}
			lines = append(lines, line)
		}
	}
	if len(lines) > 0 {
		return fmt.Errorf("%s", strings.Join(lines, "; "))
	}
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = err.Error()
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return fmt.Errorf("%s", msg)
}

func appDB(name string) error {
	if !orm.ValidName(name) {
		return fmt.Errorf("database name must be letters, digits, and underscores")
	}
	if reserved(name) {
		return fmt.Errorf("database %s is reserved", name)
	}
	return nil
}

func reserved(name string) bool {
	switch name {
	case "postgres", "template0", "template1":
		return true
	default:
		return false
	}
}
