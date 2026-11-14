package orm

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var nameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var ErrSchemaSQL = fmt.Errorf("schema changes belong in the orm")

type Column struct {
	Name       string
	Type       string
	PrimaryKey bool
	NotNull    bool
	Default    string
}

type Index struct {
	Name    string
	Unique  bool
	Columns []string
}

type Model struct {
	Name    string
	Columns []Column
	Indexes []Index
}

type Add struct {
	Table  string
	Column Column
}

type Plan struct {
	Create []Model
	Add    []Add
}

func (p Plan) Empty() bool {
	return len(p.Create) == 0 && len(p.Add) == 0
}

func ValidName(name string) bool {
	return nameRE.MatchString(name)
}

func (c Column) Canonical() string {
	s := c.Name + " " + c.Type
	if c.PrimaryKey {
		s += " primary key"
	}
	if c.NotNull {
		s += " not null"
	}
	if c.Default != "" {
		s += " default " + c.Default
	}
	return s
}

func (i Index) Canonical() string {
	s := "index " + i.Name
	if i.Unique {
		s += " unique"
	}
	for _, column := range i.Columns {
		s += " " + column
	}
	return s
}

func (m Model) Canonical() string {
	var b strings.Builder
	fmt.Fprintf(&b, "model %s\n", m.Name)
	for _, column := range m.Columns {
		fmt.Fprintf(&b, "  %s\n", column.Canonical())
	}
	for _, index := range m.Indexes {
		fmt.Fprintf(&b, "  %s\n", index.Canonical())
	}
	return b.String()
}

func Format(models []Model) string {
	var b strings.Builder
	for i, model := range models {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(model.Canonical())
	}
	return b.String()
}

func ColumnsText(model Model) string {
	parts := make([]string, len(model.Columns))
	for i, column := range model.Columns {
		parts[i] = column.Canonical()
	}
	return strings.Join(parts, ", ")
}

func QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func QuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func Parse(text string) ([]Model, error) {
	var models []Model
	var current *Model
	seen := map[string]bool{}
	seenIndex := map[string]bool{}
	lines := strings.Split(text, "\n")
	for n, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			if current == nil {
				return nil, fmt.Errorf("line %d: expected a model", n+1)
			}
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "index" {
				index, err := parseIndex(fields, n+1)
				if err != nil {
					return nil, err
				}
				if seenIndex[index.Name] {
					return nil, fmt.Errorf("index %s is declared twice", index.Name)
				}
				seenIndex[index.Name] = true
				current.Indexes = append(current.Indexes, index)
				continue
			}
			column, err := parseColumn(line, n+1)
			if err != nil {
				return nil, err
			}
			for _, existing := range current.Columns {
				if existing.Name == column.Name {
					return nil, fmt.Errorf("model %s column %s is declared twice", current.Name, column.Name)
				}
			}
			if column.PrimaryKey {
				for _, existing := range current.Columns {
					if existing.PrimaryKey {
						return nil, fmt.Errorf("model %s has two primary keys", current.Name)
					}
				}
			}
			current.Columns = append(current.Columns, column)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != "model" {
			return nil, fmt.Errorf("line %d: expected a model", n+1)
		}
		if current != nil {
			if err := finish(current); err != nil {
				return nil, err
			}
		}
		name := fields[1]
		if !ValidName(name) {
			return nil, fmt.Errorf("line %d: model name must be letters, digits, and underscores", n+1)
		}
		if name == "sqlx_orm" || strings.HasPrefix(name, "sqlite_") {
			return nil, fmt.Errorf("line %d: model name %s is reserved", n+1, name)
		}
		if seen[name] {
			return nil, fmt.Errorf("model %s is declared twice", name)
		}
		seen[name] = true
		models = append(models, Model{Name: name})
		current = &models[len(models)-1]
	}
	if current != nil {
		if err := finish(current); err != nil {
			return nil, err
		}
	}
	return models, nil
}

