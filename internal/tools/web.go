package tools

import (
	"fmt"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "html",
		Aliases: []string{"ht"},
		Summary: "re-indent HTML",
		Usage: []string{
			"fmt page.html            re-indent with 2 spaces",
			"-i 4 < page.html",
			"[-o file]",
		},
		Run: func(args []string) int { return runMarkup("html", args) },
	})
	cli.Register(cli.Command{
		Name:    "css",
		Aliases: []string{"cs"},
		Summary: "re-indent CSS",
		Usage: []string{
			"fmt styles.css",
			"- < styles.css",
			"[-o file]",
		},
		Run: func(args []string) int { return runMarkup("css", args) },
	})
}

func runMarkup(kind string, args []string) int {
	fs := cli.NewFlagSet(kind)
	indent := fs.String("i", "  ", "indent string")
	out := fs.String("o", "", "write to a file")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}
	src := string(data)

	var result string
	switch kind {
	case "html":
		result = formatHTML(src, *indent)
	case "css":
		result = formatCSS(src, *indent)
	}
	if result == "" {
		return cli.Fail(fmt.Errorf("%s: nothing to format", label))
	}
	result += "\n"
	if err := cli.WriteOutput(*out, []byte(result)); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true, "!doctype": true,
}

func formatHTML(src, indent string) string {
	var b strings.Builder
	depth := 0
	i := 0

	write := func(text string) {
		b.WriteString(strings.Repeat(indent, depth))
		b.WriteString(text)
		b.WriteByte('\n')
	}

	for i < len(src) {
		lt := strings.IndexByte(src[i:], '<')
		if lt < 0 {
			if text := strings.TrimSpace(src[i:]); text != "" {
				write(text)
			}
			break
		}
		lt += i
		if text := strings.TrimSpace(src[i:lt]); text != "" {
			write(text)
		}

		var gt int
		if strings.HasPrefix(src[lt:], "<!--") {
			end := strings.Index(src[lt:], "-->")
			if end < 0 {
				break
			}
			gt = lt + end + 2
		} else {
			end := strings.IndexByte(src[lt:], '>')
			if end < 0 {
				break
			}
			gt = lt + end
		}

		tag := strings.TrimSpace(src[lt : gt+1])
		nxt := gt + 1
		lower := strings.ToLower(tag)

		switch {
		case strings.HasPrefix(lower, "</"):
			if depth > 0 {
				depth--
			}
			write(tag)
		case strings.HasPrefix(lower, "<!--"):
			write(tag)
		case strings.HasPrefix(lower, "<!") || strings.HasPrefix(lower, "<?"):
			write(tag)
		case strings.HasSuffix(lower, "/>"):
			write(tag)
		default:
			name := tagName(lower)
			if voidElements[name] {
				write(tag)
				break
			}
			// Inline content like <p>text</p> stays on one line.
			closeIdx := strings.Index(strings.ToLower(src[nxt:]), "</"+name)
			if closeIdx >= 0 {
				candidate := src[nxt : nxt+closeIdx]
				closeEnd := strings.IndexByte(src[nxt+closeIdx:], '>')
				if closeEnd >= 0 && !strings.Contains(candidate, "<") &&
					len(candidate) < 120 && !strings.Contains(candidate, "\n") {
					full := tag + candidate + src[nxt+closeIdx:nxt+closeIdx+closeEnd+1]
					write(strings.TrimSpace(full))
					i = nxt + closeIdx + closeEnd + 1
					continue
				}
			}
			write(tag)
			depth++
		}
		i = nxt
	}
	return strings.TrimRight(b.String(), "\n")
}

func tagName(lowerTag string) string {
	t := strings.TrimPrefix(lowerTag, "<")
	t = strings.TrimPrefix(t, "/")
	end := strings.IndexAny(t, " \t\n/>")
	if end >= 0 {
		t = t[:end]
	}
	return t
}

func formatCSS(src, indent string) string {
	var b strings.Builder
	depth := 0
	var buf strings.Builder

	flush := func() {
		text := strings.TrimSpace(buf.String())
		buf.Reset()
		if text == "" {
			return
		}
		b.WriteString(strings.Repeat(indent, depth))
		b.WriteString(text)
		b.WriteByte('\n')
	}

	i := 0
	for i < len(src) {
		c := src[i]

		if c == '/' && i+1 < len(src) && src[i+1] == '*' {
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				buf.WriteString(src[i:])
				break
			}
			flush()
			b.WriteString(strings.Repeat(indent, depth))
			b.WriteString(src[i : i+2+end+2])
			b.WriteByte('\n')
			i += 2 + end + 2
			continue
		}

		switch c {
		case '{':
			if text := strings.TrimSpace(buf.String()); text != "" {
				buf.Reset()
				b.WriteString(strings.Repeat(indent, depth) + text + " {\n")
			} else {
				b.WriteString(strings.Repeat(indent, depth) + "{\n")
			}
			depth++
		case '}':
			flush()
			if depth > 0 {
				depth--
			}
			b.WriteString(strings.Repeat(indent, depth) + "}\n")
		case ';':
			buf.WriteByte(';')
			flush()
		default:
			buf.WriteByte(c)
		}
		i++
	}
	flush()
	return strings.TrimRight(b.String(), "\n")
}
