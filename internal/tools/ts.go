package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "ts",
		Aliases: []string{"t"},
		Summary: "convert timestamps, dates and relative times in both directions",
		Usage: []string{
			"                 show the current time in every format",
			"1700000000       read a unix timestamp (s, ms, us or ns by digit count)",
			"2026-11-23       read a date or date-time",
			"-1h              relative to now (supports s m h d w)",
			"[--tz Asia/Shanghai] [--json]",
		},
		Run: runTS,
	})
}

var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	time.RFC1123,
	time.RFC1123Z,
	"15:04:05",
	"15:04",
}

func runTS(args []string) int {
	fs := cli.NewFlagSet("ts")
	asJSON := fs.Bool("json", false, "emit a single JSON object for scripting")
	tz := fs.String("tz", "", "time zone for the local columns, e.g. Asia/Shanghai")

	// Pull a leading relative duration out before flag parsing: "-1h" would
	// otherwise be rejected as an unknown flag.
	var rel *time.Duration
	if len(args) > 0 {
		if d, ok := parseRelative(args[0]); ok {
			rel = &d
			args = args[1:]
		}
	}
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	loc := time.Local
	if *tz != "" {
		l, err := time.LoadLocation(*tz)
		if err != nil {
			return cli.Fail(fmt.Errorf("unknown time zone %q", *tz))
		}
		loc = l
	}

	now := time.Now()
	var t time.Time
	switch {
	case rel != nil:
		t = now.Add(*rel)
	case fs.NArg() > 0:
		v := fs.Arg(0)
		if ts, ok := parseTimestamp(v); ok {
			t = ts
		} else if parsed, err := parseDate(v, loc); err == nil {
			t = parsed
		} else {
			return cli.Fail(fmt.Errorf("cannot read %q as a timestamp or a date", v))
		}
	default:
		t = now
	}

	tl := t.In(loc)
	isoYear, isoWeek := tl.ISOWeek()
	isoDay := int(tl.Weekday())
	if isoDay == 0 {
		isoDay = 7
	}

	if *asJSON {
		doc := map[string]any{
			"unix":      t.Unix(),
			"unixMilli": t.UnixMilli(),
			"unixNano":  t.UnixNano(),
			"rfc3339":   tl.Format(time.RFC3339),
			"utc":       t.UTC().Format(time.RFC3339),
			"local":     tl.Format("2006-01-02 15:04:05 -0700 MST"),
			"isoWeek":   fmt.Sprintf("%d-W%02d", isoYear, isoWeek),
			"weekday":   tl.Weekday().String(),
		}
		out, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(out))
		return cli.ExitOK
	}

	rows := [][2]string{
		{"unix", strconv.FormatInt(t.Unix(), 10)},
		{"unix_ms", strconv.FormatInt(t.UnixMilli(), 10)},
		{"unix_ns", strconv.FormatInt(t.UnixNano(), 10)},
		{"rfc3339", tl.Format(time.RFC3339)},
		{"utc", t.UTC().Format(time.RFC3339)},
		{"local", tl.Format("2006-01-02 15:04:05 -0700 MST")},
		{"date", tl.Format("2006-01-02")},
		{"time", tl.Format("15:04:05")},
		{"weekday", tl.Weekday().String()},
		{"iso_week", fmt.Sprintf("%d-W%02d-%d", isoYear, isoWeek, isoDay)},
	}
	if rel != nil {
		rows = append(rows, [2]string{"relative", cli.HumanDuration(t.Sub(now))})
	}
	cli.KV(os.Stdout, rows)
	return cli.ExitOK
}

func parseRelative(s string) (time.Duration, bool) {
	if s == "" {
		return 0, false
	}
	sign := time.Duration(1)
	i := 0
	if s[0] == '+' || s[0] == '-' {
		if s[0] == '-' {
			sign = -1
		}
		i = 1
	}
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i || j == len(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s[i:j])
	if err != nil {
		return 0, false
	}
	var unit time.Duration
	switch strings.ToLower(s[j:]) {
	case "s", "sec":
		unit = time.Second
	case "m", "min":
		unit = time.Minute
	case "h", "hr":
		unit = time.Hour
	case "d", "day":
		unit = 24 * time.Hour
	case "w", "week":
		unit = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	return sign * time.Duration(n) * unit, true
}

func parseTimestamp(s string) (time.Time, bool) {
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, false
		}
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), true
	}
	digits := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	if digits == "" {
		return time.Time{}, false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return time.Time{}, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	switch {
	case len(digits) <= 10:
		return time.Unix(n, 0), true
	case len(digits) <= 13:
		return time.UnixMilli(n), true
	case len(digits) <= 16:
		return time.UnixMicro(n), true
	default:
		return time.Unix(0, n), true
	}
}

func parseDate(s string, loc *time.Location) (time.Time, error) {
	var firstErr error
	for _, layout := range dateLayouts {
		t, err := time.ParseInLocation(layout, s, loc)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// A bare clock time has no date; anchor it to today.
		if t.Year() == 0 && t.Month() == 1 && t.Day() == 1 {
			now := time.Now().In(loc)
			t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
		}
		return t, nil
	}
	return time.Time{}, firstErr
}
