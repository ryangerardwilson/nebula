package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	tokenURL      = tokenEndpoint
	httpClient    = http.DefaultClient
	listenTimeout = 5 * time.Minute
	openBrowser   = func(raw string) error {
		cmd := exec.Command("xdg-open", raw)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		return cmd.Run()
	}
)

func setup(stdout, stderr io.Writer) int {
	id, err := secretGet(rootVault, idItemTitle)
	if err != nil {
		return fail(stderr, err.Error())
	}
	secret, err := secretGet(rootVault, secretItemTitle)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(secret) == "" {
		return fail(stderr, "client secret is empty")
	}
	client := Client{ClientID: strings.TrimSpace(id), IDItem: idItemTitle, SecretItem: secretItemTitle}
	if err := saveClient(rootDir(), client); err != nil {
		return fail(stderr, err.Error())
	}
	fmt.Fprintln(stdout, "client recorded")
	return 0
}

func add(stdout, stderr io.Writer, args []string) int {
	if len(args) != 3 {
		return fail(stderr, "Use: nebula auth add <name> redirect <url>")
	}
	var err error
	switch args[1] {
	case "redirect":
		err = addRedirect(args[0], args[2])
	case "scope":
		err = addScopeName(args[0], args[2])
	default:
		return fail(stderr, "Use: nebula auth add <name> redirect <url>")
	}
	if err != nil {
		return fail(stderr, err.Error())
	}
	fmt.Fprintln(stdout, "recorded "+args[0])
	return 0
}

func addRedirect(name, redirect string) error {
	if err := checkName(name); err != nil {
		return err
	}
	if err := checkRedirect(redirect); err != nil {
		return err
	}
	root := rootDir()
	file, err := loadFile(root)
	if err != nil {
		return err
	}
	product, ok := findProduct(file, name)
	if !ok {
		product = Product{Name: name, Scopes: defaultScopes()}
	}
	product.Redirect = redirect
	return saveFile(root, putProduct(file, product))
}

func addScopeName(name, scope string) error {
	if err := checkName(name); err != nil {
		return err
	}
	root := rootDir()
	file, err := loadFile(root)
	if err != nil {
		return err
	}
	product, ok := findProduct(file, name)
	if !ok || product.Redirect == "" {
		return fmt.Errorf("product %s has no redirect", name)
	}
	scopes, err := addScope(product.Scopes, scope)
	if err != nil {
		return err
	}
	product.Scopes = scopes
	return saveFile(root, putProduct(file, product))
}

func remove(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		return fail(stderr, "Use: nebula auth remove <name>")
	}
	if err := checkName(args[0]); err != nil {
		return fail(stderr, err.Error())
	}
	root := rootDir()
	file, err := loadFile(root)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if _, ok := findProduct(file, args[0]); !ok {
		return fail(stderr, "no product "+args[0])
	}
	if err := saveFile(root, dropProduct(file, args[0])); err != nil {
		return fail(stderr, err.Error())
	}
	removeName(root, args[0])
	fmt.Fprintln(stdout, "removed "+args[0])
	return 0
}

func listProducts(stdout, stderr io.Writer) int {
	file, err := loadFile(rootDir())
	if err != nil {
		return fail(stderr, err.Error())
	}
	for _, product := range file.Products {
		session, ok := loadSession(rootDir(), product.Name)
		fmt.Fprintf(stdout, "%s %s %s\n", product.Name, product.Redirect, sessionLabel(session, ok))
	}
	return 0
}

func showURL(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		return fail(stderr, "Use: nebula auth url <name>")
	}
	raw, err := prepare(args[0])
	if err != nil {
		return fail(stderr, err.Error())
	}
	fmt.Fprintln(stdout, raw)
	return 0
}

func login(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 {
		return fail(stderr, "Use: nebula auth login <name>")
	}
	raw, err := prepare(args[0])
	if err != nil {
		return fail(stderr, err.Error())
	}
	item, err := loadPending(rootDir(), args[0], time.Now())
	if err != nil {
		return fail(stderr, err.Error())
	}
	if !loopback(item.Redirect) {
		fmt.Fprintln(stdout, raw)
		return 0
	}
	return finishLoopback(stdout, stderr, args[0], item.Redirect, raw)
}

func exchange(stdout, stderr io.Writer, args []string) int {
	if len(args) != 3 || strings.TrimSpace(args[1]) == "" || strings.TrimSpace(args[2]) == "" {
		return fail(stderr, "Use: nebula auth exchange <name> <code> <state>")
	}
	item, err := loadPending(rootDir(), args[0], time.Now())
	if err != nil {
		return fail(stderr, err.Error())
	}
	if item.State != args[2] {
		return fail(stderr, "state does not match")
	}
	return complete(stdout, stderr, args[0], args[1], item)
}

func prepare(name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	client, err := loadClient(rootDir())
	if err != nil {
		return "", err
	}
	file, err := loadFile(rootDir())
	if err != nil {
		return "", err
	}
	product, ok := findProduct(file, name)
	if !ok || product.Redirect == "" {
		return "", fmt.Errorf("product %s has no redirect", name)
	}
	verifier, challenge, state, err := newChallenge()
	if err != nil {
		return "", fmt.Errorf("unable to start login")
	}
	item := pending{State: state, Verifier: verifier, Redirect: product.Redirect, Expires: time.Now().Add(10 * time.Minute)}
	if err := savePending(rootDir(), name, item); err != nil {
		return "", err
	}
	return authorizeURL(client.ClientID, product.Redirect, state, challenge, product.Scopes), nil
}

func finishLoopback(stdout, stderr io.Writer, name, redirect, raw string) int {
	item, err := loadPending(rootDir(), name, time.Now())
	if err != nil {
		return fail(stderr, err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), listenTimeout)
	defer cancel()
	code, err := waitCode(ctx, redirect, item.State, func() {
		if openErr := openBrowser(raw); openErr != nil {
			fmt.Fprintln(stdout, raw)
		}
	})
	if err != nil {
		return fail(stderr, err.Error())
	}
	return complete(stdout, stderr, name, code, item)
}

func complete(stdout, stderr io.Writer, name, code string, item pending) int {
	client, err := loadClient(rootDir())
	if err != nil {
		return fail(stderr, err.Error())
	}
	secret, err := secretGet(rootVault, client.SecretItem)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if strings.TrimSpace(secret) == "" {
		return fail(stderr, "client secret is empty")
	}
	session, err := Exchange(context.Background(), httpClient, tokenURL, client.ClientID, secret, code, item.Redirect, item.Verifier)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if err := saveSession(rootDir(), name, session); err != nil {
		return fail(stderr, err.Error())
	}
	_ = os.Remove(filepath.Join(rootDir(), "pending", name+".json"))
	fmt.Fprint(stdout, signedInLine(name, session.Email))
	return 0
}

func checkRedirect(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return fmt.Errorf("redirect must be an https URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("redirect must not include a user, query, or fragment")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && parsed.Port() != "" && (host == "127.0.0.1" || host == "localhost") {
		return nil
	}
	return fmt.Errorf("redirect must be https, or http on 127.0.0.1 or localhost with a port")
}
