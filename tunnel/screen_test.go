package tunnel

import (
	"strings"
	"testing"
)

func TestScreenHelp(t *testing.T) {
	a := app{Home: t.TempDir()}
	out, errOut, code := run(&a, "screen", "help")
	if code != 0 || !strings.Contains(out, "nebula tunnel screen") || errOut != "" {
		t.Fatalf("help %d %s %s", code, out, errOut)
	}
	_, errOut, code = run(&a, "screen", "extra")
	if code != 2 || !strings.Contains(errOut, "nebula tunnel screen") {
		t.Fatalf("extra %d %s", code, errOut)
	}
}