func finish(model *Model) error {
	if len(model.Columns) == 0 {
		return fmt.Errorf("model %s needs a column", model.Name)
	}
	columns := map[string]bool{}
	for _, column := range model.Columns {
		columns[column.Name] = true
	}
	for _, index := range model.Indexes {
		for _, column := range index.Columns {
			if !columns[column] {
				return fmt.Errorf("model %s index %s uses unknown column %s", model.Name, index.Name, column)
			}
		}
	}
	return nil
}

func parseColumn(line string, n int) (Column, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return Column{}, fmt.Errorf("line %d: column needs a name and a type", n)
	}
	if !ValidName(fields[0]) {
		return Column{}, fmt.Errorf("line %d: column name must be letters, digits, and underscores", n)
	}
	kind := strings.ToLower(fields[1])
	switch kind {
	case "integer", "text", "real", "blob":
	default:
		return Column{}, fmt.Errorf("line %d: unknown type %s", n, fields[1])
	}
	column := Column{Name: fields[0], Type: kind}
	for i := 2; i < len(fields); {
		if fields[i] == "primary" && i+1 < len(fields) && fields[i+1] == "key" {
			column.PrimaryKey = true
			i += 2
			continue
		}
		if fields[i] == "not" && i+1 < len(fields) && fields[i+1] == "null" {
			column.NotNull = true
			i += 2
			continue
		}
		if fields[i] == "default" && i+1 < len(fields) {
			if !validDefault(fields[i+1]) {
				return Column{}, fmt.Errorf("line %d: default must be a number, null, or a quoted literal", n)
			}
			column.Default = fields[i+1]
			i += 2
			continue
		}
		return Column{}, fmt.Errorf("line %d: unknown column constraint", n)
	}
	return column, nil
}

func Compare(live, desired []Model) (Plan, error) {
	want := map[string]Model{}
	for _, model := range desired {
		want[model.Name] = model
	}
	have := map[string]Model{}
	for _, model := range live {
		have[model.Name] = model
		if _, ok := want[model.Name]; !ok {
			return Plan{}, fmt.Errorf("table %s is not in the orm", model.Name)
		}
	}
	var plan Plan
	for _, model := range desired {
		current, ok := have[model.Name]
		if !ok {
			plan.Create = append(plan.Create, model)
			continue
		}
		liveCol := map[string]Column{}
		for _, column := range current.Columns {
			liveCol[column.Name] = column
		}
		wantCol := map[string]bool{}
		for _, column := range model.Columns {
			wantCol[column.Name] = true
			got, exists := liveCol[column.Name]
			if !exists {
				if column.PrimaryKey {
					return Plan{}, fmt.Errorf("column %s.%s is a primary key; a primary key is only added with a new model", model.Name, column.Name)
				}
				if column.NotNull && column.Default == "" {
					return Plan{}, fmt.Errorf("column %s.%s is not null; a column added to an existing model is nullable", model.Name, column.Name)
				}
				plan.Add = append(plan.Add, Add{Table: model.Name, Column: column})
				continue
			}
			if !sameColumn(got, column) {
				return Plan{}, fmt.Errorf("column %s.%s does not match the orm", model.Name, column.Name)
			}
		}
		for _, column := range current.Columns {
			if !wantCol[column.Name] {
				return Plan{}, fmt.Errorf("column %s.%s is not in the orm", model.Name, column.Name)
			}
		}
	}
	return plan, nil
}

func sameColumn(live, want Column) bool {
	return live.Name == want.Name &&
		strings.EqualFold(live.Type, want.Type) &&
		live.PrimaryKey == want.PrimaryKey &&
		live.NotNull == want.NotNull
}

func Drift(plan Plan) error {
	if len(plan.Create) > 0 {
		return fmt.Errorf("model %s is not in the database", plan.Create[0].Name)
	}
	if len(plan.Add) > 0 {
		return fmt.Errorf("column %s.%s is not in the database", plan.Add[0].Table, plan.Add[0].Column.Name)
	}
	return nil
}

