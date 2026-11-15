package crypt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ryangerardwilson/nebula/crypt/internal/match"
	"github.com/ryangerardwilson/nebula/crypt/internal/op"
)

type vaultLine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type itemLine struct {
	ID       string `json:"id"`
	VaultID  string `json:"vault_id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Username string `json:"username,omitempty"`
}

// openClient starts op with stderr discarded, so a command failure cannot
// print a concealed field.
func openClient() (op.Client, error) {
	runner := runnerFactory(io.Discard, io.Discard)
	env, err := op.Env()
	if err != nil {
		return op.Client{}, err
	}
	if err := runner.Op.LookPath(); err != nil {
		return op.Client{}, errors.New("op is required.")
	}
	return op.Client{Runner: runner.Op, Env: env, Err: io.Discard}, nil
}

func vaults(stdout, stderr io.Writer, args []string) int {
	if len(args) != 0 {
		return fail(stderr, "Use: nebula crypt vaults")
	}
	client, err := openClient()
	if err != nil {
		return fail(stderr, err.Error())
	}
	listed, err := client.Vaults()
	if err != nil {
		return fail(stderr, err.Error())
	}
	for _, vault := range listed {
		if err := writeJSONLine(stdout, vaultLine{ID: vault.ID, Name: vault.Name}); err != nil {
			return fail(stderr, "Unable to print the vault list.")
		}
	}
	return 0
}

func items(stdout, stderr io.Writer, args []string) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fail(stderr, "Use: nebula crypt items <vault>")
	}
	client, err := openClient()
	if err != nil {
		return fail(stderr, err.Error())
	}
	listed, err := client.List(strings.TrimSpace(args[0]))
	if err != nil {
		return fail(stderr, err.Error())
	}
	for _, item := range listed {
		if err := writeJSONLine(stdout, itemRecord(item)); err != nil {
			return fail(stderr, "Unable to print the item list.")
		}
	}
	return 0
}

func itemRecord(item op.Item) itemLine {
	line := itemLine{
		ID:       item.ID,
		VaultID:  item.Vault.ID,
		Title:    match.DisplayTitle(item),
		Category: item.Category,
	}
	if names := match.Usernames(item); len(names) > 0 {
		line.Username = strings.Join(names, ", ")
	}
	return line
}

func copyItem(stdout, stderr io.Writer, args []string) int {
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		return fail(stderr, "Use: nebula crypt copy <vault> <id>")
	}
	client, err := openClient()
	if err != nil {
		return fail(stderr, err.Error())
	}
	secret, err := readID(client, strings.TrimSpace(args[0]), strings.TrimSpace(args[1]))
	if err != nil {
		return fail(stderr, err.Error())
	}
	runner := runnerFactory(io.Discard, io.Discard)
	if err := runner.Clip.Copy(secret, io.Discard); err != nil {
		return fail(stderr, err.Error())
	}
	return 0
}

func readID(client op.Client, vault, id string) (string, error) {
	listed, err := client.List(vault)
	if err != nil {
		return "", err
	}
	for _, item := range listed {
		if item.ID != id {
			continue
		}
		return client.Read(item.Vault.ID, item.ID, item.Category)
	}
	return "", fmt.Errorf("No item with that id in vault %s.", vault)
}

func deleteByID(stdout, stderr io.Writer, vault, id string) int {
	vault = strings.TrimSpace(vault)
	id = strings.TrimSpace(id)
	if vault == "" || id == "" {
		return fail(stderr, "Use: nebula crypt delete <vault> item <id>")
	}
	client, err := openClient()
	if err != nil {
		return fail(stderr, err.Error())
	}
	if err := client.Delete(vault, id); err != nil {
		return fail(stderr, err.Error())
	}
	fmt.Fprintf(stdout, "Deleted %s from vault %s.\n", id, vault)
	return 0
}

func writeJSONLine(out io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = out.Write(append(raw, '\n'))
	return err
}
