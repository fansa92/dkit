package tools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/fansa92/dkit/internal/cli"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	cli.Register(cli.Command{
		Name:    "gen",
		Aliases: []string{"g"},
		Summary: "generate uuid, ulid, passwords, lorem, rsa keys, totp codes",
		Usage: []string{
			"-t uuid            one UUID v4",
			"-t uuid -v 7 -c 5 five sortable UUIDs",
			"-t ulid            a ULID",
			"-t password -n 24 -s",
			"-t lorem -n 40     forty words",
			"-t rsa -b 3072 -o key.pem",
			"-t totp -k <base32 secret>",
			"-t random -n 32    hex bytes",
		},
		Run: runGen,
	})
}

func runGen(args []string) int {
	fs := cli.NewFlagSet("gen")
	typ := fs.String("t", "uuid", "uuid, ulid, password, lorem, rsa, totp, bcrypt, random")
	n := fs.Int("n", 0, "length: password chars, lorem words, random bytes")
	count := fs.Int("c", 1, "how many to generate")
	ver := fs.Int("v", 4, "uuid version: 4 or 7")
	bits := fs.Int("b", 2048, "rsa key size in bits")
	secret := fs.String("k", "", "base32 secret for totp")
	out := fs.String("o", "", "write to a file (rsa keys)")
	symbols := fs.Bool("s", false, "include symbols in passwords")
	pw := fs.String("p", "", "password for -t bcrypt")
	hashToCheck := fs.String("hash", "", "with -t bcrypt, verify against this hash instead of generating")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	switch *typ {
	case "uuid":
		for i := 0; i < *count; i++ {
			var id string
			if *ver == 7 {
				id = uuidV7()
			} else {
				id = uuidV4()
			}
			fmt.Println(id)
		}
	case "ulid":
		for i := 0; i < *count; i++ {
			fmt.Println(newULID())
		}
	case "password":
		length := *n
		if length <= 0 {
			length = 20
		}
		for i := 0; i < *count; i++ {
			pw, err := newPassword(length, *symbols)
			if err != nil {
				return cli.Fail(err)
			}
			fmt.Println(pw)
		}
	case "lorem":
		words := *n
		if words <= 0 {
			words = 30
		}
		for i := 0; i < *count; i++ {
			fmt.Println(loremWords(words))
		}
	case "random":
		size := *n
		if size <= 0 {
			size = 16
		}
		buf := make([]byte, size)
		if _, err := rand.Read(buf); err != nil {
			return cli.Fail(err)
		}
		fmt.Println(hex.EncodeToString(buf))
	case "rsa":
		key, err := rsa.GenerateKey(rand.Reader, *bits)
		if err != nil {
			return cli.Fail(err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return cli.Fail(err)
		}
		privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return cli.Fail(err)
		}
		pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

		if *out == "" {
			fmt.Print(string(privPEM))
			fmt.Println(cli.Dim("--- public key ---"))
			fmt.Print(string(pubPEM))
			return cli.ExitOK
		}
		if err := cli.WriteOutput(*out, privPEM); err != nil {
			return cli.Fail(err)
		}
		pubPath := strings.TrimSuffix(*out, ".pem") + ".pub"
		if err := cli.WriteOutput(pubPath, pubPEM); err != nil {
			return cli.Fail(err)
		}
		fmt.Printf("wrote %s and %s\n", *out, pubPath)
	case "bcrypt":
		pass := *pw
		if pass == "" {
			b, err := readStdin()
			if err != nil {
				return cli.Fail(err)
			}
			pass = strings.TrimSpace(string(b))
		}
		if pass == "" {
			return cli.Fail(errors.New("bcrypt needs -p <password> or a password on stdin"))
		}
		if *hashToCheck != "" {
			if err := bcrypt.CompareHashAndPassword([]byte(*hashToCheck), []byte(pass)); err != nil {
				fmt.Println(cli.Red("no match"))
				return cli.ExitFail
			}
			fmt.Println(cli.Green("match"))
			return cli.ExitOK
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
		if err != nil {
			return cli.Fail(err)
		}
		fmt.Println(string(hash))
	case "totp":
		if *secret == "" {
			return cli.Fail(errors.New("totp needs -k <base32 secret>"))
		}
		for i := 0; i < *count; i++ {
			code, remain, err := totpCode(*secret, 6, 30)
			if err != nil {
				return cli.Fail(err)
			}
			fmt.Printf("%s  %s\n", code, cli.Dim(fmt.Sprintf("%ds left", remain)))
			if i < *count-1 {
				time.Sleep(time.Second)
			}
		}
	default:
		return cli.Fail(fmt.Errorf("unknown type %q (uuid, ulid, password, lorem, rsa, totp, random)", *typ))
	}
	return cli.ExitOK
}

func uuidV4() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

func uuidV7() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	rand.Read(b[6:])
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func newULID() string {
	var b [16]byte
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	rand.Read(b[6:])

	// 128 bits into 26 Crockford base32 characters.
	out := make([]byte, 26)
	x := new(big.Int).SetBytes(b[:])
	radix := big.NewInt(32)
	mod := new(big.Int)
	for i := 25; i >= 0; i-- {
		x.DivMod(x, radix, mod)
		out[i] = crockford[mod.Int64()]
	}
	return string(out)
}

const (
	pwLower   = "abcdefghijkmnopqrstuvwxyz"
	pwUpper   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwDigits  = "23456789"
	pwSymbols = "!@#$%^&*()-_=+[]{};:,.?/"
)

func newPassword(length int, withSymbols bool) (string, error) {
	alphabet := pwLower + pwUpper + pwDigits
	if withSymbols {
		alphabet += pwSymbols
	}
	if length <= 0 {
		return "", errors.New("password length must be positive")
	}
	out := make([]byte, length)
	for i := range out {
		idx, err := randInt(len(alphabet))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[idx]
	}
	return string(out), nil
}

func randInt(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("empty alphabet")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

var loremCorpus = strings.Fields(`lorem ipsum dolor sit amet consectetur adipiscing elit sed do
eiusmod tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam
quis nostrud exercitation ullamco laboris nisi aliquip ex ea commodo consequat
duis aute irure in reprehenderit voluptate velit esse cillum eu fugiat nulla
pariatur excepteur sint occaecat cupidatat non proident sunt culpa qui officia
deserunt mollit anim id est laborum`)

func loremWords(n int) string {
	words := make([]string, n)
	for i := 0; i < n; i++ {
		idx, err := randInt(len(loremCorpus))
		if err != nil {
			idx = i % len(loremCorpus)
		}
		words[i] = loremCorpus[idx]
	}
	s := strings.Join(words, " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

func totpCode(secret string, digits, period int) (string, int, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	normalized = strings.TrimRight(normalized, "=")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalized)
	if err != nil {
		return "", 0, fmt.Errorf("secret is not valid base32: %w", err)
	}
	now := time.Now()
	counter := uint64(now.Unix() / int64(period))
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 |
		uint32(sum[off+1])<<16 |
		uint32(sum[off+2])<<8 |
		uint32(sum[off+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	remain := period - int(now.Unix()%int64(period))
	return fmt.Sprintf("%0*d", digits, bin%mod), remain, nil
}
