package tools

import (
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"io"

	"github.com/fansa92/dkit/internal/cli"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func init() {
	cli.Register(cli.Command{
		Name:    "img",
		Aliases: []string{"im"},
		Summary: "inspect, convert, resize, crop and recolour images",
		Usage: []string{
			"info logo.png",
			"convert logo.png -o logo.jpg -q 85",
			"resize logo.png -w 800 -o small.png        (--height optional, keeps ratio)",
			"crop logo.png -x 10 -y 10 -w 200 --height 150 -o cropped.png",
			"compress logo.png -o smaller.png",
			"blind logo.png -t deuteranopia -o sim.png",
			"svg icon.svg -w 512 -o icon.png",
		},
		Run: runImg,
	})
}

func runImg(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dkit img <info|convert|resize|crop|compress|blind> [flags]")
		return cli.ExitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "info":
		return imgInfo(rest)
	case "convert":
		return imgTransform(rest, "convert")
	case "resize":
		return imgTransform(rest, "resize")
	case "crop":
		return imgTransform(rest, "crop")
	case "compress":
		return imgTransform(rest, "compress")
	case "blind":
		return imgTransform(rest, "blind")
	case "svg":
		return imgSVG(rest)
	case "help", "-h", "--help":
		cli.Help("img")
		return cli.ExitOK
	}
	return cli.Fail(fmt.Errorf("unknown img subcommand %q", sub))
}

func loadImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", fmt.Errorf("decoding %s: %w", path, err)
	}
	return img, format, nil
}

func saveImage(path string, img image.Image, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		return enc.Encode(f, img)
	case ".jpg", ".jpeg":
		if quality <= 0 {
			quality = 85
		}
		return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
	case ".gif":
		return gif.Encode(f, img, nil)
	default:
		return fmt.Errorf("unsupported output %q: use .png, .jpg or .gif (webp encoding is not available in pure Go)", ext)
	}
}

func imgInfo(args []string) int {
	fs := cli.NewFlagSet("img info")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit img info <file>"))
	}
	path := fs.Arg(0)
	img, format, err := loadImage(path)
	if err != nil {
		return cli.Fail(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return cli.Fail(err)
	}
	b := img.Bounds()
	cli.KV(os.Stdout, [][2]string{
		{"file", path},
		{"format", format},
		{"width", strconv.Itoa(b.Dx())},
		{"height", strconv.Itoa(b.Dy())},
		{"pixels", strconv.Itoa(b.Dx() * b.Dy())},
		{"aspect", fmt.Sprintf("%.3f", float64(b.Dx())/float64(b.Dy()))},
		{"size", humanBytes(fi.Size())},
	})
	return cli.ExitOK
}

func imgTransform(args []string, mode string) int {
	fs := cli.NewFlagSet("img " + mode)
	out := fs.String("o", "", "output file (required)")
	width := fs.Int("w", 0, "width in pixels")
	height := fs.Int("height", 0, "height in pixels")
	x := fs.Int("x", 0, "crop origin x")
	y := fs.Int("y", 0, "crop origin y")
	quality := fs.Int("q", 85, "JPEG quality 1-100")
	blindType := fs.String("t", "deuteranopia", "protanopia, deuteranopia, tritanopia, achromatopsia")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		return cli.Fail(fmt.Errorf("usage: dkit img %s <file> -o <out>", mode))
	}
	if *out == "" {
		return cli.Fail(fmt.Errorf("-o <output> is required"))
	}

	src, _, err := loadImage(fs.Arg(0))
	if err != nil {
		return cli.Fail(err)
	}
	b := src.Bounds()

	var result image.Image = src
	switch mode {
	case "convert":
		result = src

	case "resize":
		w, h := *width, *height
		if w == 0 && h == 0 {
			return cli.Fail(fmt.Errorf("resize needs -w and/or -h"))
		}
		if w == 0 {
			w = int(float64(h) * float64(b.Dx()) / float64(b.Dy()))
		}
		if h == 0 {
			h = int(float64(w) * float64(b.Dy()) / float64(b.Dx()))
		}
		if w <= 0 || h <= 0 {
			return cli.Fail(fmt.Errorf("computed size is invalid: %dx%d", w, h))
		}
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
		result = dst

	case "crop":
		if *width <= 0 || *height <= 0 {
			return cli.Fail(fmt.Errorf("crop needs -w and -h"))
		}
		rect := image.Rect(b.Min.X+*x, b.Min.Y+*y, b.Min.X+*x+*width, b.Min.Y+*y+*height)
		if !rect.In(b) {
			return cli.Fail(fmt.Errorf("crop rectangle %v is outside the image %v", rect, b))
		}
		dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Src)
		result = dst

	case "compress":
		result = src

	case "blind":
		matrix, ok := blindMatrices[strings.ToLower(*blindType)]
		if !ok {
			return cli.Fail(fmt.Errorf("unknown simulation %q", *blindType))
		}
		result = simulateBlindness(src, matrix)
	}

	if err := saveImage(*out, result, *quality); err != nil {
		return cli.Fail(err)
	}
	rb := result.Bounds()
	fmt.Printf("wrote %s (%dx%d)\n", *out, rb.Dx(), rb.Dy())
	return cli.ExitOK
}

