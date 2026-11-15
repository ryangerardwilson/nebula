package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	authEndpoint  = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenEndpoint = "https://oauth2.googleapis.com/token"
)

func authorizeURL(clientID, redirect, state, challenge string, scopes []string) string {
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("redirect_uri", redirect)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	query.Set("access_type", "offline")
	return authEndpoint + "?" + query.Encode()
}

func newChallenge() (verifier, challenge, state string, err error) {
	verifier, err = randomToken(32)
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	state, err = randomToken(16)
	if err != nil {
		return "", "", "", err
	}
	return verifier, challenge, state, nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func loopback(redirect string) bool {
	parsed, err := url.Parse(redirect)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return parsed.Scheme == "http" && (host == "127.0.0.1" || host == "localhost")
}

// Exchange trades a code for a session. The client secret stays in the request body.
func Exchange(ctx context.Context, client *http.Client, endpoint, id, secret, code, redirect, verifier string) (Session, error) {
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", id)
	form.Set("client_secret", secret)
	form.Set("redirect_uri", redirect)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Session{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Session{}, reject(body)
	}
	return sessionFrom(body)
}

var errorCode = regexp.MustCompile(`^[a-z0-9_]+$`)

func reject(body []byte) error {
	var bad struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &bad)
	if !errorCode.MatchString(bad.Error) {
		bad.Error = "rejected"
	}
	return fmt.Errorf("google rejected the code: %s", bad.Error)
}

func sessionFrom(body []byte) (Session, error) {
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return Session{}, fmt.Errorf("google returned no access token")
	}
	session := Session{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		IDToken:      tok.IDToken,
		Email:        emailFrom(tok.IDToken),
	}
	if tok.ExpiresIn > 0 {
		session.Expiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return session, nil
}

func emailFrom(idToken string) string {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var body struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ""
	}
	return body.Email
}

func waitCode(ctx context.Context, redirect, state string, ready func()) (string, error) {
	parsed, err := url.Parse(redirect)
	if err != nil {
		return "", err
	}
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	ln, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		return "", err
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{Handler: callback(path, state, codeCh, errCh)}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()
	if ready != nil {
		ready()
	}
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("login timed out")
	case err := <-errCh:
		return "", err
	case code := <-codeCh:
		return code, nil
	}
}

func callback(path, state string, codeCh chan string, errCh chan error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		if query.Get("state") != state {
			http.Error(w, "state does not match", http.StatusBadRequest)
			sendErr(errCh, fmt.Errorf("state does not match"))
			return
		}
		if query.Get("error") != "" {
			http.Error(w, "google denied the login", http.StatusBadRequest)
			sendErr(errCh, fmt.Errorf("google denied the login"))
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "signed in. you can close this window.\n")
		select {
		case codeCh <- code:
		default:
		}
	})
}

func sendErr(errCh chan error, err error) {
	select {
	case errCh <- err:
	default:
	}
}

func signedInLine(name, email string) string {
	if email == "" {
		return fmt.Sprintf("signed in %s\n", name)
	}
	return fmt.Sprintf("signed in %s as %s\n", name, email)
}
