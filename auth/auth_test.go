package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHelpOmitsCommands(t *testing.T) {
	var out, errw strings.Builder
	if code := Run(nil, &out, &errw); code != 0 || out.String() != Help {
		t.Fatalf("code %d", code)
	}
	if strings.Contains(Help, "\ncommands:\n") || !strings.Contains(Help, "does not create the client") || !strings.Contains(Help, "GOOGLE_OAUTH_CLIENT_SECRET") {
		t.Fatal("help drifted")
	}
}

func TestSetupKeepsTheSecretOutOfTheFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEBULA_AUTH_ROOT", root)
	restoreGet(t, func(vault, title string) (string, error) {
		if vault != "Root" {
			t.Fatalf("vault %s", vault)
		}
		switch title {
		case "GOOGLE_OAUTH_CLIENT_ID":
			return "client-id-value", nil
		case "GOOGLE_OAUTH_CLIENT_SECRET":
			return "client-secret-value", nil
		default:
			return "", fmt.Errorf("No item titled %s in vault %s.", title, vault)
		}
	})
	var out, errw strings.Builder
	if code := Run([]string{"setup"}, &out, &errw); code != 0 || out.String() != "client recorded\n" {
		t.Fatalf("code %d out %q err %q", code, out.String(), errw.String())
	}
	raw, err := os.ReadFile(filepath.Join(root, "client.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "client-secret-value") || !strings.Contains(string(raw), "client-id-value") {
		t.Fatal(string(raw))
	}
	info, err := os.Stat(filepath.Join(root, "client.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestLoginDoesNotPrintTheToken(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEBULA_AUTH_ROOT", root)
	const secret = "client-secret-value"
	const access = "ACCESS-SECRET"
	restoreGet(t, func(_, title string) (string, error) {
		switch title {
		case "GOOGLE_OAUTH_CLIENT_ID":
			return "client-id-value", nil
		case "GOOGLE_OAUTH_CLIENT_SECRET":
			return secret, nil
		default:
			return "", fmt.Errorf("missing %s", title)
		}
	})
	var out, errw strings.Builder
	if code := Run([]string{"setup"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	port := freePort(t)
	redirect := fmt.Sprintf("http://127.0.0.1:%d/cb", port)
	out.Reset()
	if code := Run([]string{"add", "notes", "redirect", redirect}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "client_secret="+secret) {
			http.Error(w, "missing secret", http.StatusBadRequest)
			return
		}
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"ada@example.com"}`))
		fmt.Fprintf(w, `{"access_token":"%s","id_token":"hdr.%s.sig","expires_in":3600}`, access, payload)
	}))
	defer srv.Close()
	prevURL, prevClient, prevOpen, prevWait := tokenURL, httpClient, openBrowser, listenTimeout
	tokenURL = srv.URL
	httpClient = srv.Client()
	listenTimeout = 3 * time.Second
	openBrowser = func(raw string) error {
		parsed, err := url.Parse(raw)
		if err != nil {
			return err
		}
		callback, err := url.Parse(parsed.Query().Get("redirect_uri"))
		if err != nil {
			return err
		}
		query := callback.Query()
		query.Set("code", "auth-code")
		query.Set("state", parsed.Query().Get("state"))
		callback.RawQuery = query.Encode()
		var resp *http.Response
		var getErr error
		for i := 0; i < 30; i++ {
			resp, getErr = http.Get(callback.String())
			if getErr == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if getErr != nil {
			return getErr
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	t.Cleanup(func() {
		tokenURL, httpClient, openBrowser, listenTimeout = prevURL, prevClient, prevOpen, prevWait
	})
	out.Reset()
	errw.Reset()
	if code := Run([]string{"login", "notes"}, &out, &errw); code != 0 {
		t.Fatalf("code %d out %q err %q", code, out.String(), errw.String())
	}
	if out.String() != "signed in notes as ada@example.com\n" {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String()+errw.String(), access) || strings.Contains(out.String()+errw.String(), secret) {
		t.Fatal("token or secret was printed")
	}
	saved, err := os.ReadFile(filepath.Join(root, "sessions", "notes.json"))
	if err != nil || !strings.Contains(string(saved), access) {
		t.Fatalf("session %s %v", saved, err)
	}
}

func TestRedirectRulesAndScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEBULA_AUTH_ROOT", root)
	var errw strings.Builder
	if code := Run([]string{"add", "Notes", "redirect", "https://app.example/cb"}, &strings.Builder{}, &errw); code == 0 {
		t.Fatal("bad name accepted")
	}
	errw.Reset()
	if code := Run([]string{"add", "notes", "redirect", "http://127.0.0.1/cb"}, &strings.Builder{}, &errw); code == 0 {
		t.Fatal("missing port accepted")
	}
	errw.Reset()
	if code := Run([]string{"add", "notes", "redirect", "https://app.example/cb?x=1"}, &strings.Builder{}, &errw); code == 0 {
		t.Fatal("query accepted")
	}
	var out strings.Builder
	if code := Run([]string{"add", "notes", "redirect", "http://127.0.0.1:9/cb"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if code := Run([]string{"add", "notes", "scope", "https://www.googleapis.com/auth/drive.readonly"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if code := Run([]string{"add", "notes", "redirect", "https://app.example/cb"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	var file File
	raw, err := os.ReadFile(filepath.Join(root, "products.json"))
	if err != nil || json.Unmarshal(raw, &file) != nil || len(file.Products) != 1 {
		t.Fatal(string(raw))
	}
	product := file.Products[0]
	if product.Redirect != "https://app.example/cb" || !contains(product.Scopes, "https://www.googleapis.com/auth/drive.readonly") || !contains(product.Scopes, "openid") {
		t.Fatalf("%#v", product)
	}
}

func TestExchangeRejectsABadState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NEBULA_AUTH_ROOT", root)
	restoreGet(t, func(_, title string) (string, error) {
		if title == "GOOGLE_OAUTH_CLIENT_ID" {
			return "client-id-value", nil
		}
		return "client-secret-value", nil
	})
	var out, errw strings.Builder
	if code := Run([]string{"setup"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	if code := Run([]string{"add", "notes", "redirect", "https://app.example/cb"}, &out, &errw); code != 0 {
		t.Fatal(errw.String())
	}
	out.Reset()
	if code := Run([]string{"url", "notes"}, &out, &errw); code != 0 || strings.Contains(out.String(), "client-secret-value") {
		t.Fatalf("out %q err %q", out.String(), errw.String())
	}
	if code := Run([]string{"exchange", "notes", "code", "wrong"}, &strings.Builder{}, &errw); code == 0 || !strings.Contains(errw.String(), "state does not match") {
		t.Fatal(errw.String())
	}
}

func restoreGet(t *testing.T, get func(string, string) (string, error)) {
	t.Helper()
	prev := secretGet
	secretGet = get
	t.Cleanup(func() { secretGet = prev })
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
