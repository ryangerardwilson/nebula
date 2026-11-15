package run

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

const edgeFixture = `tunnel: f974019b-5ca0-4ca4-925a-628c3235708f
credentials-file: /etc/cloudflared/cred.json
protocol: http2
ingress:
  - hostname: ssh.ryangerardwilson.com
    service: http://127.0.0.1:8022
  - service: http_status:404
`

const edgeUnitText = "ExecStart=/usr/bin/cloudflared --no-autoupdate tunnel --config /etc/cloudflared/config.yml run\n"

func TestInsertIngressKeepsSSH(t *testing.T) {
	next, err := InsertIngress(edgeFixture, "app.ryangerardwilson.com", "http://127.0.0.1:18100")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(next, "hostname: ssh.ryangerardwilson.com") || !strings.Contains(next, "service: http://127.0.0.1:8022") {
		t.Fatalf("ssh rule changed\n%s", next)
	}
	if !strings.Contains(next, "hostname: app.ryangerardwilson.com") || !strings.Contains(next, "service: http://127.0.0.1:18100") {
		t.Fatalf("app rule missing\n%s", next)
	}
	ssh := strings.Index(next, "ssh.ryangerardwilson.com")
	app := strings.Index(next, "app.ryangerardwilson.com")
	catch := strings.Index(next, "http_status:404")
	if ssh < 0 || app < ssh || catch < app {
		t.Fatalf("order ssh %d app %d catch %d", ssh, app, catch)
	}
	again, err := InsertIngress(next, "app.ryangerardwilson.com", "http://127.0.0.1:18101")
	if err != nil || !strings.Contains(again, "service: http://127.0.0.1:18101") || !strings.Contains(again, "service: http://127.0.0.1:8022") {
		t.Fatalf("update %v\n%s", err, again)
	}
	if _, err := InsertIngress(edgeFixture, "ssh.ryangerardwilson.com", "http://127.0.0.1:18100"); err == nil {
		t.Fatal("ssh hostname was accepted")
	}
	if _, err := InsertIngress("tunnel: x\n", "app.ryangerardwilson.com", "http://127.0.0.1:18100"); err == nil {
		t.Fatal("config without ssh was accepted")
	}
}

func TestPortsAndRollbackChoice(t *testing.T) {
	port, text, err := Remember("", "site", "app.ryangerardwilson.com", "20261115T000000Zbbbb")
	if err != nil || port != 18100 || !strings.Contains(text, "site 18100") {
		t.Fatalf("%d %s %v", port, text, err)
	}
	port, text, err = Remember(text, "other", "other.ryangerardwilson.com", "20261115T000000Zbbbb")
	if err != nil || port != 18101 {
		t.Fatalf("second %d %v", port, err)
	}
	port, _, err = Remember(text, "site", "app.ryangerardwilson.com", "20261115T000000Zbbbb")
	if err != nil || port != 18100 {
		t.Fatalf("kept %d %v", port, err)
	}
	older, err := Earlier([]string{"20261115T000000Zbbbb", "20261114T000000Zaaaa"}, "20261115T000000Zbbbb")
	if err != nil || older != "20261114T000000Zaaaa" {
		t.Fatalf("earlier %s %v", older, err)
	}
	if _, err := Earlier([]string{"20261114T000000Zaaaa"}, "20261114T000000Zaaaa"); err == nil {
		t.Fatal("oldest release rolled back")
	}
}

func TestProjectFile(t *testing.T) {
	file, err := Parse("name = site\nbuild = \"go build -o site .\"\nstart = ./site\nhostname = app.ryangerardwilson.com\n")
	if err != nil || file.Build != "go build -o site ." || file.Name != "site" {
		t.Fatalf("%+v %v", file, err)
	}
	if _, err := normalizeHost("@"); err == nil {
		t.Fatal("apex was accepted")
	}
	if _, err := normalizeHost("ssh"); err == nil {
		t.Fatal("ssh name was accepted")
	}
	if _, err := normalizeHost("notes.ryangerardwilson.com"); err == nil {
		t.Fatal("notes was accepted")
	}
	host, err := normalizeHost("App")
	if err != nil || host != "app.ryangerardwilson.com" {
		t.Fatalf("host %s %v", host, err)
	}
}

