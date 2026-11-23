// ac3meta inspects AC-3 (Dolby Digital, ATSC A/52) elementary streams
// before program ingest: it dumps per-frame metadata (dialnorm, bsmod,
// DRC, sampling parameters), computes gain targets for metadata edits,
// and locates CRC errors, resyncs and parameter changes by frame number.
// It does not decode PCM and uses only the Go standard library.
package main

import (
	"flag"
	"fmt"
	"os"

	"ac3meta/ac3"
)

func main() {
	target := flag.Float64("target", -24, "target loudness in LKFS for normalization gain")
	newDialnorm := flag.Int("set-dialnorm", -1, "hypothetical new dialnorm (1-31); computes compensation gain")
	verbose := flag.Bool("v", false, "dump every frame")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: ac3meta [flags] <file.ac3>")
		flag.PrintDefaults()
		os.Exit(2)
	}
	fh, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer fh.Close()

	res, err := ac3.Scan(fh)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(res.Frames) == 0 {
		fmt.Println("no AC-3 frames found")
		os.Exit(1)
	}

	f0 := res.Frames[0]
	fmt.Printf("frames: %d   duration: %.3fs\n", len(res.Frames), res.Duration())
	fmt.Printf("sample rate: %d Hz   bit rate: %d kbps   frame size: %d bytes\n",
		f0.SampleRate, f0.BitRate, f0.FrameBytes)
	fmt.Printf("bsid: %d   bsmod: %d (%s)\n", f0.BSID, f0.BSMod, f0.BSModText())
	fmt.Printf("channels: %s   dialnorm: %d (loudness %.0f LKFS)\n",
		f0.ChannelConfig(), f0.Dialnorm, f0.LoudnessLKFS())
	if f0.ComprPresent {
		fmt.Printf("DRC line mode (compr): 0x%02X -> %+.2f dB\n", byte(f0.Compr), f0.ComprGainDB())
	} else {
		fmt.Println("DRC line mode (compr): not present")
	}
	if f0.DynrngSet {
		fmt.Printf("DRC RF mode (dynrng, blk0): 0x%02X -> %+.2f dB\n", byte(f0.Dynrng), f0.DynrngGainDB())
	} else {
		fmt.Println("DRC RF mode (dynrng, blk0): not present")
	}

	gains := ac3.TargetGainDB(res.Frames, *target, *newDialnorm)
	g0 := gains[0]
	fmt.Printf("gain to %.0f LKFS target: %+.2f dB (frame 0)\n", *target, g0.NormalizeGainDB)
	if *newDialnorm >= 0 {
		fmt.Printf("dialnorm %d -> %d: compensation gain %+.2f dB (frame 0)\n",
			g0.Dialnorm, *newDialnorm, g0.DialnormEditGain)
	}

	if *verbose {
		fmt.Println("\nframe  offset    dialnorm  bsmod  acmod  lfe  compr  dynrng  crc1  crc2  norm_dB")
		for i, f := range res.Frames {
			compr, dynrng := "-", "-"
			if f.ComprPresent {
				compr = fmt.Sprintf("%+5.2f", f.ComprGainDB())
			}
			if f.DynrngSet {
				dynrng = fmt.Sprintf("%+5.2f", f.DynrngGainDB())
			}
			fmt.Printf("%5d  %8d  %8d  %5d  %5d  %3t  %5s  %6s  %5t  %5t  %+7.2f\n",
				f.Index, f.Offset, f.Dialnorm, f.BSMod, f.ACMod, f.LFEOn,
				compr, dynrng, f.CRC1OK, f.CRC2OK, gains[i].NormalizeGainDB)
		}
	}

	fmt.Printf("\nevents: %d\n", len(res.Events))
	for _, e := range res.Events {
		fmt.Println(" ", e)
	}
	for _, e := range res.Events {
		if e.Kind == ac3.EventCRCError || e.Kind == ac3.EventTruncated {
			os.Exit(1)
		}
	}
}
