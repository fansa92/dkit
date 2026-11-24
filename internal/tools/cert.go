package tools

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "cert",
		Aliases: []string{"ce"},
		Summary: "parse PEM certificates: subject, issuer, expiry, SANs, fingerprint",
		Usage: []string{
			"cert.pem        read one or more PEM certificates from a file",
			"- < cert.pem    or from stdin",
			"[--json]",
		},
		Run: runCert,
	})
}

func runCert(args []string) int {
	fs := cli.NewFlagSet("cert")
	asJSON := fs.Bool("json", false, "emit JSON for scripting")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	data, label, err := cli.ReadInput(fs.Args())
	if err != nil {
		return cli.Fail(err)
	}

	var certs []*x509.Certificate
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		parsed, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			return cli.Fail(fmt.Errorf("parsing certificate in %s: %w", label, err))
		}
		certs = append(certs, parsed...)
	}
	if len(certs) == 0 {
		return cli.Fail(fmt.Errorf("no PEM CERTIFICATE block found in %s", label))
	}

	now := time.Now()

	if *asJSON {
		out := make([]map[string]any, 0, len(certs))
		for i, c := range certs {
			out = append(out, map[string]any{
				"index":      i + 1,
				"subject":    c.Subject.String(),
				"issuer":     c.Issuer.String(),
				"serial":     c.SerialNumber.Text(16),
				"notBefore":  c.NotBefore.UTC().Format(time.RFC3339),
				"notAfter":   c.NotAfter.UTC().Format(time.RFC3339),
				"expired":    now.After(c.NotAfter),
				"expiresIn":  c.NotAfter.Sub(now).Round(time.Second).String(),
				"publicKey":  keyInfo(c),
				"dnsNames":   c.DNSNames,
				"ipAddrs":    ipStrings(c),
				"isCA":       c.IsCA,
				"selfSigned": isSelfSigned(c),
				"sha256":     fingerprint(c.Raw, false),
				"sha1":       fingerprint(c.Raw, true),
			})
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		for _, c := range certs {
			if now.After(c.NotAfter) {
				return cli.ExitFail
			}
		}
		return cli.ExitOK
	}

	expiredAny := false
	for i, c := range certs {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(cli.Bold(fmt.Sprintf("[%d] %s", i+1, c.Subject.CommonName)))
		rows := [][2]string{
			{"subject", c.Subject.String()},
			{"issuer", c.Issuer.String()},
			{"serial", c.SerialNumber.Text(16)},
			{"key", keyInfo(c)},
		}
		if len(c.DNSNames) > 0 {
			rows = append(rows, [2]string{"sans", strings.Join(c.DNSNames, ", ")})
		}
		if ips := ipStrings(c); len(ips) > 0 {
			rows = append(rows, [2]string{"ips", strings.Join(ips, ", ")})
		}
		if usage := usageString(c); usage != "" {
			rows = append(rows, [2]string{"usage", usage})
		}
		rows = append(rows,
			[2]string{"not_before", c.NotBefore.Format("2006-01-02 15:04:05 -0700")},
		)
		expiry := c.NotAfter.Format("2006-01-02 15:04:05 -0700")
		if now.After(c.NotAfter) {
			expiredAny = true
			expiry += "  " + cli.Red("expired "+cli.HumanDuration(c.NotAfter.Sub(now)))
		} else {
			expiry += "  " + cli.Dim(cli.HumanDuration(c.NotAfter.Sub(now)))
		}
		rows = append(rows,
			[2]string{"not_after", expiry},
			[2]string{"sha256", fingerprint(c.Raw, false)},
			[2]string{"sha1", fingerprint(c.Raw, true)},
		)
		if isSelfSigned(c) {
			rows = append(rows, [2]string{"chain", cli.Yellow("self-signed")})
		}
		if c.IsCA {
			rows = append(rows, [2]string{"ca", "yes"})
		}
		cli.KV(os.Stdout, rows)
	}

	if expiredAny {
		return cli.ExitFail
	}
	return cli.ExitOK
}

func keyInfo(c *x509.Certificate) string {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("RSA %d bits", k.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + k.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	default:
		return c.PublicKeyAlgorithm.String()
	}
}

func ipStrings(c *x509.Certificate) []string {
	out := make([]string, 0, len(c.IPAddresses))
	for _, ip := range c.IPAddresses {
		out = append(out, ip.String())
	}
	return out
}

func usageString(c *x509.Certificate) string {
	var parts []string
	if c.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		parts = append(parts, "digitalSignature")
	}
	if c.KeyUsage&x509.KeyUsageKeyEncipherment != 0 {
		parts = append(parts, "keyEncipherment")
	}
	if c.KeyUsage&x509.KeyUsageCertSign != 0 {
		parts = append(parts, "certSign")
	}
	for _, u := range c.ExtKeyUsage {
		switch u {
		case x509.ExtKeyUsageServerAuth:
			parts = append(parts, "serverAuth")
		case x509.ExtKeyUsageClientAuth:
			parts = append(parts, "clientAuth")
		}
	}
	return strings.Join(parts, ", ")
}

func isSelfSigned(c *x509.Certificate) bool {
	return c.CheckSignatureFrom(c) == nil
}

func fingerprint(der []byte, useSHA1 bool) string {
	var sum []byte
	if useSHA1 {
		s := sha1.Sum(der)
		sum = s[:]
	} else {
		s := sha256.Sum256(der)
		sum = s[:]
	}
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}
