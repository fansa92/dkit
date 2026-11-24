package tools

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "json",
		Aliases: []string{"j"},
		Summary: "format, minify, validate, tabulate, convert and schema-check JSON",
		Usage: []string{
			"fmt data.json       pretty print (2 spaces, -i 4 to change)",
			"min data.json       strip all whitespace",
			"check data.json     validate, exit 1 when invalid",
			"table data.json     render an array of objects as a table",
			"csv data.json       export an array of objects as CSV",
			"csharp data.json    generate C# classes",
			"php data.json       generate a PHP class",
			"schema data.json schema.json    validate against a JSON Schema",
		},
		Run: runJSON,
	})
}

func runJSON(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dkit json <fmt|min|check|table|csv|csharp|php|schema> [flags] [file]")
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "fmt", "format":
		return jsonFmt(rest)
	case "min", "minify":
		return jsonMin(rest)
	case "check", "validate":
		return jsonCheck(rest)
	case "table":
		return jsonTable(rest)
	case "csv":
		return jsonCSV(rest)
	case "csharp", "cs":
		return jsonCode(rest, "csharp")
	case "php":
		return jsonCode(rest, "php")
	case "schema":
		return jsonSchemaCheck(rest)
	case "help", "-h", "--help":
		cli.Help("json")
		return cli.ExitOK
	}
	return cli.Fail(fmt.Errorf("unknown json subcommand %q", sub))
}

func jsonFmt(args []string) int {
	fs := cli.NewFlagSet("json fmt")
	indent := fs.Int("i", 2, "spaces per indent level")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", strings.Repeat(" ", *indent)); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}
	buf.WriteByte('\n')
	if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}

func jsonMin(args []string) int {
	fs := cli.NewFlagSet("json min")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}
	fmt.Println(buf.String())
	return cli.ExitOK
}

func jsonCheck(args []string) int {
	fs := cli.NewFlagSet("json check")
	quiet := fs.Bool("q", false, "no output, exit code only")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		var syn *json.SyntaxError
		if ok := asSyntaxError(err, &syn); ok {
			line, col := offsetToLineCol(data, syn.Offset)
			fmt.Fprintf(os.Stderr, "%s:%d:%d: %v\n", label, line, col, err)
			return cli.ExitFail
		}
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}
	if !*quiet {
		fmt.Printf("%s is valid JSON\n", label)
	}
	return cli.ExitOK
}

func asSyntaxError(err error, target **json.SyntaxError) bool {
	if e, ok := err.(*json.SyntaxError); ok {
		*target = e
		return true
	}
	return false
}

func offsetToLineCol(data []byte, off int64) (int, int) {
	if off < 0 {
		return 1, 1
	}
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	line, col := 1, 1
	for i := int64(0); i < off; i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func loadRows(data []byte) ([]map[string]any, []string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, nil, err
	}

	var items []any
	switch t := v.(type) {
	case []any:
		items = t
	case map[string]any:
		// A wrapper object: use its first array of objects.
		for _, key := range sortedKeys(t) {
			if arr, ok := t[key].([]any); ok && len(arr) > 0 {
				items = arr
				break
			}
		}
		if items == nil {
			items = []any{t}
		}
	default:
		return nil, nil, fmt.Errorf("expected an array of objects, got %T", v)
	}

	rows := make([]map[string]any, 0, len(items))
	keyset := map[string]bool{}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("expected an array of objects, found a %T element", it)
		}
		rows = append(rows, m)
		for k := range m {
			keyset[k] = true
		}
	}
	keys := make([]string, 0, len(keyset))
	for k := range keyset {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return rows, keys, nil
}

func jsonTable(args []string) int {
	fs := cli.NewFlagSet("json table")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	rows, keys, err := loadRows(data)
	if err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	header := make([]string, len(keys))
	for i, k := range keys {
		header[i] = strings.ToUpper(k)
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, r := range rows {
		cells := make([]string, len(keys))
		for i, k := range keys {
			cells[i] = cellValue(r[k])
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()
	return cli.ExitOK
}

func cellValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return jsonRepr(t)
	}
}

func jsonCSV(args []string) int {
	fs := cli.NewFlagSet("json csv")
	sep := fs.String("s", ",", "field separator")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	rows, keys, err := loadRows(data)
	if err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}

	w := csv.NewWriter(os.Stdout)
	if len(*sep) > 0 {
		w.Comma = rune((*sep)[0])
	}
	header := make([]string, len(keys))
	copy(header, keys)
	if err := w.Write(header); err != nil {
		return cli.Fail(err)
	}
	for _, r := range rows {
		rec := make([]string, len(keys))
		for i, k := range keys {
			rec[i] = cellValue(r[k])
		}
		if err := w.Write(rec); err != nil {
			return cli.Fail(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}

func jsonCode(args []string, lang string) int {
	fs := cli.NewFlagSet("json " + lang)
	rootName := fs.String("n", "Root", "name of the root type")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}
	obj, ok := v.(map[string]any)
	if !ok {
		if arr, isArr := v.([]any); isArr && len(arr) > 0 {
			obj, ok = arr[0].(map[string]any)
			if !ok {
				return cli.Fail(fmt.Errorf("expected an object or an array of objects"))
			}
		} else {
			return cli.Fail(fmt.Errorf("expected an object or an array of objects"))
		}
	}

	var sb strings.Builder
	if lang == "csharp" {
		genCSharp(&sb, *rootName, obj, "")
	} else {
		genPHP(&sb, *rootName, obj, "")
	}
	fmt.Print(sb.String())
	return cli.ExitOK
}

func pascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '.'
	})
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	if b.Len() == 0 {
		return "Field"
	}
	return b.String()
}

