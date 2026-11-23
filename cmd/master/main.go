package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lsafjasf/20261124/internal/demo"
	"github.com/lsafjasf/20261124/internal/master"
)

func main() {
	var (
		input      = flag.String("input", "", "source manifest JSON (default: generate and use demo)")
		out        = flag.String("out", "dist", "delivery output directory")
		withDemo   = flag.Bool("demo", false, "generate the reproducible three-shot demo source")
		noPreviews = flag.Bool("no-previews", false, "skip SDR/HDR review PNGs")
	)
	flag.Parse()

	source := *input
	if *withDemo || source == "" {
		sourceDir := filepath.Join(*out, "source")
		if _, err := demo.GenerateSource(sourceDir); err != nil {
			fail("generate demo source: %v", err)
		}
		source = filepath.Join(sourceDir, "manifest.json")
	}
	result, err := master.Process(master.ProcessOptions{
		Source: source, OutputDir: *out, WritePreviews: !*noPreviews,
	})
	if err != nil {
		fail("master: %v", err)
	}
	s := result.Report.Summary
	fmt.Printf("wrote %d frames to %s\n", s.Frames, *out)
	fmt.Printf("legal range: %t | middle-gray max delta: %.4f nit\n", s.LegalRangeOK, s.MaxMiddleGrayDeltaNits)
	fmt.Printf("HDR highlight min step: %d codes | SDR highlight min step: %d codes\n",
		s.BestHDRHighlightMinStep, s.SDRHighlightMinStep)
	fmt.Printf("HDR MaxCLL/MaxFALL: %.1f / %.1f nit\n", s.HDRMaxCLLNits, s.HDRMaxFALLNits)
	fmt.Printf("delivery: %s\nvalidation: %s\n",
		filepath.Join(*out, "delivery.json"), filepath.Join(*out, "validation.json"))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
