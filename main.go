package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("microdvd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	fps := flags.Float64("fps", 0, "source frame rate (required, must be positive)")
	pretty := flags.Bool("pretty", true, "indent JSON output")
	failOnIssues := flags.Bool("fail-on-issues", false, "exit with status 1 when cues have issues")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: microdvd -fps RATE [FILE|-]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 1 {
		flags.Usage()
		return 2
	}
	if *fps <= 0 {
		fmt.Fprintln(stderr, "microdvd: -fps must be greater than zero")
		return 2
	}

	var (
		data []byte
		err  error
	)
	if flags.NArg() == 0 || flags.Arg(0) == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(flags.Arg(0))
	}
	if err != nil {
		fmt.Fprintf(stderr, "microdvd: read input: %v\n", err)
		return 2
	}

	result, err := Convert(data, *fps)
	if err != nil {
		var encodingErr *EncodingError
		if errors.As(err, &encodingErr) {
			fmt.Fprintf(stderr, "microdvd: %v\n", err)
		} else {
			fmt.Fprintf(stderr, "microdvd: convert: %v\n", err)
		}
		return 2
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if *pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(stderr, "microdvd: write output: %v\n", err)
		return 2
	}
	if *failOnIssues && result.IssueCount > 0 {
		return 1
	}
	return 0
}
