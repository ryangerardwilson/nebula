package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
)

// fakeTerm feeds the loop keys and keeps the last frame it painted. A read
// with no key queued is one idle pass.
type fakeTerm struct {
	keys  chan string
	quit  chan struct{}
	mu    sync.Mutex
	frame string
	idle  int
}

func (f *fakeTerm) read(time.Duration) (string, bool, error) {
	select {
	case k := <-f.keys:
		return k, true, nil
	case <-f.quit:
		return "", false, io.EOF
	case <-time.After(time.Millisecond):
		f.mu.Lock()
		f.idle++
		f.mu.Unlock()
		return "", false, nil
	}
}

func (f *fakeTerm) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.frame = string(p)
	f.mu.Unlock()
	return len(p), nil
}

func (f *fakeTerm) shown() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frame
}

func (f *fakeTerm) passes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.idle
}

// row is the painted line for host, or "".
func (f *fakeTerm) row(host string) string {
	for _, line := range strings.Split(f.shown(), "\n") {
		if strings.Contains(line, host) {
			return line
		}
	}
	return ""
}

// idlePasses waits until the loop has run n more passes with no key.
func (f *fakeTerm) idlePasses(t *testing.T, n int) {
	t.Helper()
	want := f.passes() + n
	waitFor(t, "idle passes", func() bool { return f.passes() >= want })
}

// placeLog is a fake session writer.
type placeLog struct {
	mu    sync.Mutex
	calls []string
}

func (p *placeLog) write(cursor string) error {
	p.mu.Lock()
	p.calls = append(p.calls, cursor)
	p.mu.Unlock()
	return nil
}

func (p *placeLog) got() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func threeRoutes() routes.File {
	return routes.File{Zone: routes.Zone, Routes: []routes.Route{
		{Name: routes.Apex, Port: 8080},
		{Name: "lab", Port: 9090},
		{Name: "notes", Port: 9091},
	}}
}

// fakeOps never touches systemd, ports, or the route store.
func fakeOps(file routes.File) ops {
	return ops{
		add: func(_, name, port string) (routes.File, error) {
			n, err := routes.ParsePort(port)
			if err != nil {
				return routes.File{}, err
			}
			next, _, err := routes.Upsert(file, name, n)
			return next, err
		},
		remove: func(_, name string) (routes.File, error) {
			return routes.Remove(file, name)
		},
		service: func(context.Context) bool { return true },
		listen:  func(context.Context, int) bool { return true },
	}
}

// startLoop runs the loop on m with fakes. The loop owns m until done.
func startLoop(t *testing.T, m *model, o ops) (*fakeTerm, *placeLog, <-chan error) {
	term := &fakeTerm{keys: make(chan string), quit: make(chan struct{})}
	log := &placeLog{}
	done := make(chan error, 1)
	go func() { done <- loop(m, screen{keys: term.read, out: term, ops: o, place: log.write}) }()
	t.Cleanup(func() { close(term.quit) })
	return term, log, done
}

func quit(t *testing.T, term *fakeTerm, done <-chan error) {
	t.Helper()
	term.keys <- "q"
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(quitWait + time.Second):
		t.Fatal("loop did not quit")
	}
}

func TestIdleLoopLeavesTheSessionAlone(t *testing.T) {
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, log, done := startLoop(t, &m, fakeOps(threeRoutes()))
	waitFor(t, "first scan", func() bool { return strings.Contains(term.shown(), "service running") })
	term.idlePasses(t, 50)
	if calls := log.got(); len(calls) != 0 {
		t.Fatalf("idle passes wrote the session %q", calls)
	}
	quit(t, term, done)
	if calls := log.got(); len(calls) != 0 {
		t.Fatalf("quit with no move wrote the session %q", calls)
	}
}

func TestSessionPlaceWrittenOncePerChange(t *testing.T) {
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, log, done := startLoop(t, &m, fakeOps(threeRoutes()))
	term.keys <- "j"
	waitFor(t, "lab", func() bool { return len(log.got()) == 1 })
	term.idlePasses(t, 30)
	term.keys <- "?"
	term.keys <- "?"
	term.idlePasses(t, 30)
	term.keys <- "j"
	waitFor(t, "notes", func() bool { return len(log.got()) == 2 })
	term.idlePasses(t, 30)
	quit(t, term, done)
	calls := log.got()
	if strings.Join(calls, " ") != "lab notes" {
		t.Fatalf("places %q", calls)
	}
}

func TestPlacerKeepsOnlyTheNewestPlace(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	log := &placeLog{}
	p := startPlacer(func(cursor string) error {
		if cursor == "lab" {
			close(started)
			<-release
		}
		return log.write(cursor)
	}, routes.Apex)
	defer p.stop()
	p.set(routes.Apex)
	p.set("lab")
	<-started
	for _, place := range []string{"notes", "www", "www"} {
		p.set(place)
	}
	close(release)
	waitFor(t, "drain", func() bool { return p.pending() == 0 })
	p.set("www")
	time.Sleep(10 * time.Millisecond)
	if calls := log.got(); strings.Join(calls, " ") != "lab www" || p.pending() != 0 {
		t.Fatalf("places %q", calls)
	}
}

