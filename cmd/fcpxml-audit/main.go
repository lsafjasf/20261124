package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"fcpxmlcompat"
)

type options struct {
	source    string
	candidate string
	out       string
	report    string
	quiet     bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	command := "audit"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		command = args[0]
		args = args[1:]
	}
	switch command {
	case "audit", "repair":
	default:
		fmt.Fprintf(stderr, "unknown command %q (want audit or repair)\n", command)
		return 1
	}

	var opts options
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.source, "source", "", "source/reference FCPXML")
	flags.StringVar(&opts.source, "in", "", "alias for -source")
	flags.StringVar(&opts.candidate, "candidate", "", "converted FCPXML to audit")
	flags.StringVar(&opts.candidate, "converted", "", "alias for -candidate")
	flags.StringVar(&opts.out, "out", "", "write the conservative repaired FCPXML")
	flags.StringVar(&opts.report, "report", "-", "JSON report path, or - for stdout")
	flags.BoolVar(&opts.quiet, "quiet", false, "suppress the one-line terminal summary")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if opts.source == "" {
		fmt.Fprintln(stderr, "-source is required")
		return 1
	}
	if command == "repair" && opts.out == "" {
		fmt.Fprintln(stderr, "repair requires -out")
		return 1
	}

	source, err := fcpxml.ParseDocumentFile(opts.source)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var candidate *fcpxml.Document
	if opts.candidate != "" {
		candidate, err = fcpxml.ParseDocumentFile(opts.candidate)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	var report *fcpxml.Report
	var repair *fcpxml.RepairResult
	if opts.out != "" {
		repairCandidate := candidate
		if repairCandidate == nil {
			repairCandidate = source
		}
		report, repair, err = fcpxml.AuditAndRepair(source, repairCandidate)
	} else {
		report, err = fcpxml.AuditDocuments(source, candidate)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if repair != nil {
		if err := fcpxml.SaveRepair(repair, opts.out); err != nil {
			fmt.Fprintln(stderr, err)
		}
	}
	if err := writeReport(opts.report, stdout, report); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !opts.quiet {
		printSummary(stderr, report)
	}
	if report.Status != "compatible" || report.Summary.LossCount > 0 {
		return 2
	}
	return 0
}

func writeReport(path string, stdout io.Writer, report *fcpxml.Report) error {
	if path == "" || path == "-" {
		return fcpxml.WriteReport(stdout, report)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create report %s: %w", path, err)
	}
	defer file.Close()
	if err := fcpxml.WriteReport(file, report); err != nil {
		return err
	}
	return nil
}

func printSummary(w io.Writer, report *fcpxml.Report) {
	summary := report.Summary
	line := fmt.Sprintf("%s: %d loss(es), %d warning(s)", report.Status, summary.LossCount, summary.WarningCount)
	if summary.EarliestFrame != "" {
		line += fmt.Sprintf(", earliest loss frame %s (%ss)", summary.EarliestFrame, summary.EarliestSeconds)
	}
	if report.Repair != nil {
		line += fmt.Sprintf(", repair=%s", report.Repair.Strategy)
	}
	fmt.Fprintln(w, line)
}
