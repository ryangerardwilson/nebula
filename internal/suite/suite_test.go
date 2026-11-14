package suite

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpNamesBothTools(t *testing.T) {
	var out, errw bytes.Buffer
	if code := run(nil, &out, &errw); code != 0 {
		t.Fatal(code)
	}
	text := out.String()
	if strings.Contains(text, "\ncommands:\n") {
		t.Fatal(text)
	}
	for _, want := range []string{
		"nebula sql create database app",
		"nebula s3 put ./today.txt s3://notes/today.txt",
		"features:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s\n%s", want, text)
		}
	}
	out.Reset()
	if code := run([]string{"version"}, &out, &errw); code != 0 || strings.TrimSpace(out.String()) != Version {
		t.Fatalf("%d %q", code, out.String())
	}
	if code := run([]string{"sql"}, &out, &errw); code != 2 {
		t.Fatal(code)
	}
}
