package run

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/nebula/tunnel"
)

// File is the project file nebula.toml.
type File struct {
	Name     string
	Build    string
	Start    string
	Hostname string
}

func Parse(text string) (File, error) {
	var file File
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return File{}, fmt.Errorf("nebula.toml line %q needs a value", line)
		}
		key = strings.TrimSpace(key)
		value = unquote(strings.TrimSpace(value))
		if seen[key] {
			return File{}, fmt.Errorf("nebula.toml repeats %s", key)
		}
		seen[key] = true
		switch key {
		case "name":
			file.Name = value
		case "build":
			file.Build = value
		case "start":
			file.Start = value
		case "hostname":
			file.Hostname = value
		default:
			return File{}, fmt.Errorf("nebula.toml has unknown key %s", key)
		}
	}
	if file.Name == "" || file.Start == "" || file.Hostname == "" {
		return File{}, fmt.Errorf("nebula.toml needs name, start, and hostname")
	}
	return file, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func normalizeHost(hostname string) (string, error) {
	return tunnel.NormalizeHost(hostname)
}

func checkName(name string) error {
	if name == "" || len(name) > 31 {
		return fmt.Errorf("app name must be a short lowercase label")
	}
	for i, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' && i > 0 || r == '-' && i > 0
		if !ok {
			return fmt.Errorf("app name must be a short lowercase label")
		}
	}
	return nil
}

func checkStart(start string) error {
	if start == "" || strings.ContainsAny(start, "\r\n'") {
		return fmt.Errorf("start command is empty or unsafe")
	}
	return nil
}

func checkBuild(build string) error {
	if strings.ContainsAny(build, "\r\n") {
		return fmt.Errorf("build command is unsafe")
	}
	return nil
}
