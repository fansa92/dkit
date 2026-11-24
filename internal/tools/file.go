package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "file",
		Aliases: []string{"f"},
		Summary: "split, merge and find duplicate files",
		Usage: []string{
			"split big.bin -s 100MB -o parts/     one part per chunk",
			"merge parts/ -o restored.bin        concatenate parts in name order",
			"dupes ./projects                    group identical files by content",
			"info big.bin                        size in several units",
		},
		Run: runFile,
	})
}

func runFile(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dkit file <split|merge|dupes|info> [flags]")
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "split":
		return fileSplit(rest)
	case "merge":
		return fileMerge(rest)
	case "dupes", "duplicate":
		return fileDupes(rest)
	case "info":
		return fileInfo(rest)
	case "help", "-h", "--help":
		cli.Help("file")
		return cli.ExitOK
	}
	return cli.Fail(fmt.Errorf("unknown file subcommand %q", sub))
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	for _, suffix := range []struct {
		s string
		m int64
	}{
		{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1},
	} {
		if strings.HasSuffix(s, suffix.s) {
			mult = suffix.m
			s = strings.TrimSuffix(s, suffix.s)
			break
		}
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size", s)
	}
	return int64(n * float64(mult)), nil
}

func fileSplit(args []string) int {
	fs := cli.NewFlagSet("file split")
	sizeStr := fs.String("s", "10MB", "chunk size, e.g. 512KB, 100MB, 2GB")
	outDir := fs.String("o", "", "output directory (default: alongside the input)")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit file split <file> -s 100MB [-o dir]"))
	}
	path := fs.Arg(0)
	size, err := parseSize(*sizeStr)
	if err != nil {
		return cli.Fail(err)
	}
	if size <= 0 {
		return cli.Fail(fmt.Errorf("chunk size must be positive"))
	}

	src, err := os.Open(path)
	if err != nil {
		return cli.Fail(err)
	}
	defer src.Close()

	dir := *outDir
	if dir == "" {
		dir = filepath.Dir(path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return cli.Fail(err)
	}

	base := filepath.Base(path)
	buf := make([]byte, 1<<20)
	part := 1
	var written int64
	var out *os.File

	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if out == nil {
				name := filepath.Join(dir, fmt.Sprintf("%s.%03d", base, part))
				out, err = os.Create(name)
				if err != nil {
					return cli.Fail(err)
				}
				fmt.Printf("  %s\n", name)
			}
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return cli.Fail(werr)
			}
			written += int64(n)
			if written >= size {
				out.Close()
				out = nil
				written = 0
				part++
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return cli.Fail(rerr)
		}
	}
	if out != nil {
		out.Close()
	}
	fmt.Printf("\n%d parts written to %s\n", part-1+boolToInt(written > 0), dir)
	return cli.ExitOK
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func fileMerge(args []string) int {
	fs := cli.NewFlagSet("file merge")
	out := fs.String("o", "", "output file (required)")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 || *out == "" {
		return cli.Fail(fmt.Errorf("usage: dkit file merge <dir|part1 part2 ...> -o out.bin"))
	}

	var parts []string
	if fi, err := os.Stat(fs.Arg(0)); err == nil && fi.IsDir() {
		entries, err := os.ReadDir(fs.Arg(0))
		if err != nil {
			return cli.Fail(err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				parts = append(parts, filepath.Join(fs.Arg(0), e.Name()))
			}
		}
		sort.Strings(parts)
	} else {
		parts = fs.Args()
	}
	if len(parts) == 0 {
		return cli.Fail(fmt.Errorf("no parts to merge"))
	}

	dst, err := os.Create(*out)
	if err != nil {
		return cli.Fail(err)
	}
	defer dst.Close()

	var total int64
	for _, p := range parts {
		f, err := os.Open(p)
		if err != nil {
			return cli.Fail(err)
		}
		n, err := io.Copy(dst, f)
		f.Close()
		if err != nil {
			return cli.Fail(err)
		}
		total += n
	}
	fmt.Printf("merged %d parts into %s (%s)\n", len(parts), *out, humanBytes(total))
	return cli.ExitOK
}

type fileEntry struct {
	path string
	size int64
	hash string
}

func fileDupes(args []string) int {
	fs := cli.NewFlagSet("file dupes")
	minSize := fs.Int64("min", 1, "ignore files smaller than this many bytes")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}

	bySize := map[int64][]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".svn" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < *minSize {
			return nil
		}
		bySize[info.Size()] = append(bySize[info.Size()], path)
		return nil
	})
	if err != nil {
		return cli.Fail(err)
	}

	var groups [][]fileEntry
	for size, paths := range bySize {
		if len(paths) < 2 {
			continue
		}
		byHash := map[string][]string{}
		for _, p := range paths {
			h, err := hashFile(p)
			if err != nil {
				continue
			}
			byHash[h] = append(byHash[h], p)
		}
		for h, ps := range byHash {
			if len(ps) < 2 {
				continue
			}
			sort.Strings(ps)
			g := make([]fileEntry, len(ps))
			for i, p := range ps {
				g[i] = fileEntry{path: p, size: size, hash: h}
			}
			groups = append(groups, g)
		}
	}

	if len(groups) == 0 {
		fmt.Printf("no duplicate files under %s\n", root)
		return cli.ExitOK
	}

	var wasted int64
	for _, g := range groups {
		wasted += g[0].size * int64(len(g)-1)
	}
	fmt.Printf("%d duplicate groups, %s reclaimable\n\n", len(groups), humanBytes(wasted))
	for _, g := range groups {
		fmt.Printf("%s  (%s each, %d copies)\n", cli.Cyan(g[0].hash[:12]), humanBytes(g[0].size), len(g))
		for _, f := range g {
			fmt.Println("  " + f.path)
		}
		fmt.Println()
	}
	return cli.ExitOK
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileInfo(args []string) int {
	fs := cli.NewFlagSet("file info")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit file info <path>"))
	}
	fi, err := os.Stat(fs.Arg(0))
	if err != nil {
		return cli.Fail(err)
	}
	rows := [][2]string{
		{"name", fi.Name()},
		{"size", humanBytes(fi.Size())},
		{"bytes", strconv.FormatInt(fi.Size(), 10)},
		{"modified", fi.ModTime().Format("2006-01-02 15:04:05")},
		{"mode", fi.Mode().String()},
		{"is_dir", strconv.FormatBool(fi.IsDir())},
	}
	cli.KV(os.Stdout, rows)
	return cli.ExitOK
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
