// Package compare measures the old per-channel clamp against the
// hue-preserving compressor and renders side-by-side preview strips.
package compare

import (
	"hdrthumb/fixtures"
	"hdrthumb/hdr"
)

// Result is one fixture/preset comparison.
type Result struct {
	Scene       string          `json:"scene"`
	Description string          `json:"description"`
	Preset      string          `json:"preset"`
	Naive       Metrics         `json:"naive_clamp"`
	Compressed  Metrics         `json:"compressed"`
	Report      hdr.FrameReport `json:"report"`
}

// NaiveFrame applies the old independent channel clamp to every pixel.
func NaiveFrame(src hdr.Frame) hdr.Frame {
	out := hdr.NewFrame(src.Width, src.Height)
	for i, c := range src.Pixels {
		out.Pixels[i] = hdr.NaiveClamp(c)
	}
	return out
}

// Run evaluates every scene with every preset.
func Run(scenes []fixtures.Scene, presets []hdr.Preset) []Result {
	results := make([]Result, 0, len(scenes)*len(presets))
	for _, scene := range scenes {
		naive := NaiveFrame(scene.Frame)
		for _, preset := range presets {
			compressed, report := hdr.CompressFrame(scene.Frame, preset)
			results = append(results, Result{
				Scene:       scene.Name,
				Description: scene.Description,
				Preset:      preset.Name,
				Naive:       Measure(scene.Frame, naive),
				Compressed:  Measure(scene.Frame, compressed),
				Report:      report,
			})
		}
	}
	return results
}
