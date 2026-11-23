package main

import (
	"os"
	"path/filepath"

	"github.com/ryangerardwilson/nebula/command"
	"github.com/ryangerardwilson/nebula/crypt"
	"github.com/ryangerardwilson/nebula/internal/backup"
	"github.com/ryangerardwilson/nebula/internal/suite"
	"github.com/ryangerardwilson/nebula/run"
	"github.com/ryangerardwilson/nebula/s3"
	"github.com/ryangerardwilson/nebula/tunnel"
)

func main() {
	os.Exit(dispatch(os.Args))
}

func dispatch(args []string) int {
	switch filepath.Base(args[0]) {
	case "sql":
		return command.Main(args[1:])
	case "s3":
		return s3.Main(args[1:])
	case "run":
		return run.Main(args[1:])
	case "tunnel":
		return tunnel.Run(args[1:], os.Stdout, os.Stderr)
	case "crypt":
		return crypt.Run(args[1:], os.Stdout, os.Stderr)
	}
	if len(args) < 2 {
		return suite.Main(nil)
	}
	switch args[1] {
	case "sql":
		return command.Main(args[2:])
	case "s3":
		return s3.Main(args[2:])
	case "run":
		return run.Main(args[2:])
	case "tunnel":
		return tunnel.Run(args[2:], os.Stdout, os.Stderr)
	case "crypt":
		return crypt.Run(args[2:], os.Stdout, os.Stderr)
	case "keep":
		return backup.KeepMain(args[2:], os.Stdout, os.Stderr)
	default:
		return suite.Main(args[1:])
	}
}
