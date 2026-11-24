package tools

import (
	"fmt"
	"os"
	"regexp"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "regex",
		Aliases: []string{"r"},
		Summary: "test a regular expression: list matches or replace them",
		Usage: []string{
			"-p '\\d+' data.txt              list every match with line and column",
			"-p '(\\w+)@(\\w+)' -r '$2@$1'    rewrite matches, $1..$n expand groups",
			"-p 'foo' -i                     case-insensitive",
			"[--count] [--quiet]",
		},
		Run: runRegex,
	})
}

func runRegex(args []string) int {
	fs := cli.NewFlagSet("regex")
	pattern := fs.String("p", "", "pattern (RE2 syntax)")
	replace := fs.String("r", "", "replacement template; enables rewrite mode")
	ignoreCase := fs.Bool("i", false, "case-insensitive")
	countOnly := fs.Bool("count", false, "print only the number of matches")
	quiet := fs.Bool("quiet", false, "no output, exit 1 when nothing matches")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if *pattern == "" {
		return cli.Fail(fmt.Errorf("missing -p <pattern>"))
	}

	expr := *pattern
	if *ignoreCase {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return cli.Fail(fmt.Errorf("bad pattern: %w", err))
	}

	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	text := string(data)

	if *replace != "" {
		out := re.ReplaceAllString(text, *replace)
		fmt.Print(out)
		return cli.ExitOK
	}

	locs := re.FindAllStringIndex(text, -1)
	if *countOnly {
		fmt.Println(len(locs))
		if len(locs) == 0 {
			return cli.ExitFail
		}
		return cli.ExitOK
	}
	if *quiet {
		if len(locs) == 0 {
			return cli.ExitFail
		}
		return cli.ExitOK
	}
	if len(locs) == 0 {
		fmt.Fprintf(os.Stderr, "no match in %s\n", label)
		return cli.ExitFail
	}

	fmt.Printf("%s  %d match", cli.Bold(label), len(locs))
	if len(locs) != 1 {
		fmt.Print("es")
	}
	fmt.Println()

	maxShown := 50
	for i, loc := range locs {
		if i >= maxShown {
			fmt.Println(cli.Dim(fmt.Sprintf("  ... %d more", len(locs)-maxShown)))
			break
		}
		line, col := lineColAt(text, loc[0])
		matched := text[loc[0]:loc[1]]
		fmt.Printf("  %d:%d  %s\n", line, col, highlight(matched))
	}
	return cli.ExitOK
}

func lineColAt(text string, offset int) (int, int) {
	line, col := 1, 1
	for i := 0; i < offset && i < len(text); i++ {
		if text[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func highlight(s string) string {
	if cli.NoColor {
		return `"` + s + `"`
	}
	return "\x1b[43m\x1b[30m" + s + "\x1b[0m"
}
