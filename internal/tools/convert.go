package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fansa92/dkit/internal/cli"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

func init() {
	cli.Register(cli.Command{
		Name:    "yaml",
		Aliases: []string{"y"},
		Summary: "convert between YAML and JSON",
		Usage: []string{
			"to-json config.yaml",
			"from-json config.json",
			"[-o file]",
		},
		Run: func(args []string) int { return runConvert("yaml", args) },
	})
	cli.Register(cli.Command{
		Name:    "toml",
		Aliases: []string{"tm"},
		Summary: "convert between TOML and JSON",
		Usage: []string{
			"to-json Cargo.toml",
			"from-json config.json",
			"[-o file]",
		},
		Run: func(args []string) int { return runConvert("toml", args) },
	})
}

func runConvert(format string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "usage: dkit %s <to-json|from-json> [flags] [file]\n", format)
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	fs := cli.NewFlagSet(format + " " + sub)
	out := fs.String("o", "", "write the result to a file")
	indent := fs.Int("i", 2, "JSON indent width")
	if code, ok := cli.Parse(fs, rest); !ok {
		return code
	}

	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}

	var result []byte
	switch sub {
	case "to-json":
		var v any
		switch format {
		case "yaml":
			if err := yaml.Unmarshal(data, &v); err != nil {
				return cli.Fail(fmt.Errorf("%s: %w", label, err))
			}
		case "toml":
			if err := toml.Unmarshal(data, &v); err != nil {
				return cli.Fail(fmt.Errorf("%s: %w", label, err))
			}
		}
		b, err := json.MarshalIndent(normalizeDoc(v), "", spaces(*indent))
		if err != nil {
			return cli.Fail(err)
		}
		result = append(b, '\n')
	case "from-json":
		dec := json.NewDecoder(bytes.NewReader(data))
		var v any
		if err := dec.Decode(&v); err != nil {
			return cli.Fail(fmt.Errorf("%s: %w", label, err))
		}
		switch format {
		case "yaml":
			b, err := yaml.Marshal(v)
			if err != nil {
				return cli.Fail(err)
			}
			result = b
		case "toml":
			b, err := toml.Marshal(v)
			if err != nil {
				return cli.Fail(err)
			}
			result = b
		}
	default:
		if sub == "help" || sub == "-h" || sub == "--help" {
			cli.Help(format)
			return cli.ExitOK
		}
		return cli.Fail(fmt.Errorf("unknown subcommand %q, use to-json or from-json", sub))
	}

	if err := cli.WriteOutput(*out, result); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return string(bytes.Repeat([]byte(" "), n))
}

// normalizeDoc rewrites the map[any]any shape YAML can produce into the
// map[string]any shape encoding/json expects.
func normalizeDoc(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[k] = normalizeDoc(val)
		}
		return m
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normalizeDoc(val)
		}
		return m
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalizeDoc(e)
		}
		return out
	}
	return v
}