func TestSlowRemovePaintsIntentAndKeepsKeys(t *testing.T) {
	release := make(chan struct{})
	o := fakeOps(threeRoutes())
	o.remove = func(_, name string) (routes.File, error) {
		<-release
		return routes.Remove(threeRoutes(), name)
	}
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, log, done := startLoop(t, &m, o)
	for _, k := range []string{"j", "d", "y"} {
		term.keys <- k
	}
	waitFor(t, "removing row", func() bool { return strings.Contains(term.row("lab.ryangerardwilson.com"), "removing") })
	term.keys <- "j"
	waitFor(t, "cursor on notes", func() bool { return strings.HasPrefix(term.row("notes.ryangerardwilson.com"), "> notes") })
	if !strings.Contains(term.row("lab.ryangerardwilson.com"), "removing") {
		t.Fatalf("intent lost before the answer\n%s", term.shown())
	}
	close(release)
	waitFor(t, "removed", func() bool {
		return strings.Contains(term.shown(), "removed lab") && term.row("lab.ryangerardwilson.com") == ""
	})
	if !strings.HasPrefix(term.row("notes.ryangerardwilson.com"), "> notes") {
		t.Fatalf("cursor left notes\n%s", term.shown())
	}
	quit(t, term, done)
	if calls := log.got(); calls[len(calls)-1] != "notes" {
		t.Fatalf("places %q", calls)
	}
}

func TestFailedRemoveRestoresTheRow(t *testing.T) {
	o := fakeOps(threeRoutes())
	o.remove = func(string, string) (routes.File, error) {
		return routes.File{}, errors.New("records are locked")
	}
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, _, done := startLoop(t, &m, o)
	waitFor(t, "first scan", func() bool { return strings.Contains(term.shown(), "service running") })
	for _, k := range []string{"j", "d", "y"} {
		term.keys <- k
	}
	waitFor(t, "error", func() bool { return strings.Contains(term.shown(), "records are locked") })
	if row := term.row("lab.ryangerardwilson.com"); !strings.HasSuffix(strings.TrimSuffix(row, "\x1b[K"), "up") {
		t.Fatalf("row not restored %q\n%s", row, term.shown())
	}
	quit(t, term, done)
	if len(m.file.Routes) != 3 || len(m.removing) != 0 {
		t.Fatalf("cache %+v removing %v", m.file, m.removing)
	}
}

func TestQuitDrainsAPendingRemove(t *testing.T) {
	release := make(chan struct{})
	removed := make(chan string, 1)
	o := fakeOps(threeRoutes())
	o.remove = func(_, name string) (routes.File, error) {
		<-release
		removed <- name
		return routes.Remove(threeRoutes(), name)
	}
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, _, done := startLoop(t, &m, o)
	for _, k := range []string{"j", "d", "y", "q"} {
		term.keys <- k
	}
	waitFor(t, "saving frame", func() bool { return strings.Contains(term.shown(), "saving") })
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if name := <-removed; name != "lab" || len(m.file.Routes) != 2 {
		t.Fatalf("removed %q cache %+v", name, m.file)
	}
}

func TestSaveRunsOnTheWorker(t *testing.T) {
	release := make(chan struct{})
	o := fakeOps(threeRoutes())
	add := o.add
	o.add = func(home, name, port string) (routes.File, error) {
		<-release
		return add(home, name, port)
	}
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, _, done := startLoop(t, &m, o)
	for _, k := range []string{"a", "w", "w", "w", "tab", "3", "0", "0", "0", "enter"} {
		term.keys <- k
	}
	waitFor(t, "saving", func() bool { return strings.Contains(term.shown(), "saving") })
	close(release)
	waitFor(t, "saved", func() bool {
		return strings.Contains(term.shown(), "saved") && term.row("www.ryangerardwilson.com") != ""
	})
	quit(t, term, done)
	if m.mode != modeList || len(m.file.Routes) != 4 || m.name.value != "" {
		t.Fatalf("mode %s cache %+v", m.mode, m.file)
	}
}

func TestSlowScanKeepsKeys(t *testing.T) {
	release := make(chan struct{})
	o := fakeOps(threeRoutes())
	o.service = func(context.Context) bool {
		<-release
		return true
	}
	m := newModel(t.TempDir(), threeRoutes(), false)
	term, _, done := startLoop(t, &m, o)
	term.keys <- "j"
	waitFor(t, "cursor on lab", func() bool { return strings.HasPrefix(term.row("lab.ryangerardwilson.com"), "> lab") })
	if !strings.Contains(term.shown(), "checking service") {
		t.Fatalf("scan answered early\n%s", term.shown())
	}
	close(release)
	waitFor(t, "scan", func() bool { return strings.Contains(term.shown(), "service running") })
	quit(t, term, done)
}

func TestWorkerReportsATimeoutAndKeepsTheLateAnswer(t *testing.T) {
	release := make(chan struct{})
	w := startWorker()
	defer w.stop()
	w.push(job{kind: jobRemove, name: "lab", label: "remove lab", wait: 10 * time.Millisecond, run: func(context.Context) result {
		<-release
		return result{file: threeRoutes()}
	}})
	first := <-w.results
	if first.err == nil || first.err.Error() != "remove lab timed out" || first.late || first.name != "lab" {
		t.Fatalf("first %+v", first)
	}
	close(release)
	late := <-w.results
	if late.err != nil || !late.late || len(late.file.Routes) != 3 {
		t.Fatalf("late %+v", late)
	}
}
