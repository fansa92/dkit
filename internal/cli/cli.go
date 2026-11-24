// Package cli holds the command registry, terminal formatting and the small
// IO conventions every dkit tool shares: read stdin, write stdout, chain with pipes.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Exit codes. Anything non-zero makes a shell script stop with `set -e`.
const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2
)

// Command is one subcommand of dkit.
type Command struct {
	Name    string
	Aliases []string
	Summary string
	Usage   []string
	Run     func(args []string) int
}

var commands = map[string]Command{}
var aliasOf = map[string]string{}

// Build metadata, filled in by main from -ldflags -X at release time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// Register adds a command to the global registry. Tools call this from init().
// Aliases are optional 1-2 letter shortcuts; they must not collide with another
// command name or alias, which is checked here because a clash is always a
// programming mistake rather than a runtime condition.
func Register(c Command) {
	if prev, taken := aliasOf[c.Name]; taken {
		panic("dkit: command " + c.Name + " is already the alias of " + prev)
	}
	commands[c.Name] = c
	for _, a := range c.Aliases {
		if a == "" || a == c.Name {
			continue
		}
		if prev, taken := aliasOf[a]; taken && prev != c.Name {
			panic("dkit: alias " + a + " is claimed by both " + prev + " and " + c.Name)
		}
		if _, clash := commands[a]; clash {
			panic("dkit: alias " + a + " collides with the command of the same name")
		}
		aliasOf[a] = c.Name
	}
}

// Lookup resolves a command name or alias.
func Lookup(name string) (Command, bool) {
	if c, ok := commands[name]; ok {
		return c, true
	}
	if canon, ok := aliasOf[name]; ok {
		return commands[canon], true
	}
	return Command{}, false
}

// SetBuildInfo records the version, git commit and build date. The release
// workflow passes all three through -ldflags, so `dkit about` can report the
// exact build a binary came from.
func SetBuildInfo(v, c, d string) {
	if v != "" {
		version = v
	}
	commit = c
	date = d
}

// Version returns the build version.
func Version() string { return version }

// Build returns the git commit and build date, either of which may be empty
// for binaries built with a plain `go build`.
func Build() (commitID, built string) { return commit, date }

// Names returns the registered command names in alphabetical order.
func Names() []string {
	out := make([]string, 0, len(commands))
	for n := range commands {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run dispatches to a subcommand and returns the process exit code.
func Run(args []string) int {
	args = extractGlobalFlags(args)
	if len(args) == 0 {
		printHelp()
		return ExitUsage
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) > 1 {
			return commandHelp(args[1])
		}
		printHelp()
		return ExitOK
	case "version", "-v", "--version":
		fmt.Println("dkit " + version)
		return ExitOK
	case "list":
		if len(args) > 1 && (args[1] == "-a" || args[1] == "--aliases") {
			printAliases()
			return ExitOK
		}
		for _, n := range Names() {
			fmt.Println(n)
		}
		return ExitOK
	}

	c, ok := Lookup(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "dkit: unknown command %q\n", args[0])
		if near := suggest(args[0]); len(near) > 0 {
			fmt.Fprintln(os.Stderr, "did you mean: "+strings.Join(near, ", "))
			return ExitUsage
		}
		fmt.Fprintln(os.Stderr)
		printHelp()
		return ExitUsage
	}
	return c.Run(args[1:])
}

// suggest lists commands and aliases that start with the given prefix, so a
// half-typed word points at the right name instead of dumping the whole help.
func suggest(word string) []string {
	var out []string
	for _, n := range Names() {
		if strings.HasPrefix(n, word) {
			out = append(out, n)
		}
	}
	for a, canon := range aliasOf {
		if strings.HasPrefix(a, word) {
			out = append(out, a+" ("+canon+")")
		}
	}
	sort.Strings(out)
	return out
}

