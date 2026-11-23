package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"tagfix"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "inspect":
		err = runInspect(os.Args[2:])
	case "apply":
		err = runApply(os.Args[2:])
	case "selftest":
		err = runSelfTest(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tagfix:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  tagfix inspect [-track N] FILE
  tagfix apply [-track N] [-full-range] -primaries N -transfer N -matrix N [-report FILE] FILE
  tagfix selftest [-dir DIR] [-report FILE]
`)
}

func runInspect(args []string) error {
	flags := flag.NewFlagSet("inspect", flag.ContinueOnError)
	track := flags.Int("track", 0, "zero-based AVC video track")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("inspect requires exactly one FILE")
	}
	inspection, err := tagfix.InspectFileTrack(flags.Arg(0), *track)
	if err != nil {
		return err
	}
	return writeJSON(os.Stdout, inspection)
}

func runApply(args []string) error {
	flags := flag.NewFlagSet("apply", flag.ContinueOnError)
	track := flags.Int("track", 0, "zero-based AVC video track")
	primaries := flags.Uint("primaries", 0, "colour_primaries code")
	transfer := flags.Uint("transfer", 0, "transfer_characteristics code")
	matrix := flags.Uint("matrix", 0, "matrix_coefficients code")
	fullRange := flags.Bool("full-range", false, "set video_full_range_flag")
	reportPath := flags.String("report", "", "also write the JSON report here")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("apply requires exactly one FILE")
	}
	if *primaries > 255 || *transfer > 255 || *matrix > 255 {
		return errors.New("tag codes must fit in one byte")
	}
	tags := tagfix.ColorTags{
		Primaries: uint8(*primaries),
		Transfer:  uint8(*transfer),
		Matrix:    uint8(*matrix),
		FullRange: *fullRange,
	}
	report, applyErr := tagfix.ApplyFile(flags.Arg(0), tags, tagfix.WithTrack(*track))
	if report != nil {
		if err := writeReport(report, *reportPath); err != nil {
			return err
		}
	}
	return applyErr
}

func writeReport(report any, path string) error {
	if path == "" {
		return writeJSON(os.Stdout, report)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func writeJSON(writer *os.File, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
