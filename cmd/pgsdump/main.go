// pgsdump reads a PGS (.sup) stream, rebuilds epoch state in segment
// arrival order (palette, window, object, display, clear) and prints
// exact segment timings plus the first erroneous segment, if any.
package main

import (
	"fmt"
	"os"

	"pgsrebuild/pgs"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: pgsdump <file.sup>\n")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	segs, err := pgs.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	rep := pgs.Rebuild(segs)

	fmt.Println("segments (arrival order):")
	for _, s := range rep.Segments {
		fmt.Printf("  #%02d %-7s pts=%s dts=%s size=%d\n",
			s.Index, pgs.TypeName(s.Type), pgs.FormatPTS(s.PTS), pgs.FormatPTS(s.DTS), s.Size)
	}
	fmt.Println("display sets:")
	for _, set := range rep.Sets {
		fmt.Printf("  set %d: %s -> %s (segments #%d..#%d)\n",
			set.Index, pgs.FormatPTS(set.StartPTS), pgs.FormatPTS(set.EndPTS),
			set.SegmentFirst, set.SegmentLast)
	}
	fmt.Println("epochs:")
	for _, ep := range rep.Epochs {
		fmt.Printf("  epoch %d: %s -> %s palettes=%d windows=%d objects=%d sets=%v\n",
			ep.Index, pgs.FormatPTS(ep.StartPTS), pgs.FormatPTS(ep.EndPTS),
			len(ep.Palettes), len(ep.Windows), len(ep.Objects), ep.Sets)
	}
	if rep.FirstError != nil {
		fmt.Printf("first error: %s\n", rep.FirstError)
		os.Exit(1)
	}
	fmt.Println("first error: none")
}
