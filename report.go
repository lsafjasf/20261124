package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// StrategyStats summarizes one policy over the whole stress image.
type StrategyStats struct {
	Name             StrategyName
	Pixels           int
	OutOfGamutInputs int
	ExcursionPixels  int
	HueFlipPixels    int
	MaxDeltaE        float64
	MaxDeltaEIndex   int
	MeanDeltaE       float64
	MaxLumaAbsError  float64
	MeanLumaAbsError float64
	MaxLumaRelError  float64
	MeanLumaRelError float64
	MaxHueRotation   float64
	Flips            []HueFlip
	DeltaE           []float64
}

// PixelMetric is one row of the per-pixel CSV evidence.
type PixelMetric struct {
	Index     int
	DeltaE    float64
	LumaAbs   float64
	LumaRel   float64 // zero when not applicable (anchor pixels, near-black)
	RelValid  bool
	HueFlip   bool
	Alpha     float64
	Face      string
	NegShift  float64
	Excursion bool
}

func evaluate(name StrategyName, source []RGB, oog []bool) (*StrategyStats, []PixelMetric) {
	stats := &StrategyStats{Name: name, Pixels: len(source)}
	metrics := make([]PixelMetric, len(source))
	var sumDE, sumLumaAbs, sumLumaRel float64
	for i, src := range source {
		res := RunStrategy(name, src)
		if oog[i] {
			stats.OutOfGamutInputs++
		}
		if res.PreClipExcursion {
			stats.ExcursionPixels++
		}

		srcLab := XYZToLab(src.toXYZ(rgb2xyz2020))
		ref := RGB{clamp01(res.OutputLinear709.R), clamp01(res.OutputLinear709.G), clamp01(res.OutputLinear709.B)}
		outLab := XYZToLab(ref.toXYZ(rgb2xyz709))
		de := DeltaE2000(srcLab, outLab)
		stats.DeltaE = append(stats.DeltaE, de)
		sumDE += de
		if de > stats.MaxDeltaE {
			stats.MaxDeltaE = de
			stats.MaxDeltaEIndex = i
		}

		srcY := src.toXYZ(rgb2xyz2020).Y
		outY := ref.toXYZ(rgb2xyz709).Y
		lumaAbs := math.Abs(outY - srcY)
		lumaRel := 0.0
		relValid := math.Abs(srcY) >= 0.2 && res.Boundary.HitFace != "Y<=0 anchor"
		if relValid {
			lumaRel = lumaAbs / math.Abs(srcY)
			if lumaRel > stats.MaxLumaRelError {
				stats.MaxLumaRelError = lumaRel
			}
			sumLumaRel += lumaRel
		}
		if lumaAbs > stats.MaxLumaAbsError {
			stats.MaxLumaAbsError = lumaAbs
		}
		sumLumaAbs += lumaAbs

		flip := hueFlip(i, srcLab, outLab)
		isFlip := flip != nil
		if isFlip {
			stats.HueFlipPixels++
			stats.Flips = append(stats.Flips, *flip)
		}
		rot := hueRotation(srcLab, outLab)
		if rot > stats.MaxHueRotation {
			stats.MaxHueRotation = rot
		}

		metrics[i] = PixelMetric{
			Index: i, DeltaE: de, LumaAbs: lumaAbs, LumaRel: lumaRel, RelValid: relValid, HueFlip: isFlip,
			Alpha: res.Boundary.Alpha, Face: res.Boundary.HitFace,
			NegShift: res.Neg.Shift, Excursion: res.PreClipExcursion,
		}
	}
	stats.MeanDeltaE = sumDE / float64(len(source))
	stats.MeanLumaAbsError = sumLumaAbs / float64(len(source))
	eligible := 0
	for _, m := range metrics {
		if m.RelValid {
			eligible++
		}
	}
	if eligible > 0 {
		stats.MeanLumaRelError = sumLumaRel / float64(eligible)
	}
	sort.Slice(stats.Flips, func(i, j int) bool { return stats.Flips[i].Difference > stats.Flips[j].Difference })
	return stats, metrics
}

// WriteCSV writes one evidence row per pixel for a strategy.
func WriteCSV(dir string, name StrategyName, rows []PixelMetric) (string, error) {
	path := filepath.Join(dir, string(name)+"_pixels.csv")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := w.WriteString("index,deltaE2000,luma_abs_error,luma_rel_error,hue_flip,boundary_alpha,boundary_face,negative_shift,matrix_excursion\n"); err != nil {
		return "", err
	}
	for _, r := range rows {
		rel := ""
		if r.RelValid {
			rel = fmt.Sprintf("%.8f", r.LumaRel)
		}
		_, err := fmt.Fprintf(w, "%d,%.6f,%.8f,%s,%t,%.8f,%s,%.8f,%t\n",
			r.Index, r.DeltaE, r.LumaAbs, rel, r.HueFlip, r.Alpha, r.Face, r.NegShift, r.Excursion)
		if err != nil {
			return "", err
		}
	}
	return path, w.Flush()
}

// heatmapImage encodes DeltaE2000 as a grayscale ramp scaled to the global max.
// heatmapImage encodes DeltaE2000 as a grayscale ramp. The ramp top is the
// 95th-percentile DeltaE so a few extreme highlight pixels do not compress
// the broad mid-range into near-black.
func heatmapImage(delta []float64, width, height int) *Image {
	sorted := append([]float64(nil), delta...)
	sort.Float64s(sorted)
	cut := sorted[int(float64(len(sorted)-1)*0.95)]
	if cut <= 0 {
		cut = 1
	}
	im := newImage(width, height)
	for i, v := range delta {
		// Black means no difference; bright pixels mark large DeltaE2000.
		g := math.Sqrt(clamp01(v / cut))
		im.Pixels[i] = RGB{g, g, g}
	}
	return im
}
