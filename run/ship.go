package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Box is the SSH host. Tests substitute a memory box.
type Box interface {
	Read(ctx context.Context, path string) (string, error)
	Write(ctx context.Context, path, body string) error
	Upload(ctx context.Context, local, remote string) error
	Output(ctx context.Context, script string) (string, error)
}

// Spec is one site to run. DNS is nil in tests.
type Spec struct {
	Name     string
	Hostname string
	Start    string
	Dir      string
	DNS      func(ctx context.Context, hostname, tunnelID string) error
}

// Result is the release that is current after Deploy.
type Result struct {
	Name     string
	Hostname string
	Port     int
	Release  string
}

func Deploy(ctx context.Context, box Box, spec Spec, release string) (Result, error) {
	spec, err := prepareSpec(spec, release)
	if err != nil {
		return Result{}, err
	}
	if err := box.Upload(ctx, spec.Dir, releaseDir(spec.Name, release)); err != nil {
		return Result{}, err
	}
	port, err := place(ctx, box, spec, release)
	if err != nil {
		return Result{}, err
	}
	if err := publishEdge(ctx, box, spec, port); err != nil {
		return Result{}, err
	}
	return Result{Name: spec.Name, Hostname: spec.Hostname, Port: port, Release: release}, nil
}

func prepareSpec(spec Spec, release string) (Spec, error) {
	if err := checkName(spec.Name); err != nil {
		return Spec{}, err
	}
	if err := checkStart(spec.Start); err != nil {
		return Spec{}, err
	}
	host, err := normalizeHost(spec.Hostname)
	if err != nil {
		return Spec{}, err
	}
	if err := checkRelease(release); err != nil {
		return Spec{}, err
	}
	info, err := os.Stat(spec.Dir)
	if err != nil || !info.IsDir() {
		return Spec{}, fmt.Errorf("site directory is missing")
	}
	spec.Hostname = host
	return spec, nil
}

func place(ctx context.Context, box Box, spec Spec, release string) (int, error) {
	text, err := box.Read(ctx, portsPath)
	if errors.Is(err, os.ErrNotExist) {
		text = ""
		err = nil
	}
	if err != nil {
		return 0, err
	}
	port, next, err := Remember(text, spec.Name, spec.Hostname, release)
	if err != nil {
		return 0, err
	}
	if err := box.Write(ctx, portsPath, next); err != nil {
		return 0, err
	}
	unit, err := UnitText(spec.Name, spec.Start, port)
	if err != nil {
		return 0, err
	}
	if err := box.Write(ctx, unitPath(spec.Name), unit); err != nil {
		return 0, err
	}
	script, err := SwitchScript(spec.Name, release)
	if err != nil {
		return 0, err
	}
	if _, err := box.Output(ctx, script); err != nil {
		return 0, err
	}
	return port, nil
}

func publishEdge(ctx context.Context, box Box, spec Spec, port int) error {
	path, body, id, err := edgeConfig(ctx, box)
	if err != nil {
		return err
	}
	service := fmt.Sprintf("http://127.0.0.1:%d", port)
	next, err := InsertIngress(body, spec.Hostname, service)
	if err != nil {
		return err
	}
	if spec.DNS != nil {
		if err := spec.DNS(ctx, spec.Hostname, id); err != nil {
			return err
		}
	}
	nextPath := path + ".nebula-next"
	if err := box.Write(ctx, nextPath, next); err != nil {
		return err
	}
	script, err := ReloadScript(path, nextPath)
	if err != nil {
		return err
	}
	_, err = box.Output(ctx, script)
	return err
}

func edgeConfig(ctx context.Context, box Box) (string, string, string, error) {
	unit, err := box.Read(ctx, edgeUnitFile)
	if errors.Is(err, os.ErrNotExist) {
		unit, err = box.Read(ctx, edgeFallback)
	}
	if err != nil {
		return "", "", "", err
	}
	path, err := ConfigPath(unit)
	if err != nil {
		return "", "", "", err
	}
	body, err := box.Read(ctx, path)
	if err != nil {
		return "", "", "", err
	}
	id, err := TunnelID(body)
	if err != nil {
		return "", "", "", err
	}
	return path, body, id, nil
}

func Rollback(ctx context.Context, box Box, name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	text, err := box.Read(ctx, portsPath)
	if err != nil {
		return "", err
	}
	if !hasApp(text, name) {
		return "", fmt.Errorf("no app %s", name)
	}
	earlier, err := earlierRelease(ctx, box, name)
	if err != nil {
		return "", err
	}
	script, err := SwitchScript(name, earlier)
	if err != nil {
		return "", err
	}
	if _, err := box.Output(ctx, script); err != nil {
		return "", err
	}
	next, err := SetRelease(text, name, earlier)
	if err != nil {
		return "", err
	}
	if err := box.Write(ctx, portsPath, next); err != nil {
		return "", err
	}
	return earlier, nil
}

func earlierRelease(ctx context.Context, box Box, name string) (string, error) {
	list, err := box.Output(ctx, "ls -1 -- "+shellQuote(appDir(name)+"/releases"))
	if err != nil {
		return "", err
	}
	link, err := box.Output(ctx, "readlink -- "+shellQuote(currentLink(name)))
	if err != nil {
		return "", err
	}
	current, err := releaseFromLink(link)
	if err != nil {
		return "", err
	}
	return Earlier(releaseIDs(list), current)
}

func releaseIDs(text string) []string {
	var ids []string
	for _, line := range strings.Fields(text) {
		if checkRelease(line) == nil {
			ids = append(ids, line)
		}
	}
	return ids
}

func releaseFromLink(text string) (string, error) {
	text = strings.TrimSpace(text)
	const prefix = "releases/"
	if !strings.HasPrefix(text, prefix) {
		return "", fmt.Errorf("current release is unknown")
	}
	id := strings.TrimPrefix(text, prefix)
	if err := checkRelease(id); err != nil {
		return "", err
	}
	return id, nil
}

func hasApp(text, name string) bool {
	rows, err := parsePorts(text)
	if err != nil {
		return false
	}
	for _, row := range rows {
		if row.name == name {
			return true
		}
	}
	return false
}

func List(ctx context.Context, box Box) (string, error) {
	text, err := box.Read(ctx, portsPath)
	if errors.Is(err, os.ErrNotExist) {
		return "no apps\n", nil
	}
	if err != nil {
		return "", err
	}
	rows, err := parsePorts(text)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "no apps\n", nil
	}
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "%-16s %-40s %5d  %s\n", row.name, row.host, row.port, row.release)
	}
	return b.String(), nil
}

func Logs(ctx context.Context, box Box, name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	return box.Output(ctx, "journalctl -u "+shellQuote(unitName(name))+" -n 80 --no-pager")
}
