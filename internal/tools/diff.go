package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "diff",
		Aliases: []string{"d"},
		Summary: "line-by-line diff of two texts, unified format",
		Usage: []string{
			"old.txt new.txt",
			"- < new.txt -f old.txt",
			"[--stat] [--context 3]",
		},
		Run: runDiff,
	})
}

type diffOp struct {
	kind byte // ' ' same, '-' removed, '+' added
	text string
	aIdx int
	bIdx int
}

func runDiff(args []string) int {
	fs := cli.NewFlagSet("diff")
	stat := fs.Bool("stat", false, "print only the change counts")
	ctx := fs.Int("c", 3, "context lines around a change")
	oldFile := fs.String("f", "", "old file when the new text comes from stdin")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	var aData, bData []byte
	var aName, bName string
	if *oldFile != "" {
		b, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		old, err := os.ReadFile(*oldFile)
		if err != nil {
			return cli.Fail(err)
		}
		aData, aName = old, *oldFile
		bData, bName = b, "<stdin>"
	} else {
		if fs.NArg() < 2 {
			return cli.Fail(fmt.Errorf("usage: dkit diff <old> <new>, or dkit diff -f old < new"))
		}
		old, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			return cli.Fail(err)
		}
		newData, err := os.ReadFile(fs.Arg(1))
		if err != nil {
			return cli.Fail(err)
		}
		aData, aName = old, fs.Arg(0)
		bData, bName = newData, fs.Arg(1)
	}

	a := splitLines(string(aData))
	b := splitLines(string(bData))
	ops := diffLines(a, b)

	var added, removed int
	for _, op := range ops {
		switch op.kind {
		case '+':
			added++
		case '-':
			removed++
		}
	}

	if *stat {
		fmt.Printf("%s: %d lines\n%s: %d lines\n", aName, len(a), bName, len(b))
		fmt.Printf("%d insertions(+), %d deletions(-)\n", added, removed)
		if added == 0 && removed == 0 {
			fmt.Println("files are identical")
		}
		return cli.ExitOK
	}

	if added == 0 && removed == 0 {
		fmt.Printf("%s and %s are identical\n", aName, bName)
		return cli.ExitOK
	}

	fmt.Printf("%s\n%s\n", cli.Bold("--- "+aName), cli.Bold("+++ "+bName))
	printHunks(ops, *ctx)
	return cli.ExitOK
}

func printHunks(ops []diffOp, context int) {
	if context < 0 {
		context = 0
	}
	// Mark which ops to show: changed ops plus context around them.
	show := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind == ' ' {
			continue
		}
		lo := i - context
		if lo < 0 {
			lo = 0
		}
		hi := i + context
		if hi >= len(ops) {
			hi = len(ops) - 1
		}
		for j := lo; j <= hi; j++ {
			show[j] = true
		}
	}

	// Walk the visible runs and emit a hunk header for each.
	i := 0
	for i < len(ops) {
		if !show[i] {
			i++
			continue
		}
		start := i
		for i < len(ops) && show[i] {
			i++
		}
		run := ops[start:i]
		aStart, bStart := run[0].aIdx, run[0].bIdx
		aCount, bCount := 0, 0
		for _, op := range run {
			if op.kind != '+' {
				aCount++
			}
			if op.kind != '-' {
				bCount++
			}
		}
		fmt.Println(cli.Cyan(fmt.Sprintf("@@ -%d,%d +%d,%d @@", aStart, aCount, bStart, bCount)))
		for _, op := range run {
			switch op.kind {
			case '+':
				fmt.Println(cli.Green("+" + op.text))
			case '-':
				fmt.Println(cli.Red("-" + op.text))
			default:
				fmt.Println(" " + op.text)
			}
		}
	}
}

// diffLines computes an LCS-based diff. The table is O(n*m); inputs are capped
// so a stray huge file fails loudly instead of hanging.
func diffLines(a, b []string) []diffOp {
	const maxCells = 25_000_000
	if len(a)*len(b) > maxCells {
		// Fall back to a coarse whole-file replacement.
		ops := make([]diffOp, 0, len(a)+len(b))
		for i, l := range a {
			ops = append(ops, diffOp{kind: '-', text: l, aIdx: i + 1, bIdx: 1})
		}
		for j, l := range b {
			ops = append(ops, diffOp{kind: '+', text: l, aIdx: len(a) + 1, bIdx: j + 1})
		}
		return ops
	}

	n, m := len(a), len(b)
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
				continue
			}
			if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}

	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: ' ', text: a[i], aIdx: i + 1, bIdx: j + 1})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			ops = append(ops, diffOp{kind: '-', text: a[i], aIdx: i + 1, bIdx: j + 1})
			i++
		default:
			ops = append(ops, diffOp{kind: '+', text: b[j], aIdx: i + 1, bIdx: j + 1})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{kind: '-', text: a[i], aIdx: i + 1, bIdx: m + 1})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{kind: '+', text: b[j], aIdx: n + 1, bIdx: j + 1})
	}
	return ops
}

func trimForDiff(s string) string { return strings.TrimRight(s, "\r") }
