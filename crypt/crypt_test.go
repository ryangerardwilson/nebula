package crypt

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/ryangerardwilson/nebula/crypt/internal/cli"
	"github.com/ryangerardwilson/nebula/crypt/internal/clip"
	"github.com/ryangerardwilson/nebula/crypt/internal/op"
)

func TestHelpOmitsCommandsAndAScreen(t *testing.T) {
	var out, errw bytes.Buffer
	if code := Run(nil, &out, &errw); code != 0 || out.String() != Help {
		t.Fatalf("code %d", code)
	}
	if strings.Contains(Help, "\ncommands:\n") || strings.Contains(Help, "crypt screen") {
		t.Fatal(Help)
	}
	for _, needle := range []string{"nebula crypt vaults", "nebula crypt items Root", "nebula crypt copy Root ITEM", "crypt delete <vault> item <id>", "c3po tui pass"} {
		if !strings.Contains(Help, needle) {
			t.Fatalf("missing %s", needle)
		}
	}
}

func TestWordForms(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"list", "Root"}, []string{"Root", "-l"}},
		{[]string{"get", "Root", "GH_EXAMPLE"}, []string{"Root", "GH_EXAMPLE"}},
		{[]string{"describe", "Root", "GH_EXAMPLE"}, []string{"Root", "--show", "GH_EXAMPLE"}},
		{[]string{"create", "Root", "GH_EXAMPLE"}, []string{"Root", "--create", "--title", "GH_EXAMPLE", "--category", "api-credential"}},
		{[]string{"create", "Root", "GH_EXAMPLE", "login", "ada"}, []string{"Root", "--create", "--title", "GH_EXAMPLE", "--category", "login", "--username", "ada"}},
		{[]string{"create", "Root", "template", "item.json"}, []string{"Root", "--create", "--template", "item.json"}},
		{[]string{"put", "Root", "GH_EXAMPLE", "name", "NEXT"}, []string{"Root", "--edit", "GH_EXAMPLE", "--title", "NEXT"}},
		{[]string{"put", "Root", "GH_EXAMPLE", "template", "item.json"}, []string{"Root", "--edit", "GH_EXAMPLE", "--template", "item.json"}},
		{[]string{"delete", "Root", "GH_EXAMPLE"}, []string{"Root", "--delete", "GH_EXAMPLE"}},
	}
	for _, tc := range cases {
		got, err := legacyArgs(tc.in)
		if err != nil || strings.Join(got, " ") != strings.Join(tc.want, " ") {
			t.Fatalf("%v -> %v %v", tc.in, got, err)
		}
	}
	if _, err := legacyArgs([]string{"put", "Root", "GH_EXAMPLE", "template", "item.json", "name", "NEXT"}); err == nil {
		t.Fatal("template and name were both accepted")
	}
}

func TestItemsHideAConcealedValue(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
	body := `[{"id":"abc","title":"GH_EXAMPLE","category":"LOGIN","vault":{"id":"v1"},"additional_information":"ada","fields":[{"label":"password","value":"super-secret-value"},{"label":"username","value":"ada"}]}]`
	fake := &fakeOp{out: map[string][]byte{
		"item list --vault Root --long --format=json": []byte(body),
	}}
	withRunner(t, fake, &fakeClip{})
	var out, errw bytes.Buffer
	if code := Run([]string{"items", "Root"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	text := out.String()
	if strings.Contains(text, "super-secret-value") || !strings.Contains(text, "GH_EXAMPLE") || !strings.Contains(text, "ada") {
		t.Fatal(text)
	}
}

func TestCopyPrintsNothing(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
	body := `[{"id":"abc","title":"GH_EXAMPLE","category":"API_CREDENTIAL","vault":{"id":"v1"},"fields":[{"label":"credential","value":"super-secret-value"}]}]`
	fake := &fakeOp{out: map[string][]byte{
		"item list --vault Root --long --format=json": []byte(body),
		"read --no-newline op://v1/abc/credential":    []byte("super-secret-value"),
	}}
	clip := &fakeClip{}
	withRunner(t, fake, clip)
	var out, errw bytes.Buffer
	if code := Run([]string{"copy", "Root", "abc"}, &out, &errw); code != 0 || out.Len() != 0 || errw.Len() != 0 {
		t.Fatalf("code %d out %q err %q", code, out.String(), errw.String())
	}
	if clip.text != "super-secret-value" || strings.Contains(out.String()+errw.String(), "super-secret-value") {
		t.Fatal("secret leaked or was not copied")
	}
}

func TestGetReadsTheExactTitle(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
	body := `[{"id":"id-1","title":"GOOGLE_OAUTH_CLIENT_ID","category":"API_CREDENTIAL","vault":{"id":"v1"},"fields":[{"value":"super-secret-value"}]},{"id":"id-2","title":"Other","category":"API_CREDENTIAL","vault":{"id":"v1"}}]`
	fake := &fakeOp{out: map[string][]byte{
		"item list --vault Root --long --format=json": []byte(body),
		"read --no-newline op://v1/id-1/credential":   []byte("client-id-value"),
	}}
	withRunner(t, fake, &fakeClip{})
	got, err := Get("Root", "GOOGLE_OAUTH_CLIENT_ID")
	if err != nil || got != "client-id-value" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := Get("Root", "MISSING"); err == nil || err.Error() != "No item titled MISSING in vault Root." || strings.Contains(err.Error(), "super-secret-value") {
		t.Fatal(err)
	}
}

func TestDeleteByIDDoesNotListTheSecret(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
	fake := &fakeOp{out: map[string][]byte{
		"item delete abc --vault Root": []byte("super-secret-value"),
	}}
	withRunner(t, fake, &fakeClip{})
	var out, errw bytes.Buffer
	if code := Run([]string{"delete", "Root", "item", "abc"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if out.String() != "Deleted abc from vault Root.\n" || strings.Contains(out.String()+errw.String(), "super-secret-value") {
		t.Fatalf("out %q err %q", out.String(), errw.String())
	}
}

func TestLegacyListHidesFieldValues(t *testing.T) {
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "test-token")
	body := `[{"id":"abc","title":"GH_EXAMPLE","category":"API_CREDENTIAL","fields":[{"value":"super-secret-value"}]}]`
	fake := &fakeOp{out: map[string][]byte{
		"item list --vault Root --long --format=json": []byte(body),
	}}
	withRunner(t, fake, &fakeClip{})
	var out, errw bytes.Buffer
	if code := Run([]string{"Root", "-l"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if strings.Contains(out.String(), "super-secret-value") || !strings.Contains(out.String(), "GH_EXAMPLE") {
		t.Fatal(out.String())
	}
}

type fakeOp struct {
	out map[string][]byte
}

func (f *fakeOp) LookPath() error { return nil }

func (f *fakeOp) Run(_ []string, _ []byte, _ io.Writer, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	body, ok := f.out[key]
	if !ok {
		return nil, errUnexpected(key)
	}
	return body, nil
}

type unexpected string

func errUnexpected(key string) error { return unexpected(key) }

func (e unexpected) Error() string { return "unexpected op " + string(e) }

type fakeClip struct{ text string }

func (f *fakeClip) Copy(text string, _ io.Writer) error {
	f.text = text
	return nil
}

func withRunner(t *testing.T, opRunner op.Runner, board clip.Clipboard) {
	t.Helper()
	prev := runnerFactory
	runnerFactory = func(out, errw io.Writer) cli.Runner {
		return cli.Runner{Out: out, Err: errw, Op: opRunner, Clip: board, Help: Help, Version: "0.1.0"}
	}
	t.Cleanup(func() { runnerFactory = prev })
}
