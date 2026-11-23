// Command inspect runs the frame-accurate SDR/HDR review against two VX1
// files, writes report.json plus the earliest-anomaly frame images.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"vxcheck/internal/inspect"
	"vxcheck/internal/vx"
)

func main() {
	sdrPath := flag.String("sdr", "artifacts/sdr.vx", "SDR VX1 file")
	hdrPath := flag.String("hdr", "artifacts/hdr.vx", "HDR VX1 file")
	outDir := flag.String("out", "artifacts", "output directory")
	flag.Parse()

	sdr := mustOpen(*sdrPath)
	hdr := mustOpen(*hdrPath)

	report := inspect.Run(sdr, hdr)

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*outDir+"/report.json", raw, 0o644); err != nil {
		fail(err)
	}

	anomaly := report.FirstTimeline
	if anomaly == nil {
		anomaly = report.FirstAnomaly
	}
	if anomaly != nil {
		tick := anomaly.SDRTick
		exportAtTick(*outDir, sdr, hdr, tick, "anomaly")
		// For a boundary shift, export the frame inside the SDR boundary but
		// outside the drifted HDR boundary: the subtitle bar is present on
		// only one side.
		if anomaly.Kind == "subtitle-boundary-shift" {
			exportAtTick(*outDir, sdr, hdr, tick+1, "anomaly-boundary")
		}
	}

	// Always export the dropped-frame moment explicitly: the HDR side has no
	// frame at this tick, so the following HDR frame is shown instead.
	for _, fnd := range report.Findings {
		if fnd.Kind == "frame-dropped-in-hdr" {
			exportDrop(*outDir, sdr, hdr, fnd.SDRTick)
			break
		}
	}

	fmt.Printf("findings=%d matched_pairs=%d\n", len(report.Findings), report.Tone.MatchedPairs)
	if report.FirstTimeline != nil {
		f := report.FirstTimeline
		fmt.Printf("first timeline anomaly: %s (%s) at %.4fs — %s\n", f.Kind, f.Severity, f.SDRSec, f.Detail)
	}
	for _, f := range report.HeaderAnomalies {
		fmt.Printf("header anomaly: %s (%s) — %s\n", f.Kind, f.Severity, f.Detail)
	}
}

func exportAtTick(dir string, sdr, hdr *vx.File, tick int64, prefix string) {
	sdrIdx := inspect.FrameAtTick(sdr, tick)
	hdrIdx := inspect.FrameAtTick(hdr, tick)
	if sdrIdx < 0 {
		sdrIdx = inspect.FrameAtOrBefore(sdr, tick)
	}
	if hdrIdx < 0 {
		hdrIdx = inspect.FrameAtOrBefore(hdr, tick)
	}
	writeFramePNG(dir+"/"+prefix+"_sdr.png", sdr, sdrIdx)
	writeFramePNG(dir+"/"+prefix+"_hdr.png", hdr, hdrIdx)
	writePair(dir+"/"+prefix+"_pair.png", sdr.Frames[sdrIdx], hdr.Frames[hdrIdx], sdr.Header.Width, sdr.Header.Height)
}

func exportDrop(dir string, sdr, hdr *vx.File, tick int64) {
	sdrIdx := inspect.FrameAtTick(sdr, tick)
	hdrIdx := inspect.FrameAtOrBefore(hdr, tick) // frame following the gap
	if hdrIdx < 0 {
		hdrIdx = 0
	}
	writeFramePNG(dir+"/drop_sdr.png", sdr, sdrIdx)
	writeFramePNG(dir+"/drop_hdr.png", hdr, hdrIdx)
	writePair(dir+"/drop_pair.png", sdr.Frames[sdrIdx], hdr.Frames[hdrIdx], sdr.Header.Width, sdr.Header.Height)
}

func writePair(path string, sdr, hdr vx.Frame, width, height int) {
	out, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer out.Close()
	if err := inspect.WriteSideBySide(out, sdr, hdr, width, height); err != nil {
		fail(err)
	}
}

func mustOpen(path string) *vx.File {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	vf, err := vx.Read(f)
	if err != nil {
		fail(fmt.Errorf("parse %s: %w", path, err))
	}
	return vf
}

func writeFramePNG(path string, f *vx.File, idx int) {
	out, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer out.Close()
	if err := inspect.WritePNG(out, f.Frames[idx], f.Header.Width, f.Header.Height); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "inspect:", err)
	os.Exit(1)
}
