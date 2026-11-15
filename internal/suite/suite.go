package suite

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const Version = "0.1.0"

// Main runs nebula help and version. sql and s3 are dispatched by the command.
func Main(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		fmt.Fprint(out, paint(helpText, out))
		return 0
	}
	if args[0] == "version" && len(args) == 1 {
		fmt.Fprintln(out, Version)
		return 0
	}
	fmt.Fprintln(errw, "usage: nebula help")
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

const helpText = `nebula
Postgres on the SSH host, and object storage for this laptop

global actions:
  nebula help
    show this help
  nebula version
    print the installed version

features:
  talk to Postgres on the SSH host
  # sql <arguments>
  nebula sql help
  nebula sql create database app
  nebula sql app query "select 1"

  store, retrieve, and delete objects
  # s3 make bucket <name> | s3 put <path> s3://<bucket>/<key> | s3 get s3://<bucket>/<key> <path> | s3 list [s3://<bucket>] | s3 delete s3://<bucket>/<key>
  nebula s3 make bucket notes
  nebula s3 put ./today.txt s3://notes/today.txt
  nebula s3 get s3://notes/today.txt ./today.txt
  nebula s3 list s3://notes
  nebula s3 delete s3://notes/today.txt

  run a site on the SSH host
  # run <directory> | run list | run logs <name> | run rollback <name>
  nebula run ./site
  nebula run list

  publish a laptop port on a hostname
  # tunnel add <name> on <port> | tunnel list | tunnel screen
  nebula tunnel add notes on 8080
  nebula tunnel list
  nebula tunnel screen
`
