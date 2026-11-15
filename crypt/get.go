package crypt

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/nebula/crypt/internal/match"
)

// Get returns the credential or password of the one item titled title.
// The value is not printed, and it is not included in an error.
func Get(vault, title string) (string, error) {
	vault = strings.TrimSpace(vault)
	title = strings.TrimSpace(title)
	if vault == "" || title == "" {
		return "", fmt.Errorf("Vault and title must not be empty.")
	}
	client, err := openClient()
	if err != nil {
		return "", err
	}
	listed, err := client.List(vault)
	if err != nil {
		return "", err
	}
	hits := match.Exact(listed, title)
	switch len(hits) {
	case 0:
		return "", fmt.Errorf("No item titled %s in vault %s.", title, vault)
	case 1:
		return client.Read(hits[0].Vault.ID, hits[0].ID, hits[0].Category)
	default:
		return "", fmt.Errorf("More than one item titled %s in vault %s.", title, vault)
	}
}
