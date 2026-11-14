package pg

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/ryangerardwilson/nebula/internal/orm"
)

const lockSQL = "BEGIN;\nSELECT pg_advisory_xact_lock(hashtext(current_database())::bigint)"

// Rewrite turns the sqlite statements applications already send into Postgres.
// SQL that needs no change is returned unchanged.
func Rewrite(sql string) (string, error) {
	rewritten := replaceImmediate(sql)
	if !codeHas(rewritten, "changes") && !codeHas(rewritten, "pragma") {
		return rewritten, nil
	}
	parts, err := orm.SplitSQL(rewritten)
	if err != nil {
		return "", err
	}
	merged, err := mergeChanges(parts)
	if err != nil {
		return "", err
	}
	var kept []string
	for _, part := range merged {
		if strings.TrimSpace(part) == "" {
			continue
		}
		if firstWord(part) == "pragma" {
			continue
		}
		kept = append(kept, strings.TrimSpace(part))
	}
	if len(kept) == 0 {
		return "", nil
	}
	return strings.Join(kept, ";\n") + ";\n", nil
}

func replaceImmediate(sql string) string {
	var b strings.Builder
	for i := 0; i < len(sql); {
		next, ok := copyLiteral(sql, i, &b)
		if ok {
			i = next
			continue
		}
		n := matchBeginLock(sql[i:])
		if n > 0 {
			b.WriteString(lockSQL)
			i += n
			if !nextNonSpaceIs(sql[i:], ";") {
				b.WriteByte(';')
			}
			continue
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
}

func mergeChanges(parts []string) ([]string, error) {
	var out []string
	for _, part := range parts {
		if !codeHas(part, "changes") {
			out = append(out, part)
			continue
		}
		at, err := previousWrite(out)
		if err != nil {
			return nil, err
		}
		body := replaceCall(part, "changes", "(SELECT count(*) FROM sqlx_updated)")
		out[at] = "WITH sqlx_updated AS (\n" + strings.TrimSpace(out[at]) + " RETURNING 1 AS sqlx_changed\n)\n" + strings.TrimSpace(body)
	}
	return out, nil
}

func previousWrite(parts []string) (int, error) {
	for i := len(parts) - 1; i >= 0; i-- {
		switch firstWord(parts[i]) {
		case "insert", "update", "delete":
			if codeHas(parts[i], "returning") {
				return 0, fmt.Errorf("changes() cannot follow RETURNING")
			}
			return i, nil
		case "begin", "commit", "rollback", "end", "savepoint", "release", "select":
			continue
		default:
			return 0, fmt.Errorf("changes() needs a previous insert, update, or delete")
		}
	}
	return 0, fmt.Errorf("changes() needs a previous insert, update, or delete")
}

func replaceCall(sql, name, repl string) string {
	var b strings.Builder
	for i := 0; i < len(sql); {
		next, ok := copyLiteral(sql, i, &b)
		if ok {
			i = next
			continue
		}
		n := matchCall(sql[i:], name)
		if n > 0 {
			b.WriteString(repl)
			i += n
			continue
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String()
}

func codeHas(sql, name string) bool {
	for i := 0; i < len(sql); {
		next, ok := skipLiteral(sql, i)
		if ok {
			i = next
			continue
		}
		if matchCall(sql[i:], name) > 0 || matchWord(sql[i:], name) > 0 {
			return true
		}
		i++
	}
	return false
}

func copyLiteral(sql string, i int, b *strings.Builder) (int, bool) {
	next, ok := skipLiteral(sql, i)
	if !ok {
		return i, false
	}
	b.WriteString(sql[i:next])
	return next, true
}

func skipLiteral(sql string, i int) (int, bool) {
	if i >= len(sql) {
		return i, false
	}
	switch sql[i] {
	case '\'', '"':
		return skipQuote(sql, i), true
	}
	if sql[i] == '-' && i+1 < len(sql) && sql[i+1] == '-' {
		j := i
		for j < len(sql) && sql[j] != '\n' {
			j++
		}
		return j, true
	}
	if sql[i] == '/' && i+1 < len(sql) && sql[i+1] == '*' {
		end := strings.Index(sql[i+2:], "*/")
		if end < 0 {
			return len(sql), true
		}
		return i + 2 + end + 2, true
	}
	return i, false
}

func skipQuote(sql string, i int) int {
	quote := sql[i]
	j := i + 1
	for j < len(sql) {
		if sql[j] == quote {
			if j+1 < len(sql) && sql[j+1] == quote {
				j += 2
				continue
			}
			return j + 1
		}
		j++
	}
	return len(sql)
}

func matchBeginLock(s string) int {
	n := matchWord(s, "begin")
	if n == 0 {
		return 0
	}
	rest := s[n:]
	sp := 0
	for sp < len(rest) && (rest[sp] == ' ' || rest[sp] == '\t' || rest[sp] == '\n' || rest[sp] == '\r') {
		sp++
	}
	if sp == 0 {
		return 0
	}
	m := matchWord(rest[sp:], "immediate")
	if m == 0 {
		m = matchWord(rest[sp:], "exclusive")
	}
	if m == 0 {
		return 0
	}
	return n + sp + m
}

func matchCall(s, name string) int {
	n := matchWord(s, name)
	if n == 0 {
		return 0
	}
	rest := strings.TrimLeft(s[n:], " \t\r\n")
	skipped := len(s[n:]) - len(rest)
	if !strings.HasPrefix(rest, "(") {
		return 0
	}
	return n + skipped + 1 + matchParen(rest[1:])
}

func matchParen(s string) int {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}

func matchWord(s, word string) int {
	if len(s) < len(word) || !strings.EqualFold(s[:len(word)], word) {
		return 0
	}
	if len(s) > len(word) && isIdent(s[len(word)]) {
		return 0
	}
	return len(word)
}

func nextNonSpaceIs(s, prefix string) bool {
	return strings.HasPrefix(strings.TrimLeft(s, " \t\r\n"), prefix)
}

func firstWord(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	end := 0
	for end < len(stmt) {
		r := rune(stmt[end])
		if unicode.IsSpace(r) {
			break
		}
		end++
	}
	return strings.ToLower(stmt[:end])
}

func isIdent(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
