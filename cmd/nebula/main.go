package main

import (
	"os"
	"path/filepath"

	"github.com/ryangerardwilson/nebula/command"
	"github.com/ryangerardwilson/nebula/internal/suite"
	"github.com/ryangerardwilson/nebula/s3x"
)

func main() {
	os.Exit(dispatch(os.Args))
}

func dispatch(args []string) int {
	switch filepath.Base(args[0]) {
	case "sql":
		return command.Main(args[1:])
	case "s3":
		return s3x.Main(args[1:])
	}
	if len(args) < 2 {
		return suite.Main(nil)
	}
	switch args[1] {
	case "sql":
		return command.Main(args[2:])
	case "s3":
		return s3x.Main(args[2:])
	default:
		return suite.Main(args[1:])
	}
}