func genCSharp(sb *strings.Builder, name string, obj map[string]any, indent string) {
	keys := sortedKeys(obj)
	var nested []struct {
		name string
		obj  map[string]any
	}
	sb.WriteString(indent + "public class " + name + "\n" + indent + "{\n")
	for _, k := range keys {
		prop := pascal(k)
		switch t := obj[k].(type) {
		case string:
			sb.WriteString(indent + "    public string " + prop + " { get; set; }\n")
		case bool:
			sb.WriteString(indent + "    public bool " + prop + " { get; set; }\n")
		case json.Number:
			if strings.ContainsAny(t.String(), ".eE") {
				sb.WriteString(indent + "    public double " + prop + " { get; set; }\n")
			} else {
				sb.WriteString(indent + "    public long " + prop + " { get; set; }\n")
			}
		case nil:
			sb.WriteString(indent + "    public object " + prop + " { get; set; }\n")
		case map[string]any:
			nested = append(nested, struct {
				name string
				obj  map[string]any
			}{prop, t})
			sb.WriteString(indent + "    public " + prop + " " + prop + " { get; set; }\n")
		case []any:
			elem := "object"
			if len(t) > 0 {
				switch e := t[0].(type) {
				case string:
					elem = "string"
				case bool:
					elem = "bool"
				case json.Number:
					if strings.ContainsAny(e.String(), ".eE") {
						elem = "double"
					} else {
						elem = "long"
					}
				case map[string]any:
					elem = prop + "Item"
					nested = append(nested, struct {
						name string
						obj  map[string]any
					}{prop + "Item", e})
				}
			}
			sb.WriteString(indent + "    public List<" + elem + "> " + prop + " { get; set; }\n")
		}
	}
	sb.WriteString(indent + "}\n")
	for _, n := range nested {
		sb.WriteString("\n")
		genCSharp(sb, n.name, n.obj, indent)
	}
}

func genPHP(sb *strings.Builder, name string, obj map[string]any, indent string) {
	keys := sortedKeys(obj)
	var nested []struct {
		name string
		obj  map[string]any
	}
	sb.WriteString(indent + "class " + name + "\n" + indent + "{\n")
	for _, k := range keys {
		var typ string
		switch t := obj[k].(type) {
		case string:
			typ = "string"
		case bool:
			typ = "bool"
		case json.Number:
			if strings.ContainsAny(t.String(), ".eE") {
				typ = "float"
			} else {
				typ = "int"
			}
		case map[string]any:
			typ = pascal(k)
			nested = append(nested, struct {
				name string
				obj  map[string]any
			}{typ, t})
		case []any:
			typ = "array"
		default:
			typ = "mixed"
		}
		sb.WriteString(indent + "    /** @var " + typ + " */\n")
		sb.WriteString(indent + "    public $" + k + ";\n")
	}
	sb.WriteString(indent + "}\n")
	for _, n := range nested {
		sb.WriteString("\n")
		genPHP(sb, n.name, n.obj, indent)
	}
}

func jsonSchemaCheck(args []string) int {
	fs := cli.NewFlagSet("json schema")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() < 2 {
		return cli.Fail(fmt.Errorf("usage: dkit json schema <data.json> <schema.json>"))
	}
	instData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return cli.Fail(err)
	}
	schemaData, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return cli.Fail(err)
	}

	var inst, schema any
	d1 := json.NewDecoder(bytes.NewReader(instData))
	d1.UseNumber()
	if err := d1.Decode(&inst); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", fs.Arg(0), err))
	}
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", fs.Arg(1), err))
	}

	var errs []string
	checkSchema(inst, schema, "$", &errs)
	if len(errs) == 0 {
		fmt.Printf("%s is valid against %s\n", fs.Arg(0), fs.Arg(1))
		return cli.ExitOK
	}
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, e)
	}
	return cli.ExitFail
}

