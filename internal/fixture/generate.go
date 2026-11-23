package fixture

import (
	"io"
	"math"

	"vxcheck/internal/vx"
)

// WriteSDR writes the reference timeline: every frame in order, clean
// metadata, continuous audio, exact subtitle boundaries.
func WriteSDR(w io.Writer, spec Spec) error {
	header := vx.Header{
		Version: 1,
		Title:   "review-sdr",
		Width:   spec.Width,
		Height:  spec.Height,
		RateNum: spec.RateNum,
		RateDen: spec.RateDen,
		Format:  "yuv444p8",
		Color: vx.Color{
			Primaries: "bt709",
			Transfer:  "bt1886",
			Matrix:    "bt709",
			Range:     "limited",
		},
		Audio: vx.Audio{SampleRate: spec.AudioRate, Channels: 1, Depth: 16},
		Notes: "SDR reference grade of the shared timeline",
	}
	vw, err := vx.NewWriter(w, header)
	if err != nil {
		return err
	}
	if err := writeFrames(vw, spec, "sdr", false); err != nil {
		return err
	}
	if err := writeAudio(vw, spec, false); err != nil {
		return err
	}
	for _, sub := range spec.Subtitles() {
		if err := vw.WriteSubtitle(vx.Subtitle{ID: sub.ID, Start: sub.Start, End: sub.End, Text: sub.Text}); err != nil {
			return err
		}
	}
	return vw.Close()
}

// WriteHDR writes the review HDR version with the documented production
// defects from spec. Geometry and grade follow the shared timeline.
func WriteHDR(w io.Writer, spec Spec) error {
	header := vx.Header{
		Version: 1,
		Title:   "review-hdr",
		Width:   spec.Width,
		Height:  spec.Height,
		RateNum: spec.RateNum,
		RateDen: spec.RateDen,
		Format:  "yuv444p8",
		Color: vx.Color{
			Primaries: "bt2020",
			Transfer:  "pq",
			Matrix:    "bt2020ncl",
			Range:     "limited",
		},
		Audio: vx.Audio{SampleRate: spec.AudioRate, Channels: 1, Depth: 16},
		Notes: "HDR grade of the shared timeline",
	}
	if spec.BadMeta {
		// Wrong metadata: transfer says SDR while primaries say HDR.
		header.Color.Transfer = "bt1886"
	}
	vw, err := vx.NewWriter(w, header)
	if err != nil {
		return err
	}
	if err := writeFrames(vw, spec, "hdr", true); err != nil {
		return err
	}
	if err := writeAudio(vw, spec, true); err != nil {
		return err
	}
	for _, sub := range spec.Subtitles() {
		err := vw.WriteSubtitle(vx.Subtitle{
			ID:    sub.ID,
			Start: sub.Start + spec.SubtitleShiftTick,
			End:   sub.End + spec.SubtitleShiftTick,
			Text:  sub.Text,
		})
		if err != nil {
			return err
		}
	}
	return vw.Close()
}

func writeFrames(vw *vx.Writer, spec Spec, grade string, hdr bool) error {
	cuts := map[int]bool{}
	for _, c := range spec.SceneCuts() {
		cuts[c] = true
	}
	for n := 0; n < spec.Frames; n++ {
		if hdr && n == spec.DropFrame {
			continue // defect 1: dropped frame, no replacement written
		}
		yy, uu, vv, _, _ := rgbToYUV(renderRGB(spec, n), grade)
		frame := vx.Frame{
			Num: uint32(n),
			PTS: int64(n),
			Key: n == 0 || cuts[n],
			Cut: cuts[n],
			Y:   yy,
			U:   uu,
			V:   vv,
		}
		if hdr && n == spec.BadNumFrame {
			frame.Num = spec.BadNumValue // defect 2: corrupt frame-number metadata
		}
		if err := vw.WriteFrame(frame); err != nil {
			return err
		}
	}
	return nil
}

func writeAudio(vw *vx.Writer, spec Spec, hdr bool) error {
	perFrame := spec.SampleRatePerFrame()
	for n := 0; n < spec.Frames; n++ {
		if hdr && n == spec.AudioGapFrame {
			continue // defect 4: one frame's worth of PCM is missing
		}
		// Sample positions stay anchored to the real audio timeline, so the
		// missing block leaves a measurable gap instead of being renumbered.
		start := uint64(n * perFrame)
		samples := make([]int16, perFrame)
		for i := range samples {
			pos := int(start) + i
			t := float64(pos) / float64(spec.AudioRate)
			tone := math.Sin(2*math.Pi*440*t) * 0.25
			samples[i] = int16(tone * 32767)
		}
		if err := vw.WriteAudio(vx.AudioPacket{SampleStart: start, Samples: samples}); err != nil {
			return err
		}
	}
	return nil
}
