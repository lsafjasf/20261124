package main

import (
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"subdrift/audit"
	"subdrift/subtitle"
)

const defaultProject = "testdata/reel_500s.json"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "subdrift:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage(os.Stderr)
		return fmt.Errorf("missing command")
	}
	switch args[0] {
	case "audit":
		return runAudit(args[1:])
	case "convert":
		return runConvert(args[1:])
	case "report":
		return runReport(args[1:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil
	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintf(w, `subdrift - subtitle timebase drift diagnostic

Usage:
  subdrift audit   [-project testdata/reel_500s.json] [-json]
  subdrift convert [-project file] [-legacy] [-out file|-]
  subdrift report  [-project file] [-markdown path] [-csv path] [-svg path] [-json path]

The fixed path keeps frame instants exact and quantizes once at SRT milliseconds.
The legacy path reinterprets 24000/1001 frames at nominal 24 fps and round-trips
through a 90 kHz clock and ASS centiseconds.
`)
}

func runAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectPath := fs.String("project", defaultProject, "project JSON")
	asJSON := fs.Bool("json", false, "emit the machine-readable report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := loadReport(*projectPath)
	if err != nil {
		return err
	}
	if *asJSON {
		data, err := report.JSON()
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	printAudit(report)
	return nil
}

func runConvert(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectPath := fs.String("project", defaultProject, "project JSON")
	outPath := fs.String("out", "-", "output SRT path, or - for stdout")
	legacy := fs.Bool("legacy", false, "reproduce the buggy path instead of the fixed path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	project, err := subtitle.LoadProject(*projectPath)
	if err != nil {
		return err
	}
	mode := audit.ReferenceMode(project)
	if *legacy {
		mode = audit.LegacyMode(project)
	}
	text, err := audit.RenderSRTString(project, mode)
	if err != nil {
		return err
	}
	return writeOutput(*outPath, []byte(text))
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	projectPath := fs.String("project", defaultProject, "project JSON")
	markdownPath := fs.String("markdown", "docs/ERROR_CURVE.md", "Markdown report path")
	csvPath := fs.String("csv", "artifacts/error_curve.csv", "curve CSV path")
	svgPath := fs.String("svg", "artifacts/error_curve.svg", "curve SVG path")
	jsonPath := fs.String("json", "artifacts/audit.json", "machine-readable JSON path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := loadReport(*projectPath)
	if err != nil {
		return err
	}
	if *markdownPath != "" {
		if err := writeOutput(*markdownPath, []byte(report.Markdown())); err != nil {
			return err
		}
		fmt.Println("wrote", *markdownPath)
	}
	if *csvPath != "" {
		if err := writeOutput(*csvPath, []byte(report.CSV())); err != nil {
			return err
		}
		fmt.Println("wrote", *csvPath)
	}
	if *svgPath != "" {
		if err := writeOutput(*svgPath, []byte(report.SVG())); err != nil {
			return err
		}
		fmt.Println("wrote", *svgPath)
	}
	if *jsonPath != "" {
		data, err := report.JSON()
		if err != nil {
			return err
		}
		if err := writeOutput(*jsonPath, append(data, '\n')); err != nil {
			return err
		}
		fmt.Println("wrote", *jsonPath)
	}
	return nil
}

func loadReport(path string) (*audit.Report, error) {
	project, err := subtitle.LoadProject(path)
	if err != nil {
		return nil, err
	}
	return audit.Run(project)
}

func printAudit(r *audit.Report) {
	p := r.Project
	fmt.Printf("project: %s\n", r.Project.Name)
	fmt.Printf("rates:   video=%s nominal=%s pts=%s intermediate=%s\n",
		p.VideoRate, p.NominalRate, p.PTSTimebase, p.Intermediate)
	duration := new(big.Rat).Quo(new(big.Rat).SetInt64(p.DurationFrames()), p.VideoRate.Rat())
	fmt.Printf("cues:    %d, duration=%d frames (%s s)\n",
		len(p.Cues), p.DurationFrames(), duration.FloatString(3))
	fmt.Printf("first:   %s\n", r.FirstErrorText())
	if fe := r.FirstError; fe != nil {
		if fe.FirstStage >= 0 {
			s := fe.Stages[fe.FirstStage]
			fmt.Printf("stage:   %s expected %s @ %s, got %d (%.6f ms cumulative)\n",
				s.Name, s.Expected.RatString(), s.To, s.Output.Count,
				ratFloat(s.CumulativeErrorMS))
		}
		fmt.Printf("detail:  legacy=%s fixed=%s error=%+d ms (%s frames)\n",
			subtitle.FormatTimestamp(fe.OutputMS), subtitle.FormatTimestamp(fe.ReferenceMS),
			fe.ErrorMS, fe.ErrorFrames.FloatString(3))
	}

	fmt.Printf("\n%-8s %-6s %8s %13s %11s %11s %8s\n",
		"cue", "edge", "frame", "exact_ms", "legacy_ms", "fixed_ms", "error_ms")
	for _, b := range r.Boundaries {
		fmt.Printf("%-8s %-6s %8d %13s %11d %11d %+8d\n",
			b.CueID, b.Endpoint, b.Frame, b.GroundMS.FloatString(3),
			b.OutputMS, b.ReferenceMS, b.ErrorMS)
	}

	fmt.Printf("\nlayer isolation at frame %d:\n", p.DurationFrames())
	for _, v := range r.Variants {
		fmt.Printf("  %-20s output=%6d ms error=%+5d ms\n", v.Name, v.OutputMS, v.ErrorMS)
	}
	fmt.Printf("max |error|: %d ms; curve points: %d\n", r.MaxAbsErrorMS, len(r.Curve))
}

func ratFloat(r *big.Rat) float64 {
	if r == nil {
		return 0
	}
	v, _ := r.Float64()
	return v
}

func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
