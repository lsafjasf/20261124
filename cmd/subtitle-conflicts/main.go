package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"subtitleconflicts/internal/review"
)

func main() {
	var (
		configPath   = flag.String("config", "", "path to review config JSON")
		outPath      = flag.String("out", "", "optional JSON report output path")
		format       = flag.String("format", "text", "stdout format: text or json")
		failOnManual = flag.Bool("fail-on-manual", false, "exit with status 3 when manual review is required")
	)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s -config review.json [options]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()

	if *configPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(os.Stderr, "error: -format must be text or json")
		os.Exit(2)
	}

	cfg, err := review.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	report, err := review.Analyze(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *outPath != "" {
		if err := writeJSONFile(*outPath, report); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}
	if *format == "json" {
		err = review.WriteJSON(os.Stdout, report)
	} else {
		err = review.WriteText(os.Stdout, report)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *failOnManual && report.ManualReviewCount > 0 {
		os.Exit(3)
	}
}

func writeJSONFile(path string, report *review.Report) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if err := review.WriteJSON(file, report); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
