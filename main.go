package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	dir := "artifacts"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}

	preview, source := Synthesize()
	oog := outOfGamutMask(source)

	paths := []string{}
	write := func(path string, err error) {
		if err != nil {
			fatal(err)
		}
		paths = append(paths, path)
	}
	write(WritePPM16(dir, "source_rec2020_preview.p6.ppm", preview))
	write(WritePPM8ASCII(dir, "source_rec2020_preview.p3.ppm", preview))
	write(WriteMaskPPM(dir, "out_of_gamut_mask.p6.ppm", imageWidth, imageHeight, oog))

	allStats := make([]*StrategyStats, 0, len(allStrategies))
	metricSets := make([][]PixelMetric, 0, len(allStrategies))
	for _, name := range allStrategies {
		stats, metrics := evaluate(name, source, oog)
		allStats = append(allStats, stats)
		metricSets = append(metricSets, metrics)

		out := newImage(imageWidth, imageHeight)
		flipMark := make([]bool, len(source))
		for i, src := range source {
			res := RunStrategy(name, src)
			out.Pixels[i] = res.Display709
		}
		for _, f := range stats.Flips {
			flipMark[f.Index] = true
		}
		write(WritePPM16(dir, string(name)+".p6.ppm", out))
		write(WritePPM8ASCII(dir, string(name)+".p3.ppm", out))
		write(WriteMaskPPM(dir, string(name)+"_hueflip_mask.p6.ppm", imageWidth, imageHeight, flipMark))
		write(WritePPM16(dir, string(name)+"_deltaE_heatmap.p6.ppm",
			heatmapImage(stats.DeltaE, imageWidth, imageHeight)))
		write(WriteCSV(dir, name, metrics))
	}

	explain := walkthrough()
	reportPath, err := writeReport(dir, allStats, paths, explain)
	if err != nil {
		fatal(err)
	}
	fmt.Println("evidence directory:", dir)
	fmt.Println("report:", reportPath)
	for _, s := range allStats {
		fmt.Printf("%s: maxDE=%.2f hueFlips=%d maxHueRot=%.1fdeg maxLumaAbs=%.4f\n",
			s.Name, s.MaxDeltaE, s.HueFlipPixels, s.MaxHueRotation, s.MaxLumaAbsError)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

// WalkthroughRow is the per-step explanation of one neon pixel.
type WalkthroughRow struct {
	Step string
	RGB  RGB
	Note string
}

func walkthrough() []WalkthroughRow {
	rows := []WalkthroughRow{}
	samples := []struct {
		c    RGB
		note string
	}{
		{neonSwatches[5], "negative green channel, blue-violet highlight"},
		{neonSwatches[0], "neon violet, blue channel above one"},
	}
	for si, sample := range samples {
		src := sample.c
		rows = append(rows, WalkthroughRow{
			Step: fmt.Sprintf("case %d source Rec.2020 linear", si+1), RGB: src, Note: sample.note,
		})
		lifted, neg := liftNegatives(src, luma2020)
		rows = append(rows, WalkthroughRow{
			Step: fmt.Sprintf("case %d 1 negative lift (2020)", si+1), RGB: lifted,
			Note: fmt.Sprintf("shift=%.4f rescale=%.4f luma %.6f->%.6f", neg.Shift, neg.Rescale, neg.LumaBefore, neg.LumaAfter),
		})
		raw := Rec2020To709Linear(lifted)
		rows = append(rows, WalkthroughRow{Step: fmt.Sprintf("case %d 2 matrix to Rec.709", si+1), RGB: raw, Note: "colorimetric NCLT conversion"})
		lifted709, neg709 := liftNegatives(raw, luma709)
		liftNote := fmt.Sprintf("luma kept at %.6f", lifted709.luma(luma709))
		if neg709.HadNegative {
			liftNote = fmt.Sprintf("shift=%.4f, %s", neg709.Shift, liftNote)
		}
		rows = append(rows, WalkthroughRow{Step: fmt.Sprintf("case %d 3 negative lift (709)", si+1), RGB: lifted709, Note: liftNote})
		out, bound := compressToBoundary(lifted709)
		rows = append(rows, WalkthroughRow{
			Step: fmt.Sprintf("case %d 4 boundary search", si+1), RGB: out,
			Note: fmt.Sprintf("alpha=%.6f face=%s iterations=%d lumaErr=%.2e", bound.Alpha, bound.HitFace, bound.Iterations, bound.LumaError),
		})
	}
	return rows
}

func writeReport(dir string, stats []*StrategyStats, paths []string, explain []WalkthroughRow) (string, error) {
	path := filepath.Join(dir, "report.txt")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	fmt.Fprintf(f, "Rec.2020 -> Rec.709 explainable gamut boundary compression\n")
	fmt.Fprintf(f, "image: %dx%d = %d linear Rec.2020 pixels; OOG pixels: %d\n\n",
		imageWidth, imageHeight, imageWidth*imageHeight, stats[0].OutOfGamutInputs)

	fmt.Fprintln(f, "strategy comparison")
	fmt.Fprintln(f, "-------------------")
	ordered := append([]*StrategyStats(nil), stats...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, s := range ordered {
		fmt.Fprintf(f, "%s\n", s.Name)
		fmt.Fprintf(f, "  max DeltaE2000      : %8.3f (pixel %d)\n", s.MaxDeltaE, s.MaxDeltaEIndex)
		fmt.Fprintf(f, "  mean DeltaE2000     : %8.3f\n", s.MeanDeltaE)
		fmt.Fprintf(f, "  hue flip pixels     : %8d (circular hue rotation > 90 deg)\n", s.HueFlipPixels)
		fmt.Fprintf(f, "  max hue rotation    : %8.2f deg\n", s.MaxHueRotation)
		fmt.Fprintf(f, "  max abs luma error  : %8.4f (CIE Y units)\n", s.MaxLumaAbsError)
		fmt.Fprintf(f, "  mean abs luma error : %8.4f\n", s.MeanLumaAbsError)
		fmt.Fprintf(f, "  max relative luma err: %7.4f (pixels with |Y|>=0.2 only)\n", s.MaxLumaRelError)
		fmt.Fprintf(f, "  mean relative luma err: %6.4f\n", s.MeanLumaRelError)
		fmt.Fprintf(f, "  pre-clamp excursions : %8d\n", s.ExcursionPixels)
		for k, fl := range s.Flips {
			if k >= 5 {
				break
			}
			fmt.Fprintf(f, "    flip pixel %d: %.1f deg -> %.1f deg (delta %.1f)\n",
				fl.Index, fl.SourceHue, fl.OutputHue, fl.Difference)
		}
	}

	fmt.Fprintln(f, "\nexplainable walkthrough (neon violet swatch)")
	fmt.Fprintln(f, "--------------------------------------------")
	for _, r := range explain {
		fmt.Fprintf(f, "%-30s R=%8.4f G=%8.4f B=%8.4f  %s\n", r.Step, r.RGB.R, r.RGB.G, r.RGB.B, r.Note)
	}

	fmt.Fprintln(f, "\nevidence files")
	fmt.Fprintln(f, "--------------")
	for _, p := range paths {
		fmt.Fprintln(f, filepath.Base(p))
	}
	fmt.Fprintln(f, filepath.Base(path))
	return path, f.Sync()
}
