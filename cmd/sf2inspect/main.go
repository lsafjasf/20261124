package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"sf2inspect/internal/sf2"
)

func main() {
	jsonOut := flag.Bool("json", false, "emit the full report as JSON")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"usage: %s [--json] file.sf2\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	fh, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal("open %s: %v", flag.Arg(0), err)
	}
	defer fh.Close()

	f, err := sf2.Parse(fh)
	if err != nil {
		fatal("parse: %v", err)
	}
	rep := f.Inspect()

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fatal("encode: %v", err)
		}
	} else {
		printText(rep)
	}

	if countCategory(rep, sf2.SevError) > 0 {
		os.Exit(1)
	}
}

func countCategory(rep *sf2.Report, sev string) int {
	n := 0
	for _, d := range rep.Diagnostics {
		if d.Severity == sev {
			n++
		}
	}
	return n
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sf2inspect: "+format+"\n", args...)
	os.Exit(2)
}
