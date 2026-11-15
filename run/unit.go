package run

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

func appDir(name string) string { return "/var/lib/nebula/apps/" + name }

func releaseDir(name, release string) string {
	return appDir(name) + "/releases/" + release
}

func currentLink(name string) string { return appDir(name) + "/current" }

func unitName(name string) string { return "nebula-" + name + ".service" }

func unitPath(name string) string { return "/etc/systemd/system/" + unitName(name) }

func NewRelease(now time.Time) (string, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	id := now.UTC().Format("20060102T150405Z") + fmt.Sprintf("%02x%02x", b[0], b[1])
	if err := checkRelease(id); err != nil {
		return "", err
	}
	return id, nil
}

func UnitText(name, start string, port int) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	if err := checkStart(start); err != nil {
		return "", err
	}
	if port < portFirst || port > portLast {
		return "", fmt.Errorf("port %d is outside the app range", port)
	}
	return fmt.Sprintf(unitFmt, name, currentLink(name), port, start), nil
}

const unitFmt = `[Unit]
Description=Nebula app %s
After=network-online.target

[Service]
WorkingDirectory=%s
Environment=HOST=127.0.0.1
Environment=PORT=%d
ExecStart=/bin/sh -c 'exec %s'
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
`

func SwitchScript(name, release string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	if err := checkRelease(release); err != nil {
		return "", err
	}
	target := "releases/" + release
	return fmt.Sprintf("set -eu\nln -sfn -- %s %s\nsystemctl daemon-reload\nsystemctl enable --now %s\nsystemctl restart %s\n",
		shellQuote(target), shellQuote(currentLink(name)), shellQuote(unitName(name)), shellQuote(unitName(name))), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
