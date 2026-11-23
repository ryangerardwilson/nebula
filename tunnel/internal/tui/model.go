// Package tui is the route screen behind nebula tunnel screen. It uses only the
// standard library: raw mode on Linux, ANSI output, and a small line
// input for the name and port fields.
package tui

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
	"github.com/ryangerardwilson/nebula/tunnel/internal/session"
)

const (
	modeList    = "list"
	modeAdd     = "add"
	modeConfirm = "confirm"
)

type action int

const (
	actNone action = iota
	actQuit
	actSave
	actRemove
)

// model is the screen's state. file is a cache of the route store's last
// answer and is only ever replaced whole. removing holds the removes the
// worker has not answered; the view is the cache with those rows marked.
type model struct {
	home     string
	file     routes.File
	cursor   int
	mode     string
	name     input
	port     input
	notice   string
	svc      serviceState
	local    map[int]bool
	confirm  string
	removing map[string]bool
	color    bool
	help     bool
}

// serviceState is what the last scan said about tunnel.service.
type serviceState int

const (
	svcStopped serviceState = iota
	svcChecking
	svcRunning
)

func newModel(home string, file routes.File, color bool) model {
	return model{
		home:     home,
		file:     file,
		mode:     modeList,
		name:     input{limit: 63},
		port:     input{limit: 5},
		svc:      svcChecking,
		local:    map[int]bool{},
		removing: map[string]bool{},
		color:    color,
	}
}

// place is the selected route's name, the session's cursor key.
func (m model) place() string {
	if m.cursor >= 0 && m.cursor < len(m.file.Routes) {
		return m.file.Routes[m.cursor].Name
	}
	return ""
}

// restore moves the cursor to the route a resumed session was on.
func (m *model) restore(sess *session.Session) {
	keys := make([]string, len(m.file.Routes))
	for i, route := range m.file.Routes {
		keys[i] = route.Name
	}
	if i, ok := sess.Index(keys); ok {
		m.cursor = i
	}
}

// key applies one key and reports what the screen loop must do next.
func (m *model) key(k string) action {
	switch m.mode {
	case modeAdd:
		return m.keyAdd(k)
	case modeConfirm:
		return m.keyConfirm(k)
	default:
		return m.keyList(k)
	}
}

func (m *model) keyList(k string) action {
	if m.help {
		switch k {
		case "?", "esc":
			m.help = false
		case "q", "ctrl+c":
			return actQuit
		}
		return actNone
	}
	switch k {
	case "?":
		m.help = true
		return actNone
	case "q", "esc", "ctrl+c":
		return actQuit
	case "j", "down":
		m.cursor = cycled(m.cursor, 1, len(m.file.Routes))
	case "k", "up":
		m.cursor = cycled(m.cursor, -1, len(m.file.Routes))
	case "a":
		m.mode = modeAdd
		m.notice = ""
		m.port.focused = false
		m.name.focused = true
	case "d":
		if name := m.place(); name != "" && !m.removing[name] {
			m.mode = modeConfirm
			m.confirm = name
		}
	}
	return actNone
}

func cycled(cursor, delta, n int) int {
	if n <= 0 {
		return 0
	}
	if cursor < 0 {
		cursor = 0
	} else if cursor >= n {
		cursor = n - 1
	}
	return (cursor + delta%n + n) % n
}

func (m *model) keyAdd(k string) action {
	switch k {
	case "esc":
		m.mode = modeList
		m.notice = ""
	case "tab", "shift+tab":
		m.name.focused, m.port.focused = !m.name.focused, m.name.focused
	case "enter":
		if m.name.focused && strings.TrimSpace(m.port.value) == "" {
			m.name.focused, m.port.focused = false, true
			return actNone
		}
		m.notice = "saving"
		return actSave
	default:
		m.focusedInput().key(k)
	}
	return actNone
}

func (m *model) focusedInput() *input {
	if m.port.focused {
		return &m.port
	}
	return &m.name
}

// keyConfirm on y marks the route as being removed; the worker does the
// remove.
func (m *model) keyConfirm(k string) action {
	switch k {
	case "y", "Y":
		m.mode = modeList
		if m.removing == nil {
			m.removing = map[string]bool{}
		}
		m.removing[m.confirm] = true
		return actRemove
	case "n", "N", "esc":
		m.mode = modeList
	}
	return actNone
}

