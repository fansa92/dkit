package tools

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/sha3"
)

func init() {
	cli.Register(cli.Command{
		Name:    "hash",
		Aliases: []string{"h"},
		Summary: "hash text or files: md5, sha*, blake2b, crc32, hmac",
		Usage: []string{
			"README.md                     sha256 of a file",
			"-a md5 --literal \"hello\"      hash a string",
			"-a all data.bin               every algorithm at once",
			"-a sha256 -c <expected> f     verify a checksum, exit 1 on mismatch",
			"-a hmac-sha256 -k secret f    keyed hash",
			"[-a algo] [-c expected] [-k key] [--literal]",
		},
		Run: runHash,
	})
}

func runHash(args []string) int {
	fs := cli.NewFlagSet("hash")
	alg := fs.String("a", "sha256", "algorithm, or 'all'")
	check := fs.String("c", "", "compare against an expected hex digest")
	key := fs.String("k", "", "key for hmac-* algorithms")
	literal := fs.Bool("literal", false, "treat the argument as text even if a file has that name")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	data, label, err := cli.Input(fs.Args(), *literal)
	if err != nil {
		return cli.Fail(err)
	}

	algs := []string{*alg}
	if *alg == "all" {
		algs = []string{"md5", "sha1", "sha256", "sha512", "sha3-256", "blake2b-256", "crc32"}
	}

	var first string
	for _, a := range algs {
		sum, err := digest(a, data, *key)
		if err != nil {
			return cli.Fail(err)
		}
		if first == "" {
			first = sum
		}
		if *check == "" {
			if len(algs) == 1 {
				fmt.Printf("%s  %s\n", sum, label)
			} else {
				fmt.Printf("%-12s %s  %s\n", a, sum, label)
			}
		}
	}

	if *check != "" {
		want := strings.ToLower(strings.TrimSpace(*check))
		got := strings.ToLower(first)
		if want == got {
			fmt.Printf("%s  OK  %s\n", cli.Green("match"), label)
			return cli.ExitOK
		}
		fmt.Printf("%s  MISMATCH  %s\n", cli.Red("fail"), label)
		fmt.Printf("  expected %s\n  got      %s\n", want, got)
		return cli.ExitFail
	}
	return cli.ExitOK
}

func digest(alg string, data []byte, key string) (string, error) {
	if strings.HasPrefix(alg, "hmac-") {
		inner := strings.TrimPrefix(alg, "hmac-")
		h, err := newHasher(inner)
		if err != nil {
			return "", err
		}
		if key == "" {
			return "", fmt.Errorf("hmac-* needs -k <key>")
		}
		m := hmac.New(func() hash.Hash { return h }, []byte(key))
		m.Write(data)
		return hex.EncodeToString(m.Sum(nil)), nil
	}
	h, err := newHasher(alg)
	if err != nil {
		return "", err
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newHasher(alg string) (hash.Hash, error) {
	switch strings.ToLower(alg) {
	case "md5":
		return md5.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "sha224":
		return sha256.New224(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha384":
		return sha512.New384(), nil
	case "sha512":
		return sha512.New(), nil
	case "sha3-256":
		return sha3.New256(), nil
	case "sha3-512":
		return sha3.New512(), nil
	case "blake2b-256":
		h, err := blake2b.New256(nil)
		if err != nil {
			return nil, err
		}
		return h, nil
	case "blake2b-512":
		h, err := blake2b.New512(nil)
		if err != nil {
			return nil, err
		}
		return h, nil
	case "crc32":
		return crc32.NewIEEE(), nil
	case "adler32":
		return adler32.New(), nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q (try: md5 sha1 sha224 sha256 sha384 sha512 sha3-256 sha3-512 blake2b-256 crc32 adler32 hmac-<algo>)", alg)
	}
}
