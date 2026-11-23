package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hdrthumb/compare"
	"hdrthumb/fixtures"
	"hdrthumb/hdr"
)

type output struct {
	Width   int              `json:"width"`
	Height  int              `json:"height"`
	Scenes  []string         `json:"scenes"`
	Presets []string         `json:"presets"`
	Results []compare.Result `json:"results"`
}

func main() {
	width := flag.Int("width", 384, "frame width")
	height := flag.Int("height", 192, "frame height")
	outDir := flag.String("out", "artifacts/comparison", "preview image directory")
	jsonPath := flag.String("json", "artifacts/comparison.json", "summary JSON path")
	flag.Parse()

	if *width <= 0 || *height <= 0 {
		fmt.Fprintln(os.Stderr, "hdrcompare: width and height must be positive")
		os.Exit(2)
	}
	if err := run(*width, *height, *outDir, *jsonPath); err != nil {
		fmt.Fprintln(os.Stderr, "hdrcompare:", err)
		os.Exit(1)
	}
}

func run(width, height int, outDir, jsonPath string) error {
	scenes := fixtures.All(width, height)
	presets := hdr.Presets()
	results := compare.Run(scenes, presets)

	sceneNames := make([]string, len(scenes))
	presetNames := make([]string, len(presets))
	for i, scene := range scenes {
		sceneNames[i] = scene.Name
	}
	for i, preset := range presets {
		presetNames[i] = preset.Name
	}

	for _, scene := range scenes {
		naive := compare.NaiveFrame(scene.Frame)
		panels := []hdr.Frame{naive}
		if err := compare.WritePNG(filepath.Join(outDir, scene.Name+"__naive.png"), naive); err != nil {
			return err
		}
		for _, preset := range presets {
			compressed, _ := hdr.CompressFrame(scene.Frame, preset)
			panels = append(panels, compressed)
			name := scene.Name + "__" + preset.Name + ".png"
			if err := compare.WritePNG(filepath.Join(outDir, name), compressed); err != nil {
				return err
			}
		}
		if err := compare.WriteStrip(filepath.Join(outDir, scene.Name+"__comparison.png"), panels, 4); err != nil {
			return err
		}
	}

	doc := output{
		Width:   width,
		Height:  height,
		Scenes:  sceneNames,
		Presets: presetNames,
		Results: results,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return err
	}

	fmt.Printf("%-13s %-8s %12s %12s %12s %5s %5s %7s %8s\n",
		"scene", "preset", "naive mean°", "fixed mean°", "fixed max°",
		"neg", "over", "unrec", "regions")
	for _, r := range results {
		fmt.Printf("%-13s %-8s %12.4f %12.6f %12.6f %5d %5d %7d %8d\n",
			r.Scene, r.Preset,
			r.Naive.HueShiftMeanDeg,
			r.Compressed.HueShiftMeanDeg,
			r.Compressed.HueShiftMaxDeg,
			r.Report.NegativeRecovered,
			r.Report.HighlightCompressed,
			r.Report.Unrecoverable,
			len(r.Report.Regions))
	}
	fmt.Println()
	fmt.Printf("preview panels: naive clamp, %s\n", strings.Join(presetNames, ", "))
	fmt.Printf("wrote %s and %s\n", outDir, jsonPath)
	return nil
}
