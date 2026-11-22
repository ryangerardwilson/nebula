package cli

import (
	"fmt"
	"strings"

	"github.com/ryangerardwilson/nebula/crypt/internal/match"
	"github.com/ryangerardwilson/nebula/crypt/internal/op"
)

func (r Runner) loadVault(name string, rest []string) int {
	name = strings.TrimSpace(name)
	if name == "" || len(rest) != 0 {
		return r.fail("Use: nebula crypt load <vault>")
	}
	if !r.forShell() {
		return r.fail(fmt.Sprintf("Load prints export lines for the shell. Run: eval \"$(nebula crypt load %s)\"", name))
	}
	name, client, ok := r.begin(name)
	if !ok {
		return 1
	}
	items, err := client.List(name)
	if err != nil {
		return r.fail(err.Error())
	}
	return r.writeExports(name, client, items)
}

func (r Runner) writeExports(name string, client op.Client, items []op.Item) int {
	if len(items) == 0 {
		return r.fail(fmt.Sprintf("No items in vault %s.", name))
	}
	chosen, skipped := exportableItems(items)
	if skipped > 0 {
		_, _ = fmt.Fprintf(r.Err, "Skipped %d items whose names are not shell variables.\n", skipped)
	}
	if len(chosen) == 0 {
		return r.fail(fmt.Sprintf("No items in vault %s have a shell variable name.", name))
	}
	values, keys, err := readAll(client, chosen)
	if err != nil {
		return r.fail(err.Error())
	}
	for _, key := range keys {
		_, _ = fmt.Fprintf(r.Out, "export %s=%s\n", key, shellQuote(values[key]))
	}
	return 0
}

func exportableItems(items []op.Item) ([]op.Item, int) {
	var chosen []op.Item
	seen := map[string]bool{}
	skipped := 0
	for _, item := range items {
		if !exportable(item) {
			skipped++
			continue
		}
		title := match.DisplayTitle(item)
		if seen[title] {
			continue
		}
		seen[title] = true
		chosen = append(chosen, item)
	}
	return chosen, skipped
}

func exportable(item op.Item) bool {
	if !shellIdentifier(match.DisplayTitle(item)) {
		return false
	}
	switch strings.ToLower(item.Category) {
	case "login", "api_credential":
		return true
	default:
		return false
	}
}
