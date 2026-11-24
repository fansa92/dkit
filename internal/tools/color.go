package tools

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/fansa92/dkit/internal/cli"
)

func init() {
	cli.Register(cli.Command{
		Name:    "color",
		Aliases: []string{"co"},
		Summary: "convert colours between hex, rgb, hsl and hsv; check WCAG contrast",
		Usage: []string{
			"'#ff8800'                    every representation, plus contrast on white and black",
			"--contrast '#ffffff' '#767676'   WCAG ratio and pass level",
			"--blind '#ff0000' -t deuteranopia",
		},
		Run: runColor,
	})
}

var namedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000",
	"blue": "#0000ff", "yellow": "#ffff00", "cyan": "#00ffff", "aqua": "#00ffff",
	"magenta": "#ff00ff", "fuchsia": "#ff00ff", "gray": "#808080", "grey": "#808080",
	"silver": "#c0c0c0", "maroon": "#800000", "olive": "#808000", "lime": "#00ff00",
	"teal": "#008080", "navy": "#000080", "purple": "#800080", "orange": "#ffa500",
	"pink": "#ffc0cb", "brown": "#a52a2a",
}

func runColor(args []string) int {
	fs := cli.NewFlagSet("color")
	contrastA := fs.String("contrast", "", "first colour for a contrast check (pass the second positionally)")
	blindType := fs.String("t", "", "simulate colour blindness: protanopia, deuteranopia, tritanopia, achromatopsia")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}

	if *contrastA != "" {
		if fs.NArg() == 0 {
			return cli.Fail(fmt.Errorf("--contrast needs a second colour"))
		}
		a, err := parseColor(*contrastA)
		if err != nil {
			return cli.Fail(err)
		}
		b, err := parseColor(fs.Arg(0))
		if err != nil {
			return cli.Fail(err)
		}
		ratio := contrastRatio(a, b)
		fmt.Printf("  %s vs %s\n", hexOf(a), hexOf(b))
		fmt.Printf("  ratio  %.2f:1\n", ratio)
		fmt.Printf("  level  %s\n", wcagLevel(ratio))
		return cli.ExitOK
	}

	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit color '#ff8800'"))
	}
	c, err := parseColor(fs.Arg(0))
	if err != nil {
		return cli.Fail(err)
	}

	if *blindType != "" {
		m, ok := blindMatrices[strings.ToLower(*blindType)]
		if !ok {
			return cli.Fail(fmt.Errorf("unknown simulation %q", *blindType))
		}
		r := m[0]*float64(c.R)/255 + m[1]*float64(c.G)/255 + m[2]*float64(c.B)/255
		g := m[3]*float64(c.R)/255 + m[4]*float64(c.G)/255 + m[5]*float64(c.B)/255
		b := m[6]*float64(c.R)/255 + m[7]*float64(c.G)/255 + m[8]*float64(c.B)/255
		sim := colorFromFloat(r, g, b, 0xffff)
		fmt.Printf("  original  %s\n", hexOf(c))
		fmt.Printf("  %-9s %s\n", *blindType, hexOf(sim))
		return cli.ExitOK
	}

	h, s, l := rgbToHSL(c)
	hv, sv, vv := rgbToHSV(c)
	lum := relLuminance(c)

	rows := [][2]string{
		{"hex", hexOf(c)},
		{"rgb", fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)},
		{"hsl", fmt.Sprintf("hsl(%.0f, %.0f%%, %.0f%%)", h, s*100, l*100)},
		{"hsv", fmt.Sprintf("hsv(%.0f, %.0f%%, %.0f%%)", hv, sv*100, vv*100)},
		{"luminance", fmt.Sprintf("%.4f", lum)},
		{"contrast_white", fmt.Sprintf("%.2f:1  %s", contrastRatio(c, color.RGBA{255, 255, 255, 255}), wcagLevel(contrastRatio(c, color.RGBA{255, 255, 255, 255})))},
		{"contrast_black", fmt.Sprintf("%.2f:1  %s", contrastRatio(c, color.RGBA{0, 0, 0, 255}), wcagLevel(contrastRatio(c, color.RGBA{0, 0, 0, 255})))},
	}
	cli.KV(os.Stdout, rows)
	return cli.ExitOK
}

func parseColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if hex, ok := namedColors[s]; ok {
		s = hex
	}

	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		switch len(hex) {
		case 3:
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		case 6:
		case 8:
			hex = hex[:6]
		default:
			return color.RGBA{}, fmt.Errorf("%q is not a valid hex colour", s)
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("%q is not a valid hex colour", s)
		}
		return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
	}

	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(s, "rgb("), ")"), ",")
		if len(parts) != 3 {
			return color.RGBA{}, fmt.Errorf("%q needs three components", s)
		}
		var vals [3]uint8
		for i, p := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 || n > 255 {
				return color.RGBA{}, fmt.Errorf("component %q is not 0-255", p)
			}
			vals[i] = uint8(n)
		}
		return color.RGBA{vals[0], vals[1], vals[2], 255}, nil
	}

	return color.RGBA{}, fmt.Errorf("cannot read %q as a colour (try #rgb, #rrggbb, rgb(...) or a CSS name)", s)
}

func hexOf(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func colorFromFloat(r, g, b float64, a uint32) color.RGBA {
	clamp := func(v float64) uint8 {
		switch {
		case v <= 0:
			return 0
		case v >= 1:
			return 255
		default:
			return uint8(v*255 + 0.5)
		}
	}
	return color.RGBA{clamp(r), clamp(g), clamp(b), uint8(a >> 8)}
}

func relLuminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

func contrastRatio(a, b color.RGBA) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func wcagLevel(ratio float64) string {
	switch {
	case ratio >= 7:
		return "AAA"
	case ratio >= 4.5:
		return "AA"
	case ratio >= 3:
		return "AA large text only"
	default:
		return "fails"
	}
}

func rgbToHSL(c color.RGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	l = (max + min) / 2

	if max == min {
		return 0, 0, l
	}
	d := max - min
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func rgbToHSV(c color.RGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	d := max - min
	if max != 0 {
		s = d / max
	}
	if d == 0 {
		return 0, s, v
	}
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}
