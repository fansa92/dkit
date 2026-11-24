package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "cron",
		Aliases: []string{"cr"},
		Summary: "parse a cron expression, explain it, and list the next runs",
		Usage: []string{
			"\"*/5 * * * *\"         explain and list the next 5 runs",
			"-n 10 \"0 9 * * 1-5\"    list more runs",
			"[--tz Asia/Shanghai] [--json]",
		},
		Run: runCron,
	})
}

var cronMacros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var monthNames = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var dowNames = map[string]int{
	"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
}

type schedule struct {
	expr          string
	hasSec        bool
	sec           map[int]bool
	min           map[int]bool
	hour          map[int]bool
	dom           map[int]bool
	month         map[int]bool
	dow           map[int]bool
	domRestricted bool
	dowRestricted bool
}

func runCron(args []string) int {
	fs := cli.NewFlagSet("cron")
	count := fs.Int("n", 5, "how many upcoming runs to list")
	tz := fs.String("tz", "", "time zone for the listed runs")
	asJSON := fs.Bool("json", false, "emit JSON for scripting")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	expr := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if expr == "" {
		b, err := readStdin()
		if err != nil {
			return cli.Fail(err)
		}
		expr = strings.TrimSpace(string(b))
	}
	if expr == "" {
		return cli.Fail(fmt.Errorf("no cron expression given"))
	}

	s, err := parseCron(expr)
	if err != nil {
		return cli.Fail(err)
	}

	loc := time.Local
	if *tz != "" {
		l, lerr := time.LoadLocation(*tz)
		if lerr != nil {
			return cli.Fail(fmt.Errorf("unknown time zone %q", *tz))
		}
		loc = l
	}

	now := time.Now().In(loc)
	runs := make([]time.Time, 0, *count)
	cursor := now
	for i := 0; i < *count; i++ {
		next, ok := s.nextAfter(cursor)
		if !ok {
			break
		}
		runs = append(runs, next)
		cursor = next
	}
	if len(runs) == 0 {
		return cli.Fail(fmt.Errorf("%q never fires in the next 5 years", expr))
	}

	meaning := s.describe()

	if *asJSON {
		list := make([]string, len(runs))
		for i, r := range runs {
			list[i] = r.Format(time.RFC3339)
		}
		doc := map[string]any{
			"expression": expr,
			"meaning":    meaning,
			"hasSeconds": s.hasSec,
			"next":       list,
		}
		b, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(b))
		return cli.ExitOK
	}

	rows := [][2]string{{"expression", expr}}
	rows = append(rows, [2]string{"meaning", cli.Cyan(meaning)})
	sec := "(omitted)"
	if s.hasSec {
		sec = s.fieldString(0)
	}
	rows = append(rows,
		[2]string{"second", sec},
		[2]string{"minute", s.fieldString(1)},
		[2]string{"hour", s.fieldString(2)},
		[2]string{"day", s.fieldString(3)},
		[2]string{"month", s.fieldString(4)},
		[2]string{"weekday", s.fieldString(5)},
	)
	cli.KV(os.Stdout, rows)

	fmt.Println()
	fmt.Println(cli.Bold("next runs"))
	for _, r := range runs {
		fmt.Printf("  %s  %s\n", r.Format("2006-01-02 15:04:05"), cli.Dim(r.Format("Mon MST")))
	}
	if s.domRestricted && s.dowRestricted {
		fmt.Println()
		fmt.Println("  " + cli.Yellow("note") + "  " + cli.Dim("both day and weekday are set, so either one matches"))
	}
	return cli.ExitOK
}

