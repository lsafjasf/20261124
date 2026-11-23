// Command admprobe is a read-only ADM BWF inspector. It parses the RIFF
// container, the chna track table and the axml ADM metadata, cross-checks
// references and prints a timeline probe. It does not render, decode or
// modify audio.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"admprobe/adm"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("admprobe", flag.ContinueOnError)
	verbose := fs.Bool("v", false, "also print INFO findings")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: admprobe [-v] <file.bwf>")
		return 2
	}
	f, err := adm.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "admprobe:", err)
		return 2
	}
	printInventory(f)
	findings := adm.Check(f)
	timeline, tlFindings := adm.ProbeTimeline(f)
	findings = append(findings, tlFindings...)
	printTimeline(timeline)
	nErr := printFindings(findings, *verbose)
	fmt.Println()
	fmt.Println("note: this is a static metadata probe; no object has been rendered or auditioned.")
	if nErr > 0 {
		return 1
	}
	return 0
}

func printInventory(f *adm.File) {
	fmt.Println("== RIFF inventory ==")
	for _, c := range f.RIFF.Chunks {
		fmt.Printf("  %-4s %8d bytes @%d\n", c.ID, c.Size, c.Offset)
	}
	if f.RIFF.HasFmt {
		fmt.Printf("  fmt: %d ch, %d Hz, %d-bit\n",
			f.RIFF.Format.NumChannels, f.RIFF.Format.SampleRate, f.RIFF.Format.BitsPerSample)
	}
	if d := f.RIFF.Find("data"); d != nil && f.RIFF.Format.ByteRate > 0 {
		sec := float64(d.Size) / float64(f.RIFF.Format.ByteRate)
		fmt.Printf("  audio duration: %.3f s\n", sec)
	}
	if f.Chna != nil {
		fmt.Println("== chna ==")
		for _, e := range f.Chna.Entries {
			fmt.Printf("  track %-3d UID=%-14s trackRef=%-16s packRef=%s\n",
				e.TrackIndex, e.UID, e.TrackRef, orDash(e.PackRef))
		}
	}
}

func printTimeline(tl []adm.ObjectTimeline) {
	fmt.Println("== timeline probe (declared metadata, not rendered) ==")
	for _, o := range tl {
		span := "(no time info)"
		if o.HasTime {
			span = fmt.Sprintf("%s -> %s", o.Start, o.End)
		}
		tracks := "none"
		if len(o.Tracks) > 0 {
			parts := make([]string, len(o.Tracks))
			for i, t := range o.Tracks {
				parts[i] = fmt.Sprint(t)
			}
			tracks = strings.Join(parts, ",")
		}
		fmt.Printf("  object %s (%q) %s  tracks=[%s]\n", o.ObjectID, o.ObjectName, span, tracks)
		for _, ct := range o.Channels {
			fmt.Printf("    channel %s (%q, %s)\n", ct.ChannelID, ct.ChannelName, ct.TypeDef)
			blocks := append([]adm.Block(nil), ct.Blocks...)
			sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Start < blocks[j].Start })
			for _, b := range blocks {
				fmt.Printf("      block %s %s -> %s  pos=%s\n",
					b.ID, b.Start, b.End, formatPos(b.Positions))
			}
		}
	}
}

func formatPos(m map[string]float64) string {
	if len(m) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%g", k, m[k])
	}
	return strings.Join(parts, " ")
}

func printFindings(fs []adm.Finding, verbose bool) int {
	fmt.Println("== findings ==")
	nErr := 0
	shown := 0
	for _, fnd := range fs {
		if fnd.Severity == adm.Error {
			nErr++
		}
		if fnd.Severity == adm.Info && !verbose {
			continue
		}
		shown++
		fmt.Printf("  [%s] %s: %s\n", fnd.Severity, fnd.Code, fnd.Message)
	}
	if shown == 0 {
		fmt.Println("  (none)")
	}
	return nErr
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
