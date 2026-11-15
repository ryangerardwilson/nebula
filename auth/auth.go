// Package auth is nebula auth, the shared Google sign-in.
package auth

import (
	"fmt"
	"io"

	"github.com/ryangerardwilson/nebula/crypt"
	"github.com/ryangerardwilson/nebula/internal/suite"
)

// Help is the full help for nebula auth.
const Help = `nebula auth
shared Google sign-in for this laptop

global actions:
  nebula auth help
    show this help
  nebula auth version
    print the nebula version

  record the Root client id
  # auth setup
  nebula auth setup

  record a product redirect or scope
  # auth add <name> redirect <url> | auth add <name> scope <scope>
  nebula auth add notes redirect http://127.0.0.1:8080/cb

  sign in, or finish a sign-in that was started elsewhere
  # auth login <name> | auth url <name> | auth exchange <name> <code> <state>
  nebula auth login notes

  list products, or remove one
  # auth list | auth remove <name>
  nebula auth list

The operator already created one Google client. The client id and secret are
the Root items GOOGLE_OAUTH_CLIENT_ID and GOOGLE_OAUTH_CLIENT_SECRET, read
through nebula crypt. Each product redirect is added on that same Google
client in the console. nebula records the redirect and performs sign-in.
It does not create the client. The client secret is not written here and is
not printed. A session file is that product's Google token, not an
application user table.
`

const usage = "Use: nebula auth setup | nebula auth add <name> redirect <url> | nebula auth add <name> scope <scope> | nebula auth remove <name> | nebula auth list | nebula auth url <name> | nebula auth login <name> | nebula auth exchange <name> <code> <state>"

// secretGet reads one exact 1Password title. Tests replace it.
var secretGet = crypt.Get

// Run runs nebula auth. args follow "nebula auth".
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
	case "setup":
		if len(args) != 1 {
			return fail(stderr, "Use: nebula auth setup")
		}
		return setup(stdout, stderr)
	case "list":
		if len(args) != 1 {
			return fail(stderr, "Use: nebula auth list")
		}
		return listProducts(stdout, stderr)
	case "add":
		return add(stdout, stderr, args[1:])
	case "remove":
		return remove(stdout, stderr, args[1:])
	case "url":
		return showURL(stdout, stderr, args[1:])
	case "login":
		return login(stdout, stderr, args[1:])
	case "exchange":
		return exchange(stdout, stderr, args[1:])
	default:
		return fail(stderr, usage)
	}
}

func writeHelp(out io.Writer) int {
	fmt.Fprint(out, Help)
	return 0
}

func fail(errw io.Writer, message string) int {
	fmt.Fprintln(errw, message)
	return 1
}
