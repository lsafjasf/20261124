// Command colorshift converts an sRGB PNG to a Rec.709 PNG through the
// scene-linear ACEScg working space.
//
// Usage:
//
//	colorshift [-depth 8|16] [-exposure stops] in.png out.png
//
// -exposure applies a photographic exposure compensation in stops
// (linear-domain multiply by 2^stops) inside ACEScg. It is a creative
// control, not a remedy for transfer-function defects.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"

	c "github.com/lsafjasf/20261124/colorxform"
)

func main() {
	depth := flag.Int("depth", 16, "output bit depth: 8 or 16")
	exposure := flag.Float64("exposure", 0, "exposure compensation in stops (linear ACEScg domain)")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: colorshift [-depth 8|16] [-exposure stops] in.png out.png")
		os.Exit(2)
	}

	in, err := os.Open(flag.Arg(0))
	if err != nil {
		fatal(err)
	}
	defer in.Close()
	src, err := png.Decode(in)
	if err != nil {
		fatal(err)
	}

	out, err := c.ConvertImage(src, c.Options{
		ExposureStops: *exposure,
		OutputDepth:   *depth,
		ClampOutput:   true,
	})
	if err != nil {
		fatal(err)
	}

	f, err := os.Create(flag.Arg(1))
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(f, out); err != nil {
		fatal(err)
	}
	fmt.Printf("%s (%d-bit) -> %s (%d-bit) via ACEScg, exposure %+.2f stops\n",
		flag.Arg(0), c.SourceDepth(src), flag.Arg(1), *depth, *exposure)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "colorshift:", err)
	os.Exit(1)
}
