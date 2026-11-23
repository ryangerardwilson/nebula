// Package archtest encodes nebula's import rules so a violation fails go
// test: the object store depends on nothing but the engine switch, the
// backup component never reaches the features that use it, and the
// features do not reach each other's internals.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	root   = "../.."
	module = "github.com/ryangerardwilson/nebula/"
)

// rules maps a package directory to the nebula packages it may not import,
// directly. A rule ending in "/" forbids that tree.
var rules = map[string][]string{
	"s3":              {"internal/backup", "internal/cache", "internal/agent", "internal/pg", "tunnel/", "run", "crypt", "command"},
	"internal/backup": {"tunnel/", "run", "crypt", "command"},
	"crypt":           {"s3", "internal/backup", "tunnel/", "run"},
	"tunnel/":         {"internal/pg", "internal/agent", "internal/cli", "internal/cache", "run", "crypt", "command"},
	"run":             {"internal/backup", "tunnel/internal/", "crypt"},
}

func TestImportsPointWhereTheyMay(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "node_modules" || strings.HasPrefix(name, ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			imp, _ := strconv.Unquote(spec.Path.Value)
			if dep, ok := strings.CutPrefix(imp, module); ok {
				if rule := forbidden(filepath.ToSlash(rel), dep); rule != "" {
					t.Errorf("%s imports %s; %s may not import %s", path, imp, filepath.ToSlash(rel), rule)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// forbidden returns the rule pkg breaks by importing dep, or "".
func forbidden(pkg, dep string) string {
	for owner, banned := range rules {
		if !within(pkg, owner) || within(dep, owner) {
			continue
		}
		for _, rule := range banned {
			if within(dep, rule) {
				return rule
			}
		}
	}
	return ""
}

// within reports whether pkg is rule, or is under rule when rule ends in "/".
func within(pkg, rule string) bool {
	if tree, ok := strings.CutSuffix(rule, "/"); ok {
		return pkg == tree || strings.HasPrefix(pkg, tree+"/")
	}
	return pkg == rule
}

func TestTheRuleCatchesAViolation(t *testing.T) {
	if forbidden("s3", "tunnel/internal/records") == "" || forbidden("tunnel/internal/tui", "internal/pg") == "" {
		t.Fatal("a forbidden import passed")
	}
	if forbidden("tunnel/internal/records", "internal/backup") != "" || forbidden("s3", "internal/engine") != "" {
		t.Fatal("an allowed import failed")
	}
}
