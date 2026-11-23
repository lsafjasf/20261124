package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	fcpxml "fcpxmlmap"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "analyze":
		return runAnalyze(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "roundtrip":
		return runRoundTrip(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runAnalyze(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sequenceID := flags.String("sequence", "", "top sequence ID (default: project sequence)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "analyze requires exactly one FCPXML file")
		return 2
	}

	document, err := fcpxml.Load(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	timeline, err := document.AnalyzeSequence(*sequenceID)
	if err != nil {
		printIssues(stderr, timeline)
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	printIssues(stderr, timeline)
	printTimeline(stdout, timeline)
	return 0
}

func runExport(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sequenceID := flags.String("sequence", "", "top sequence ID (default: project sequence)")
	verify := flags.Bool("verify", true, "verify key cuts after export/re-import")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 2 {
		fmt.Fprintln(stderr, "export requires an input and output FCPXML path")
		return 2
	}
	document, err := fcpxml.Load(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if *verify {
		result, err := document.VerifyCutRoundTrip(*sequenceID)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "verified %d key cut/signature entries\n",
			len(result.BeforeSignature))
	}
	if err := document.ExportFile(flags.Arg(1)); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s\n", flags.Arg(1))
	return 0
}

func runRoundTrip(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("roundtrip", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sequenceID := flags.String("sequence", "", "top sequence ID (default: project sequence)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "roundtrip requires exactly one FCPXML file")
		return 2
	}
	document, err := fcpxml.Load(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	result, err := document.VerifyCutRoundTrip(*sequenceID)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "round trip preserved %d signature entries (%d cuts, %d pictures)\n",
		len(result.BeforeSignature), len(result.After.Cuts), len(result.After.PictureEvents))
	return 0
}

func printTimeline(writer io.Writer, timeline *fcpxml.Timeline) {
	name := timeline.TopSequenceName
	if name == "" {
		name = timeline.TopSequenceID
	}
	fmt.Fprintf(writer, "top sequence: %s (%s)\n", name, timeline.TopSequenceID)
	fmt.Fprintf(writer, "frame duration: %s\n", timeline.FrameDuration.String())
	fmt.Fprintf(writer, "duration: %s [%d frames]\n",
		timeline.Duration.String(), timeline.DurationFrames)

	fmt.Fprintln(writer, "picture events:")
	for _, event := range timeline.PictureEvents {
		fmt.Fprintf(writer, "  frames [%d,%d) time [%s,%s) raw [%s,%s) kind=%s lane=%d sourceStart=%s path=%s\n",
			event.StartFrame, event.EndFrame, event.Start.String(), event.End.String(),
			event.RawStart.String(), event.RawEnd.String(), event.Kind, event.Lane,
			event.SourceStart.String(), event.Path)
	}
	fmt.Fprintln(writer, "transitions:")
	for _, transition := range timeline.Transitions {
		fmt.Fprintf(writer, "  [%d,%d) center=%d at %s name=%q ref=%q",
			transition.StartFrame, transition.EndFrame, transition.CenterFrame,
			transition.Center.String(), transition.Name, transition.Ref)
		if transition.HasCut {
			fmt.Fprintf(writer, " cut=%d at %s", transition.CutFrame, transition.Cut.String())
		}
		fmt.Fprintln(writer)
	}
	fmt.Fprintln(writer, "key cuts:")
	for _, cut := range timeline.Cuts {
		fmt.Fprintf(writer, "  frame=%d time=%s kind=%s\n",
			cut.Frame, cut.Time.String(), cut.Kind)
	}
	fmt.Fprintf(writer, "key frames: %v\n", timeline.KeyFrames())
}

func printIssues(writer io.Writer, timeline *fcpxml.Timeline) {
	if timeline == nil {
		return
	}
	seen := make(map[string]bool)
	allIssues := append([]fcpxml.Issue(nil), timeline.Unsupported...)
	if timeline.Validation != nil {
		allIssues = append(allIssues, timeline.Validation.Issues...)
	}
	for _, issue := range allIssues {
		key := string(issue.Severity) + "|" + issue.Code + "|" + issue.Path + "|" + issue.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		fmt.Fprintf(writer, "%s: [%s] %s: %s\n",
			issue.Severity, issue.Code, issue.Path, issue.Message)
	}
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  fcpxmlmap analyze [-sequence ID] FILE
  fcpxmlmap export  [-sequence ID] [-verify=true] INPUT OUTPUT
  fcpxmlmap roundtrip [-sequence ID] FILE

Maps nested FCPXML picture events to zero-based top-sequence frames, checks
sequence cycles and missing references, and verifies cuts after XML round trip.`)
}