func printAliases() {
	names := Names()
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		alias := ""
		if len(commands[n].Aliases) > 0 {
			alias = commands[n].Aliases[0]
		}
		pad := strings.Repeat(" ", width-len(n))
		fmt.Printf("  %s%s  %s\n", n, pad, Dim(alias))
	}
}

// extractGlobalFlags pulls program-level flags out of args and applies them.
// Tokens after a "--" separator are left alone, so `--` still protects literals.
func extractGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i, a := range args {
		if a == "--" {
			return append(out, args[i:]...)
		}
		if a == "--no-color" {
			NoColor = true
			continue
		}
		out = append(out, a)
	}
	return out
}

func printHelp() {
	fmt.Println(Bold("dkit") + " - a DevToys you can pipe")
	fmt.Println()
	fmt.Println("usage: dkit <command> [flags] [input]")
	fmt.Println()

	names := Names()
	width, aliasWidth := 0, 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
		for _, a := range commands[n].Aliases {
			if len(a) > aliasWidth {
				aliasWidth = len(a)
			}
		}
	}
	fmt.Println("commands:")
	for _, n := range names {
		alias := ""
		if a := commands[n].Aliases; len(a) > 0 {
			alias = a[0]
		}
		padName := strings.Repeat(" ", width-len(n))
		padAlias := strings.Repeat(" ", aliasWidth-len(alias))
		fmt.Printf("  %s%s  %s%s  %s\n", Cyan(n), padName, Dim(alias), padAlias, commands[n].Summary)
	}

	fmt.Println()
	fmt.Println("input:")
	fmt.Println("  every command reads a file argument or stdin, and writes stdout,")
	fmt.Println("  so they chain:  cat token.txt | dkit jwt --payload | dkit jsonpath '$..sub'")
	fmt.Println()
	fmt.Println("global:")
	fmt.Println("  --no-color   force plain output (NO_COLOR and non-tty stdout do the same)")
	fmt.Println()
	fmt.Println("aliases:")
	fmt.Println("  every command has a 1-2 letter alias, shown in the list above:")
	fmt.Println("  `dkit j` is `dkit json`, `dkit jp` is `dkit jsonpath`, `dkit t` is `dkit ts`")
	fmt.Println()
	fmt.Println(Dim("run `dkit help <command>` for the flags of one command"))
}

func commandHelp(name string) int {
	c, ok := Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "dkit: unknown command %q\n", name)
		return ExitUsage
	}
	printCommandHelp(os.Stdout, c.Name, nil)
	fmt.Println()
	fmt.Println(Dim("run `dkit " + c.Name + " --help` to list its flags"))
	return ExitOK
}

// printCommandHelp writes the block shared by `dkit help <cmd>` and
// `dkit <cmd> --help`. fs may be nil when the flags are not known yet.
// Help prints the same block as `dkit help <name>`. Commands that dispatch on
// their own subcommand call this so `<cmd> --help` works there too.
func Help(name string) {
	printCommandHelp(os.Stdout, name, nil)
}

func printCommandHelp(w io.Writer, name string, fs *flag.FlagSet) {
	if c, ok := commands[name]; ok {
		fmt.Fprintln(w, Bold(c.Name)+" - "+c.Summary)
		fmt.Fprintln(w)
		if len(c.Usage) == 0 {
			fmt.Fprintln(w, "usage: dkit "+c.Name)
		} else {
			for i, line := range c.Usage {
				if i == 0 {
					fmt.Fprintln(w, "usage: dkit "+c.Name+" "+line)
					continue
				}
				fmt.Fprintln(w, "       dkit "+c.Name+" "+line)
			}
		}
	} else {
		fmt.Fprintln(w, Bold("dkit "+name))
		fmt.Fprintln(w)
		fmt.Fprintln(w, "usage: dkit "+name+" [flags] [input]")
	}
	if fs != nil {
		printFlagList(w, fs)
	}
}

