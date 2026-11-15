package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	idItemTitle     = "GOOGLE_OAUTH_CLIENT_ID"
	secretItemTitle = "GOOGLE_OAUTH_CLIENT_SECRET"
	rootVault       = "Root"
)

var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// Client is the public half of the shared Google client.
type Client struct {
	ClientID   string `json:"client_id"`
	IDItem     string `json:"id_item"`
	SecretItem string `json:"secret_item"`
}

// Product is one app allowed to use that client.
type Product struct {
	Name     string   `json:"name"`
	Redirect string   `json:"redirect"`
	Scopes   []string `json:"scopes"`
}

// File is the product list.
type File struct {
	Products []Product `json:"products"`
}

// Session is the Google token cache for one product.
type Session struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Email        string    `json:"email,omitempty"`
}

type pending struct {
	State    string    `json:"state"`
	Verifier string    `json:"verifier"`
	Redirect string    `json:"redirect"`
	Expires  time.Time `json:"expires"`
}

func rootDir() string {
	if root := os.Getenv("NEBULA_AUTH_ROOT"); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/tmp", "nebula-auth")
	}
	return filepath.Join(home, ".config", "nebula", "auth")
}

func checkName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("name must be a short lowercase word")
	}
	return nil
}

func loadFile(root string) (File, error) {
	raw, err := os.ReadFile(filepath.Join(root, "products.json"))
	if os.IsNotExist(err) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var file File
	if err := json.Unmarshal(raw, &file); err != nil {
		return File{}, err
	}
	return file, nil
}

func saveFile(root string, file File) error {
	if file.Products == nil {
		file.Products = []Product{}
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writePrivate(filepath.Join(root, "products.json"), raw)
}

func findProduct(file File, name string) (Product, bool) {
	for _, product := range file.Products {
		if product.Name == name {
			return product, true
		}
	}
	return Product{}, false
}

func putProduct(file File, product Product) File {
	for i, existing := range file.Products {
		if existing.Name == product.Name {
			file.Products[i] = product
			return file
		}
	}
	file.Products = append(file.Products, product)
	return file
}

func dropProduct(file File, name string) File {
	next := file.Products[:0]
	for _, product := range file.Products {
		if product.Name != name {
			next = append(next, product)
		}
	}
	file.Products = next
	return file
}

func loadClient(root string) (Client, error) {
	raw, err := os.ReadFile(filepath.Join(root, "client.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return Client{}, fmt.Errorf("client is not set up")
		}
		return Client{}, err
	}
	var client Client
	if err := json.Unmarshal(raw, &client); err != nil {
		return Client{}, err
	}
	if client.ClientID == "" {
		return Client{}, fmt.Errorf("client is not set up")
	}
	if client.SecretItem == "" {
		client.SecretItem = secretItemTitle
	}
	return client, nil
}

func saveClient(root string, client Client) error {
	raw, err := json.MarshalIndent(client, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writePrivate(filepath.Join(root, "client.json"), raw)
}

func saveSession(root, name string, session Session) error {
	raw, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writePrivate(filepath.Join(root, "sessions", name+".json"), raw)
}

func loadSession(root, name string) (Session, bool) {
	raw, err := os.ReadFile(filepath.Join(root, "sessions", name+".json"))
	if err != nil {
		return Session{}, false
	}
	var session Session
	if json.Unmarshal(raw, &session) != nil || session.AccessToken == "" {
		return Session{}, false
	}
	return session, true
}

func savePending(root, name string, item pending) error {
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writePrivate(filepath.Join(root, "pending", name+".json"), raw)
}

func loadPending(root, name string, now time.Time) (pending, error) {
	raw, err := os.ReadFile(filepath.Join(root, "pending", name+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return pending{}, fmt.Errorf("login expired")
		}
		return pending{}, err
	}
	var item pending
	if err := json.Unmarshal(raw, &item); err != nil {
		return pending{}, err
	}
	if !item.Expires.IsZero() && now.After(item.Expires) {
		return pending{}, fmt.Errorf("login expired")
	}
	return item, nil
}

func removeName(root, name string) {
	_ = os.Remove(filepath.Join(root, "sessions", name+".json"))
	_ = os.Remove(filepath.Join(root, "pending", name+".json"))
}

func writePrivate(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func defaultScopes() []string {
	return []string{"openid", "email", "profile"}
}

func addScope(scopes []string, scope string) ([]string, error) {
	scope = strings.TrimSpace(scope)
	if !validScope(scope) {
		return nil, fmt.Errorf("scope must be openid, email, profile, or a Google API scope")
	}
	for _, existing := range scopes {
		if existing == scope {
			return scopes, nil
		}
	}
	return append(scopes, scope), nil
}

func validScope(scope string) bool {
	switch scope {
	case "openid", "email", "profile":
		return true
	default:
		return strings.HasPrefix(scope, "https://www.googleapis.com/auth/") && !strings.ContainsAny(scope, " \t\r\n")
	}
}

func sessionLabel(session Session, ok bool) string {
	if !ok {
		return "signed out"
	}
	if session.Email != "" {
		return session.Email
	}
	return "signed in"
}
