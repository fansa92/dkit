package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "jsonpath",
		Aliases: []string{"jp"},
		Summary: "query JSON with a path: $.users[*].name or $..id",
		Usage: []string{
			"'$.items[0].name' data.json",
			"'$..id' - < data.json",
			"[--raw] [--json]",
		},
		Run: runJSONPath,
	})
}

type segment struct {
	kind string // key, index, wildcard, recursive
	key  string
	idx  int
}

type match struct {
	path string
	val  any
}

func runJSONPath(args []string) int {
	fs := cli.NewFlagSet("jsonpath")
	raw := fs.Bool("raw", false, "print bare values, one per line")
	asJSON := fs.Bool("json", false, "emit path/value pairs as JSON")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit jsonpath '<path>' [file]"))
	}

	expr := fs.Arg(0)
	segs, err := parsePath(expr)
	if err != nil {
		return cli.Fail(err)
	}

	data, label, err := cli.ReadInput(fs.Args()[1:])
	if err != nil {
		return cli.Fail(err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return cli.Fail(fmt.Errorf("%s is not valid JSON: %w", label, err))
	}

	var out []match
	walk(root, "$", segs, &out)

	if len(out) == 0 {
		return cli.ExitFail
	}

	if *asJSON {
		docs := make([]map[string]any, len(out))
		for i, m := range out {
			docs[i] = map[string]any{"path": m.path, "value": m.val}
		}
		b, _ := json.MarshalIndent(docs, "", "  ")
		fmt.Println(string(b))
		return cli.ExitOK
	}

	for _, m := range out {
		if *raw {
			fmt.Println(bareValue(m.val))
			continue
		}
		fmt.Printf("%s %s %s\n", cli.Cyan(m.path), cli.Dim("="), jsonRepr(m.val))
	}
	return cli.ExitOK
}

func parsePath(expr string) ([]segment, error) {
	var segs []segment
	i := 0
	if strings.HasPrefix(expr, "$") {
		i = 1
	}
	for i < len(expr) {
		switch {
		case strings.HasPrefix(expr[i:], ".."):
			i += 2
			j := i
			for j < len(expr) && expr[j] != '.' && expr[j] != '[' {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("expected a name after '..'")
			}
			segs = append(segs, segment{kind: "recursive", key: expr[i:j]})
			i = j
		case expr[i] == '.':
			i++
			j := i
			for j < len(expr) && expr[j] != '.' && expr[j] != '[' {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("expected a name after '.'")
			}
			segs = append(segs, segment{kind: "key", key: expr[i:j]})
			i = j
		case expr[i] == '[':
			end := strings.Index(expr[i:], "]")
			if end < 0 {
				return nil, fmt.Errorf("unclosed '[' in %q", expr)
			}
			inner := strings.TrimSpace(expr[i+1 : i+end])
			i += end + 1
			switch {
			case inner == "*":
				segs = append(segs, segment{kind: "wildcard"})
			case len(inner) >= 2 && (inner[0] == '\'' || inner[0] == '"'):
				q := inner[0]
				if inner[len(inner)-1] != q {
					return nil, fmt.Errorf("unclosed quote in %q", inner)
				}
				segs = append(segs, segment{kind: "key", key: inner[1 : len(inner)-1]})
			default:
				n, err := strconv.Atoi(inner)
				if err != nil {
					return nil, fmt.Errorf("unsupported selector [%s]: only indexes, quoted keys and * work", inner)
				}
				segs = append(segs, segment{kind: "index", idx: n})
			}
		default:
			j := i
			for j < len(expr) && expr[j] != '.' && expr[j] != '[' {
				j++
			}
			segs = append(segs, segment{kind: "key", key: expr[i:j]})
			i = j
		}
	}
	return segs, nil
}

func walk(v any, path string, segs []segment, out *[]match) {
	if len(segs) == 0 {
		*out = append(*out, match{path: path, val: v})
		return
	}
	s := segs[0]
	rest := segs[1:]

	switch s.kind {
	case "key":
		if m, ok := v.(map[string]any); ok {
			if val, ok := m[s.key]; ok {
				walk(val, path+"."+s.key, rest, out)
			}
		}
	case "index":
		if a, ok := v.([]any); ok {
			i := s.idx
			if i < 0 {
				i += len(a)
			}
			if i >= 0 && i < len(a) {
				walk(a[i], fmt.Sprintf("%s[%d]", path, i), rest, out)
			}
		}
	case "wildcard":
		switch t := v.(type) {
		case []any:
			for i, e := range t {
				walk(e, fmt.Sprintf("%s[%d]", path, i), rest, out)
			}
		case map[string]any:
			for _, k := range sortedKeys(t) {
				walk(t[k], path+"."+k, rest, out)
			}
		}
	case "recursive":
		descend(v, path, s.key, func(val any, p string) {
			walk(val, p, rest, out)
		})
	}
}

func descend(v any, path, key string, fn func(any, string)) {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range sortedKeys(t) {
			p := path + "." + k
			if key == "*" || k == key {
				fn(t[k], p)
			}
			descend(t[k], p, key, fn)
		}
	case []any:
		for i, e := range t {
			descend(e, fmt.Sprintf("%s[%d]", path, i), key, fn)
		}
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func jsonRepr(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func bareValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return jsonRepr(v)
}
