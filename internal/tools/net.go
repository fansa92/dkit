package tools

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "ip",
		Aliases: []string{"i"},
		Summary: "inspect an IPv4/IPv6 address or subnet",
		Usage: []string{
			"192.168.1.10/24      network, mask, broadcast, host range",
			"10.0.0.0/8 -n 4      also show the first N usable addresses",
			"2001:db8::1",
		},
		Run: runIP,
	})
	cli.Register(cli.Command{
		Name:    "urlq",
		Aliases: []string{"u"},
		Summary: "parse a URL or a query string into its parts",
		Usage: []string{
			"'https://api.example.com:8443/v1/items?page=2&q=go#top'",
			"--query 'page=2&q=go'    parse a bare query string",
		},
		Run: runURLQ,
	})
}

func runIP(args []string) int {
	fs := cli.NewFlagSet("ip")
	show := fs.Int("n", 0, "list the first N usable addresses")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit ip <addr|cidr>"))
	}

	input := fs.Arg(0)
	if strings.Contains(input, "/") {
		prefix, err := netip.ParsePrefix(input)
		if err != nil {
			return cli.Fail(fmt.Errorf("%q is not a valid subnet: %w", input, err))
		}
		prefix = prefix.Masked()
		first, last, broadcast := prefixRange(prefix)
		bits := prefix.Addr().BitLen()
		hostBits := bits - prefix.Bits()

		rows := [][2]string{
			{"input", input},
			{"network", prefix.String()},
			{"version", versionOf(prefix.Addr())},
			{"netmask", maskString(prefix)},
			{"wildcard", wildcardString(prefix)},
			{"broadcast", broadcast.String()},
			{"first_host", first.String()},
			{"last_host", last.String()},
			{"prefix_len", strconv.Itoa(prefix.Bits())},
		}
		if hostBits == 0 {
			rows = append(rows, [2]string{"usable_hosts", "1"})
		} else if hostBits == 1 {
			rows = append(rows, [2]string{"usable_hosts", "2"})
		} else if hostBits < 62 {
			rows = append(rows, [2]string{"usable_hosts", strconv.FormatUint(uint64(1)<<hostBits-2, 10)})
		} else {
			rows = append(rows, [2]string{"usable_hosts", "2^" + strconv.Itoa(hostBits) + " - 2"})
		}
		rows = append(rows, [2]string{"scope", addressKind(prefix.Addr())})
		cli.KV(os.Stdout, rows)

		if *show > 0 {
			fmt.Println()
			fmt.Println(cli.Bold("first usable addresses"))
			cur := first
			for i := 0; i < *show && cur.IsValid(); i++ {
				if cur.Compare(last) > 0 {
					break
				}
				fmt.Println("  " + cur.String())
				cur = cur.Next()
			}
		}
		return cli.ExitOK
	}

	addr, err := netip.ParseAddr(input)
	if err != nil {
		return cli.Fail(fmt.Errorf("%q is not a valid IP address: %w", input, err))
	}
	rows := [][2]string{
		{"input", input},
		{"canonical", addr.String()},
		{"version", versionOf(addr)},
		{"kind", addressKind(addr)},
		{"is_loopback", strconv.FormatBool(addr.IsLoopback())},
		{"is_private", strconv.FormatBool(addr.IsPrivate())},
		{"is_multicast", strconv.FormatBool(addr.IsMulticast())},
		{"is_link_local", strconv.FormatBool(addr.IsLinkLocalUnicast())},
	}
	if addr.Is4() {
		b := addr.As4()
		rows = append(rows, [2]string{
			"integer", strconv.FormatUint(uint64(b[0])<<24|uint64(b[1])<<16|uint64(b[2])<<8|uint64(b[3]), 10),
		})
	}
	cli.KV(os.Stdout, rows)
	return cli.ExitOK
}

func versionOf(a netip.Addr) string {
	if a.Is4() {
		return "IPv4"
	}
	return "IPv6"
}

