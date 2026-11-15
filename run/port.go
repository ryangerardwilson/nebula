package run

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	portFirst = 18100
	portLast  = 18199
	portsPath = "/var/lib/nebula/apps/ports"
)

type portRow struct {
	name    string
	port    int
	host    string
	release string
}

func Remember(text, name, host, release string) (int, string, error) {
	rows, err := parsePorts(text)
	if err != nil {
		return 0, "", err
	}
	for i, row := range rows {
		if row.name != name {
			continue
		}
		rows[i].host = host
		rows[i].release = release
		return row.port, formatPorts(rows), nil
	}
	used := map[int]bool{}
	for _, row := range rows {
		used[row.port] = true
	}
	port := freePort(used)
	if port == 0 {
		return 0, "", fmt.Errorf("no free port")
	}
	rows = append(rows, portRow{name: name, port: port, host: host, release: release})
	return port, formatPorts(rows), nil
}

func SetRelease(text, name, release string) (string, error) {
	rows, err := parsePorts(text)
	if err != nil {
		return "", err
	}
	found := false
	for i, row := range rows {
		if row.name == name {
			rows[i].release = release
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("no app %s", name)
	}
	return formatPorts(rows), nil
}

func freePort(used map[int]bool) int {
	for port := portFirst; port <= portLast; port++ {
		if !used[port] {
			return port
		}
	}
	return 0
}

func parsePorts(text string) ([]portRow, error) {
	var rows []portRow
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 {
			return nil, fmt.Errorf("ports line %q is not name, port, hostname, release", line)
		}
		port, err := strconv.Atoi(fields[1])
		if err != nil || port < portFirst || port > portLast {
			return nil, fmt.Errorf("ports line %q has a port outside %d-%d", line, portFirst, portLast)
		}
		rows = append(rows, portRow{name: fields[0], port: port, host: fields[2], release: fields[3]})
	}
	return rows, nil
}

func formatPorts(rows []portRow) string {
	sort.Slice(rows, func(i, j int) bool { return rows[i].name < rows[j].name })
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "%s %d %s %s\n", row.name, row.port, row.host, row.release)
	}
	return b.String()
}

func Earlier(ids []string, current string) (string, error) {
	sort.Strings(ids)
	at := -1
	for i, id := range ids {
		if id == current {
			at = i
		}
	}
	if at < 0 {
		return "", fmt.Errorf("release %s is not on the host", current)
	}
	if at == 0 {
		return "", fmt.Errorf("no earlier release")
	}
	return ids[at-1], nil
}

func checkRelease(id string) error {
	if len(id) != 20 || id[8] != 'T' || id[15] != 'Z' {
		return fmt.Errorf("release %s is not a nebula release id", id)
	}
	for i, r := range id {
		switch i {
		case 8, 15:
			continue
		}
		ok := r >= '0' && r <= '9' || i > 15 && r >= 'a' && r <= 'f'
		if !ok {
			return fmt.Errorf("release %s is not a nebula release id", id)
		}
	}
	return nil
}