// printFlagList renders the flag table. Go's own output breaks each flag across
// two lines and starts with "Usage of x:", which does not match the rest of the
// help text, so the table is rebuilt here.
func printFlagList(w io.Writer, fs *flag.FlagSet) {
	var left, desc []string
	fs.VisitAll(func(f *flag.Flag) {
		l := "-" + f.Name
		if !isBoolFlag(f) {
			l += " " + flagTypeName(f)
		}
		d := f.Usage
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
			d += " (default " + f.DefValue + ")"
		}
		left = append(left, l)
		desc = append(desc, d)
	})
	if len(left) == 0 {
		return
	}

	width := 0
	for _, l := range left {
		if len(l) > width {
			width = len(l)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	for i, l := range left {
		fmt.Fprintf(w, "  %s%s  %s\n", Dim(l), spaces(width-len(l)), desc[i])
	}
}

func flagTypeName(f *flag.Flag) string {
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return "value"
	}
	switch g.Get().(type) {
	case string:
		return "string"
	case bool:
		return "bool"
	case int, int64:
		return "int"
	case uint:
		return "uint"
	case float64:
		return "float"
	case time.Duration:
		return "duration"
	}
	return "value"
}

// ReadInput returns the payload for a command: the file named in args, or stdin
// when args is empty or its first element is "-".
func ReadInput(args []string) (data []byte, label string, err error) {
	if len(args) == 0 || args[0] == "-" {
		data, err = io.ReadAll(os.Stdin)
		return data, "<stdin>", err
	}
	data, err = os.ReadFile(args[0])
	return data, args[0], err
}

// Input is the friendlier variant: a first argument that names an existing file
// is read as that file, anything else is taken as a literal string, and no
// argument at all reads stdin. forceLiteral skips the file probe.
func Input(args []string, forceLiteral bool) (data []byte, label string, err error) {
	if len(args) == 0 || args[0] == "-" {
		data, err = io.ReadAll(os.Stdin)
		return data, "<stdin>", err
	}
	if !forceLiteral {
		if fi, statErr := os.Stat(args[0]); statErr == nil && !fi.IsDir() {
			data, err = os.ReadFile(args[0])
			return data, args[0], err
		}
	}
	return []byte(args[0]), "<arg>", nil
}

// WriteOutput sends data to the -o target, or stdout when target is empty.
func WriteOutput(target string, data []byte) error {
	if target == "" || target == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

// NewFlagSet returns a FlagSet that reports usage errors through the exit code
// instead of killing the process. The default usage printer is silenced: Parse
// renders help itself so that `dkit <cmd> --help` and `dkit help <cmd>` look
// the same.
func NewFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {}
	return fs
}

// Parse reorders args so flags may follow positional arguments, then parses.
// Go's flag package stops at the first positional token, which would silently
// swallow `dkit ts 1700 --tz UTC`.
func Parse(fs *flag.FlagSet, args []string) (int, bool) {
	err := fs.Parse(ReorderFlags(fs, args))
	switch {
	case err == nil:
		return ExitOK, true
	case err == flag.ErrHelp:
		printCommandHelp(os.Stdout, fs.Name(), fs)
		return ExitOK, false
	default:
		fmt.Fprintf(os.Stderr, "dkit: run `dkit %s --help` to see the flags\n", fs.Name())
		return ExitUsage, false
	}
}

// ReorderFlags moves flag tokens, and the values of flags that take one, ahead
// of positional arguments. A lone "-" and negative numbers stay positional.
func ReorderFlags(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, a)
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' && !looksNegative(a) {
			name := strings.TrimLeft(a, "-")
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				flags = append(flags, a)
				continue
			}
			f := fs.Lookup(name)
			flags = append(flags, a)
			if f != nil && !isBoolFlag(f) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func looksNegative(s string) bool {
	if len(s) < 2 || s[0] != '-' {
		return false
	}
	r := s[1]
	return r >= '0' && r <= '9'
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// Fail prints an error the way every dkit command does and returns the code.
func Fail(err error) int {
	fmt.Fprintf(os.Stderr, "dkit: %v\n", err)
	return ExitFail
}
