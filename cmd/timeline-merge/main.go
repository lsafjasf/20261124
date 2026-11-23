package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"timeline"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("timeline-merge", flag.ContinueOnError)
	flags.SetOutput(stderr)
	ancestorPath := flags.String("ancestor", "", "shared ancestor timeline JSON")
	oursPath := flags.String("ours", "", "timeline JSON edited by ours")
	theirsPath := flags.String("theirs", "", "timeline JSON edited by theirs")
	jsonOutput := flags.Bool("json", false, "emit {timeline, conflicts} as JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *ancestorPath == "" || *oursPath == "" || *theirsPath == "" {
		fmt.Fprintln(stderr, "usage: timeline-merge -ancestor BASE.json -ours OURS.json -theirs THEIRS.json [-json]")
		return 2
	}

	ancestor, err := os.ReadFile(*ancestorPath)
	if err != nil {
		return fail(stderr, err)
	}
	ours, err := os.ReadFile(*oursPath)
	if err != nil {
		return fail(stderr, err)
	}
	theirs, err := os.ReadFile(*theirsPath)
	if err != nil {
		return fail(stderr, err)
	}

	result, err := timeline.MergeJSON(ancestor, ours, theirs)
	if err != nil {
		return fail(stderr, err)
	}
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return fail(stderr, err)
		}
	} else if err := result.Format(stdout); err != nil {
		return fail(stderr, err)
	}

	if !result.Clean() {
		return 1
	}
	return 0
}

func fail(stderr io.Writer, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 2
	}
	fmt.Fprintf(stderr, "timeline-merge: %v\n", err)
	return 2
}