func parseCron(expr string) (*schedule, error) {
	original := expr
	if macro, ok := cronMacros[strings.ToLower(expr)]; ok {
		expr = macro
	}
	fields := strings.Fields(expr)

	s := &schedule{expr: original}
	var err error

	switch len(fields) {
	case 5:
		if s.min, err = parseField(fields[0], 0, 59, nil); err != nil {
			return nil, fmt.Errorf("minute: %w", err)
		}
		if s.hour, err = parseField(fields[1], 0, 23, nil); err != nil {
			return nil, fmt.Errorf("hour: %w", err)
		}
		if s.dom, err = parseField(fields[2], 1, 31, nil); err != nil {
			return nil, fmt.Errorf("day: %w", err)
		}
		if s.month, err = parseField(fields[3], 1, 12, monthNames); err != nil {
			return nil, fmt.Errorf("month: %w", err)
		}
		if s.dow, err = parseField(fields[4], 0, 7, dowNames); err != nil {
			return nil, fmt.Errorf("weekday: %w", err)
		}
		s.sec = map[int]bool{0: true}
		s.domRestricted = !isStar(fields[2])
		s.dowRestricted = !isStar(fields[4])
	case 6:
		s.hasSec = true
		if s.sec, err = parseField(fields[0], 0, 59, nil); err != nil {
			return nil, fmt.Errorf("second: %w", err)
		}
		if s.min, err = parseField(fields[1], 0, 59, nil); err != nil {
			return nil, fmt.Errorf("minute: %w", err)
		}
		if s.hour, err = parseField(fields[2], 0, 23, nil); err != nil {
			return nil, fmt.Errorf("hour: %w", err)
		}
		if s.dom, err = parseField(fields[3], 1, 31, nil); err != nil {
			return nil, fmt.Errorf("day: %w", err)
		}
		if s.month, err = parseField(fields[4], 1, 12, monthNames); err != nil {
			return nil, fmt.Errorf("month: %w", err)
		}
		if s.dow, err = parseField(fields[5], 0, 7, dowNames); err != nil {
			return nil, fmt.Errorf("weekday: %w", err)
		}
		s.domRestricted = !isStar(fields[3])
		s.dowRestricted = !isStar(fields[5])
	default:
		return nil, fmt.Errorf("expected 5 or 6 fields, got %d", len(fields))
	}

	// Sunday is both 0 and 7.
	if s.dow[7] {
		s.dow[0] = true
		delete(s.dow, 7)
	}
	return s, nil
}

func isStar(s string) bool {
	return s == "*" || s == "?"
}

func parseField(spec string, lo, hi int, names map[string]int) (map[int]bool, error) {
	out := make(map[int]bool)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			v, err := strconv.Atoi(part[idx+1:])
			if err != nil || v <= 0 {
				return nil, fmt.Errorf("bad step in %q", part)
			}
			step = v
			part = part[:idx]
		}

		var start, end int
		switch {
		case isStar(part):
			start, end = lo, hi
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			a, err1 := fieldValue(ab[0], names)
			b, err2 := fieldValue(ab[1], names)
			if err1 != nil {
				return nil, err1
			}
			if err2 != nil {
				return nil, err2
			}
			if a > b {
				return nil, fmt.Errorf("range %q runs backwards", part)
			}
			start, end = a, b
		default:
			v, err := fieldValue(part, names)
			if err != nil {
				return nil, err
			}
			start, end = v, v
		}

		if start < lo || end > hi {
			return nil, fmt.Errorf("%q is outside %d..%d", part, lo, hi)
		}
		for v := start; v <= end; v += step {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("field %q matched nothing", spec)
	}
	return out, nil
}