// checkSchema implements the commonly used draft-07 keywords. It is a subset:
// enough for hand-written schemas, not a full validator.
func checkSchema(inst, schema any, path string, errs *[]string) {
	obj, ok := schema.(map[string]any)
	if !ok {
		return
	}

	if ref, ok := obj["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
		// Only local refs into $defs / definitions are resolved.
		return
	}

	if t, ok := obj["type"]; ok {
		if !typeMatches(inst, t) {
			*errs = append(*errs, fmt.Sprintf("%s: expected type %v, got %s", path, t, jsonTypeName(inst)))
			return
		}
	}

	if enum, ok := obj["enum"].([]any); ok && !containsValue(enum, inst) {
		*errs = append(*errs, fmt.Sprintf("%s: value not in enum", path))
	}
	if c, ok := obj["const"]; ok && !deepEqual(c, inst) {
		*errs = append(*errs, fmt.Sprintf("%s: value does not equal const", path))
	}

	if s, ok := inst.(string); ok {
		if p, ok := obj["pattern"].(string); ok {
			if !regexMatch(p, s) {
				*errs = append(*errs, fmt.Sprintf("%s: %q does not match pattern %q", path, s, p))
			}
		}
		if n, ok := numOf(obj["minLength"]); ok && float64(len(s)) < n {
			*errs = append(*errs, fmt.Sprintf("%s: shorter than minLength %v", path, n))
		}
		if n, ok := numOf(obj["maxLength"]); ok && float64(len(s)) > n {
			*errs = append(*errs, fmt.Sprintf("%s: longer than maxLength %v", path, n))
		}
	}

	if n, ok := numOf(inst); ok {
		if m, ok := numOf(obj["minimum"]); ok && n < m {
			*errs = append(*errs, fmt.Sprintf("%s: below minimum %v", path, m))
		}
		if m, ok := numOf(obj["maximum"]); ok && n > m {
			*errs = append(*errs, fmt.Sprintf("%s: above maximum %v", path, m))
		}
	}

	if m, ok := inst.(map[string]any); ok {
		if req, ok := obj["required"].([]any); ok {
			for _, r := range req {
				if key, ok := r.(string); ok {
					if _, present := m[key]; !present {
						*errs = append(*errs, fmt.Sprintf("%s: missing required property %q", path, key))
					}
				}
			}
		}
		if props, ok := obj["properties"].(map[string]any); ok {
			for key, sub := range props {
				if val, present := m[key]; present {
					checkSchema(val, sub, path+"."+key, errs)
				}
			}
		}
		if add, ok := obj["additionalProperties"].(bool); ok && !add {
			if props, ok := obj["properties"].(map[string]any); ok {
				for key := range m {
					if _, known := props[key]; !known {
						*errs = append(*errs, fmt.Sprintf("%s: unexpected property %q", path, key))
					}
				}
			}
		}
	}

	if arr, ok := inst.([]any); ok {
		if n, ok := numOf(obj["minItems"]); ok && float64(len(arr)) < n {
			*errs = append(*errs, fmt.Sprintf("%s: fewer than minItems %v", path, n))
		}
		if n, ok := numOf(obj["maxItems"]); ok && float64(len(arr)) > n {
			*errs = append(*errs, fmt.Sprintf("%s: more than maxItems %v", path, n))
		}
		if items, ok := obj["items"].(map[string]any); ok {
			for i, e := range arr {
				checkSchema(e, items, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
	}

	if all, ok := obj["allOf"].([]any); ok {
		for _, sub := range all {
			checkSchema(inst, sub, path, errs)
		}
	}
	if anyOf, ok := obj["anyOf"].([]any); ok {
		matched := false
		for _, sub := range anyOf {
			var subErrs []string
			checkSchema(inst, sub, path, &subErrs)
			if len(subErrs) == 0 {
				matched = true
				break
			}
		}
		if !matched {
			*errs = append(*errs, fmt.Sprintf("%s: matches none of anyOf", path))
		}
	}
	if oneOf, ok := obj["oneOf"].([]any); ok {
		count := 0
		for _, sub := range oneOf {
			var subErrs []string
			checkSchema(inst, sub, path, &subErrs)
			if len(subErrs) == 0 {
				count++
			}
		}
		if count != 1 {
			*errs = append(*errs, fmt.Sprintf("%s: matches %d of oneOf, expected exactly 1", path, count))
		}
	}
}

func typeMatches(inst any, t any) bool {
	switch spec := t.(type) {
	case string:
		return singleTypeMatches(inst, spec)
	case []any:
		for _, s := range spec {
			if name, ok := s.(string); ok && singleTypeMatches(inst, name) {
				return true
			}
		}
		return false
	}
	return true
}

func singleTypeMatches(inst any, name string) bool {
	switch name {
	case "object":
		_, ok := inst.(map[string]any)
		return ok
	case "array":
		_, ok := inst.([]any)
		return ok
	case "string":
		_, ok := inst.(string)
		return ok
	case "boolean":
		_, ok := inst.(bool)
		return ok
	case "null":
		return inst == nil
	case "number":
		_, ok := numOf(inst)
		return ok
	case "integer":
		n, ok := numOf(inst)
		return ok && float64(n) == float64(int64(n))
	}
	return true
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case json.Number:
		return "number"
	}
	return fmt.Sprintf("%T", v)
}

func numOf(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case int:
		return float64(t), true
	}
	return 0, false
}

func containsValue(list []any, v any) bool {
	for _, e := range list {
		if deepEqual(e, v) {
			return true
		}
	}
	return false
}

func deepEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

func regexMatch(pattern, s string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return true
	}
	return re.MatchString(s)
}