func TestDeployWritesTheAppAndKeepsSSH(t *testing.T) {
	box := newMem()
	box.files[edgeUnitFile] = edgeUnitText
	box.files["/etc/cloudflared/config.yml"] = edgeFixture
	var gotID string
	spec := Spec{
		Name: "site", Hostname: "app.ryangerardwilson.com", Start: "./site", Dir: t.TempDir(),
		DNS: func(_ context.Context, hostname, id string) error {
			if hostname != "app.ryangerardwilson.com" {
				t.Fatalf("dns host %s", hostname)
			}
			gotID = id
			return nil
		},
	}
	res, err := Deploy(context.Background(), box, spec, "20261115T010203Zabcd")
	if err != nil {
		t.Fatal(err)
	}
	if res.Port != 18100 || gotID != "f974019b-5ca0-4ca4-925a-628c3235708f" {
		t.Fatalf("%+v id %s", res, gotID)
	}
	next := box.files["/etc/cloudflared/config.yml.nebula-next"]
	if !strings.Contains(next, "127.0.0.1:8022") || !strings.Contains(next, "app.ryangerardwilson.com") {
		t.Fatalf("next config\n%s", next)
	}
	unit := box.files[unitPath("site")]
	if !strings.Contains(unit, "PORT=18100") || !strings.Contains(unit, "HOST=127.0.0.1") || !strings.Contains(unit, "exec ./site") {
		t.Fatalf("unit\n%s", unit)
	}
	joined := strings.Join(box.scripts, "\n")
	if !strings.Contains(joined, "ln -sfn") || !strings.Contains(joined, "systemd-run") || !strings.Contains(joined, "127.0.0.1:8022") {
		t.Fatalf("scripts\n%s", joined)
	}
	if len(box.uploads) != 1 || !strings.Contains(box.uploads[0], "20261115T010203Zabcd") {
		t.Fatalf("upload %#v", box.uploads)
	}
}

func TestDNSRefusalDoesNotWriteTheEdge(t *testing.T) {
	box := newMem()
	box.files[edgeUnitFile] = edgeUnitText
	box.files["/etc/cloudflared/config.yml"] = edgeFixture
	spec := Spec{
		Name: "site", Hostname: "app.ryangerardwilson.com", Start: "./site", Dir: t.TempDir(),
		DNS: func(context.Context, string, string) error {
			return errors.New("dns points somewhere else")
		},
	}
	_, err := Deploy(context.Background(), box, spec, "20261115T010203Zabcd")
	if err == nil || !strings.Contains(err.Error(), "points somewhere else") {
		t.Fatal(err)
	}
	if _, ok := box.files["/etc/cloudflared/config.yml.nebula-next"]; ok {
		t.Fatal("edge file was written after a dns refusal")
	}
	if strings.Contains(box.files["/etc/cloudflared/config.yml"], "app.ryangerardwilson.com") {
		t.Fatal("live edge config changed")
	}
}

func TestRollbackMovesTheRelease(t *testing.T) {
	box := newMem()
	box.files[portsPath] = "site 18100 app.ryangerardwilson.com 20261115T000000Zbbbb\n"
	box.out = func(script string) (string, error) {
		if strings.Contains(script, "readlink") {
			return "releases/20261115T000000Zbbbb\n", nil
		}
		if strings.Contains(script, "ls -1") {
			return "20261114T000000Zaaaa\n20261115T000000Zbbbb\n", nil
		}
		return "", nil
	}
	id, err := Rollback(context.Background(), box, "site")
	if err != nil || id != "20261114T000000Zaaaa" {
		t.Fatalf("%s %v", id, err)
	}
	if !strings.Contains(box.files[portsPath], "20261114T000000Zaaaa") {
		t.Fatalf("ports %s", box.files[portsPath])
	}
	if !strings.Contains(strings.Join(box.scripts, "\n"), "20261114T000000Zaaaa") {
		t.Fatal(box.scripts)
	}
	text, err := List(context.Background(), box)
	if err != nil || !strings.Contains(text, "site") || !strings.Contains(text, "18100") {
		t.Fatalf("list %s %v", text, err)
	}
}

func TestHelpHasNoCommandsSection(t *testing.T) {
	var out, errw strings.Builder
	if code := runCLI(nil, &out, &errw, nil); code != 0 {
		t.Fatal(code)
	}
	if strings.Contains(out.String(), "\ncommands:\n") || !strings.Contains(out.String(), "nebula run ./site") || !strings.Contains(out.String(), "OAuth callback") {
		t.Fatalf("%s", out.String())
	}
	if code := runCLI([]string{"logs"}, &out, &errw, nil); code != 2 {
		t.Fatal(code)
	}
}

type memBox struct {
	files   map[string]string
	scripts []string
	uploads []string
	out     func(string) (string, error)
}

func newMem() *memBox {
	return &memBox{files: map[string]string{}}
}

func (m *memBox) Read(_ context.Context, path string) (string, error) {
	body, ok := m.files[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return body, nil
}

func (m *memBox) Write(_ context.Context, path, body string) error {
	m.files[path] = body
	return nil
}

func (m *memBox) Upload(_ context.Context, _, remote string) error {
	m.uploads = append(m.uploads, remote)
	return nil
}

func (m *memBox) Output(_ context.Context, script string) (string, error) {
	m.scripts = append(m.scripts, script)
	if m.out != nil {
		return m.out(script)
	}
	return "", nil
}
