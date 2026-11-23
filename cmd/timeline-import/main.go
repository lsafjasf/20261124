package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"

	"example.com/timelineimport"
)

func main() {
	inputPath := flag.String("in", "testdata/mixed-fps.json", "input timeline JSON")
	roundTripPath := flag.String("roundtrip", "artifacts/roundtrip.json", "normalized round-trip JSON")
	errorsPath := flag.String("errors", "artifacts/cut-errors.csv", "per-cut error CSV")
	summaryPath := flag.String("summary", "artifacts/summary.json", "summary JSON")
	flag.Parse()

	original, err := timeline.ImportFile(*inputPath)
	if err != nil {
		log.Fatalf("import: %v", err)
	}
	roundTripData, err := timeline.RoundTripBytes(original)
	if err != nil {
		log.Fatalf("build round-trip: %v", err)
	}
	reimported, err := timeline.Import(bytes.NewReader(roundTripData))
	if err != nil {
		log.Fatalf("re-import round-trip: %v", err)
	}
	if err := timeline.VerifyRoundTrip(original, reimported); err != nil {
		log.Fatalf("round-trip verification: %v", err)
	}

	rows, err := timeline.BuildCutErrors(original, reimported)
	if err != nil {
		log.Fatalf("build cut errors: %v", err)
	}
	summary := timeline.Summarize(original, rows)
	if summary.MaxRoundTripErrorSeconds != "0.000000000000" {
		log.Fatalf("round-trip is not exact: max error %s s", summary.MaxRoundTripErrorSeconds)
	}
	maxCutError, ok := new(big.Rat).SetString(summary.MaxCutErrorFrames)
	if !ok {
		log.Fatalf("internal error: invalid max cut error %q", summary.MaxCutErrorFrames)
	}
	limit := new(big.Rat).SetFrac64(500001, 1000000)
	if maxCutError.Cmp(limit) > 0 {
		log.Fatalf("cut quantization exceeds half a sequence frame: %s", summary.MaxCutErrorFrames)
	}

	if err := writeRoundTrip(*roundTripPath, roundTripData); err != nil {
		log.Fatal(err)
	}
	if err := writeErrors(*errorsPath, rows); err != nil {
		log.Fatal(err)
	}
	if err := writeSummary(*summaryPath, summary); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("events=%d sequence=%s fps duration=%s s\n", summary.EventCount, summary.SequenceFPS, summary.DurationSeconds)
	fmt.Printf("round-trip=exact max-cut-error=%s frame (%s s)\n", summary.MaxCutErrorFrames, summary.MaxCutErrorSeconds)
	fmt.Printf("legacy-gap0/round-each-end-error=%s frame (%s s)\n", summary.LegacyEndErrorFrames, summary.LegacyEndErrorSeconds)
	fmt.Printf("wrote %s, %s, %s\n", *roundTripPath, *errorsPath, *summaryPath)
}

func writeRoundTrip(path string, data []byte) error {
	return writeFile(path, append(data, '\n'))
}

func writeErrors(path string, rows []timeline.CutError) error {
	var buf bytes.Buffer
	if err := timeline.WriteCutErrorsCSV(&buf, rows); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

func writeSummary(path string, summary timeline.Summary) error {
	var buf bytes.Buffer
	if err := timeline.WriteSummaryJSON(&buf, summary); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
