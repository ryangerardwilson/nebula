package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ryangerardwilson/nebula/tunnel/internal/routes"
	"github.com/ryangerardwilson/nebula/tunnel/internal/secrets"
	"github.com/ryangerardwilson/nebula/tunnel/internal/session"
	"github.com/ryangerardwilson/nebula/tunnel/internal/store"
)

const (
	scanEvery = 2 * time.Second
	keyWait   = 100 * time.Millisecond
	quitWait  = 5 * time.Second
	drainTick = 20 * time.Millisecond
)

// Run opens the route editor on the terminal.
func Run(home string, load func() (secrets.Tunnel, error), ensure store.Ensure, sess *session.Session) error {
	file, err := routes.EnsureFile(routes.Path(home))
	if err != nil {
		return err
	}
	in, out := os.Stdin, os.Stdout
	restore, err := enterRaw(int(in.Fd()))
	if err != nil {
		return fmt.Errorf("nebula tunnel screen needs a terminal")
	}
	defer restore()
	m := newModel(home, file, os.Getenv("NO_COLOR") == "")
	m.restore(sess)
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(out, "\x1b[0m\x1b[?25h\x1b[?1049l")
	return loop(&m, screen{
		keys:  func(wait time.Duration) (string, bool, error) { return readKey(in, wait) },
		out:   out,
		ops:   liveOps(load, ensure),
		place: func(cursor string) error { return sess.Place("", cursor) },
	})
}

// screen is what the loop talks to: keys in, frames out, the worker's I/O,
// and the session place writer. Tests replace each one.
type screen struct {
	keys  func(wait time.Duration) (string, bool, error)
	out   io.Writer
	ops   ops
	place func(cursor string) error
}

// live is one open screen: the model, the worker that does its I/O, and
// the session place writer. Only the loop goroutine touches it.
type live struct {
	m        *model
	s        screen
	work     *worker
	placer   *placer
	frame    string    // the frame on screen
	placed   string    // the place last handed to placer
	next     time.Time // when the next scan is due
	scanning bool      // a scan is queued or running
	writes   int       // adds and removes not answered yet
	quitting bool
}

// loop paints, reads one key, and applies whatever the worker answered. A
// pass with no key does no I/O beyond painting a changed frame.
func loop(m *model, s screen) error {
	l := &live{m: m, s: s, work: startWorker(), placed: m.place()}
	l.placer = startPlacer(s.place, l.placed)
	defer l.work.stop()
	defer l.placer.stop()
	l.scan()
	for {
		l.paint()
		k, ok, err := s.keys(keyWait)
		if err != nil {
			return err
		}
		if ok && l.key(k) == actQuit {
			l.drain()
			return nil
		}
		l.answers()
		if time.Now().After(l.next) {
			l.scan()
		}
	}
}

// key applies one key. I/O goes to the worker, and the next frame already
// shows the intent.
func (l *live) key(k string) action {
	act := l.m.key(k)
	switch act {
	case actSave:
		l.write(saveJob(l.s.ops, l.m.home, l.m.name.value, l.m.port.value))
	case actRemove:
		l.write(removeJob(l.s.ops, l.m.home, l.m.confirm))
	}
	l.note()
	return act
}

// write queues an add or a remove. A full queue drops the intent at once.
func (l *live) write(j job) {
	if l.work.push(j) {
		l.writes++
		return
	}
	l.m.answer(result{kind: j.kind, name: j.name, err: errBusy})
}

// scan queues a service and port check unless one is already waiting.
func (l *live) scan() {
	l.next = time.Now().Add(scanEvery)
	if l.scanning || l.quitting {
		return
	}
	l.scanning = l.work.push(scanJob(l.s.ops, l.m.file.Routes))
}

// answers applies every worker result that has arrived, without waiting.
func (l *live) answers() {
	for {
		select {
		case r := <-l.work.results:
			l.apply(r)
		default:
			return
		}
	}
}

func (l *live) apply(r result) {
	if !r.late {
		if r.kind == jobScan {
			l.scanning = false
		} else {
			l.writes--
		}
	}
	l.m.answer(r)
	if r.kind != jobScan && r.err == nil {
		l.scan()
	}
	l.note()
}

// note hands the session writer the place when it changed. It never waits.
func (l *live) note() {
	if place := l.m.place(); place != l.placed {
		l.placed = place
		l.placer.set(place)
	}
}

// drain gives pending adds, removes, and the session place a bounded time
// to finish, painting how many are left.
func (l *live) drain() {
	l.quitting = true
	l.m.help = false
	deadline := time.Now().Add(quitWait)
	for {
		n := l.writes + l.placer.pending()
		if n == 0 || time.Now().After(deadline) {
			return
		}
		l.m.notice = fmt.Sprintf("saving %d", n)
		l.paint()
		select {
		case r := <-l.work.results:
			l.apply(r)
		case <-time.After(drainTick):
		}
	}
}

func (l *live) paint() {
	l.frame = paint(l.s.out, l.m.View(), l.frame)
}

// paint draws one frame from the top left, unless it equals last. Each
// line is erased to its end, and the rest of the screen is cleared. It
// returns the frame now on screen.
func paint(out io.Writer, frame, last string) string {
	if frame == last {
		return last
	}
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	var b strings.Builder
	b.WriteString("\x1b[H")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line + "\x1b[K")
	}
	b.WriteString("\x1b[J")
	_, _ = io.WriteString(out, b.String())
	return frame
}
