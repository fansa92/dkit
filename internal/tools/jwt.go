package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "jwt",
		Aliases: []string{"jw"},
		Summary: "decode a token, show claims and whether it has expired",
		Usage: []string{
			"[token] [flags]",
			"[flags] < token.txt",
		},
		Run: runJWT,
	})
}

var timeClaims = map[string]bool{
	"exp":       true,
	"iat":       true,
	"nbf":       true,
	"auth_time": true,
}

func runJWT(args []string) int {
	fs := cli.NewFlagSet("jwt")
	asJSON := fs.Bool("json", false, "emit a single JSON object for scripting")
	onlyPayload := fs.Bool("payload", false, "print just the decoded payload")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	var raw string
	if fs.NArg() > 0 {
		raw = fs.Arg(0)
	} else {
		b, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		raw = string(b)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cli.Fail(errors.New("no token given"))
	}

	parts := strings.Split(raw, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return cli.Fail(fmt.Errorf("not a JWT: expected 2 or 3 dot-separated segments, got %d", len(parts)))
	}

	header, err := decodeSegment(parts[0])
	if err != nil {
		return cli.Fail(fmt.Errorf("decoding header: %w", err))
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return cli.Fail(fmt.Errorf("decoding payload: %w", err))
	}

	var sig []byte
	if len(parts) == 3 {
		sig, _ = base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[2], "="))
	}
	alg, _ := header["alg"].(string)

	now := time.Now()
	var exp, iat, nbf time.Time
	var hasExp, hasIat, hasNbf bool
	if t, ok := claimTime(payload, "exp"); ok {
		exp, hasExp = t, true
	}
	if t, ok := claimTime(payload, "iat"); ok {
		iat, hasIat = t, true
	}
	if t, ok := claimTime(payload, "nbf"); ok {
		nbf, hasNbf = t, true
	}

	if *onlyPayload {
		out, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(out))
		return cli.ExitOK
	}

	if *asJSON {
		doc := map[string]any{
			"header":  header,
			"payload": payload,
		}
		if len(parts) == 3 {
			doc["signature"] = parts[2]
			doc["signatureBytes"] = len(sig)
			doc["verified"] = false
		}
		if hasExp {
			doc["expiresAt"] = exp.UTC().Format(time.RFC3339)
			doc["expired"] = now.After(exp)
			doc["expiresIn"] = exp.Sub(now).Round(time.Second).String()
		}
		if hasIat {
			doc["issuedAt"] = iat.UTC().Format(time.RFC3339)
		}
		out, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(out))
		if hasExp && now.After(exp) {
			return cli.ExitFail
		}
		return cli.ExitOK
	}

	fmt.Println(cli.Bold("header"))
	cli.KV(os.Stdout, flatten(header))
	fmt.Println()
	fmt.Println(cli.Bold("payload"))
	rows := flatten(payload)
	for i, r := range rows {
		if !timeClaims[r[0]] {
			continue
		}
		if t, ok := claimTime(payload, r[0]); ok {
			note := cli.HumanDuration(t.Sub(now))
			if r[0] == "exp" && now.After(t) {
				note = cli.Red("expired " + note)
			} else {
				note = cli.Dim(note)
			}
			rows[i][1] = t.Local().Format("2006-01-02 15:04:05") + "  " + note +
				cli.Dim(" ("+r[1]+")")
		}
	}
	cli.KV(os.Stdout, rows)
	fmt.Println()
	fmt.Println(cli.Bold("signature"))
	if len(parts) < 3 {
		fmt.Println("  " + cli.Yellow("absent") + "  alg=none, unsecured token")
	} else {
		fmt.Printf("  %s %s  %s\n", alg, fmt.Sprintf("%dB", len(sig)), cli.Dim("not verified (dkit does not check signatures)"))
	}

	if hasNbf && now.Before(nbf) {
		fmt.Println()
		fmt.Println("  " + cli.Yellow("not valid yet") + "  " + cli.Dim("nbf is "+cli.HumanDuration(nbf.Sub(now))))
	}
	if hasExp && now.After(exp) {
		return cli.ExitFail
	}
	if hasIat && hasExp && iat.After(exp) {
		fmt.Println("  " + cli.Yellow("iat is after exp") + "  " + cli.Dim("suspicious token"))
	}
	return cli.ExitOK
}

func readStdin() ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(os.Stdin)
	return buf.Bytes(), err
}

func decodeSegment(s string) (map[string]any, error) {
	dec := base64.RawURLEncoding
	b, err := dec.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

func claimTime(m map[string]any, key string) (time.Time, bool) {
	v, ok := m[key]
	if !ok {
		return time.Time{}, false
	}
	num, ok := v.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	sec, err := num.Int64()
	if err != nil {
		f, ferr := num.Float64()
		if ferr != nil {
			return time.Time{}, false
		}
		sec = int64(f)
	}
	return time.Unix(sec, 0), true
}

// flatten renders a decoded JSON object as sorted key/value rows.
func flatten(m map[string]any) [][2]string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([][2]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, [2]string{k, renderValue(m[k])})
	}
	return rows
}

func renderValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}
