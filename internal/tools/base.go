package tools

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "base",
		Aliases: []string{"b"},
		Summary: "convert a number between binary, octal, decimal and hex",
		Usage: []string{
			"255              show 255 in every base",
			"0xff             prefixes decide the input base",
			"-f 16 -t 2 ff    explicit conversion, value only, pipe friendly",
			"[--json]",
		},
		Run: runBase,
	})
}

func runBase(args []string) int {
	fs := cli.NewFlagSet("base")
	from := fs.Int("f", 0, "input base (0 = auto-detect from prefix, decimal otherwise)")
	to := fs.Int("t", 0, "output base (0 = show bin/oct/dec/hex)")
	asJSON := fs.Bool("json", false, "emit JSON for scripting")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	var raw string
	if fs.NArg() > 0 {
		raw = fs.Arg(0)
	} else {
		b, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		raw = strings.TrimSpace(string(b))
	}
	raw = strings.ReplaceAll(strings.TrimSpace(raw), "_", "")
	if raw == "" {
		return cli.Fail(fmt.Errorf("no number given"))
	}

	body, prefixBase, neg := splitPrefix(raw)
	base := *from
	if base == 0 {
		base = prefixBase
	}
	if base == 0 {
		base = 10
	}
	if base < 2 || base > 36 {
		return cli.Fail(fmt.Errorf("base %d out of range 2..36", base))
	}

	n, ok := new(big.Int).SetString(body, base)
	if !ok {
		return cli.Fail(fmt.Errorf("%q is not a valid base-%d number", body, base))
	}
	if neg {
		n.Neg(n)
	}

	if *to != 0 {
		if *to < 2 || *to > 36 {
			return cli.Fail(fmt.Errorf("output base %d out of range 2..36", *to))
		}
		if *asJSON {
			emitJSON(map[string]any{"value": n.Text(*to), "base": *to, "input": raw})
			return cli.ExitOK
		}
		fmt.Println(n.Text(*to))
		return cli.ExitOK
	}

	rows := [][2]string{
		{"bin", prefixed(n, 2, "0b")},
		{"oct", prefixed(n, 8, "0o")},
		{"dec", n.Text(10)},
		{"hex", prefixed(n, 16, "0x")},
	}
	if *asJSON {
		doc := map[string]any{}
		for _, r := range rows {
			doc[r[0]] = r[1]
		}
		doc["input"] = raw
		doc["inputBase"] = base
		emitJSON(doc)
		return cli.ExitOK
	}
	cli.KV(os.Stdout, rows)
	return cli.ExitOK
}

func splitPrefix(s string) (body string, base int, neg bool) {
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if len(s) > 2 {
		switch strings.ToLower(s[:2]) {
		case "0x":
			return s[2:], 16, neg
		case "0b":
			return s[2:], 2, neg
		case "0o":
			return s[2:], 8, neg
		}
	}
	return s, 0, neg
}

func prefixed(n *big.Int, base int, prefix string) string {
	if n.Sign() < 0 {
		return "-" + prefix + new(big.Int).Abs(n).Text(base)
	}
	return prefix + n.Text(base)
}

func emitJSON(v any) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println("{}")
		return
	}
	fmt.Println(string(out))
}
