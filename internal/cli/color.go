package cli

import (
	"os"
)

// NoColor disables ANSI output. It starts true when NO_COLOR is set or stdout
// is not a terminal, and Run flips it on when the global --no-color flag is
// passed.
var NoColor = os.Getenv("NO_COLOR") != "" || !isTerminal(os.Stdout)

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if NoColor {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold returns s in bold.
func Bold(s string) string { return paint("1", s) }

// Dim returns s in a dimmed weight.
func Dim(s string) string { return paint("2", s) }

// Cyan returns s in cyan.
func Cyan(s string) string { return paint("36", s) }

// Green returns s in green.
func Green(s string) string { return paint("32", s) }

// Yellow returns s in yellow.
func Yellow(s string) string { return paint("33", s) }

// Red returns s in red.
func Red(s string) string { return paint("31", s) }
