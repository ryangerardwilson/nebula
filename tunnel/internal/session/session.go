// Package session keeps the tunnel screen's session. c3po owns screen
// sessions, so this package asks c3po to open, note, place, and close one
// through c3po __session, and c3po is their one writer. The session stays
// after the screen closes so a later --resume can open the same screen.
package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// c3poWait bounds one c3po __session call, so a stuck c3po cannot hold a
// screen's session writer for good.
const c3poWait = 10 * time.Second

// runC3po runs c3po __session with args and returns its stdout.
var runC3po = func(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c3poWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, "c3po", append([]string{"__session"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

// Body is the durable screen state. Cursor is a stable row key. Path is a
// directory, a file, or a parent id. Args are the flags a one-shot screen
// should replay. Secrets are not stored.
type Body struct {
	Screen string   `json:"screen"`
	CWD    string   `json:"cwd,omitempty"`
	Path   string   `json:"path,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
	Args   []string `json:"args,omitempty"`
}

// Session is one open screen. A nil Session ignores Note, Place, and Close
// so model tests that never open a session stay quiet.
type Session struct {
	id       string
	screen   string
	body     Body
	restored bool
}

// TakeResume removes one --resume <id> or --resume=<id> from args.
// A bare --resume is an error. The id must be a session UUID.
func TakeResume(args []string) (string, []string, error) {
	var id string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--resume":
			if id != "" {
				return "", nil, errors.New("nebula: --resume was given twice")
			}
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", nil, errors.New("nebula: --resume needs a session id")
			}
			i++
			id = args[i]
			if !idPattern.MatchString(id) {
				return "", nil, errors.New("nebula: --resume needs a session id")
			}
		case strings.HasPrefix(arg, "--resume="):
			if id != "" {
				return "", nil, errors.New("nebula: --resume was given twice")
			}
			id = strings.TrimPrefix(arg, "--resume=")
			if !idPattern.MatchString(id) {
				return "", nil, errors.New("nebula: --resume needs a session id")
			}
		default:
			rest = append(rest, arg)
		}
	}
	return id, rest, nil
}

// Open asks c3po to load id or create a session for this process. A missing
// id returns an error so restore does not start an empty screen.
func Open(screen, id string) (*Session, error) {
	if screen == "" {
		return nil, errors.New("nebula: session screen is empty")
	}
	if id != "" && !idPattern.MatchString(id) {
		return nil, errors.New("nebula: --resume needs a session id")
	}
	args := []string{"open", screen, pid()}
	if id != "" {
		args = append(args, id)
	}
	out, err := runC3po(args...)
	if err != nil {
		return nil, err
	}
	var opened struct {
		ID       string `json:"id"`
		Restored bool   `json:"restored"`
		Place    *Body  `json:"place"`
	}
	if err := json.Unmarshal(out, &opened); err != nil || opened.ID == "" {
		return nil, errors.New("nebula: c3po __session open printed no session")
	}
	sess := &Session{id: opened.ID, screen: screen, body: Body{Screen: screen}}
	if opened.Restored && opened.Place != nil {
		sess.body = *opened.Place
		sess.restored = true
	}
	return sess, nil
}

// Restored returns the saved body when this session was opened by id.
func (s *Session) Restored() (Body, bool) {
	if s == nil || !s.restored {
		return Body{}, false
	}
	return s.body, true
}

// ID is the session id a later --resume uses.
func (s *Session) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

// Note merges non-empty fields into the session and asks c3po to store them.
// A non-nil Args slice replaces the saved args.
func (s *Session) Note(note Body) error {
	if s == nil {
		return nil
	}
	if note.CWD != "" {
		s.body.CWD = note.CWD
	}
	if note.Path != "" {
		s.body.Path = note.Path
	}
	if note.Cursor != "" {
		s.body.Cursor = note.Cursor
	}
	if note.Args != nil {
		s.body.Args = append([]string(nil), note.Args...)
	}
	raw, err := json.Marshal(note)
	if err != nil {
		return err
	}
	_, err = runC3po("note", s.id, pid(), string(raw))
	return err
}

// Place sets the path and cursor even when they are empty and asks c3po to
// store them.
func (s *Session) Place(path, cursor string) error {
	if s == nil {
		return nil
	}
	s.body.Path = path
	s.body.Cursor = cursor
	_, err := runC3po("place", s.id, pid(), path, cursor)
	return err
}

// Index returns the row whose key matches the restored cursor.
func (s *Session) Index(keys []string) (int, bool) {
	if s == nil || !s.restored || s.body.Cursor == "" {
		return 0, false
	}
	for i, key := range keys {
		if key == s.body.Cursor {
			return i, true
		}
	}
	return 0, false
}

// Close asks c3po to drop the active row and keep the session.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	_, err := runC3po("close", s.id)
	return err
}

func pid() string {
	return strconv.Itoa(os.Getpid())
}

// Interactive reports whether w is a terminal. Tests that pass a buffer
// are not interactive, so a screen can skip opening a session for them.
func Interactive(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
