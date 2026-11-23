package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"flacseek"
)

const (
	exitOK         = 0
	exitError      = 1
	exitRepairable = 2
	exitCorrupt    = 3
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return exitError
	}
	switch args[0] {
	case "check", "scan":
		return runCheck(args[1:])
	case "repair", "reindex":
		return runRepair(args[1:])
	case "-h", "--help", "help":
		usage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		return exitError
	}
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "write a JSON report")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: flacseek check [-json] FILE.flac")
		return exitError
	}
	report, err := flacseek.InspectFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan %s: %v\n", fs.Arg(0), err)
		return exitError
	}
	if *jsonOut {
		if err := writeJSON(report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
	} else {
		printReport(report)
	}
	switch {
	case report.AudioState != flacseek.AudioOK:
		return exitCorrupt
	case report.IndexState == flacseek.IndexRepairable:
		return exitRepairable
	case report.IndexState == flacseek.IndexUnusable:
		return exitCorrupt
	default:
		return exitOK
	}
}

func runRepair(args []string) int {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "write a JSON result")
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: flacseek repair [-json] INPUT.flac OUTPUT.flac")
		return exitError
	}
	result, err := flacseek.RebuildIndex(fs.Arg(0), fs.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "repair: %v\n", err)
		if errors.Is(err, flacseek.ErrAudioCorrupt) {
			return exitCorrupt
		}
		return exitError
	}
	if *jsonOut {
		if err := writeJSON(result); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitError
		}
		return exitOK
	}
	fmt.Printf("input:  %s\n", result.Input)
	fmt.Printf("output: %s\n", result.Output)
	fmt.Printf("input SHA-256:  %s (unchanged)\n", result.InputSHA256)
	fmt.Printf("output SHA-256: %s\n", result.OutputSHA256)
	fmt.Printf("seek points: %d\n", result.SeekPointCount)
	fmt.Printf("audio: %s, index: %s\n",
		result.Report.AudioState, result.Report.IndexState)
	return exitOK
}

func printReport(report *flacseek.Report) {
	if report.File != "" {
		fmt.Printf("file: %s\n", report.File)
	}
	fmt.Printf("SHA-256: %s\n", report.SHA256)
	fmt.Printf("audio: %s (%d frames, %d samples)\n",
		report.AudioState, report.FrameCount, report.DecodedSamples)
	fmt.Printf("index: %s (%d seek points)\n", report.IndexState, len(report.SeekPoints))
	if report.StreamInfo != nil {
		si := report.StreamInfo
		fmt.Printf("stream: %d Hz, %d ch, %d-bit, total %d samples\n",
			si.SampleRate, si.Channels, si.BitsPerSample, si.TotalSamples)
	}
	if len(report.SeekChecks) > 0 {
		valid := 0
		for _, check := range report.SeekChecks {
			if check.Valid {
				valid++
			}
		}
		fmt.Printf("seek checks: %d/%d valid\n", valid, len(report.SeekChecks))
	}
	if len(report.Issues) > 0 {
		fmt.Println("issues:")
		for _, issue := range report.Issues {
			if issue.Offset > 0 {
				fmt.Printf("  [%s] %s @ %d: %s\n",
					issue.Severity, issue.Code, issue.Offset, issue.Message)
			} else {
				fmt.Printf("  [%s] %s: %s\n", issue.Severity, issue.Code, issue.Message)
			}
		}
	}
	if report.Repairable {
		fmt.Println("repairable: yes (use repair to write a new indexed copy)")
	}
}

func writeJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  flacseek check [-json] FILE.flac
  flacseek repair [-json] INPUT.flac OUTPUT.flac

check exit codes:
  0  audio and seek index are valid
  2  audio is valid but the seek index is stale, duplicate, or missing
  3  audio is corrupt or truncated
  1  usage or I/O error`)
}