var blindMatrices = map[string][9]float64{
	"protanopia":    {0.567, 0.433, 0.000, 0.558, 0.442, 0.000, 0.000, 0.242, 0.758},
	"deuteranopia":  {0.625, 0.375, 0.000, 0.700, 0.300, 0.000, 0.000, 0.300, 0.700},
	"tritanopia":    {0.950, 0.050, 0.000, 0.000, 0.433, 0.567, 0.000, 0.475, 0.525},
	"protanomaly":   {0.817, 0.183, 0.000, 0.333, 0.667, 0.000, 0.000, 0.125, 0.875},
	"deuteranomaly": {0.800, 0.200, 0.000, 0.258, 0.742, 0.000, 0.000, 0.142, 0.858},
	"achromatopsia": {0.299, 0.587, 0.114, 0.299, 0.587, 0.114, 0.299, 0.587, 0.114},
}

func simulateBlindness(src image.Image, m [9]float64) image.Image {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for yy := b.Min.Y; yy < b.Max.Y; yy++ {
		for xx := b.Min.X; xx < b.Max.X; xx++ {
			r16, g16, b16, a16 := src.At(xx, yy).RGBA()
			r := float64(r16>>8) / 255
			g := float64(g16>>8) / 255
			bl := float64(b16>>8) / 255

			nr := m[0]*r + m[1]*g + m[2]*bl
			ng := m[3]*r + m[4]*g + m[5]*bl
			nb := m[6]*r + m[7]*g + m[8]*bl

			dst.Set(xx-b.Min.X, yy-b.Min.Y, colorFromFloat(nr, ng, nb, a16))
		}
	}
	return dst
}

func imgSVG(args []string) int {
	fs := cli.NewFlagSet("img svg")
	out := fs.String("o", "", "output PNG path (required)")
	width := fs.Int("w", 512, "output width in pixels")
	height := fs.Int("height", 0, "output height, defaults to the SVG aspect ratio")
	if code, ok := cli.Parse(fs, args); !ok {
		return code
	}
	if *out == "" {
		return cli.Fail(fmt.Errorf("svg needs -o <out.png>"))
	}

	var r io.Reader
	if fs.NArg() > 0 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			return cli.Fail(err)
		}
		defer f.Close()
		r = f
	} else {
		r = os.Stdin
	}

	icon, err := oksvg.ReadIconStream(r, oksvg.WarnErrorMode)
	if err != nil {
		return cli.Fail(fmt.Errorf("parsing SVG: %w", err))
	}

	w := *width
	h := *height
	if h == 0 {
		if icon.ViewBox.H > 0 && icon.ViewBox.W > 0 {
			h = int(float64(w) * icon.ViewBox.H / icon.ViewBox.W)
		} else {
			h = w
		}
	}
	if w <= 0 || h <= 0 || w > 20000 || h > 20000 {
		return cli.Fail(fmt.Errorf("invalid output size %dx%d", w, h))
	}

	icon.SetTarget(0, 0, float64(w), float64(h))
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, rgba, rgba.Bounds())
	dasher := rasterx.NewDasher(w, h, scanner)
	icon.Draw(dasher, 1.0)

	if err := saveImage(*out, rgba, 0); err != nil {
		return cli.Fail(err)
	}
	fmt.Printf("wrote %s (%dx%d)\n", *out, w, h)
	return cli.ExitOK
}
