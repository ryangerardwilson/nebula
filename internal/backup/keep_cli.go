package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// KeepMain runs nebula keep.
func KeepMain(args []string, out, errw io.Writer) int {
	return NewKeeper().main(args, out, errw)
}

func (k *Keeper) main(args []string, out, errw io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		fmt.Fprint(out, keepHelp)
		return 0
	}
	switch args[0] {
	case "mark":
		if len(args) != 3 {
			return keepUsage(errw, "usage: nebula keep mark <name> <path>")
		}
		return keepDone(errw, k.Mark(args[1], args[2]))
	case "restore":
		if len(args) != 3 {
			return keepUsage(errw, "usage: nebula keep restore <name> <path>")
		}
		how, err := k.Restore(args[1], args[2])
		if err == nil {
			fmt.Fprintf(out, "%s %s\n", how, args[2])
		}
		return keepDone(errw, err)
	case "adopt":
		if len(args) != 3 {
			return keepUsage(errw, "usage: nebula keep adopt <name> <path>")
		}
		return keepDone(errw, k.Adopt(args[1], args[2]))
	case "run":
		if len(args) != 2 {
			return keepUsage(errw, "usage: nebula keep run <name>")
		}
		return keepDone(errw, k.Run(args[1]))
	case "status":
		return k.status(args[1:], out, errw)
	case "flush":
		return k.flush(args[1:], out, errw)
	case "sweep":
		if len(args) != 1 {
			return keepUsage(errw, "usage: nebula keep sweep")
		}
		return keepDone(errw, k.Sweep())
	default:
		return keepUsage(errw, "usage: nebula keep help")
	}
}

func (k *Keeper) status(args []string, out, errw io.Writer) int {
	asJSON := len(args) == 1 && args[0] == "--json"
	if len(args) > 1 || len(args) == 1 && !asJSON {
		return keepUsage(errw, "usage: nebula keep status [--json]")
	}
	recs, err := k.Records()
	if err != nil {
		return keepDone(errw, err)
	}
	if asJSON {
		type row struct {
			Record
			State string `json:"state"`
		}
		rows := make([]row, 0, len(recs))
		for _, rec := range recs {
			rows = append(rows, row{Record: rec, State: rec.State()})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return keepDone(errw, enc.Encode(rows))
	}
	for _, rec := range recs {
		fmt.Fprintln(out, statusLine(rec))
	}
	return 0
}

func statusLine(rec Record) string {
	line := rec.Name + " " + rec.State()
	if !rec.KeptAt.IsZero() {
		line += " kept " + rec.KeptAt.Format(time.RFC3339)
	}
	if rec.State() == "ok" {
		return line
	}
	if !rec.DirtySince.IsZero() {
		line += " unkept since " + rec.DirtySince.Format(time.RFC3339)
	}
	if rec.Error != "" {
		line += ": " + rec.Error
	}
	return line
}

func (k *Keeper) flush(args []string, out, errw io.Writer) int {
	timeout := 20 * time.Second
	switch {
	case len(args) == 0:
	case len(args) == 2 && args[0] == "--timeout":
		d, err := time.ParseDuration(args[1])
		if err != nil || d <= 0 {
			return keepUsage(errw, "usage: nebula keep flush [--timeout 20s]")
		}
		timeout = d
	default:
		return keepUsage(errw, "usage: nebula keep flush [--timeout 20s]")
	}
	left, err := k.Flush(timeout)
	if err != nil {
		return keepDone(errw, err)
	}
	for _, name := range left {
		fmt.Fprintf(out, "%s still unkept\n", name)
	}
	if len(left) > 0 {
		return 1
	}
	return 0
}

func keepDone(errw io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(errw, err.Error())
	if errors.Is(err, ErrKeepUsage) {
		return 2
	}
	return 1
}

func keepUsage(errw io.Writer, msg string) int {
	fmt.Fprintln(errw, msg)
	return 2
}

const keepHelp = `keep
keep a durable copy of each application sqlite file at s3://backups/<name>.sqlite

global actions:
  nebula keep help
    show this help

features:
  say a database changed; the copy follows in the background
  # mark <name> <path>
  nebula keep mark c3po /run/user/1000/c3po/c3po.sqlite

  bring a missing database back from its copy, at startup
  # restore <name> <path>
  nebula keep restore c3po /run/user/1000/c3po/c3po.sqlite

  show how far each copy lags
  # status [--json]
  nebula keep status

  vouch once for a current file that did not come from restore
  # adopt <name> <path>
  nebula keep adopt c3po /run/user/1000/c3po/c3po.sqlite

  upload every lagging copy now, or catch a missed mark
  # flush [--timeout 20s] | sweep
  nebula keep flush --timeout 20s
  nebula keep sweep
`
