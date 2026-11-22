package crypt

import (
	"fmt"
	"strings"
)

func legacyArgs(args []string) ([]string, error) {
	switch args[0] {
	case "list":
		return listArgs(args[1:])
	case "get":
		return getArgs(args[1:])
	case "describe":
		return describeArgs(args[1:])
	case "create":
		return createArgs(args[1:])
	case "put":
		return putArgs(args[1:])
	case "delete":
		return deleteArgs(args[1:])
	case "load":
		return loadArgs(args[1:])
	default:
		return nil, fmt.Errorf("%s", usage)
	}
}

func loadArgs(rest []string) ([]string, error) {
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return nil, fmt.Errorf("Use: nebula crypt load <vault>")
	}
	return []string{rest[0], "--load"}, nil
}

func listArgs(rest []string) ([]string, error) {
	if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
		return nil, fmt.Errorf("Use: nebula crypt list <vault>")
	}
	return []string{rest[0], "-l"}, nil
}

func getArgs(rest []string) ([]string, error) {
	if len(rest) < 2 || strings.TrimSpace(rest[0]) == "" {
		return nil, fmt.Errorf("Use: nebula crypt get <vault> <name>")
	}
	for _, name := range rest[1:] {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Use: nebula crypt get <vault> <name>")
		}
	}
	return rest, nil
}

func describeArgs(rest []string) ([]string, error) {
	if len(rest) != 2 || strings.TrimSpace(rest[0]) == "" || strings.TrimSpace(rest[1]) == "" {
		return nil, fmt.Errorf("Use: nebula crypt describe <vault> <name>")
	}
	return []string{rest[0], "--show", rest[1]}, nil
}

func createArgs(rest []string) ([]string, error) {
	switch len(rest) {
	case 2:
		if rest[1] == "" || rest[1] == "template" || rest[1] == "login" {
			return nil, fmt.Errorf("Use: nebula crypt create <vault> <name>")
		}
		return []string{rest[0], "--create", "--title", rest[1], "--category", "api-credential"}, nil
	case 3:
		if rest[1] != "template" || strings.TrimSpace(rest[2]) == "" {
			return nil, fmt.Errorf("Use: nebula crypt create <vault> template <file>")
		}
		return []string{rest[0], "--create", "--template", rest[2]}, nil
	case 4:
		if rest[2] != "login" || strings.TrimSpace(rest[1]) == "" || strings.TrimSpace(rest[3]) == "" {
			return nil, fmt.Errorf("Use: nebula crypt create <vault> <name> login <username>")
		}
		return []string{rest[0], "--create", "--title", rest[1], "--category", "login", "--username", rest[3]}, nil
	default:
		return nil, fmt.Errorf("Use: nebula crypt create <vault> <name>")
	}
}

func putArgs(rest []string) ([]string, error) {
	if len(rest) < 2 || strings.TrimSpace(rest[0]) == "" || strings.TrimSpace(rest[1]) == "" {
		return nil, fmt.Errorf("Use: nebula crypt put <vault> <name>")
	}
	flags, err := pairs(rest[2:])
	if err != nil {
		return nil, err
	}
	if flags["template"] != "" && (flags["name"] != "" || flags["username"] != "") {
		return nil, fmt.Errorf("Use the template or the flags, not both.")
	}
	args := []string{rest[0], "--edit", rest[1]}
	if flags["template"] != "" {
		return append(args, "--template", flags["template"]), nil
	}
	if flags["name"] != "" {
		args = append(args, "--title", flags["name"])
	}
	if flags["username"] != "" {
		args = append(args, "--username", flags["username"])
	}
	return args, nil
}

func pairs(args []string) (map[string]string, error) {
	out := map[string]string{}
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("Use: nebula crypt put <vault> <name>")
	}
	for i := 0; i < len(args); i += 2 {
		key, value := args[i], args[i+1]
		switch key {
		case "name", "username", "template":
			if out[key] != "" || strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("Use: nebula crypt put <vault> <name>")
			}
			out[key] = value
		default:
			return nil, fmt.Errorf("Use: nebula crypt put <vault> <name>")
		}
	}
	return out, nil
}

func deleteArgs(rest []string) ([]string, error) {
	if len(rest) != 2 || strings.TrimSpace(rest[0]) == "" || strings.TrimSpace(rest[1]) == "" || rest[1] == "item" {
		return nil, fmt.Errorf("Use: nebula crypt delete <vault> <name>")
	}
	return []string{rest[0], "--delete", rest[1]}, nil
}
