package tools

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "xml",
		Aliases: []string{"x"},
		Summary: "format, validate and convert XML",
		Usage: []string{
			"fmt config.xml         re-indent, 2 spaces",
			"check config.xml       validate, exit 1 when malformed",
			"to-json config.xml     attributes become @name, text becomes #text",
			"[-o file]",
		},
		Run: runXML,
	})
}

func runXML(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dkit xml <fmt|check|to-json> [flags] [file]")
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	fs := cli.NewFlagSet("xml " + sub)
	indent := fs.String("i", "  ", "indent string for fmt")
	out := fs.String("o", "", "write the result to a file")
	if code, ok := cli.Parse(fs, rest); !ok {
		return code
	}
	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}

	switch sub {
	case "fmt":
		var buf bytes.Buffer
		dec := xml.NewDecoder(bytes.NewReader(data))
		dec.Strict = false
		enc := xml.NewEncoder(&buf)
		enc.Indent("", *indent)
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return cli.Fail(fmt.Errorf("%s: %w", label, err))
			}
			if err := enc.EncodeToken(tok); err != nil {
				return cli.Fail(fmt.Errorf("%s: %w", label, err))
			}
		}
		if err := enc.Flush(); err != nil {
			return cli.Fail(err)
		}
		buf.WriteByte('\n')
		if err := cli.WriteOutput(*out, buf.Bytes()); err != nil {
			return cli.Fail(err)
		}
		return cli.ExitOK

	case "check":
		dec := xml.NewDecoder(bytes.NewReader(data))
		dec.Strict = false
		for {
			_, err := dec.Token()
			if err == io.EOF {
				fmt.Printf("%s is well-formed XML\n", label)
				return cli.ExitOK
			}
			if err != nil {
				return cli.Fail(fmt.Errorf("%s: %w", label, err))
			}
		}

	case "to-json":
		var n xmlNode
		if err := xml.Unmarshal(data, &n); err != nil {
			return cli.Fail(fmt.Errorf("%s: %w", label, err))
		}
		if n.XMLName.Local == "" {
			return cli.Fail(errors.New("no root element found"))
		}
		doc := map[string]any{n.XMLName.Local: nodeValue(n)}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return cli.Fail(err)
		}
		b = append(b, '\n')
		if err := cli.WriteOutput(*out, b); err != nil {
			return cli.Fail(err)
		}
		return cli.ExitOK
	case "help", "-h", "--help":
		cli.Help("xml")
		return cli.ExitOK
	}
	return cli.Fail(fmt.Errorf("unknown xml subcommand %q", sub))
}

type xmlNode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Content string     `xml:",chardata"`
	Nodes   []xmlNode  `xml:",any"`
}

func nodeValue(n xmlNode) any {
	text := strings.TrimSpace(n.Content)
	if len(n.Attrs) == 0 && len(n.Nodes) == 0 {
		return text
	}

	m := make(map[string]any, len(n.Attrs)+len(n.Nodes)+1)
	for _, a := range n.Attrs {
		m["@"+a.Name.Local] = a.Value
	}
	if text != "" {
		m["#text"] = text
	}

	groups := map[string][]any{}
	var order []string
	for _, c := range n.Nodes {
		name := c.XMLName.Local
		if _, seen := groups[name]; !seen {
			order = append(order, name)
		}
		groups[name] = append(groups[name], nodeValue(c))
	}
	for _, name := range order {
		if len(groups[name]) == 1 {
			m[name] = groups[name][0]
		} else {
			m[name] = groups[name]
		}
	}
	return m
}
