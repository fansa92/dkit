package tools

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "text",
		Aliases: []string{"tx"},
		Summary: "case conversion, counting, inspect, sort, dedupe, delimiter tables",
		Usage: []string{
			"case -m snake        upper lower title camel pascal snake kebab constant sentence",
			"count                bytes, runes, words, lines",
			"inspect              find non-ASCII, control and zero-width characters",
			"sort [-r] [-n] [-u]",
			"dedup [-c]           drop duplicate lines, -c prefixes counts",
			"delim -d ,           render delimited text as an aligned table",
			"listcmp other.txt    lines only in A, only in B, and shared",
		},
		Run: runText,
	})
}

func runText(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dkit text <case|count|inspect|sort|dedup|delim|listcmp> [flags] [file]")
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "case":
		return textCase(rest)
	case "count", "wc":
		return textCount(rest)
	case "inspect":
		return textInspect(rest)
	case "sort":
		return textSort(rest)
	case "dedup", "uniq":
		return textDedup(rest)
	case "delim":
		return textDelim(rest)
	case "listcmp":
		return textListCmp(rest)
	case "help", "-h", "--help":
		cli.Help("text")
		return cli.ExitOK
	}
	return cli.Fail(fmt.Errorf("unknown text subcommand %q", sub))
}

func textCase(args []string) int {
	fs := cli.NewFlagSet("text case")
	mode := fs.String("m", "upper", "upper, lower, title, camel, pascal, snake, kebab, constant, sentence")
	literal := fs.Bool("literal", false, "treat the argument as text even if a file has that name")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, _, err := cli.Input(fs.Args(), *literal)
	if err != nil {
		return cli.Fail(err)
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return cli.ExitOK
	}

	var out string
	switch *mode {
	case "upper":
		out = strings.ToUpper(s)
	case "lower":
		out = strings.ToLower(s)
	case "title":
		words := strings.Fields(strings.ToLower(s))
		for i := range words {
			words[i] = capitalize(words[i])
		}
		out = strings.Join(words, " ")
	case "camel":
		words := splitWords(s)
		for i, w := range words {
			lw := strings.ToLower(w)
			if i == 0 {
				out += lw
				continue
			}
			out += capitalize(lw)
		}
	case "pascal":
		for _, w := range splitWords(s) {
			out += capitalize(strings.ToLower(w))
		}
	case "snake":
		out = strings.ToLower(strings.Join(splitWords(s), "_"))
	case "kebab":
		out = strings.ToLower(strings.Join(splitWords(s), "-"))
	case "constant":
		out = strings.ToUpper(strings.Join(splitWords(s), "_"))
	case "sentence":
		lower := strings.ToLower(s)
		if lower != "" {
			r, size := utf8.DecodeRuneInString(lower)
			out = string(unicode.ToUpper(r)) + lower[size:]
		}
	default:
		return cli.Fail(fmt.Errorf("unknown mode %q", *mode))
	}
	fmt.Println(out)
	return cli.ExitOK
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func splitWords(s string) []string {
	runes := []rune(s)
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	for i, r := range runes {
		if r == '_' || r == '-' || r == ' ' || r == '.' || r == '/' || r == '\\' {
			flush()
			continue
		}
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

func textCount(args []string) int {
	fs := cli.NewFlagSet("text count")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	s := string(data)
	lines := countLines(s)
	cli.KV(os.Stdout, [][2]string{
		{"source", label},
		{"bytes", strconv.Itoa(len(data))},
		{"characters", strconv.Itoa(utf8.RuneCountInString(s))},
		{"words", strconv.Itoa(len(strings.Fields(s)))},
		{"lines", strconv.Itoa(lines)},
		{"max_line", strconv.Itoa(longestLine(s))},
	})
	return cli.ExitOK
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func longestLine(s string) int {
	max := 0
	for _, l := range strings.Split(s, "\n") {
		if n := utf8.RuneCountInString(l); n > max {
			max = n
		}
	}
	return max
}

func textInspect(args []string) int {
	fs := cli.NewFlagSet("text inspect")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	s := string(data)

	var nonASCII, control, zeroWidth, fullWidth, tabs, crlf int
	for _, r := range s {
		if r > 127 {
			nonASCII++
		}
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			control++
		}
		if r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff' {
			zeroWidth++
		}
		if r >= 0xff01 && r <= 0xff60 {
			fullWidth++
		}
		if r == '\t' {
			tabs++
		}
	}
	crlf = strings.Count(s, "\r\n")

	cli.KV(os.Stdout, [][2]string{
		{"source", label},
		{"non_ascii", strconv.Itoa(nonASCII)},
		{"control", strconv.Itoa(control)},
		{"zero_width", strconv.Itoa(zeroWidth)},
		{"full_width_punct", strconv.Itoa(fullWidth)},
		{"tabs", strconv.Itoa(tabs)},
		{"crlf", strconv.Itoa(crlf)},
		{"utf8_valid", strconv.FormatBool(utf8.ValidString(s))},
	})
	if zeroWidth > 0 {
		fmt.Println()
		fmt.Println("  " + cli.Yellow("note") + "  " + cli.Dim("zero-width characters are often invisible copy/paste artifacts"))
	}
	return cli.ExitOK
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func textSort(args []string) int {
	fs := cli.NewFlagSet("text sort")
	reverse := fs.Bool("r", false, "reverse order")
	numeric := fs.Bool("n", false, "numeric order")
	unique := fs.Bool("u", false, "drop duplicates")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, _, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	lines := splitLines(string(data))

	sort.SliceStable(lines, func(i, j int) bool {
		if *numeric {
			a, errA := strconv.ParseFloat(strings.TrimSpace(lines[i]), 64)
			b, errB := strconv.ParseFloat(strings.TrimSpace(lines[j]), 64)
			if errA == nil && errB == nil && a != b {
				return a < b
			}
		}
		return lines[i] < lines[j]
	})
	if *reverse {
		for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
			lines[i], lines[j] = lines[j], lines[i]
		}
	}
	if *unique {
		lines = uniqueLines(lines)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return cli.ExitOK
}

func uniqueLines(lines []string) []string {
	seen := map[string]bool{}
	out := lines[:0]
	for _, l := range lines {
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

func textDedup(args []string) int {
	fs := cli.NewFlagSet("text dedup")
	withCount := fs.Bool("c", false, "prefix each line with its count")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, _, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	lines := splitLines(string(data))

	order := make([]string, 0, len(lines))
	counts := map[string]int{}
	for _, l := range lines {
		if _, seen := counts[l]; !seen {
			order = append(order, l)
		}
		counts[l]++
	}
	for _, l := range order {
		if *withCount {
			fmt.Printf("%7d %s\n", counts[l], l)
		} else {
			fmt.Println(l)
		}
	}
	return cli.ExitOK
}

func textDelim(args []string) int {
	fs := cli.NewFlagSet("text delim")
	delim := fs.String("d", ",", "field delimiter")
	header := fs.Bool("header", false, "first row is a header (uppercased)")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, _, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	r := csv.NewReader(strings.NewReader(string(data)))
	if len(*delim) > 0 {
		r.Comma = rune((*delim)[0])
	}
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return cli.Fail(err)
	}
	if len(records) == 0 {
		return cli.ExitOK
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i, rec := range records {
		if *header && i == 0 {
			for j := range rec {
				rec[j] = strings.ToUpper(rec[j])
			}
		}
		fmt.Fprintln(tw, strings.Join(rec, "\t"))
	}
	tw.Flush()
	return cli.ExitOK
}

func textListCmp(args []string) int {
	fs := cli.NewFlagSet("text listcmp")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() < 2 {
		return cli.Fail(fmt.Errorf("usage: dkit text listcmp <a.txt> <b.txt>"))
	}
	aData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return cli.Fail(err)
	}
	bData, err := os.ReadFile(fs.Arg(1))
	if err != nil {
		return cli.Fail(err)
	}

	a := splitLines(string(aData))
	b := splitLines(string(bData))
	aSet := map[string]bool{}
	bSet := map[string]bool{}
	for _, l := range a {
		aSet[l] = true
	}
	for _, l := range b {
		bSet[l] = true
	}

	var onlyA, onlyB, both, dupA []string
	for _, l := range a {
		if bSet[l] {
			if !containsString(both, l) {
				both = append(both, l)
			}
		} else if !containsString(onlyA, l) {
			onlyA = append(onlyA, l)
		}
	}
	for _, l := range b {
		if !aSet[l] && !containsString(onlyB, l) {
			onlyB = append(onlyB, l)
		}
	}
	seen := map[string]int{}
	for _, l := range a {
		seen[l]++
		if seen[l] == 2 {
			dupA = append(dupA, l)
		}
	}

	fmt.Printf("%s  %d\n", cli.Bold("only in "+fs.Arg(0)), len(onlyA))
	for _, l := range onlyA {
		fmt.Println("  " + l)
	}
	fmt.Printf("%s  %d\n", cli.Bold("only in "+fs.Arg(1)), len(onlyB))
	for _, l := range onlyB {
		fmt.Println("  " + l)
	}
	fmt.Printf("%s  %d\n", cli.Bold("in both"), len(both))
	for _, l := range both {
		fmt.Println("  " + l)
	}
	if len(dupA) > 0 {
		fmt.Printf("%s  %d\n", cli.Bold("duplicated in "+fs.Arg(0)), len(dupA))
		for _, l := range dupA {
			fmt.Println("  " + l)
		}
	}
	return cli.ExitOK
}

func containsString(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