func fieldValue(s string, names map[string]int) (int, error) {
	if names != nil {
		if v, ok := names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return v, nil
}

func (s *schedule) dayMatch(t time.Time) bool {
	domOK := s.dom[t.Day()]
	dowOK := s.dow[int(t.Weekday())]
	switch {
	case s.domRestricted && s.dowRestricted:
		return domOK || dowOK
	case s.domRestricted:
		return domOK
	case s.dowRestricted:
		return dowOK
	default:
		return true
	}
}

// nextAfter returns the first firing time strictly after t.
func (s *schedule) nextAfter(t time.Time) (time.Time, bool) {
	if s.hasSec {
		t = t.Truncate(time.Second).Add(time.Second)
	} else {
		t = t.Truncate(time.Minute).Add(time.Minute)
	}

	loc := t.Location()
	limit := t.AddDate(5, 0, 0)

	for t.Before(limit) {
		if !s.month[int(t.Month())] {
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
			continue
		}
		if !s.dayMatch(t) {
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
			continue
		}
		if !s.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc).Add(time.Hour)
			continue
		}
		if !s.min[t.Minute()] {
			t = t.Truncate(time.Minute).Add(time.Minute)
			continue
		}
		if s.hasSec && !s.sec[t.Second()] {
			t = t.Add(time.Second)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

func (s *schedule) fieldString(idx int) string {
	if s.expr != "" {
		fields := strings.Fields(cronMacros[strings.ToLower(s.expr)])
		if len(fields) == 0 {
			fields = strings.Fields(s.expr)
		}
		if len(fields) == 5 && idx >= 1 && idx <= 5 {
			return fields[idx-1]
		}
		if len(fields) == 6 && idx >= 0 && idx <= 5 {
			return fields[idx]
		}
	}
	return ""
}

func (s *schedule) describe() string {
	everyDay := allOf(s.dom, 1, 31) && allOf(s.month, 1, 12) && allOf(s.dow, 0, 6)
	everyHour := allOf(s.hour, 0, 23)

	if everyDay && everyHour {
		if allOf(s.min, 0, 59) {
			return "every minute"
		}
		if step, ok := uniformStep(s.min, 0, 59); ok {
			return fmt.Sprintf("every %d minutes", step)
		}
	}
	if everyDay && len(s.hour) == 1 && allOf(s.min, 0, 59) {
		return fmt.Sprintf("every minute during hour %02d", firstOf(s.hour))
	}
	if everyDay && len(s.min) == 1 && len(s.hour) == 1 {
		return fmt.Sprintf("every day at %02d:%02d", firstOf(s.hour), firstOf(s.min))
	}
	if allOf(s.min, 0, 59) == false && len(s.min) == 1 && len(s.hour) == 1 && s.dowRestricted {
		return fmt.Sprintf("at %02d:%02d on %s", firstOf(s.hour), firstOf(s.min), weekdayList(s.dow))
	}
	if len(s.min) == 1 && len(s.hour) == 1 && s.domRestricted {
		return fmt.Sprintf("at %02d:%02d on day %s", firstOf(s.hour), firstOf(s.min), numberList(s.dom))
	}
	if step, ok := uniformStep(s.hour, 0, 23); ok && len(s.min) == 1 {
		return fmt.Sprintf("every %d hours at minute %d", step, firstOf(s.min))
	}
	return "custom schedule"
}

func allOf(set map[int]bool, lo, hi int) bool {
	for v := lo; v <= hi; v++ {
		if !set[v] {
			return false
		}
	}
	return true
}

func firstOf(set map[int]bool) int {
	vals := sortedInts(set)
	if len(vals) == 0 {
		return 0
	}
	return vals[0]
}

func sortedInts(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func uniformStep(set map[int]bool, lo, hi int) (int, bool) {
	vals := sortedInts(set)
	if len(vals) < 2 || vals[0] != lo {
		return 0, false
	}
	step := vals[1] - vals[0]
	for i := 2; i < len(vals); i++ {
		if vals[i]-vals[i-1] != step {
			return 0, false
		}
	}
	if vals[len(vals)-1] > hi {
		return 0, false
	}
	return step, true
}

func weekdayList(set map[int]bool) string {
	names := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	vals := sortedInts(set)
	if len(vals) > 2 {
		consecutive := true
		for i := 1; i < len(vals); i++ {
			if vals[i]-vals[i-1] != 1 {
				consecutive = false
				break
			}
		}
		if consecutive && vals[len(vals)-1] < len(names) {
			return names[vals[0]] + " to " + names[vals[len(vals)-1]]
		}
	}
	var parts []string
	for _, v := range vals {
		if v < len(names) {
			parts = append(parts, names[v])
		}
	}
	return strings.Join(parts, ", ")
}

func numberList(set map[int]bool) string {
	vals := sortedInts(set)
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}
