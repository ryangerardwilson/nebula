package session

import (
	"errors"
	"strings"
	"testing"
)

func TestSessionAsksC3po(t *testing.T) {
	var calls []string
	orig := runC3po
	t.Cleanup(func() { runC3po = orig })
	runC3po = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if args[0] == "open" && len(args) == 4 {
			return []byte(`{"id":"01a17622-5fd9-7bc2-81ff-3e200a8c16b7","restored":true,"place":{"screen":"tunnel","path":"routes","cursor":"www"}}`), nil
		}
		if args[0] == "open" {
			return []byte(`{"id":"01a17622-5fd9-7bc2-81ff-3e200a8c16b7","restored":false}`), nil
		}
		return nil, nil
	}
	sess, err := Open("tunnel", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sess.Restored(); ok || sess.ID() == "" {
		t.Fatalf("new session restored %v id %q", ok, sess.ID())
	}
	if err := sess.Place("routes", "www"); err != nil {
		t.Fatal(err)
	}
	if err := sess.Note(Body{Args: []string{"-a"}}); err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open("tunnel", sess.ID())
	if err != nil {
		t.Fatal(err)
	}
	if body, ok := again.Restored(); !ok || body.Path != "routes" || body.Cursor != "www" {
		t.Fatalf("restored %+v %v", body, ok)
	}
	want := []string{"open tunnel", "place " + sess.ID(), "note " + sess.ID(), "close " + sess.ID(), "open tunnel"}
	if len(calls) != len(want) {
		t.Fatalf("calls %q", calls)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(calls[i], prefix) {
			t.Fatalf("call %d %q, want %q", i, calls[i], prefix)
		}
	}
	if !strings.HasSuffix(calls[2], `{"screen":"","args":["-a"]}`) {
		t.Fatalf("note %q", calls[2])
	}
}

func TestOpenReportsC3poErrors(t *testing.T) {
	orig := runC3po
	t.Cleanup(func() { runC3po = orig })
	runC3po = func(args ...string) ([]byte, error) { return nil, errors.New("c3po: session x was not found") }
	if _, err := Open("tunnel", "01a17622-5fd9-7bc2-81ff-3e200a8c16b7"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err %v", err)
	}
	if _, err := Open("tunnel", "bad"); err == nil {
		t.Fatal("bad id")
	}
}
