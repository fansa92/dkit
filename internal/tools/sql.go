package tools

import (
	"fmt"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "sql",
		Aliases: []string{"s"},
		Summary: "reformat SQL: clause keywords on their own line (basic)",
		Usage: []string{
			"query.sql",
			"- < query.sql",
			"[--indent 2] [--keep-case]",
		},
		Run: runSQL,
	})
}

// clauseWords start a new line when formatting. This is a deliberately small
// formatter: it re-flows text, it does not parse SQL semantics or dialects.
var clauseWords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "HAVING": true,
	"LIMIT": true, "OFFSET": true, "UNION": true, "UNION ALL": true,
	"VALUES": true, "SET": true, "UPDATE": true, "INSERT": true,
	"INSERT INTO": true, "DELETE": true, "DELETE FROM": true,
	"GROUP BY": true, "ORDER BY": true, "JOIN": true,
	"LEFT JOIN": true, "RIGHT JOIN": true, "INNER JOIN": true,
	"OUTER JOIN": true, "CROSS JOIN": true, "FULL JOIN": true,
}

// sqlKeywords are uppercased during formatting. Identifiers, column names and
// function names are left exactly as written.
var sqlKeywords = map[string]bool{
	"AND": true, "OR": true, "ON": true, "AS": true, "IN": true,
	"LIKE": true, "IS": true, "NOT": true, "NULL": true, "BETWEEN": true,
	"EXISTS": true, "CASE": true, "WHEN": true, "THEN": true, "ELSE": true,
	"END": true, "DISTINCT": true, "ALL": true, "ASC": true, "DESC": true,
	"USING": true, "WITH": true, "RETURNING": true, "INTO": true,
}

type sqlTok struct {
	kind int // 0 word, 1 quoted, 2 punct, 3 number
	text string
}

func runSQL(args []string) int {
	fs := cli.NewFlagSet("sql")
	indent := fs.String("indent", "  ", "indent string")
	keepCase := fs.Bool("keep-case", false, "do not uppercase keywords")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	toks := mergeClauses(lexSQL(string(data)))
	if len(toks) == 0 {
		return cli.Fail(fmt.Errorf("%s: no SQL found", label))
	}
	fmt.Println(formatSQL(toks, *indent, !*keepCase))
	return cli.ExitOK
}

func lexSQL(s string) []sqlTok {
	var toks []sqlTok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"' || c == '`':
			q := c
			j := i + 1
			for j < len(s) {
				if s[j] == q {
					if j+1 < len(s) && s[j+1] == q {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			if j > len(s) {
				j = len(s)
			}
			toks = append(toks, sqlTok{1, s[i:j]})
			i = j
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			if k := strings.IndexByte(s[i:], '\n'); k < 0 {
				i = len(s)
			} else {
				i += k
			}
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			if k := strings.Index(s[i+2:], "*/"); k < 0 {
				i = len(s)
			} else {
				i += k + 4
			}
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && (isSQLWordChar(s[j]) || s[j] == '.') {
				j++
			}
			toks = append(toks, sqlTok{3, s[i:j]})
			i = j
		case isSQLWordStart(c):
			j := i
			for j < len(s) && isSQLWordChar(s[j]) {
				j++
			}
			toks = append(toks, sqlTok{0, s[i:j]})
			i = j
		default:
			toks = append(toks, sqlTok{2, string(c)})
			i++
		}
	}
	return toks
}

func isSQLWordStart(c byte) bool {
	return c == '_' || c == '@' || c == ':' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSQLWordChar(c byte) bool {
	return isSQLWordStart(c) || (c >= '0' && c <= '9') || c == '$'
}

func mergeClauses(toks []sqlTok) []sqlTok {
	out := make([]sqlTok, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		if toks[i].kind == 0 && i+1 < len(toks) && toks[i+1].kind == 0 {
			pair := strings.ToUpper(toks[i].text) + " " + strings.ToUpper(toks[i+1].text)
			if clauseWords[pair] {
				out = append(out, sqlTok{0, pair})
				i++
				continue
			}
		}
		out = append(out, toks[i])
	}
	return out
}

func formatSQL(toks []sqlTok, indent string, upper bool) string {
	var buf []byte
	depth := 0

	trimEnd := func() {
		for len(buf) > 0 && (buf[len(buf)-1] == ' ' || buf[len(buf)-1] == '\n') {
			buf = buf[:len(buf)-1]
		}
	}
	newline := func(level int) {
		trimEnd()
		buf = append(buf, '\n')
		buf = append(buf, strings.Repeat(indent, level)...)
	}
	last := func() byte {
		if len(buf) == 0 {
			return 0
		}
		return buf[len(buf)-1]
	}

	for _, t := range toks {
		up := strings.ToUpper(t.text)
		switch {
		case t.kind == 0 && clauseWords[up]:
			if len(buf) > 0 {
				buf = append(buf, '\n')
			}
			buf = append(buf, up...)
			newline(1)
		case t.kind == 0 && depth == 0 && (up == "AND" || up == "OR"):
			newline(1)
			buf = append(buf, up...)
			buf = append(buf, ' ')
		case t.kind == 2 && t.text == "(":
			depth++
			buf = append(buf, '(')
		case t.kind == 2 && t.text == ")":
			if depth > 0 {
				depth--
			}
			trimEnd()
			buf = append(buf, ')')
		case t.kind == 2 && t.text == ",":
			buf = append(buf, ',')
			if depth == 0 {
				newline(1)
			} else {
				buf = append(buf, ' ')
			}
		default:
			if l := last(); l != 0 && l != ' ' && l != '\n' && l != '(' {
				buf = append(buf, ' ')
			}
			if t.kind == 0 && upper && sqlKeywords[up] {
				buf = append(buf, up...)
			} else {
				buf = append(buf, t.text...)
			}
		}
	}
	return strings.TrimSpace(string(buf))
}
