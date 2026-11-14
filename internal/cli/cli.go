package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ryangerardwilson/nebula/internal/agent"
	"github.com/ryangerardwilson/nebula/internal/backup"
	"github.com/ryangerardwilson/nebula/internal/cache"
	"github.com/ryangerardwilson/nebula/internal/engine"
	"github.com/ryangerardwilson/nebula/internal/layout"
	"github.com/ryangerardwilson/nebula/s3x"
	"golang.org/x/term"
)

const Version = "0.1.0"

type App struct {
	Out       io.Writer
	Err       io.Writer
	Query     func(db, sql string) error
	QueryJSON func(db, sql string) error
	Exec      func(db, sql string) error
	Columns   func(db, table string) (string, error)
	Once      func(db, id, sql string) error
	Inspect   func(db string) (string, error)
	Schema    func(db string) (string, error)
	Migrate   func(db, path string) error
	Create    func(name string) error
	Rename    func(oldName, newName string) error
	Drop      func(name string) error
	List      func() (string, error)
	Backup    func() error
	Status    func() (string, error)
	Logs      func() error
}

func Main(args []string) int {
	return App{Out: os.Stdout, Err: os.Stderr}.Run(args)
}

func (a App) Run(args []string) int {
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		fmt.Fprint(a.Out, paint(helpText, a.Out))
		return 0
	}
	switch args[0] {
	case "version", "status", "logs", "backup", "list", "create", "rename", "drop":
		return a.runAdmin(args)
	}
	return a.runDatabase(args)
}

func (a App) runAdmin(args []string) int {
	switch args[0] {
	case "version":
		if len(args) != 1 {
			return a.usage("usage: sql version")
		}
		fmt.Fprintln(a.Out, Version)
		return 0
	case "status":
		if len(args) != 1 {
			return a.usage("usage: sql status")
		}
		text, err := a.status()
		if err != nil {
			return a.op(err)
		}
		fmt.Fprint(a.Out, text)
		return 0
	case "logs":
		if len(args) != 1 {
			return a.usage("usage: sql logs")
		}
		return a.op(a.logs())
	case "backup":
		if len(args) != 1 {
			return a.usage("usage: sql backup")
		}
		return a.op(a.backup())
	case "list":
		if len(args) != 2 || args[1] != "databases" {
			return a.usage("usage: sql list databases")
		}
		text, err := a.list()
		if err != nil {
			return a.op(err)
		}
		fmt.Fprint(a.Out, text)
		if text != "" && !strings.HasSuffix(text, "\n") {
			fmt.Fprintln(a.Out)
		}
		return 0
	case "create":
		if len(args) != 3 || args[1] != "database" {
			return a.usage("usage: sql create database <name>")
		}
		if err := a.create(args[2]); err != nil {
			return a.op(err)
		}
		fmt.Fprintf(a.Out, "database %s\n", args[2])
		return 0
	case "rename":
		if len(args) != 4 || args[1] != "database" {
			return a.usage("usage: sql rename database <old> <new>")
		}
		return a.op(a.rename(args[2], args[3]))
	case "drop":
		if len(args) != 3 || args[1] != "database" {
			return a.usage("usage: sql drop database <name>")
		}
		return a.op(a.drop(args[2]))
	}
	return a.usage("usage: sql help")
}

func (a App) runDatabase(args []string) int {
	if len(args) < 2 {
		return a.usage("usage: sql help")
	}
	switch args[1] {
	case "query":
		return a.runQuery(args)
	case "exec":
		if len(args) < 3 {
			return a.usage("usage: sql <database> exec <sql>")
		}
		sql, err := statementSQL(args[2:], "sql <database> exec --file <path>")
		if err != nil {
			return a.op(err)
		}
		return a.op(a.exec(args[0], sql))
	case "columns":
		if len(args) != 3 {
			return a.usage("usage: sql <database> columns <table>")
		}
		text, err := a.columns(args[0], args[2])
		if err != nil {
			return a.op(err)
		}
		fmt.Fprint(a.Out, text)
		return 0
	case "once":
		if len(args) < 4 {
			return a.usage("usage: sql <database> once <id> <sql>")
		}
		sql, err := onceSQL(args[3:])
		if err != nil {
			return a.op(err)
		}
		return a.op(a.once(args[0], args[2], sql))
	case "inspect":
		if len(args) != 2 {
			return a.usage("usage: sql <database> inspect")
		}
		text, err := a.inspect(args[0])
		if err != nil {
			return a.op(err)
		}
		fmt.Fprint(a.Out, text)
		return 0
	case "schema":
		if len(args) != 2 {
			return a.usage("usage: sql <database> schema")
		}
		text, err := a.schema(args[0])
		if err != nil {
			return a.op(err)
		}
		fmt.Fprint(a.Out, text)
		return 0
	case "migrate":
		if len(args) != 3 {
			return a.usage("usage: sql <database> migrate <path>")
		}
		return a.op(a.migrate(args[0], args[2]))
	}
	return a.usage("usage: sql help")
}

func (a App) runQuery(args []string) int {
	if len(args) >= 3 && args[2] == "--json" {
		if len(args) < 4 {
			return a.usage("usage: sql <database> query --json <sql>")
		}
		sql, err := statementSQL(args[3:], "sql <database> query --json --file <path>")
		if err != nil {
			return a.op(err)
		}
		return a.op(a.queryJSON(args[0], sql))
	}
	if len(args) < 3 {
		return a.usage("usage: sql <database> query <sql>")
	}
	sql, err := statementSQL(args[2:], "sql <database> query --file <path>")
	if err != nil {
		return a.op(err)
	}
	return a.op(a.query(args[0], sql))
}

