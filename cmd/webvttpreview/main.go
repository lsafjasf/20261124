package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"webvttpreview/webvtt"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "webvttpreview:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("webvttpreview", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "input .vtt file, or - for stdin")
	svgPath := flags.String("svg", "preview.svg", "SVG output path, or - for stdout")
	jsonPath := flags.String("json", "", "optional JSON report path")
	width := flags.Float64("width", 320, "narrow viewport width")
	height := flags.Float64("height", 180, "narrow viewport height")
	fontSize := flags.Float64("font-size", 16, "fixed cue font size")
	lineHeight := flags.Float64("line-height", 20, "cue line height")
	maxLines := flags.Int("max-lines", 2, "per-cue visible line limit")
	at := flags.String("at", "auto", "sample time: auto, WebVTT timestamp, or Go duration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" && flags.NArg() > 0 {
		*input = flags.Arg(0)
	}
	if *input == "" {
		return fmt.Errorf("missing input; use -input file.vtt or pass a file path")
	}

	source, closeInput, err := openInput(*input)
	if err != nil {
		return err
	}
	defer closeInput()
	doc, err := webvtt.Parse(source)
	if err != nil {
		return fmt.Errorf("parse %s: %w", *input, err)
	}

	opts := webvtt.DefaultOptions()
	opts.Viewport.Width = *width
	opts.Viewport.Height = *height
	opts.Viewport.FontSize = *fontSize
	opts.Viewport.LineHeight = *lineHeight
	opts.Viewport.MaxLines = *maxLines
	if strings.EqualFold(*at, "auto") {
		opts.AutoAt = true
	} else {
		sampleAt, err := parseSampleTime(*at)
		if err != nil {
			return fmt.Errorf("invalid -at value: %w", err)
		}
		opts.AutoAt = false
		opts.At = sampleAt
	}
	report, err := webvtt.Analyze(doc, opts)
	if err != nil {
		return err
	}
	svg, err := webvtt.RenderSVG(report)
	if err != nil {
		return err
	}
	if err := writeOutput(*svgPath, svg, stdout); err != nil {
		return err
	}
	if *jsonPath != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := writeOutput(*jsonPath, append(data, '\n'), stdout); err != nil {
			return err
		}
	}

	fmt.Fprintf(stdout, "wrote %s (%d cues, %d issues, sample %s)\n",
		displayPath(*svgPath), len(report.Cues), len(report.Issues),
		webvtt.FormatTimestamp(time.Duration(report.SampleTimeMS)*time.Millisecond))
	if len(report.ActiveCueIDs) > 0 {
		fmt.Fprintf(stdout, "active cue IDs: %s\n", strings.Join(report.ActiveCueIDs, ", "))
	}
	for _, issue := range report.Issues {
		fmt.Fprintf(stdout, "%-16s cue IDs: %s\n", issue.Code, strings.Join(issue.CueIDs, ", "))
	}
	return nil
}

func openInput(path string) (io.Reader, func(), error) {
	if path == "-" {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return file, func() { _ = file.Close() }, nil
}

func writeOutput(path string, data []byte, stdout io.Writer) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func parseSampleTime(value string) (time.Duration, error) {
	if parsed, err := webvtt.ParseTimestamp(value); err == nil {
		return parsed, nil
	}
	return time.ParseDuration(value)
}

func displayPath(path string) string {
	if path == "-" {
		return "stdout"
	}
	return path
}
