package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"vobsubtext"
)

func main() {
	var (
		idxPath = flag.String("idx", "", "input VobSub .idx file")
		subPath = flag.String("sub", "", "input .sub file (default: same basename as .idx)")
		outDir  = flag.String("out", "vobsub-out", "output directory")
		rounds  = flag.Int("rounds", 3, "repeated ASS boundary/composite verification rounds")
		marker  = flag.String("marker", "[VobSub %04d]", "non-dialogue ASS marker; %04d is the cue index")
		demo    = flag.Bool("demo", false, "generate the built-in .idx/.sub fixture, then convert it")
	)
	flag.Parse()

	if *demo {
		sourceDir := filepath.Join(*outDir, "source")
		generatedIdx, generatedSub, err := vobsub.WriteDemoPairs(sourceDir)
		if err != nil {
			fail(2, "generate demo: %v", err)
		}
		*idxPath, *subPath = generatedIdx, generatedSub
	}
	if *idxPath == "" {
		fail(2, "-idx is required (or use -demo)")
	}

	result, err := vobsub.Convert(vobsub.ConvertOptions{
		IndexPath: *idxPath,
		SubPath:   *subPath,
		OutDir:    *outDir,
		Rounds:    *rounds,
		ASS:       vobsub.ASSOptions{MarkerFormat: *marker},
	})
	if err != nil {
		fail(2, "convert: %v", err)
	}

	fmt.Printf("cues: %d, migrated: %d, held for review: %d\n",
		result.Review.Summary.Total, result.Review.Summary.Migrated, result.Review.Summary.NotMigrated)
	fmt.Printf("text: %s\nreview: %s\n", result.ASSPath, filepath.Join(*outDir, "review.md"))
	if result.Verification == nil || !result.Verification.OK {
		if result.Verification != nil {
			for _, failure := range result.Verification.Failures {
				fmt.Fprintln(os.Stderr, "verify:", failure)
			}
		}
		fail(3, "boundary/composite verification failed")
	}
	fmt.Printf("verify: PASS (%d rounds, %d checks)\n", result.Verification.Rounds, result.Verification.Checks)
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}
