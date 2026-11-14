// Package command is the sql command inside nebula.
package command

import "github.com/ryangerardwilson/nebula/internal/cli"

// Main runs sqlx.
func Main(args []string) int {
	return cli.Main(args)
}
