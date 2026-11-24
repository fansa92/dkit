package tools

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"mime/quotedprintable"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "enc",
		Aliases: []string{"e"},
		Summary: "encode or decode base64, url, html, hex, base58, gzip, punycode",
		Usage: []string{
			"-t base64 hello",
			"-t base64 -d aGVsbG8=",
			"-t gzip -o backup.gz backup.log",
			"-t base64img logo.png        (prints a data URL)",
			"[-t type] [-d] [-w] [-o file] [--literal]",
		},
		Run: runEnc,
	})
}

var encTypes = []string{
	"base64", "base64url", "base64raw", "base32", "base64img",
	"url", "urlpath", "html", "hex", "base58", "gzip", "zlib", "qp", "punycode", "escape",
}

func runEnc(args []string) int {
	fs := cli.NewFlagSet("enc")
	typ := fs.String("t", "base64", "encoding: "+strings.Join(encTypes, ", "))
	decode := fs.Bool("d", false, "decode instead of encode")
	out := fs.String("o", "", "write the result to a file")
	literal := fs.Bool("literal", false, "treat the argument as text even if a file has that name")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	data, label, err := cli.Input(fs.Args(), *literal)
	if err != nil {
		return cli.Fail(err)
	}

	var result []byte
	if *decode {
		result, err = decodeAs(*typ, data)
	} else {
		result, err = encodeAs(*typ, data)
	}
	if err != nil {
		return cli.Fail(fmt.Errorf("%s (%s): %w", *typ, label, err))
	}

	if *decode && isImageType(*typ) && *out == "" {
		return cli.Fail(errors.New("decoding an image needs -o <file>"))
	}

	textOut := !isBinaryType(*typ, *decode)
	if textOut && *out == "" && !bytes.HasSuffix(result, []byte("\n")) {
		result = append(result, '\n')
	}
	if err := cli.WriteOutput(*out, result); err != nil {
		return cli.Fail(err)
	}
	return cli.ExitOK
}

func isImageType(t string) bool { return t == "base64img" }

func isBinaryType(t string, decoding bool) bool {
	switch t {
	case "gzip", "zlib":
		return true
	case "base64img":
		return decoding
	}
	return false
}

func encodeAs(typ string, data []byte) ([]byte, error) {
	switch typ {
	case "base64":
		return []byte(base64.StdEncoding.EncodeToString(data)), nil
	case "base64url":
		return []byte(base64.URLEncoding.EncodeToString(data)), nil
	case "base64raw":
		return []byte(base64.RawURLEncoding.EncodeToString(data)), nil
	case "base32":
		return []byte(base32.StdEncoding.EncodeToString(data)), nil
	case "hex":
		return []byte(hex.EncodeToString(data)), nil
	case "url":
		return []byte(url.QueryEscape(string(data))), nil
	case "urlpath":
		return []byte(url.PathEscape(string(data))), nil
	case "html":
		return []byte(html.EscapeString(string(data))), nil
	case "base58":
		return []byte(base58Encode(data)), nil
	case "gzip":
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "zlib":
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "qp":
		var buf bytes.Buffer
		w := quotedprintable.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "escape":
		q := strconv.Quote(string(data))
		return []byte(q[1 : len(q)-1]), nil
	case "punycode":
		enc, err := punycodeEncode(string(data))
		if err != nil {
			return nil, err
		}
		return []byte("xn--" + enc), nil
	case "base64img":
		return []byte("data:" + http.DetectContentType(data) + ";base64," +
			base64.StdEncoding.EncodeToString(data)), nil
	default:
		return nil, fmt.Errorf("unknown encoding %q", typ)
	}
}

func decodeAs(typ string, data []byte) ([]byte, error) {
	s := strings.TrimSpace(string(data))
	switch typ {
	case "base64":
		b, err := base64.StdEncoding.DecodeString(stripWhitespace(s))
		return b, err
	case "base64url":
		b, err := base64.URLEncoding.DecodeString(stripWhitespace(s))
		if err != nil {
			b, err = base64.RawURLEncoding.DecodeString(stripWhitespace(s))
		}
		return b, err
	case "base64raw":
		return base64.RawURLEncoding.DecodeString(stripWhitespace(s))
	case "base32":
		b, err := base32.StdEncoding.DecodeString(strings.ToUpper(stripWhitespace(s)))
		return b, err
	case "hex":
		return hex.DecodeString(stripWhitespace(s))
	case "url":
		v, err := url.QueryUnescape(s)
		return []byte(v), err
	case "urlpath":
		v, err := url.PathUnescape(s)
		return []byte(v), err
	case "html":
		return []byte(html.UnescapeString(s)), nil
	case "base58":
		return base58Decode(s)
	case "gzip":
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case "zlib":
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case "qp":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data)))
	case "escape":
		v, err := strconv.Unquote(`"` + string(data) + `"`)
		if err != nil {
			return nil, err
		}
		return []byte(v), nil
	case "punycode":
		s = strings.TrimPrefix(strings.ToLower(s), "xn--")
		dec, err := punycodeDecode(s)
		if err != nil {
			return nil, err
		}
		return []byte(dec), nil
	case "base64img":
		if idx := strings.Index(s, "base64,"); idx >= 0 {
			s = s[idx+len("base64,"):]
		}
		return base64.StdEncoding.DecodeString(stripWhitespace(s))
	default:
		return nil, fmt.Errorf("unknown encoding %q", typ)
	}
}

func stripWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func base58Encode(data []byte) string {
	x := new(big.Int).SetBytes(data)
	radix := big.NewInt(58)
	mod := new(big.Int)
	var out []byte
	for x.Sign() > 0 {
		x.DivMod(x, radix, mod)
		out = append(out, base58Alphabet[mod.Int64()])
	}
	for _, b := range data {
		if b != 0 {
			break
		}
		out = append(out, '1')
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func base58Decode(s string) ([]byte, error) {
	x := new(big.Int)
	radix := big.NewInt(58)
	for _, r := range s {
		idx := strings.IndexRune(base58Alphabet, r)
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character %q", r)
		}
		x.Mul(x, radix)
		x.Add(x, big.NewInt(int64(idx)))
	}
	out := x.Bytes()
	leading := 0
	for leading < len(s) && s[leading] == '1' {
		leading++
	}
	return append(make([]byte, leading), out...), nil
}

// punycode, RFC 3492.
const (
	pcBase        = 36
	pcTMin        = 1
	pcTMax        = 26
	pcSkew        = 38
	pcDamp        = 700
	pcInitialBias = 72
	pcInitialN    = 128
)

func pcAdapt(delta, numPoints int, first bool) int {
	if first {
		delta /= pcDamp
	} else {
		delta /= 2
	}
	delta += delta / numPoints
	k := 0
	for delta > ((pcBase-pcTMin)*pcTMax)/2 {
		delta /= pcBase - pcTMin
		k += pcBase
	}
	return k + (pcBase-pcTMin+1)*delta/(delta+pcSkew)
}

func pcDigit(d int) byte {
	if d < 26 {
		return byte('a' + d)
	}
	return byte('0' + d - 26)
}

func pcDecodeDigit(c byte) int {
	switch {
	case c >= 'a' && c <= 'z':
		return int(c - 'a')
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= '0' && c <= '9':
		return int(c-'0') + 26
	}
	return -1
}

func punycodeEncode(s string) (string, error) {
	runes := []rune(s)
	var out []byte
	for _, r := range runes {
		if r < pcInitialN {
			out = append(out, byte(r))
		}
	}
	b := len(out)
	h := b
	if b > 0 {
		out = append(out, '-')
	}

	n := pcInitialN
	delta := 0
	bias := pcInitialBias

	for h < len(runes) {
		m := 0x7fffffff
		for _, r := range runes {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}
		if m == 0x7fffffff {
			return "", errors.New("punycode: no more code points")
		}
		if (m-n)*(h+1) > 0x7fffffff-delta {
			return "", errors.New("punycode: overflow")
		}
		delta += (m - n) * (h + 1)
		n = m
		for _, r := range runes {
			if int(r) < n {
				delta++
				if delta > 0x7fffffff {
					return "", errors.New("punycode: overflow")
				}
			}
			if int(r) == n {
				q := delta
				for k := pcBase; ; k += pcBase {
					t := k - bias
					if t < pcTMin {
						t = pcTMin
					} else if t > pcTMax {
						t = pcTMax
					}
					if q < t {
						break
					}
					out = append(out, pcDigit(t+(q-t)%(pcBase-t)))
					q = (q - t) / (pcBase - t)
				}
				out = append(out, pcDigit(q))
				bias = pcAdapt(delta, h+1, h == b)
				delta = 0
				h++
			}
		}
		delta++
		n++
	}
	return string(out), nil
}

func punycodeDecode(s string) (string, error) {
	var output []rune
	i := 0
	if pos := strings.LastIndexByte(s, '-'); pos > 0 {
		for _, r := range s[:pos] {
			if r >= pcInitialN {
				return "", errors.New("punycode: non-basic code point before delimiter")
			}
			output = append(output, r)
		}
		i = pos + 1
	}

	n := pcInitialN
	idx := 0
	bias := pcInitialBias

	for i < len(s) {
		oldi := idx
		w := 1
		for k := pcBase; ; k += pcBase {
			if i >= len(s) {
				return "", errors.New("punycode: truncated input")
			}
			d := pcDecodeDigit(s[i])
			i++
			if d < 0 {
				return "", fmt.Errorf("punycode: invalid digit %q", s[i-1])
			}
			idx += d * w
			t := k - bias
			if t < pcTMin {
				t = pcTMin
			} else if t > pcTMax {
				t = pcTMax
			}
			if d < t {
				break
			}
			w *= pcBase - t
		}
		bias = pcAdapt(idx-oldi, len(output)+1, oldi == 0)
		n += idx / (len(output) + 1)
		idx %= len(output) + 1
		output = append(output, 0)
		copy(output[idx+1:], output[idx:])
		output[idx] = rune(n)
		idx++
	}
	return string(output), nil
}
