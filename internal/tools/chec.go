package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "iban",
		Aliases: []string{"ib"},
		Summary: "validate an IBAN and show its parts",
		Usage: []string{
			"DE89370400440532013000",
			"'GB82 WEST 1234 5698 7654 32'   spaces are ignored",
		},
		Run: runIBAN,
	})
}

// ibanLengths holds the official IBAN lengths for the countries people hit most.
var ibanLengths = map[string]int{
	"AL": 28, "AD": 24, "AT": 20, "AZ": 28, "BH": 22, "BE": 16, "BA": 20,
	"BR": 29, "BG": 22, "CR": 22, "HR": 21, "CY": 28, "CZ": 24, "DK": 18,
	"DO": 28, "EE": 20, "FO": 18, "FI": 18, "FR": 27, "GE": 22, "DE": 22,
	"GI": 23, "GR": 27, "GL": 18, "GT": 28, "HU": 28, "IS": 26, "IE": 22,
	"IL": 23, "IT": 27, "JO": 30, "KZ": 20, "XK": 20, "KW": 30, "LV": 21,
	"LB": 28, "LI": 21, "LT": 20, "LU": 20, "MK": 19, "MT": 31, "MR": 27,
	"MU": 30, "MD": 24, "MC": 27, "ME": 22, "NL": 18, "NO": 15, "PK": 24,
	"PS": 29, "PL": 28, "PT": 25, "QA": 29, "RO": 24, "LC": 32, "SM": 27,
	"ST": 25, "SA": 24, "RS": 22, "SC": 31, "SK": 24, "SI": 19, "ES": 24,
	"SE": 24, "CH": 21, "TL": 23, "TN": 24, "TR": 26, "UA": 29, "AE": 23,
	"GB": 22, "VA": 22, "VG": 24,
}

func runIBAN(args []string) int {
	fs := cli.NewFlagSet("iban")
	quiet := fs.Bool("q", false, "no output, exit code only")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	var raw string
	if fs.NArg() > 0 {
		raw = strings.Join(fs.Args(), "")
	} else {
		data, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		raw = string(data)
	}

	iban := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if iban == "" {
		return cli.Fail(fmt.Errorf("no IBAN given"))
	}

	errs := checkIBAN(iban)
	if len(errs) > 0 {
		if *quiet {
			return cli.ExitFail
		}
		for _, e := range errs {
			fmt.Println("  " + cli.Red("invalid") + "  " + e)
		}
		return cli.ExitFail
	}

	if *quiet {
		return cli.ExitOK
	}
	country := iban[:2]
	cli.KV(os.Stdout,
		[][2]string{
			{"iban", groupIBAN(iban)},
			{"country", country},
			{"check_digits", iban[2:4]},
			{"bban", iban[4:]},
			{"length", fmt.Sprint(len(iban))},
			{"status", cli.Green("valid")},
		})
	return cli.ExitOK
}

func checkIBAN(iban string) []string {
	var errs []string
	for _, r := range iban {
		if !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return []string{fmt.Sprintf("character %q is not A-Z or 0-9", r)}
		}
	}
	if len(iban) < 5 {
		return []string{"too short to be an IBAN"}
	}
	country := iban[:2]
	if want, ok := ibanLengths[country]; ok && len(iban) != want {
		errs = append(errs, fmt.Sprintf("%s expects %d characters, got %d", country, want, len(iban)))
	} else if !ok && (len(iban) < 15 || len(iban) > 34) {
		errs = append(errs, fmt.Sprintf("length %d is outside the 15-34 range", len(iban)))
	}

	// Move the first four characters to the end, map letters to numbers,
	// then the whole thing must be 1 mod 97.
	rearranged := iban[4:] + iban[:4]
	remainder := 0
	for _, r := range rearranged {
		var chunk string
		if r >= 'A' && r <= 'Z' {
			chunk = fmt.Sprintf("%d", int(r-'A')+10)
		} else {
			chunk = string(r)
		}
		for _, d := range chunk {
			remainder = (remainder*10 + int(d-'0')) % 97
		}
	}
	if remainder != 1 {
		errs = append(errs, fmt.Sprintf("checksum failed: mod 97 is %d, expected 1", remainder))
	}
	return errs
}

func groupIBAN(iban string) string {
	var parts []string
	for i := 0; i < len(iban); i += 4 {
		end := i + 4
		if end > len(iban) {
			end = len(iban)
		}
		parts = append(parts, iban[i:end])
	}
	return strings.Join(parts, " ")
}