func addressKind(a netip.Addr) string {
	switch {
	case a.IsLoopback():
		return "loopback"
	case a.IsPrivate():
		return "private (RFC 1918 / ULA)"
	case a.IsLinkLocalUnicast():
		return "link-local"
	case a.IsMulticast():
		return "multicast"
	case a.IsUnspecified():
		return "unspecified"
	default:
		return "public"
	}
}

func prefixRange(p netip.Prefix) (first, last, broadcast netip.Addr) {
	base := p.Masked().Addr()
	hostBits := base.BitLen() - p.Bits()

	bcastBytes := base.AsSlice()
	for i := 0; i < hostBits; i++ {
		idx := len(bcastBytes) - 1 - i/8
		bcastBytes[idx] |= 1 << (i % 8)
	}
	broadcast, _ = netip.AddrFromSlice(bcastBytes)

	switch {
	case hostBits == 0:
		return base, base, base
	case hostBits == 1:
		return base, broadcast, broadcast
	}
	firstBytes := base.AsSlice()
	firstBytes[len(firstBytes)-1] |= 1
	first, _ = netip.AddrFromSlice(firstBytes)
	last = broadcast.Prev()
	return first, last, broadcast
}

func maskString(p netip.Prefix) string {
	if p.Addr().Is4() {
		var m [4]byte
		for i := 0; i < 4; i++ {
			remain := p.Bits() - i*8
			switch {
			case remain >= 8:
				m[i] = 255
			case remain <= 0:
				m[i] = 0
			default:
				m[i] = byte(255 << (8 - remain))
			}
		}
		return fmt.Sprintf("%d.%d.%d.%d", m[0], m[1], m[2], m[3])
	}
	return "/" + strconv.Itoa(p.Bits())
}

func wildcardString(p netip.Prefix) string {
	if p.Addr().Is4() {
		var w [4]byte
		for i := 0; i < 4; i++ {
			remain := p.Bits() - i*8
			switch {
			case remain >= 8:
				w[i] = 0
			case remain <= 0:
				w[i] = 255
			default:
				w[i] = byte(255 >> remain)
			}
		}
		return fmt.Sprintf("%d.%d.%d.%d", w[0], w[1], w[2], w[3])
	}
	return "host bits inverted"
}

func runURLQ(args []string) int {
	fs := cli.NewFlagSet("urlq")
	queryOnly := fs.String("query", "", "parse a bare query string instead of a URL")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	raw := *queryOnly
	if raw == "" {
		if fs.NArg() == 0 {
			data, _, err := cli.Input(nil, false)
			if err != nil {
				return cli.Fail(err)
			}
			raw = strings.TrimSpace(string(data))
		} else {
			raw = fs.Arg(0)
		}
	}
	if raw == "" {
		return cli.Fail(fmt.Errorf("nothing to parse"))
	}

	if *queryOnly != "" || !strings.Contains(raw, "://") {
		vals, err := url.ParseQuery(strings.TrimPrefix(raw, "?"))
		if err != nil {
			return cli.Fail(err)
		}
		printQuery(vals)
		return cli.ExitOK
	}

	u, err := url.Parse(raw)
	if err != nil {
		return cli.Fail(err)
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443 (default)"
		case "http":
			port = "80 (default)"
		default:
			port = "-"
		}
	}
	user := "-"
	if u.User != nil {
		user = u.User.Username()
		if _, hasPass := u.User.Password(); hasPass {
			user += " (password set)"
		}
	}
	cli.KV(os.Stdout, [][2]string{
		{"scheme", u.Scheme},
		{"host", u.Hostname()},
		{"port", port},
		{"path", u.EscapedPath()},
		{"user", user},
		{"fragment", orDash(u.Fragment)},
	})
	if u.RawQuery != "" {
		fmt.Println()
		fmt.Println(cli.Bold("query"))
		vals, err := url.ParseQuery(u.RawQuery)
		if err != nil {
			return cli.Fail(err)
		}
		printQuery(vals)
	}
	return cli.ExitOK
}

func printQuery(vals url.Values) {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range vals[k] {
			fmt.Printf("  %s = %s\n", cli.Cyan(k), v)
		}
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
