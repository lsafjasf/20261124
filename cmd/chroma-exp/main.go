package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"chromaexp/internal/experiment"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := experiment.DefaultConfig()
	flag.IntVar(&cfg.Width, "width", cfg.Width, "fixture width (use an odd value to test the last partial block)")
	flag.IntVar(&cfg.Height, "height", cfg.Height, "fixture height")
	flag.IntVar(&cfg.BlockW, "block-width", cfg.BlockW, "tile width")
	flag.IntVar(&cfg.BlockH, "block-height", cfg.BlockH, "tile height")
	out := flag.String("out", "artifacts/chroma-halfpixel", "output directory")
	flag.Parse()

	result, err := experiment.Run(cfg)
	if err != nil {
		return err
	}
	if err := experiment.Verify(result); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	if err := writeOutputs(*out, result); err != nil {
		return err
	}
	printResult(result, *out)
	return nil
}

func writeOutputs(dir string, result *experiment.Result) error {
	files := []struct {
		name string
		fn   func(io.Writer) error
	}{
		{"pixels.csv", func(w io.Writer) error { return experiment.WritePixelsCSV(w, result) }},
		{"coordinates.csv", func(w io.Writer) error { return experiment.WriteCoordinatesCSV(w, result) }},
		{"blocks.csv", func(w io.Writer) error { return experiment.WriteBlocksCSV(w, result) }},
		{"report.json", func(w io.Writer) error { return experiment.WriteJSON(w, result) }},
		{"reference.png", func(w io.Writer) error { return experiment.WritePNG(w, result.Reference) }},
		{"wrong.png", func(w io.Writer) error { return experiment.WritePNG(w, result.WrongRGB) }},
		{"fixed.png", func(w io.Writer) error { return experiment.WritePNG(w, result.FixedRGB) }},
		{"diff_amplified.png", func(w io.Writer) error {
			return experiment.WriteDiffPNG(w, experiment.DiffImage(result))
		}},
	}
	for _, file := range files {
		path := filepath.Join(dir, file.name)
		if err := writeFile(path, file.fn); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func writeFile(path string, fn func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func printResult(r *experiment.Result, out string) {
	cfg := r.Config
	fmt.Printf("fixture=%s %dx%d chroma=%dx%d block=%dx%d\n",
		r.Fixture.Name, cfg.Width, cfg.Height, r.Fixture.Cb.W, r.Fixture.Cb.H, cfg.BlockW, cfg.BlockH)
	fmt.Println("\ncoordinate / weight audit:")
	printAxis(r.XRows, cfg.Width)
	fmt.Println("\nblock audit:")
	for _, b := range r.Blocks {
		fmt.Printf("  block %d [%d,%d)-[%d,%d) wrong tiled diff=%g fixed tiled diff=%g wrong-vs-fixed max=%g\n",
			b.Index, b.X0, b.Y0, b.X1, b.Y1,
			b.WrongWholeVsTiled, b.FixedWholeVsTiled, b.WrongVsFixed)
	}
	s := r.Summary
	fmt.Printf(`
summary:
  wrong vs fixed: max |dCb|=%g max |dCr|=%g max sum|dRGB|=%g mean sum|dRGB|=%g
  wrong vs reference: mean sum|dRGB|=%g max=%g
  fixed vs reference: mean sum|dRGB|=%g max=%g
  block seams: wrong=%g fixed=%g block-local-control=%g
  luma delta=%g odd-width sample shift=%g

outputs: %s
`, s.MaxAbsDeltaCb, s.MaxAbsDeltaCr, s.MaxAbsRGB, s.MeanAbsRGB,
		s.WrongMeanRefAbsRGB, s.WrongMaxRefAbsRGB,
		s.FixedMeanRefAbsRGB, s.FixedMaxRefAbsRGB,
		s.WrongTiledMismatch, s.FixedTiledMismatch, s.BlockLocalMismatch,
		s.MaxLumaDelta, s.OddWidthSampleShift, out)
}

func printAxis(rows []experiment.AxisRow, width int) {
	edge := (width - 1) / 2
	for _, row := range rows {
		if row.Position != 0 && row.Position != width-1 &&
			(row.Position < edge-1 || row.Position > edge+2) {
			continue
		}
		fmt.Printf("  x=%02d wrong q=%7.4f luma=%6.2f [%d:%5.3f %d:%5.3f] | fixed q=%7.4f luma=%6.2f [%d:%5.3f %d:%5.3f]\n",
			row.Position,
			row.Wrong.Coordinate, 2*row.Wrong.Coordinate+1,
			row.Wrong.I0, row.Wrong.W0, row.Wrong.I1, row.Wrong.W1,
			row.Fixed.Coordinate, 2*row.Fixed.Coordinate+1,
			row.Fixed.I0, row.Fixed.W0, row.Fixed.I1, row.Fixed.W1)
	}
}
