// Command pgspreview renders one PGS composition to a PPM image and reports
// the clipped, scaled window coordinates as JSON.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lsafjasf/20261124/pgs"
)

type report struct {
	Input       string           `json:"input"`
	At          string           `json:"at"`
	AtPTS       uint32           `json:"at_pts"`
	PTS         uint32           `json:"pts"`
	Time        string           `json:"time"`
	Source      size             `json:"source"`
	Output      size             `json:"output"`
	PaletteID   uint8            `json:"palette_id"`
	Windows     []pgs.WindowInfo `json:"windows"`
	Objects     []pgs.ObjectInfo `json:"objects"`
	OutputImage string           `json:"output_image"`
}

type size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "pgspreview:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	args = normalizeArgs(args)
	fs := flag.NewFlagSet("pgspreview", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "input .sup file")
	at := fs.String("at", "", "presentation time (seconds, HH:MM:SS.mmm, duration, or Nt ticks)")
	pts := fs.Int64("pts", -1, "presentation timestamp in 90 kHz ticks (overrides -at)")
	width := fs.Int("width", 0, "output width (0 keeps PGS width)")
	height := fs.Int("height", 0, "output height (0 derives from aspect ratio)")
	out := fs.String("out", "", "output PPM path (default preview.ppm)")
	jsonOut := fs.String("json", "-", "JSON report path, - for stdout, or empty to suppress")
	background := fs.String("background", "black", "PPM backdrop: black, white, checker, or #RRGGBB")
	checkerSize := fs.Int("checker-size", 16, "checker square size in output pixels")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: pgspreview [flags] <input.sup> [time] [output.ppm]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if *input == "" && len(rest) > 0 {
		*input, rest = rest[0], rest[1:]
	}
	if *at == "" && len(rest) > 0 {
		*at, rest = rest[0], rest[1:]
	}
	if *out == "" && len(rest) > 0 {
		*out, rest = rest[0], rest[1:]
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(rest, " "))
	}
	if *input == "" {
		fs.Usage()
		return errors.New("missing input .sup file")
	}
	if *out == "" {
		*out = "preview.ppm"
	}

	query := timeQuery{}
	var err error
	if *pts >= 0 {
		if *pts > 0xffffffff {
			return fmt.Errorf("PTS %d exceeds 32 bits", *pts)
		}
		query = timeQuery{usePTS: true, pts: uint32(*pts)}
		query.duration = time.Duration(*pts) * time.Second / pgs.Clock
	} else if *at == "" {
		query = timeQuery{}
	} else {
		query, err = parseTimestamp(*at)
		if err != nil {
			return err
		}
	}
	if query.duration < 0 {
		return fmt.Errorf("negative presentation time %s", query.duration)
	}
	atPTS := query.pts
	if !query.usePTS {
		atPTS = uint32(query.duration / time.Second * pgs.Clock)
		atPTS += uint32((query.duration % time.Second) * pgs.Clock / time.Second)
	}

	bgFunc, err := parseBackground(*background, *checkerSize)
	if err != nil {
		return err
	}
	stream, err := pgs.OpenSUP(*input)
	if err != nil {
		return err
	}
	frame, err := stream.Render(pgs.RenderOptions{
		At: query.duration, PTS: query.pts, UsePTS: query.usePTS,
		Width: *width, Height: *height,
	})
	if err != nil {
		return err
	}

	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	writeErr := pgs.WritePPMFunc(f, frame.Image, bgFunc)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	rep := report{
		Input:       *input,
		At:          query.duration.String(),
		AtPTS:       atPTS,
		PTS:         frame.PTS,
		Time:        frame.Time.String(),
		Source:      size{Width: frame.SourceWidth, Height: frame.SourceHeight},
		Output:      size{Width: frame.Width, Height: frame.Height},
		PaletteID:   frame.PaletteID,
		Windows:     frame.Windows,
		Objects:     frame.Objects,
		OutputImage: *out,
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	switch *jsonOut {
	case "":
	case "-":
		if _, err := stdout.Write(data); err != nil {
			return err
		}
	default:
		if err := os.WriteFile(*jsonOut, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

type timeQuery struct {
	duration time.Duration
	pts      uint32
	usePTS   bool
}

// normalizeArgs permits flags before or after positional arguments. The
// standard flag package stops at the first positional argument.
func normalizeArgs(args []string) []string {
	valueFlags := map[string]bool{
		"input": true, "at": true, "pts": true, "width": true, "height": true,
		"out": true, "json": true, "background": true, "checker-size": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			flags = append(flags, arg)
			continue
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue
		}
		if valueFlags[name] && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func parseTimestamp(s string) (timeQuery, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return timeQuery{}, nil
	}
	if strings.HasSuffix(s, "t") {
		ticks, err := strconv.ParseInt(strings.TrimSuffix(s, "t"), 10, 64)
		if err != nil || ticks < 0 || ticks > 0xffffffff {
			return timeQuery{}, fmt.Errorf("invalid tick timestamp %q", s)
		}
		return timeQuery{
			duration: time.Duration(ticks) * time.Second / pgs.Clock,
			pts:      uint32(ticks),
			usePTS:   true,
		}, nil
	}
	if strings.Count(s, ":") == 2 {
		var hh, mm int
		var ss float64
		if _, err := fmt.Sscanf(s, "%d:%d:%f", &hh, &mm, &ss); err != nil {
			return timeQuery{}, fmt.Errorf("invalid timestamp %q", s)
		}
		if hh < 0 || mm < 0 || mm >= 60 || ss < 0 || ss >= 60 {
			return timeQuery{}, fmt.Errorf("invalid timestamp %q", s)
		}
		return timeQuery{duration: time.Duration(hh)*time.Hour +
			time.Duration(mm)*time.Minute +
			time.Duration(ss*float64(time.Second))}, nil
	}
	if seconds, err := strconv.ParseFloat(s, 64); err == nil {
		if seconds < 0 {
			return timeQuery{}, fmt.Errorf("negative timestamp %q", s)
		}
		return timeQuery{duration: time.Duration(seconds * float64(time.Second))}, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return timeQuery{}, fmt.Errorf("invalid timestamp %q: want seconds, duration, or HH:MM:SS.mmm", s)
	}
	return timeQuery{duration: d}, nil
}

func parseBackground(s string, checkerSize int) (pgs.BackgroundFunc, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "black":
		return pgs.SolidBackground(color.NRGBA{A: 255}), nil
	case "white":
		return pgs.SolidBackground(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), nil
	case "checker":
		return pgs.CheckerBackground(
			color.NRGBA{R: 210, G: 210, B: 210, A: 255},
			color.NRGBA{R: 90, G: 90, B: 90, A: 255},
			checkerSize,
		), nil
	}
	hex := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(hex) != 6 {
		return nil, fmt.Errorf("invalid background %q (want black, white, checker, or #RRGGBB)", s)
	}
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid background %q: %w", s, err)
	}
	return pgs.SolidBackground(color.NRGBA{
		R: uint8(value >> 16),
		G: uint8(value >> 8),
		B: uint8(value),
		A: 255,
	}), nil
}
