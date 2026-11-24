package tools

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "chmod",
		Aliases: []string{"cm"},
		Summary: "convert between octal and symbolic file permissions",
		Usage: []string{
			"755            -> rwxr-xr-x",
			"rwxr-xr-x      -> 755",
			"u+x            understood for reference only: prints 755 style result",
		},
		Run: runChmod,
	})
	cli.Register(cli.Command{
		Name:    "semver",
		Aliases: []string{"sv"},
		Summary: "parse, compare and bump semantic versions",
		Usage: []string{
			"1.2.3-beta.1+build.5",
			"-bump patch|minor|major    next version",
			"-cmp 1.3.0                 prints -1, 0 or 1",
			"-check                     exit 1 when not valid semver",
		},
		Run: runSemver,
	})
	cli.Register(cli.Command{
		Name:    "curl",
		Aliases: []string{"c"},
		Summary: "turn a curl command into Go, Python or JavaScript code",
		Usage: []string{
			"'curl -X POST https://api.x/v1 -H \"Content-Type: application/json\" -d \"{\\\"a\\\":1}\"'",
			"-t go|python|js",
		},
		Run: runCurl,
	})
}

func runChmod(args []string) int {
	fs := cli.NewFlagSet("chmod")
	mode := fs.String("m", "", "mode to convert (alternative to the positional argument)")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	input := *mode
	if input == "" {
		if fs.NArg() == 0 {
			return cli.Fail(fmt.Errorf("usage: dkit chmod <755|rwxr-xr-x>"))
		}
		input = fs.Arg(0)
	}
	input = strings.TrimSpace(input)

	octalRe := regexp.MustCompile(`^0?([0-7]{3,4})$`)
	if m := octalRe.FindStringSubmatch(input); m != nil {
		v, err := strconv.ParseInt(m[1], 8, 32)
		if err != nil {
			return cli.Fail(err)
		}
		sym := octalToSymbolic(int(v))
		fmt.Printf("  octal     %s\n", m[1])
		fmt.Printf("  symbolic  %s\n", cli.Cyan(sym))
		fmt.Printf("  owner     %s\n", describeTriad(sym[0:3]))
		fmt.Printf("  group     %s\n", describeTriad(sym[3:6]))
		fmt.Printf("  other     %s\n", describeTriad(sym[6:9]))
		return cli.ExitOK
	}

	if len(input) == 9 {
		v, err := symbolicToOctal(input)
		if err != nil {
			return cli.Fail(err)
		}
		fmt.Printf("  symbolic  %s\n", input)
		fmt.Printf("  octal     %s\n", cli.Cyan(strconv.FormatInt(int64(v), 8)))
		return cli.ExitOK
	}

	if strings.ContainsAny(input, "+-=") && len(input) <= 4 {
		return cli.Fail(fmt.Errorf("relative modes like %q need a base mode; try an octal or 9-character form", input))
	}
	return cli.Fail(fmt.Errorf("%q is neither an octal mode nor a 9-character symbolic mode", input))
}

func octalToSymbolic(v int) string {
	var b strings.Builder
	for i := 2; i >= 0; i-- {
		bits := (v >> (i * 3)) & 7
		b.WriteByte(triadChar(bits&4 != 0, 'r'))
		b.WriteByte(triadChar(bits&2 != 0, 'w'))
		b.WriteByte(triadChar(bits&1 != 0, 'x'))
	}
	return b.String()
}

func triadChar(on bool, c byte) byte {
	if on {
		return c
	}
	return '-'
}

func symbolicToOctal(s string) (int, error) {
	var parts [3]int
	for i := 0; i < 9; i++ {
		g := i / 3
		switch s[i] {
		case 'r':
			parts[g] |= 4
		case 'w':
			parts[g] |= 2
		case 'x', 's', 't':
			parts[g] |= 1
		case '-':
		default:
			return 0, fmt.Errorf("unexpected character %q at position %d", s[i], i+1)
		}
	}
	return parts[0]<<6 | parts[1]<<3 | parts[2], nil
}

func describeTriad(t string) string {
	var b []string
	if t[0] == 'r' {
		b = append(b, "read")
	}
	if t[1] == 'w' {
		b = append(b, "write")
	}
	if t[2] == 'x' {
		b = append(b, "execute")
	}
	if len(b) == 0 {
		return "none"
	}
	return strings.Join(b, ", ")
}

