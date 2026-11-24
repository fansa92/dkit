package tools

import (
	"fmt"
	"os"
	"runtime"

	"github.com/fansa92/dkit/internal/cli"
)

const (
	repoURL  = "https://github.com/fansa92/dkit"
	upstream = "https://github.com/DevToys-app/DevToys"
)

func init() {
	cli.Register(cli.Command{
		Name:    "about",
		Aliases: []string{"a"},
		Summary: "what dkit is, which build this is, and where it comes from",
		Usage: []string{
			"                 the short version: purpose, build info, links",
			"--short          one line, for scripts and login banners",
		},
		Run: runAbout,
	})
}

func runAbout(args []string) int {
	fs := cli.NewFlagSet("about")
	short := fs.Bool("short", false, "print a single line instead of the full card")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	ver := cli.Version()
	commit, date := cli.Build()
	count := len(cli.Names())

	if *short {
		fmt.Printf("dkit %s (%s) %s/%s %s, %d commands\n",
			ver, shortCommit(commit), runtime.GOOS, runtime.GOARCH, runtime.Version(), count)
		return cli.ExitOK
	}

	fmt.Println(cli.Bold("dkit "+ver) + " - a DevToys you can pipe")
	fmt.Println()
	fmt.Println("  A single static binary for the small things you keep opening a web")
	fmt.Println("  toolbox for: format JSON, read a JWT, convert a timestamp, hash a")
	fmt.Println("  file, decode a certificate, split a big file. Every command reads a")
	fmt.Println("  file or stdin and writes stdout, so it composes in pipes and CI.")
	fmt.Println()

	fmt.Println(cli.Bold("build"))
	cli.KV(os.Stdout, [][2]string{
		{"version", ver},
		{"commit", orUnknown(commit, "unknown (built without -ldflags)")},
		{"built", orUnknown(date, "unknown")},
		{"go", runtime.Version()},
		{"platform", runtime.GOOS + "/" + runtime.GOARCH},
	})

	fmt.Println()
	fmt.Println(cli.Bold("project"))
	cli.KV(os.Stdout, [][2]string{
		{"commands", fmt.Sprintf("%d", count)},
		{"repo", repoURL},
		{"license", "MIT"},
		{"inspired", "DevToys " + upstream + " (the GUI original)"},
	})

	fmt.Println()
	fmt.Println(cli.Bold("try"))
	fmt.Println("  dkit help")
	fmt.Println("  dkit list -a")
	fmt.Println("  echo '{\"a\":1}' | dkit json fmt")
	fmt.Println("  cat token.txt | dkit jwt --payload | dkit jsonpath '$..sub'")
	return cli.ExitOK
}

// shortCommit trims a git SHA to seven characters and names the no-ldflags
// case, so the one-line form never prints an empty pair of parentheses.
func shortCommit(c string) string {
	if c == "" {
		return "dev"
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func orUnknown(s, fallback string) string {
	if s == "" {
		return cli.Dim(fallback)
	}
	return s
}
