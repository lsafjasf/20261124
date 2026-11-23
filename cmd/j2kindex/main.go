package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"j2kindex/j2k"
)

func main() {
	var pretty bool
	var strict bool
	flag.BoolVar(&pretty, "pretty", true, "indent JSON output")
	flag.BoolVar(&strict, "strict", false, "exit with status 3 when structural errors are present")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"usage: j2kindex [-pretty=true] [-strict] [file|-]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	data, err := readInput(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "j2kindex: %v\n", err)
		os.Exit(2)
	}
	result := j2k.Analyze(data)
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "j2kindex: encode JSON: %v\n", err)
		os.Exit(2)
	}
	if strict {
		for _, issue := range result.Issues {
			if issue.Severity == j2k.SeverityError {
				os.Exit(3)
			}
		}
	}
}

func readInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("empty input")
	}
	return data, nil
}
