package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryangerardwilson/nebula/tunnel"
)

const helpText = `nebula run
run a site on the SSH host and a free hostname

global actions:
  nebula run help
    show this help
  nebula run list
    show name, hostname, port, and release
  nebula run logs <name>
    show the last 80 log lines
  nebula run rollback <name>
    switch back one release and restart

features:
  build a directory and replace the running release
  # run <directory>
  nebula run ./site

notes:
  The directory has nebula.toml with name, build, start, and hostname.
  The build runs on this laptop. The process listens on 127.0.0.1
  and PORT on the SSH host. A new table stays on nebula sql.
  rollback moves the current release. It does not change the database.
  The ssh hostname on the host tunnel is left in place.

exit: 0 ok, 1 error, 2 usage
`

// Main runs nebula run.
func Main(args []string) int {
	return runCLI(args, os.Stdout, os.Stderr, nil)
}

func runCLI(args []string, out, errw io.Writer, box Box) int {
	if len(args) == 0 || helpArgs(args) {
		fmt.Fprint(out, helpText)
		return 0
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return usage(errw)
		}
		return show(out, errw, box, func(ctx context.Context, box Box) (string, error) {
			return List(ctx, box)
		})
	case "logs":
		if len(args) != 2 {
			return usage(errw)
		}
		return show(out, errw, box, func(ctx context.Context, box Box) (string, error) {
			return Logs(ctx, box, args[1])
		})
	case "rollback":
		if len(args) != 2 {
			return usage(errw)
		}
		return rollback(out, errw, box, args[1])
	default:
		if len(args) != 1 {
			return usage(errw)
		}
		return deploy(out, errw, box, args[0])
	}
}

func helpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
}

func usage(errw io.Writer) int {
	fmt.Fprint(errw, helpText)
	return 2
}

func show(out, errw io.Writer, box Box, fn func(context.Context, Box) (string, error)) int {
	box, err := needBox(box)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, err := fn(ctx, box)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	fmt.Fprint(out, text)
	return 0
}

func rollback(out, errw io.Writer, box Box, name string) int {
	box, err := needBox(box)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, err := Rollback(ctx, box, name)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	fmt.Fprintf(out, "rolled back %s to %s\n", name, id)
	return 0
}

func deploy(out, errw io.Writer, box Box, dir string) int {
	file, err := readProject(dir)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	if err := localBuild(dir, file.Build); err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	box, err = needBox(box)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	release, err := NewRelease(time.Now())
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	res, err := Deploy(ctx, box, Spec{
		Name: file.Name, Hostname: file.Hostname, Start: file.Start, Dir: dir, DNS: liveDNS,
	}, release)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 1
	}
	fmt.Fprintf(out, "ran %s on %s port %d release %s\n", res.Name, res.Hostname, res.Port, res.Release)
	return 0
}

func readProject(dir string) (File, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "nebula.toml"))
	if err != nil {
		return File{}, fmt.Errorf("nebula.toml is missing")
	}
	file, err := Parse(string(raw))
	if err != nil {
		return File{}, err
	}
	if err := checkBuild(file.Build); err != nil {
		return File{}, err
	}
	return file, nil
}

func needBox(box Box) (Box, error) {
	if box != nil {
		return box, nil
	}
	return OpenSSH()
}

func localBuild(dir, command string) error {
	if command == "" {
		return nil
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(buf.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("build: %s", msg)
	}
	return nil
}

func liveDNS(ctx context.Context, hostname, tunnelID string) error {
	return tunnel.EnsureDNS(ctx, hostname, tunnelID)
}
