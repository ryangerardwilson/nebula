package records

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func get(db, app, kind, key string) ([]byte, bool, error) {
	var rows []struct {
		Body string `json:"body"`
	}
	sql := "SELECT body FROM record WHERE app = " + quote(app) + " AND kind = " + quote(kind) + " AND item_key = " + quote(key) + " ORDER BY id LIMIT 1"
	if err := queryJSON(db, sql, &rows); err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return []byte(rows[0].Body), true, nil
}

// put writes one record. An existing record keeps its id.
func put(db, app, kind, key string, body []byte) error {
	where := " WHERE app = " + quote(app) + " AND kind = " + quote(kind) + " AND item_key = " + quote(key)
	sql := "BEGIN;\nUPDATE record SET body = " + quote(string(body)) + where + ";\n" +
		"INSERT INTO record(app, kind, item_key, body) SELECT " + quote(app) + ", " + quote(kind) + ", " + quote(key) + ", " + quote(string(body)) + " WHERE changes() = 0;\nCOMMIT;\n"
	return execSQL(db, sql)
}

func meta(db, key string) (string, bool, error) {
	var rows []struct {
		Value *string `json:"value"`
	}
	if err := queryJSON(db, "SELECT value FROM meta WHERE key = "+quote(key)+" LIMIT 1", &rows); err != nil {
		return "", false, err
	}
	if len(rows) == 0 || rows[0].Value == nil {
		return "", len(rows) > 0, nil
	}
	return *rows[0].Value, true, nil
}

func setMeta(db, key, value string) error {
	sql := "BEGIN;\nDELETE FROM meta WHERE key = " + quote(key) + ";\n" +
		"INSERT INTO meta(key, value) VALUES (" + quote(key) + ", " + quote(value) + ");\nCOMMIT;\n"
	return execSQL(db, sql)
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func queryJSON(db, sql string, dest any) error {
	raw, err := sqliteRun(db, sql, true)
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("sqlite json: %w", err)
	}
	return nil
}

func execSQL(db, sql string) error {
	if err := sqliteExec(db, sql); err != nil {
		return err
	}
	publishObject(db)
	return nil
}

func sqliteExec(db, sql string) error {
	_, err := sqliteRun(db, sql, false)
	return err
}

func sqliteRun(db, sql string, asJSON bool) (string, error) {
	bin := os.Getenv("SQLX_SQLITE")
	if bin == "" {
		bin = "/usr/bin/sqlite3"
	}
	args := []string{"-batch", "-cmd", ".timeout 5000"}
	if asJSON {
		args = append(args, "-json")
	}
	args = append(args, db)
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(sql)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func restoreObject(path string) error {
	if os.Getenv("SQLX_ROOT") != "" {
		return nil
	}
	bin, args, err := s3Args("get", "s3://backups/"+databaseName+".sqlite", path)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, bin, args...).Run()
}

func publishObject(path string) {
	if os.Getenv("SQLX_ROOT") != "" || path == "" {
		return
	}
	bin, args, err := s3Args("put", path, "s3://backups/"+databaseName+".sqlite")
	if err != nil {
		return
	}
	go runS3(bin, args)
}

func runS3(bin string, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, bin, args...).Run()
}

func s3Args(action string, rest ...string) (string, []string, error) {
	if bin, err := exec.LookPath("s3"); err == nil {
		return bin, append([]string{action}, rest...), nil
	}
	bin, err := exec.LookPath("nebula")
	if err != nil {
		return "", nil, err
	}
	return bin, append([]string{"s3", action}, rest...), nil
}
