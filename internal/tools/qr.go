package tools

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
	"github.com/skip2/go-qrcode"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

func init() {
	cli.Register(cli.Command{
		Name:    "qr",
		Aliases: []string{"q"},
		Summary: "generate a QR code as a PNG file or as terminal art",
		Usage: []string{
			"'https://example.com' -o qr.png",
			"'hello' --terminal",
			"[-s 256] [-l low|medium|high|highest]",
		},
		Run: runQR,
	})
	cli.Register(cli.Command{
		Name:    "md",
		Aliases: []string{"m"},
		Summary: "render Markdown to HTML",
		Usage: []string{
			"README.md -o README.html --full",
			"- < notes.md",
			"[--full] [-o file]",
		},
		Run: runMD,
	})
}

func runQR(args []string) int {
	fs := cli.NewFlagSet("qr")
	size := fs.Int("s", 256, "PNG side length in pixels")
	levelStr := fs.String("l", "medium", "error correction: low, medium, high, highest")
	out := fs.String("o", "", "output PNG path")
	terminal := fs.Bool("terminal", false, "draw the code in the terminal instead of a file")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	var content string
	if fs.NArg() > 0 {
		content = strings.Join(fs.Args(), " ")
	} else {
		data, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		content = strings.TrimSpace(string(data))
	}
	if content == "" {
		return cli.Fail(fmt.Errorf("nothing to encode"))
	}

	var level qrcode.RecoveryLevel
	switch strings.ToLower(*levelStr) {
	case "low", "l":
		level = qrcode.Low
	case "medium", "m":
		level = qrcode.Medium
	case "high", "q":
		level = qrcode.High
	case "highest", "h":
		level = qrcode.Highest
	default:
		return cli.Fail(fmt.Errorf("unknown level %q, use low, medium, high or highest", *levelStr))
	}

	code, err := qrcode.New(content, level)
	if err != nil {
		return cli.Fail(err)
	}

	if *terminal {
		fmt.Println(code.ToSmallString(false))
		return cli.ExitOK
	}

	if *out == "" {
		return cli.Fail(fmt.Errorf("give -o qr.png, or --terminal to print in the console"))
	}
	if err := code.WriteFile(*size, *out); err != nil {
		return cli.Fail(err)
	}
	fmt.Printf("wrote %s (%dx%d, %s correction)\n", *out, *size, *size, *levelStr)
	return cli.ExitOK
}

func runMD(args []string) int {
	fs := cli.NewFlagSet("md")
	full := fs.Bool("full", false, "wrap the HTML in a complete document")
	out := fs.String("o", "", "write to a file instead of stdout")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}

	engine := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table, extension.Strikethrough),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)
	var buf bytes.Buffer
	if err := engine.Convert(data, &buf); err != nil {
		return cli.Fail(fmt.Errorf("%s: %w", label, err))
	}

	result := buf.Bytes()
	if *full {
		var doc bytes.Buffer
		doc.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
		doc.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
		doc.WriteString("<title>" + stdhtml.EscapeString(strings.TrimSuffix(label, ".md")) + "</title>\n")
		doc.WriteString("<style>\nbody{max-width:46rem;margin:3rem auto;padding:0 1rem;")
		doc.WriteString("font:16px/1.65 system-ui,sans-serif;color:#222}\n")
		doc.WriteString("pre{background:#f5f5f5;padding:.8rem;overflow:auto;border-radius:6px}\n")
		doc.WriteString("code{font-family:ui-monospace,Consolas,monospace;font-size:.92em}\n")
		doc.WriteString("table{border-collapse:collapse}td,th{border:1px solid #ddd;padding:.4rem .7rem}\n")
		doc.WriteString("blockquote{margin:0;padding-left:1rem;border-left:3px solid #ddd;color:#555}\n")
		doc.WriteString("</style>\n</head>\n<body>\n")
		doc.Write(result)
		doc.WriteString("</body>\n</html>\n")
		result = doc.Bytes()
	}

	if err := cli.WriteOutput(*out, result); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}