type semver struct {
	major, minor, patch int
	pre                 []string
	build               string
	raw                 string
}

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.\-]+))?(?:\+([0-9A-Za-z.\-]+))?$`)

func parseSemver(s string) (semver, error) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return semver{}, fmt.Errorf("%q is not a semantic version", s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	var pre []string
	if m[4] != "" {
		pre = strings.Split(m[4], ".")
	}
	return semver{major: major, minor: minor, patch: patch, pre: pre, build: m[5], raw: s}, nil
}

func (v semver) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	if len(v.pre) > 0 {
		s += "-" + strings.Join(v.pre, ".")
	}
	if v.build != "" {
		s += "+" + v.build
	}
	return s
}

func compareSemver(a, b semver) int {
	if a.major != b.major {
		return sign(a.major - b.major)
	}
	if a.minor != b.minor {
		return sign(a.minor - b.minor)
	}
	if a.patch != b.patch {
		return sign(a.patch - b.patch)
	}
	return comparePre(a.pre, b.pre)
}

func comparePre(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1 // a release outranks a prerelease
	}
	if len(b) == 0 {
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		ai, aErr := strconv.Atoi(a[i])
		bi, bErr := strconv.Atoi(b[i])
		switch {
		case aErr == nil && bErr == nil:
			if ai != bi {
				return sign(ai - bi)
			}
		case aErr == nil:
			return -1 // numeric identifiers rank lower than alphanumeric
		case bErr == nil:
			return 1
		default:
			if a[i] != b[i] {
				if a[i] < b[i] {
					return -1
				}
				return 1
			}
		}
	}
	return sign(len(a) - len(b))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func runSemver(args []string) int {
	fs := cli.NewFlagSet("semver")
	bump := fs.String("bump", "", "bump patch, minor or major")
	cmpTo := fs.String("cmp", "", "compare against another version")
	check := fs.Bool("check", false, "exit 1 when the input is not valid semver")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit semver <version> [-bump patch] [-cmp other]"))
	}

	v, err := parseSemver(fs.Arg(0))
	if err != nil {
		if *check {
			fmt.Fprintln(os.Stderr, err)
			return cli.ExitFail
		}
		return cli.Fail(err)
	}

	if *check {
		fmt.Printf("%s is valid semver\n", v.String())
		return cli.ExitOK
	}

	if *cmpTo != "" {
		other, err := parseSemver(*cmpTo)
		if err != nil {
			return cli.Fail(err)
		}
		c := compareSemver(v, other)
		word := "equal to"
		switch c {
		case -1:
			word = "lower than"
		case 1:
			word = "higher than"
		}
		fmt.Printf("%s is %s %s (%d)\n", v.String(), word, other.String(), c)
		return cli.ExitOK
	}

	if *bump != "" {
		next := v
		next.pre = nil
		next.build = ""
		switch *bump {
		case "patch":
			next.patch++
		case "minor":
			next.minor++
			next.patch = 0
		case "major":
			next.major++
			next.minor = 0
			next.patch = 0
		default:
			return cli.Fail(fmt.Errorf("unknown bump %q, use patch, minor or major", *bump))
		}
		fmt.Println(next.String())
		return cli.ExitOK
	}

	pre := "-"
	if len(v.pre) > 0 {
		pre = strings.Join(v.pre, ".")
	}
	build := v.build
	if build == "" {
		build = "-"
	}
	cli.KV(os.Stdout, [][2]string{
		{"version", v.String()},
		{"major", strconv.Itoa(v.major)},
		{"minor", strconv.Itoa(v.minor)},
		{"patch", strconv.Itoa(v.patch)},
		{"prerelease", pre},
		{"build", build},
	})
	return cli.ExitOK
}

func runCurl(args []string) int {
	fs := cli.NewFlagSet("curl")
	lang := fs.String("t", "go", "target language: go, python, js")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	var raw string
	if fs.NArg() > 0 {
		raw = strings.Join(fs.Args(), " ")
	} else {
		data, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		raw = string(data)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cli.Fail(fmt.Errorf("nothing to convert"))
	}

	req, err := parseCurl(raw)
	if err != nil {
		return cli.Fail(err)
	}

	switch *lang {
	case "go", "golang":
		fmt.Print(renderGo(req))
	case "python", "py":
		fmt.Print(renderPython(req))
	case "js", "javascript", "node", "fetch":
		fmt.Print(renderJS(req))
	default:
		return cli.Fail(fmt.Errorf("unsupported language %q, use go, python or js", *lang))
	}
	return cli.ExitOK
}

type httpReq struct {
	method  string
	url     string
	headers [][2]string
	body    string
	user    string
}

func parseCurl(cmd string) (httpReq, error) {
	tokens := splitCommandLine(cmd)
	req := httpReq{method: ""}

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "curl":
			continue
		case "-X", "--request":
			if i+1 < len(tokens) {
				req.method = strings.ToUpper(tokens[i+1])
				i++
			}
		case "-H", "--header":
			if i+1 < len(tokens) {
				addHeader(&req, tokens[i+1])
				i++
			}
		case "-d", "--data", "--data-raw", "--data-binary", "--data-ascii":
			if i+1 < len(tokens) {
				req.body = tokens[i+1]
				i++
			}
		case "-u", "--user":
			if i+1 < len(tokens) {
				req.user = tokens[i+1]
				i++
			}
		case "--url":
			if i+1 < len(tokens) {
				req.url = tokens[i+1]
				i++
			}
		default:
			t := tokens[i]
			if strings.HasPrefix(t, "-") {
				continue
			}
			if req.url == "" && (strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")) {
				req.url = t
			}
		}
	}

	if req.url == "" {
		return req, fmt.Errorf("no URL found in the command")
	}
	if req.method == "" {
		if req.body != "" {
			req.method = "POST"
		} else {
			req.method = "GET"
		}
	}
	return req, nil
}

func addHeader(req *httpReq, h string) {
	name, value, found := strings.Cut(h, ":")
	if !found {
		return
	}
	req.headers = append(req.headers, [2]string{strings.TrimSpace(name), strings.TrimSpace(value)})
}

func splitCommandLine(s string) []string {
	s = strings.ReplaceAll(s, "\\\n", " ")
	var tokens []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			if c == '\\' && i+1 < len(s) && quote == '"' {
				i++
				cur.WriteByte(s[i])
				continue
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t' || c == '\n':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

func renderGo(r httpReq) string {
	var b strings.Builder
	b.WriteString("package main\n\nimport (\n\t\"fmt\"\n\t\"io\"\n\t\"net/http\"\n")
	if r.body != "" {
		b.WriteString("\t\"strings\"\n")
	}
	b.WriteString(")\n\nfunc main() {\n")

	bodyArg := "nil"
	if r.body != "" {
		bodyArg = "strings.NewReader(`" + r.body + "`)"
	}
	fmt.Fprintf(&b, "\treq, err := http.NewRequest(%q, %q, %s)\n", r.method, r.url, bodyArg)
	b.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	for _, h := range r.headers {
		fmt.Fprintf(&b, "\treq.Header.Set(%q, %q)\n", h[0], h[1])
	}
	if r.user != "" {
		name, pass, _ := strings.Cut(r.user, ":")
		fmt.Fprintf(&b, "\treq.SetBasicAuth(%q, %q)\n", name, pass)
	}
	b.WriteString("\n\tresp, err := http.DefaultClient.Do(req)\n")
	b.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	b.WriteString("\tdefer resp.Body.Close()\n\n")
	b.WriteString("\tbody, err := io.ReadAll(resp.Body)\n")
	b.WriteString("\tif err != nil {\n\t\tpanic(err)\n\t}\n")
	b.WriteString("\tfmt.Println(resp.Status)\n")
	b.WriteString("\tfmt.Println(string(body))\n}\n")
	return b.String()
}

func renderPython(r httpReq) string {
	var b strings.Builder
	b.WriteString("import requests\n\n")
	if len(r.headers) > 0 {
		b.WriteString("headers = {\n")
		for _, h := range r.headers {
			fmt.Fprintf(&b, "    %q: %q,\n", h[0], h[1])
		}
		b.WriteString("}\n\n")
	}
	fn := strings.ToLower(r.method)
	fmt.Fprintf(&b, "response = requests.%s(%q", fn, r.url)
	if len(r.headers) > 0 {
		b.WriteString(", headers=headers")
	}
	if r.body != "" {
		fmt.Fprintf(&b, ", data=%q", r.body)
	}
	if r.user != "" {
		name, pass, _ := strings.Cut(r.user, ":")
		fmt.Fprintf(&b, ", auth=(%q, %q)", name, pass)
	}
	b.WriteString(")\n\n")
	b.WriteString("print(response.status_code)\n")
	b.WriteString("print(response.text)\n")
	return b.String()
}

func renderJS(r httpReq) string {
	var b strings.Builder
	fmt.Fprintf(&b, "const res = await fetch(%q, {\n", r.url)
	fmt.Fprintf(&b, "  method: %q,\n", r.method)
	if len(r.headers) > 0 {
		b.WriteString("  headers: {\n")
		for _, h := range r.headers {
			fmt.Fprintf(&b, "    %q: %q,\n", h[0], h[1])
		}
		b.WriteString("  },\n")
	}
	if r.body != "" {
		fmt.Fprintf(&b, "  body: %q,\n", r.body)
	}
	b.WriteString("});\n\n")
	b.WriteString("console.log(res.status, await res.text());\n")
	return b.String()
}
