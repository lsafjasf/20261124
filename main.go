package main

import (
	"fmt"
	"image/png"
	"os"
)

func main() {
	samples := makeSamples(25)
	printSampleTable(samples)
	printMidGray()
	img := renderChart(samples)
	out := "artifacts/pq_chart.png"
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("\nchart written to %s\n", out)
}

// printSampleTable dumps the curve samples: float pipeline values, the
// single 10-bit quantization, and per-peak 8-bit output with CLIP marks.
func printSampleTable(samples []float64) {
	fmt.Println("curve samples (float64 pipeline, one 10-bit quantization):")
	fmt.Println("     nits |  pq f64  | code10 | roundtrip | 1000nit | 2000nit | 4000nit")
	for _, n := range samples {
		pq := PQEncode(n)
		code := QuantizePQ(pq)
		back := DecodePQ10(code)
		fmt.Printf("%9.3f | %8.5f | %6d | %9.3f |", n, pq, code, back)
		for _, peak := range DisplayPeaks {
			level, clipped := DisplayMap(back, peak)
			if clipped {
				fmt.Print("   CLIP  |")
			} else {
				fmt.Printf(" %7d |", level)
			}
		}
		fmt.Println()
	}
}

// printMidGray shows that the mid-gray code and decoded value are
// identical for every display peak.
func printMidGray() {
	code := EncodePQ10(MidGrayNits)
	back := DecodePQ10(code)
	fmt.Printf("\nmid gray %.2f nit -> pq %.5f -> code %d -> %.4f nit (peak-independent)\n",
		MidGrayNits, PQEncode(MidGrayNits), code, back)
	for _, peak := range DisplayPeaks {
		level, clipped := DisplayMap(back, peak)
		fmt.Printf("  peak %4.0f nit: decoded %.4f nit, 8bit=%d, clipped=%v\n", peak, back, level, clipped)
	}
}