func CreateSQL(model Model) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", QuoteIdent(model.Name))
	for i, column := range model.Columns {
		line := "  " + columnSQL(column)
		if i+1 < len(model.Columns) {
			line += ","
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(")")
	return b.String()
}

func AddColumnSQL(table string, column Column) string {
	return "ALTER TABLE " + QuoteIdent(table) + " ADD COLUMN " + columnSQL(column)
}

func columnSQL(column Column) string {
	s := QuoteIdent(column.Name) + " " + strings.ToUpper(column.Type)
	if column.PrimaryKey {
		s += " PRIMARY KEY"
	}
	if column.NotNull {
		s += " NOT NULL"
	}
	if column.Default != "" {
		s += " DEFAULT " + column.Default
	}
	return s
}

func CreateIndexSQL(table string, index Index) string {
	cols := make([]string, len(index.Columns))
	for i, column := range index.Columns {
		cols[i] = QuoteIdent(column)
	}
	unique := ""
	if index.Unique {
		unique = "UNIQUE "
	}
	return "CREATE " + unique + "INDEX IF NOT EXISTS " + QuoteIdent(index.Name) + " ON " + QuoteIdent(table) + "(" + strings.Join(cols, ", ") + ")"
}

func parseIndex(fields []string, n int) (Index, error) {
	if len(fields) < 3 || fields[0] != "index" {
		return Index{}, fmt.Errorf("line %d: index needs a name and a column", n)
	}
	if !ValidName(fields[1]) {
		return Index{}, fmt.Errorf("line %d: index name must be letters, digits, and underscores", n)
	}
	rest := fields[2:]
	index := Index{Name: fields[1]}
	if rest[0] == "unique" {
		index.Unique = true
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return Index{}, fmt.Errorf("line %d: index needs a column", n)
	}
	for _, column := range rest {
		if !ValidName(column) {
			return Index{}, fmt.Errorf("line %d: index column must be letters, digits, and underscores", n)
		}
		index.Columns = append(index.Columns, column)
	}
	return index, nil
}

func validDefault(lit string) bool {
	if lit == "null" {
		return true
	}
	if len(lit) >= 2 && lit[0] == '\'' && lit[len(lit)-1] == '\'' {
		return true
	}
	if lit == "" || lit == "-" {
		return false
	}
	for i, r := range lit {
		if i == 0 && r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func CheckDataSQL(sql string) error {
	parts, err := statements(sql)
	if err != nil {
		return err
	}
	saw := false
	for _, part := range parts {
		word := firstKeyword(part)
		if word == "" {
			continue
		}
		saw = true
		if !allowedSQL[word] {
			return ErrSchemaSQL
		}
	}
	if !saw {
		return fmt.Errorf("query needs sql")
	}
	return nil
}

var allowedSQL = map[string]bool{
	"select": true, "insert": true, "update": true, "delete": true,
	"replace": true, "with": true, "values": true,
	"begin": true, "commit": true, "rollback": true, "end": true,
	"savepoint": true, "release": true,
}

func statements(sql string) ([]string, error) {
	var parts []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inSingle = false
			}
			continue
		}
		if inDouble {
			b.WriteByte(c)
			if c == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					b.WriteByte(sql[i+1])
					i++
					continue
				}
				inDouble = false
			}
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unclosed comment")
			}
			i = i + 2 + end + 1
			continue
		}
		if c == '\'' {
			inSingle = true
			b.WriteByte(c)
			continue
		}
		if c == '"' {
			inDouble = true
			b.WriteByte(c)
			continue
		}
		if c == ';' {
			parts = append(parts, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed string")
	}
	parts = append(parts, b.String())
	return parts, nil
}

func firstKeyword(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return ""
	}
	end := 0
	for end < len(stmt) && !unicode.IsSpace(rune(stmt[end])) {
		end++
	}
	return strings.ToLower(stmt[:end])
}

// SplitSQL splits a script into statements. A semicolon inside a quote stays in the statement.
func SplitSQL(sql string) ([]string, error) {
	return statements(sql)
}
