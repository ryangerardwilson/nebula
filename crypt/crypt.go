// Package crypt is nebula crypt, the 1Password interface.
package crypt

import (
	"fmt"
	"io"
	"os"

	"github.com/ryangerardwilson/nebula/crypt/internal/cli"
	"github.com/ryangerardwilson/nebula/crypt/internal/clip"
	"github.com/ryangerardwilson/nebula/crypt/internal/op"
	"github.com/ryangerardwilson/nebula/internal/suite"
)

// Help is the full help for nebula crypt.
const Help = `nebula crypt
1Password items for this laptop

global actions:
  nebula crypt help
    show this help
  nebula crypt version
    print the nebula version

  list items in a vault
  # crypt list <vault>
  nebula crypt list Root

  copy one secret, or several
  # crypt get <vault> <name>
  nebula crypt get Root GOOGLE_OAUTH_CLIENT_ID

  show title, id, category, and field labels
  # crypt describe <vault> <name>
  nebula crypt describe Root GOOGLE_OAUTH_CLIENT_SECRET

  create an API credential or a login
  # crypt create <vault> <name> | crypt create <vault> <name> login <username> | crypt create <vault> template <file>
  nebula crypt create Root GH_EXAMPLE

  replace a secret, rename it, or set its username
  # crypt put <vault> <name> | crypt put <vault> <name> name <title> | crypt put <vault> <name> username <user>
  nebula crypt put Root GH_EXAMPLE

  delete one exact title, or one item id
  # crypt delete <vault> <name> | crypt delete <vault> item <id>
  nebula crypt delete Root GH_EXAMPLE

  list vaults, list item ids, or copy one item by id
  # crypt vaults | crypt items <vault> | crypt copy <vault> <id>
  nebula crypt vaults
  nebula crypt items Root
  nebula crypt copy Root ITEM

One match is copied with wl-copy and is not printed. list prints titles and
usernames, never passwords. items and vaults print no concealed value.
copy copies one item and prints nothing. A secret is read from stdin.
The token is OP_SERVICE_ACCOUNT_TOKEN, or the file named by
OP_SERVICE_ACCOUNT_TOKEN_FILE, or ~/.config/c3po/pass/service-account-token.
c3po pass runs these same argument forms. c3po tui pass reaches 1Password
through vaults, items, copy, and delete.
`

const usage = "Use: nebula crypt list <vault> | nebula crypt get <vault> <name> | nebula crypt describe <vault> <name> | nebula crypt create <vault> <name> | nebula crypt put <vault> <name> | nebula crypt delete <vault> <name> | nebula crypt vaults | nebula crypt items <vault> | nebula crypt copy <vault> <id>"

func realRunner(out, errw io.Writer) cli.Runner {
	return cli.Runner{
		Out:     out,
		Err:     errw,
		In:      os.Stdin,
		Op:      op.Exec{},
		Clip:    clip.WLCopy{},
		Help:    Help,
		Version: suite.Version,
	}
}

// runnerFactory builds the 1Password runner. Tests replace it.
var runnerFactory = realRunner

// Run runs nebula crypt. args follow "nebula crypt".
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeHelp(stdout)
	}
	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return fail(stderr, "Use help by itself.")
		}
		return writeHelp(stdout)
	case "version":
		if len(args) != 1 {
			return fail(stderr, "Use version by itself.")
		}
		fmt.Fprintln(stdout, suite.Version)
		return 0
	case "upgrade":
		return fail(stderr, "upgrade is not part of nebula crypt. Upgrade nebula instead.")
	case "vaults":
		return vaults(stdout, stderr, args[1:])
	case "items":
		return items(stdout, stderr, args[1:])
	case "copy":
		return copyItem(stdout, stderr, args[1:])
	case "delete":
		if len(args) == 4 && args[2] == "item" {
			return deleteByID(stdout, stderr, args[1], args[3])
		}
		return runWords(stdout, stderr, args)
	case "list", "get", "describe", "create", "put":
		return runWords(stdout, stderr, args)
	default:
		return runnerFactory(stdout, stderr).Run(args)
	}
}

func runWords(stdout, stderr io.Writer, args []string) int {
	legacy, err := legacyArgs(args)
	if err != nil {
		return fail(stderr, err.Error())
	}
	return runnerFactory(stdout, stderr).Run(legacy)
}

func writeHelp(out io.Writer) int {
	fmt.Fprint(out, Help)
	return 0
}

func fail(errw io.Writer, message string) int {
	fmt.Fprintln(errw, message)
	return 1
}
