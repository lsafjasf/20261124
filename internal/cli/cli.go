// Package cli implements the stlcheck command without calling os.Exit.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/lsafjasf/20261124/stl"
)

// Run executes stlcheck with args (without the program name) and returns an
// exit code: 0 clean, 1 findings, 2 usage or I/O failure.
func Run(program string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(filepath.Base(program), flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOutput := fs.Bool("json", false, "write machine-readable JSON")
	strict := fs.Bool("strict", false, "treat warnings as a failing result")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s [-json] [-strict] file.stl [file.stl ...]\n\n",
			filepath.Base(program))
		fmt.Fprintln(fs.Output(),
			"Validates EBU STL GSI/TTI consistency. The input files are only read.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return 2
	}

	reports := make([]stl.Report, 0, fs.NArg())
	readFailed := false
	for _, path := range fs.Args() {
		doc, err := stl.ParseFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			readFailed = true
			continue
		}
		report := doc.Validate()
		report.File = path
		reports = append(reports, report)
	}

	var err error
	if *jsonOutput {
		err = stl.WriteJSON(stdout, reports)
	} else {
		err = stl.WriteText(stdout, reports)
	}
	if err != nil {
		fmt.Fprintf(stderr, "writing report: %v\n", err)
		return 2
	}
	if readFailed {
		return 2
	}
	for _, report := range reports {
		if report.HasErrors() || (*strict && report.Summary.Warnings > 0) {
			return 1
		}
	}
	return 0
}
