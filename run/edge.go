package run

import (
	"fmt"
	"strings"
)

const (
	edgeUnit     = "cloudflared-ssh"
	edgeUnitFile = "/etc/systemd/system/cloudflared-ssh.service"
	edgeFallback = "/usr/lib/systemd/system/cloudflared-ssh.service"
	sshHost      = "ssh.ryangerardwilson.com"
	sshOrigin    = "http://127.0.0.1:8022"
)

func ConfigPath(unit string) (string, error) {
	fields := strings.Fields(unit)
	for i, field := range fields {
		if strings.HasPrefix(field, "--config=") {
			return strings.TrimPrefix(field, "--config="), nil
		}
		if field == "--config" && i+1 < len(fields) {
			return fields[i+1], nil
		}
	}
	return "", fmt.Errorf("cloudflared-ssh has no --config path")
}

func TunnelID(config string) (string, error) {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "tunnel:") {
			continue
		}
		id := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "tunnel:")), `"'`)
		if id == "" || strings.ContainsAny(id, " \t") {
			return "", fmt.Errorf("edge config has an empty tunnel id")
		}
		return id, nil
	}
	return "", fmt.Errorf("edge config has no tunnel id")
}

// InsertIngress adds one hostname before the catch-all.
// The ssh origin rule is copied through unchanged.
func InsertIngress(config, hostname, service string) (string, error) {
	if err := checkEdgeTarget(hostname, service); err != nil {
		return "", err
	}
	head, rules, foot, err := parseIngress(config)
	if err != nil {
		return "", err
	}
	sshAt := protectedRule(rules)
	if sshAt < 0 {
		return "", fmt.Errorf("edge config has no ssh origin")
	}
	kept := append([]string(nil), rules[sshAt]...)
	rules, err = upsertRule(rules, hostname, service)
	if err != nil {
		return "", err
	}
	if sshAt >= len(rules) || strings.Join(rules[sshAt], "\n") != strings.Join(kept, "\n") {
		return "", fmt.Errorf("edge edit changed the ssh rule")
	}
	return head + joinRules(rules) + foot, nil
}

func checkEdgeTarget(hostname, service string) error {
	if hostname == "" || hostname == sshHost || strings.ContainsAny(hostname, " \t\r\n'\"") {
		return fmt.Errorf("refusing hostname %q", hostname)
	}
	if !strings.HasPrefix(service, "http://127.0.0.1:") || strings.Contains(service, "8022") {
		return fmt.Errorf("refusing service %q", service)
	}
	return nil
}

func parseIngress(config string) (string, [][]string, string, error) {
	lines := strings.Split(config, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "ingress:" {
			start = i
			break
		}
	}
	if start < 0 {
		return "", nil, "", fmt.Errorf("edge config has no ingress")
	}
	rules, end := groupRules(lines, start+1)
	if len(rules) == 0 {
		return "", nil, "", fmt.Errorf("edge config has no ingress rules")
	}
	head := strings.Join(lines[:start+1], "\n") + "\n"
	foot := strings.Join(lines[end:], "\n")
	if end < len(lines) {
		foot = strings.Join(lines[end:], "\n")
	}
	return head, rules, foot, nil
}

func groupRules(lines []string, from int) ([][]string, int) {
	var rules [][]string
	var cur []string
	end := len(lines)
	for i := from; i < len(lines); i++ {
		line := lines[i]
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			end = i
			break
		}
		if isRuleStart(line) {
			if cur != nil {
				rules = append(rules, cur)
			}
			cur = []string{line}
			continue
		}
		if cur != nil && line != "" {
			cur = append(cur, line)
		}
	}
	if cur != nil {
		rules = append(rules, cur)
	}
	return rules, end
}

func isRuleStart(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return trimmed != line && strings.HasPrefix(trimmed, "- ")
}

func protectedRule(rules [][]string) int {
	for i, rule := range rules {
		text := strings.Join(rule, "\n")
		if strings.Contains(text, sshOrigin) || strings.Contains(text, "hostname: "+sshHost) {
			return i
		}
	}
	return -1
}

func upsertRule(rules [][]string, hostname, service string) ([][]string, error) {
	for i, rule := range rules {
		if ruleHost(rule) != hostname {
			continue
		}
		if protectedRule([][]string{rule}) >= 0 {
			return nil, fmt.Errorf("refusing to edit the ssh rule")
		}
		rules[i] = setService(rule, service)
		return rules, nil
	}
	at := catchAll(rules)
	if at < 0 {
		return nil, fmt.Errorf("edge config has no catch-all")
	}
	if protectedRule(rules) > at {
		return nil, fmt.Errorf("ssh rule is after the catch-all")
	}
	block := []string{"  - hostname: " + hostname, "    service: " + service}
	next := append([][]string{}, rules[:at]...)
	next = append(next, block)
	next = append(next, rules[at:]...)
	return next, nil
}

func catchAll(rules [][]string) int {
	for i, rule := range rules {
		if ruleHost(rule) == "" {
			return i
		}
	}
	return -1
}

func ruleHost(rule []string) string {
	for _, line := range rule {
		trim := strings.TrimSpace(line)
		trim = strings.TrimSpace(strings.TrimPrefix(trim, "-"))
		if strings.HasPrefix(trim, "hostname:") {
			return strings.TrimSpace(strings.TrimPrefix(trim, "hostname:"))
		}
	}
	return ""
}

func setService(rule []string, service string) []string {
	next := append([]string(nil), rule...)
	for i, line := range next {
		if strings.Contains(strings.TrimSpace(line), "service:") {
			next[i] = "    service: " + service
			return next
		}
	}
	return append(next, "    service: "+service)
}

func joinRules(rules [][]string) string {
	var b strings.Builder
	for _, rule := range rules {
		for _, line := range rule {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func ReloadScript(cfg, next string) (string, error) {
	if err := plainPath(cfg); err != nil || plainPath(next) != nil {
		return "", fmt.Errorf("unsafe edge path")
	}
	return fmt.Sprintf(reloadFmt, cfg, next, edgeUnit, edgeUnit), nil
}

func plainPath(path string) error {
	if path == "" || strings.Contains(path, "..") || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n '\"\\") {
		return fmt.Errorf("unsafe path")
	}
	return nil
}

const reloadFmt = `set -eu
cfg='%s'
next='%s'
if ! grep -q '127.0.0.1:8022' "$next"; then
  echo 'refusing an edge config without the ssh origin' >&2
  exit 1
fi
if ! grep -Fq 'ssh.ryangerardwilson.com' "$next"; then
  echo 'refusing an edge config without the ssh hostname' >&2
  exit 1
fi
bak="${cfg}.nebula-bak"
cp -a "$cfg" "$bak"
install -m 600 "$next" "$cfg"
if ! systemd-run --collect --unit=nebula-edge-reload --no-block /bin/sh -c "if ! systemctl restart %s; then cp -a '$bak' '$cfg'; systemctl restart %s || true; fi"; then
  cp -a "$bak" "$cfg"
  exit 1
fi
`