func (a App) query(db, sql string) error {
	if a.Query != nil {
		return a.Query(db, sql)
	}
	return agent.Query(context.Background(), db, sql, a.Out)
}

func (a App) queryJSON(db, sql string) error {
	if a.QueryJSON != nil {
		return a.QueryJSON(db, sql)
	}
	return agent.QueryJSON(context.Background(), db, sql, a.Out)
}

func (a App) exec(db, sql string) error {
	if a.Exec != nil {
		return a.Exec(db, sql)
	}
	return agent.Exec(context.Background(), db, sql)
}

func (a App) columns(db, table string) (string, error) {
	if a.Columns != nil {
		return a.Columns(db, table)
	}
	return agent.Columns(context.Background(), db, table)
}

func (a App) once(db, id, sql string) error {
	if a.Once != nil {
		return a.Once(db, id, sql)
	}
	return agent.Once(context.Background(), db, id, sql, a.Out)
}

func statementSQL(args []string, usage string) (string, error) {
	if len(args) >= 1 && args[0] == "--file" {
		if len(args) != 2 {
			return "", fmt.Errorf("%w: %s", agent.ErrUsage, usage)
		}
		body, err := os.ReadFile(args[1])
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return strings.Join(args, " "), nil
}

func onceSQL(args []string) (string, error) {
	if len(args) >= 1 && args[0] == "--file" {
		if len(args) != 2 {
			return "", fmt.Errorf("%w: sql <database> once <id> --file <path>", agent.ErrUsage)
		}
		body, err := os.ReadFile(args[1])
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
	return strings.Join(args, " "), nil
}

func (a App) inspect(db string) (string, error) {
	if a.Inspect != nil {
		return a.Inspect(db)
	}
	return agent.Inspect(context.Background(), db)
}

func (a App) schema(db string) (string, error) {
	if a.Schema != nil {
		return a.Schema(db)
	}
	return agent.Schema(context.Background(), db)
}

func (a App) migrate(db, path string) error {
	if a.Migrate != nil {
		return a.Migrate(db, path)
	}
	return agent.Migrate(context.Background(), db, path, a.Out)
}

func (a App) create(name string) error {
	if a.Create != nil {
		return a.Create(name)
	}
	return agent.CreateDatabase(context.Background(), name)
}

func (a App) rename(oldName, newName string) error {
	if a.Rename != nil {
		return a.Rename(oldName, newName)
	}
	return agent.RenameDatabase(oldName, newName)
}

func (a App) drop(name string) error {
	if a.Drop != nil {
		return a.Drop(name)
	}
	return agent.DropDatabase(name)
}

func (a App) list() (string, error) {
	if a.List != nil {
		return a.List()
	}
	return agent.ListDatabases()
}

func (a App) backup() error {
	if a.Backup != nil {
		return a.Backup()
	}
	return backup.Run(context.Background(), a.Out)
}

func (a App) status() (string, error) {
	if a.Status != nil {
		return a.Status()
	}
	names, err := agent.Names()
	if err != nil {
		return "", err
	}
	timer := "inactive"
	cmd := exec.Command("systemctl", "--user", "is-active", "sqlx-backup.timer")
	out, err := cmd.Output()
	if strings.TrimSpace(string(out)) == "active" {
		timer = "active"
	} else if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", err
		}
	}
	if engine.Object() {
		return fmt.Sprintf("host %s\nstore s3://%s\ncache %s\ndatabases %d\nbackup timer %s\n", engine.SSHHost(), s3x.DatabaseBucket, cache.Dir(), len(names), timer), nil
	}
	if engine.Postgres() {
		return fmt.Sprintf("host %s\ncache %s\ndatabases %d\nbackup timer %s\n", engine.SSHHost(), cache.Dir(), len(names), timer), nil
	}
	return fmt.Sprintf("data %s\ndatabases %d\nbackup timer %s\n", layout.DataDir(), len(names), timer), nil
}

func (a App) logs() error {
	if a.Logs != nil {
		return a.Logs()
	}
	cmd := exec.Command("journalctl", "--user", "-u", "sqlx-backup.service", "-n", "80", "--no-pager")
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}

func (a App) op(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(a.Err, err.Error())
	if errors.Is(err, agent.ErrUsage) {
		return 2
	}
	return 1
}

func (a App) usage(msg string) int {
	fmt.Fprintln(a.Err, msg)
	return 2
}

func paint(text string, w io.Writer) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	file, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return text
	}
	return "\033[38;5;245m" + text + "\033[0m"
}

const helpText = `sql
keep a memory sqlite database for each app and back that file up with s3

global actions:
  sql help
    show this help
  sql version
    print the installed version
  sql status
    show where the databases are and the backup timer
  sql logs
    show the recent backup log

features:
  query and inspect one database through its ORM
  # <database> query [--json] <sql> | <database> exec <sql> | <database> columns <table> | <database> inspect | <database> schema
  sql app query "select 1"
  sql app query --json "select 1"
  sql app exec "delete from item where id = 1"
  sql app exec --file ~/note.sql
  sql app query --json --file ~/note.sql
  sql app columns item
  sql app inspect
  sql app schema

  declare the ORM and migrate it into the database
  # <database> migrate <path>
  # model <name>
  #   <column> <type> [primary key] [not null] [default <literal>]
  #   index <name> [unique] <column>...
  sql app migrate ~/app/models

  create, rename, drop, and list databases
  # create database <name> | rename database <old> <new> | drop database <name> | list databases
  sql create database app
  sql rename database old app
  sql drop database app
  sql list databases

  run a one-shot script once
  # <database> once <id> <sql>
  sql app once backfill "update item set name = 'a'"

  snapshot each database and keep 10 days
  # backup
  sql backup
`
