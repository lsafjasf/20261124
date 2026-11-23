package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	truehd "truehdscan"
)

func main() {
	asJSON := flag.Bool("json", false, "emit the full diagnostic report as JSON")
	showFrames := flag.Bool("frames", false, "list every complete frame in text output")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: truehdscan [-json] [-frames] <stream.thd|->\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	name := flag.Arg(0)
	data, err := readInput(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "truehdscan:", err)
		os.Exit(1)
	}
	report := truehd.Scan(data)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintln(os.Stderr, "truehdscan:", err)
			os.Exit(1)
		}
		return
	}
	printReport(name, report, *showFrames)
}

func readInput(name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(name)
}

func printReport(name string, r *truehd.Report, showFrames bool) {
	status := "intact"
	if r.Truncated {
		status = "truncated"
	} else if len(r.Issues) > 0 {
		status = "damaged"
	}
	if len(r.Frames) == 0 {
		status = "unusable"
	}

	fmt.Printf("file: %s\n", name)
	fmt.Printf("size: %d bytes\n", r.FileSize)
	fmt.Printf("status: %s (header=%d payload=%d eof=%d)\n",
		status, r.HeaderErrors, r.PayloadErrors, r.EOFErrors)
	fmt.Printf("playable: %d frame(s), %d byte(s), %d sample(s), %.6f s in %d range(s)\n",
		len(r.Frames), r.PlayableBytes, r.PlayableSamples,
		r.PlayableDurationSeconds, len(r.PlayableRanges))
	for i, pr := range r.PlayableRanges {
		fmt.Printf("  range %d: bytes %s frames=%d..%d samples=%d decoded=%.6f..%.6f s\n",
			i, byteRange(pr.RawStart, pr.RawEnd),
			pr.FirstFrameIndex, pr.FirstFrameIndex+pr.FrameCount-1,
			pr.Samples, pr.DecodedStartSeconds, pr.DecodedEndSeconds)
	}
	if r.UncheckedPayloadFrames > 0 {
		fmt.Printf("payload checks: %d frame(s) had no substream check data\n", r.UncheckedPayloadFrames)
	}

	if len(r.Issues) > 0 {
		fmt.Printf("discarded: %d byte(s) across %d issue(s)\n", r.DiscardedBytes, len(r.Issues))
		for _, issue := range r.Issues {
			extra := ""
			if issue.DeclaredSize > 0 {
				extra = fmt.Sprintf(" declared=%d", issue.DeclaredSize)
			}
			if issue.RecoveredAt > 0 {
				extra += fmt.Sprintf(" recovered_at=%d", issue.RecoveredAt)
			}
			fmt.Printf("  %-13s packet=%d bytes=%s%s: %s\n",
				issue.Kind, issue.PacketIndex, byteRange(issue.RawStart, issue.RawEnd),
				extra, issue.Reason)
		}
	}

	if showFrames {
		for _, frame := range r.Frames {
			fmt.Printf("  frame %d packet=%d bytes=%s major=%t rate=%d samples=%d channels=%d checked=%t\n",
				frame.Index, frame.PacketIndex, byteRange(frame.RawStart, frame.RawEnd),
				frame.MajorSync, frame.SampleRate, frame.Samples, frame.Channels,
				frame.PayloadChecked)
		}
	}
}

func byteRange(start, end int64) string {
	return fmt.Sprintf("[%d..%d) (0x%x..0x%x)", start, end, start, end)
}
