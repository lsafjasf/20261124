// Command scenecheck applies MPEG-H scene configuration and update segments
// and writes a machine-readable report for every segment.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"mpeghscene/scene"
)

type report struct {
	Updates []scene.Result `json:"updates"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scenecheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("in", "-", "segment JSON/JSONL file, or - for stdin")
	outputPath := flags.String("out", "-", "report JSON file, or - for stdout")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "scenecheck: unexpected positional arguments")
		return 2
	}
	raw, err := readInput(*inputPath, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "scenecheck: %v\n", err)
		return 2
	}
	engine := scene.NewEngine()
	results, processErr := engine.ProcessStream(raw)
	output, err := json.MarshalIndent(report{Updates: results}, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "scenecheck: encode report: %v\n", err)
		return 2
	}
	output = append(output, '\n')
	if err := writeOutput(*outputPath, stdout, output); err != nil {
		fmt.Fprintf(stderr, "scenecheck: %v\n", err)
		return 2
	}
	if processErr != nil {
		fmt.Fprintf(stderr, "scenecheck: one or more updates were rejected: %v\n", processErr)
		if len(results) == 0 {
			return 2
		}
		return 1
	}
	for _, result := range results {
		if len(result.Unresolved) != 0 {
			return 3
		}
	}
	return 0
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path)
}

func writeOutput(path string, stdout io.Writer, data []byte) error {
	if path == "-" {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