// answer applies one worker result.
func (m *model) answer(r result) {
	switch r.kind {
	case jobScan:
		m.scanned(r)
	case jobRemove:
		m.removed(r)
	case jobAdd:
		m.saved(r)
	}
}

// removed drops the remove intent. On success the store's list replaces
// the cache; on failure the row comes back and the error is shown.
func (m *model) removed(r result) {
	delete(m.removing, r.name)
	if r.err != nil {
		m.notice = r.err.Error()
		return
	}
	m.replace(r.file)
	m.notice = "removed " + r.name
}

// saved applies the answer to an add. On success the store's list
// replaces the cache.
func (m *model) saved(r result) {
	if r.err != nil {
		m.notice = r.err.Error()
		return
	}
	m.replace(r.file)
	m.mode = modeList
	m.notice = "saved"
	m.name.value = ""
	m.port.value = ""
}

// scanned takes the service state and which local ports answer. A failed
// scan keeps the last ones.
func (m *model) scanned(r result) {
	if r.err != nil {
		m.notice = r.err.Error()
		return
	}
	m.local = r.local
	m.svc = svcStopped
	if r.service {
		m.svc = svcRunning
	}
}

// replace swaps the cached list for the store's answer and keeps the
// cursor on the same route while it is listed.
func (m *model) replace(file routes.File) {
	place := m.place()
	m.file = file
	for i, route := range file.Routes {
		if route.Name == place {
			m.cursor = i
			return
		}
	}
	m.cursor = max(min(m.cursor, len(file.Routes)-1), 0)
}

func (m model) View() string {
	if m.help {
		return m.viewHelp()
	}
	var b strings.Builder
	b.WriteString(m.bold("tunnel") + "  " + m.dim(routes.Zone) + "\n")
	state := "service stopped"
	if m.svc == svcChecking {
		state = "checking service"
	} else if m.svc == svcRunning {
		state = "service running"
	}
	b.WriteString(m.dim(state) + "\n\n")
	m.viewRoutes(&b)
	b.WriteString("\n")
	m.viewFooter(&b)
	if m.notice != "" {
		b.WriteString("\n" + m.notice)
	}
	b.WriteString("\n")
	return b.String()
}

func (m model) viewHelp() string {
	lines := []string{
		m.bold("tunnel"),
		"",
		"j/k    move",
		"a      add",
		"d      remove",
		"?      shortcuts",
		"q      quit",
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m model) viewRoutes(b *strings.Builder) {
	if len(m.file.Routes) == 0 {
		b.WriteString(m.dim("no routes") + "\n")
	}
	for i, route := range m.file.Routes {
		mark := "  "
		if i == m.cursor {
			mark = "> "
		}
		line := fmt.Sprintf("%s%-8s  %-32s  %5d  %s", mark, route.Name, routes.Hostname(m.file.Zone, route.Name), route.Port, m.portState(route))
		if i == m.cursor {
			line = m.bold(line)
		}
		b.WriteString(line + "\n")
	}
}

// portState is up or down from the last scan, … before one, or removing
// while a remove is pending.
func (m model) portState(route routes.Route) string {
	if m.removing[route.Name] {
		return "removing"
	}
	up, known := m.local[route.Port]
	switch {
	case !known:
		return "…"
	case up:
		return "up"
	default:
		return "down"
	}
}

func (m model) viewFooter(b *strings.Builder) {
	switch m.mode {
	case modeAdd:
		b.WriteString("name  " + m.name.view(m.color) + "\n")
		b.WriteString("port  " + m.port.view(m.color) + "\n")
		b.WriteString(m.dim("host  "+m.preview()) + "\n")
		b.WriteString(m.dim("enter save    esc cancel"))
	case modeConfirm:
		b.WriteString("remove " + m.confirm + "? y or n")
	}
}

func (m model) preview() string {
	raw := strings.TrimSpace(m.name.value)
	if raw == "" {
		return "…"
	}
	_, hostname, err := routes.Normalize(m.file.Zone, raw)
	if err != nil {
		return "…"
	}
	return hostname
}

func (m model) bold(s string) string {
	if !m.color {
		return s
	}
	return "\x1b[1m" + s + "\x1b[22m"
}

func (m model) dim(s string) string {
	if !m.color {
		return s
	}
	return "\x1b[38;5;245m" + s + "\x1b[39m"
}
