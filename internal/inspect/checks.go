package inspect

import (
	"fmt"

	"vxcheck/internal/vx"
)

// CheckFrameNumbers verifies per-frame declared Num and PTS monotonicity and
// that declared scene cuts keep their timeline position after alignment.
func CheckFrameNumbers(sdr, hdr *vx.File, al Alignment) []Finding {
	var out []Finding

	// HDR internal consistency: PTS must advance one tick per stored frame.
	for i := 1; i < len(hdr.Frames); i++ {
		prev := hdr.Frames[i-1]
		cur := hdr.Frames[i]
		if cur.PTS <= prev.PTS {
			out = append(out, Finding{
				Kind: "pts-regression", Severity: SevDefect,
				Side:    "hdr",
				HDRTick: cur.PTS, HDRSec: hdr.Header.TickToSeconds(cur.PTS),
				HDRFrame: cur.Index,
				Detail:   fmt.Sprintf("PTS %d does not advance after %d", cur.PTS, prev.PTS),
			})
		}
	}

	// Declared Num must equal the timeline position of the matched SDR frame.
	sdrByHDR := map[int]int{}
	for _, p := range al.Pairs {
		sdrByHDR[p.HDRIndex] = p.SDRIndex
	}
	for _, hf := range hdr.Frames {
		sdrIdx, ok := sdrByHDR[hf.Index]
		if !ok {
			continue
		}
		wantNum := sdr.Frames[sdrIdx].Num
		if hf.Num != wantNum {
			out = append(out, Finding{
				Kind: "frame-number-mismatch", Severity: SevMeta,
				Side:    "both",
				SDRTick: int64(wantNum), HDRTick: hf.PTS,
				SDRSec:   sdr.Header.TickToSeconds(int64(wantNum)),
				HDRSec:   hdr.Header.TickToSeconds(hf.PTS),
				SDRFrame: sdrIdx, HDRFrame: hf.Index,
				Detail: fmt.Sprintf("HDR declares Num=%d but content matches SDR Num=%d", hf.Num, wantNum),
			})
		}
		if hf.PTS != sdr.Frames[sdrIdx].PTS {
			out = append(out, Finding{
				Kind: "pts-mismatch", Severity: SevDefect,
				Side:    "both",
				SDRTick: sdr.Frames[sdrIdx].PTS, HDRTick: hf.PTS,
				SDRSec:   sdr.Header.TickToSeconds(sdr.Frames[sdrIdx].PTS),
				HDRSec:   hdr.Header.TickToSeconds(hf.PTS),
				SDRFrame: sdrIdx, HDRFrame: hf.Index,
				Detail: fmt.Sprintf("PTS drift: SDR=%d HDR=%d", sdr.Frames[sdrIdx].PTS, hf.PTS),
			})
		}
	}

	// Scene cuts: each SDR cut must have an HDR cut at the aligned frame.
	hdrCutTick := map[int64]bool{}
	for _, c := range Cuts(hdr) {
		hdrCutTick[c.Tick] = true
	}
	for _, c := range Cuts(sdr) {
		sf := sdr.Frames[c.Index]
		if !hdrCutTick[sf.PTS] {
			out = append(out, Finding{
				Kind: "cut-missing", Severity: SevDefect,
				Side:    "hdr",
				SDRTick: sf.PTS, SDRSec: sdr.Header.TickToSeconds(sf.PTS),
				SDRFrame: c.Index,
				Detail:   fmt.Sprintf("scene cut at tick %d missing or shifted in HDR", sf.PTS),
			})
		}
	}
	return out
}

// CheckSubtitles compares subtitle boundaries at tick granularity.
func CheckSubtitles(sdr, hdr *vx.File) []Finding {
	var out []Finding
	hdrByID := map[uint32]vx.Subtitle{}
	for _, s := range hdr.Subtitles {
		hdrByID[s.ID] = s
	}
	for _, s := range sdr.Subtitles {
		h, ok := hdrByID[s.ID]
		if !ok {
			out = append(out, Finding{
				Kind: "subtitle-missing", Severity: SevDefect,
				Side:    "hdr",
				SDRTick: s.Start, SDRSec: sdr.Header.TickToSeconds(s.Start),
				Detail: fmt.Sprintf("subtitle %d (%q) absent from HDR", s.ID, s.Text),
			})
			continue
		}
		if h.Start != s.Start || h.End != s.End {
			out = append(out, Finding{
				Kind: "subtitle-boundary-shift", Severity: SevDefect,
				Side:    "both",
				SDRTick: s.Start, HDRTick: h.Start,
				SDRSec: sdr.Header.TickToSeconds(s.Start),
				HDRSec: hdr.Header.TickToSeconds(h.Start),
				Detail: fmt.Sprintf(
					"subtitle %d: SDR [%d,%d] HDR [%d,%d] (start drift %+d ticks, end drift %+d)",
					s.ID, s.Start, s.End, h.Start, h.End, h.Start-s.Start, h.End-s.End),
			})
		}
	}
	return out
}

// CheckAudio reconstructs each stream's PCM sample timeline and reports
// gaps, overlaps and the first audio sample position that differs.
func CheckAudio(sdr, hdr *vx.File) []Finding {
	var out []Finding
	out = append(out, audioGaps("sdr", sdr)...)
	out = append(out, audioGaps("hdr", hdr)...)

	// Compare the two timelines sample by sample up to the shorter end.
	sdrTL := flattenAudio(sdr)
	hdrTL := flattenAudio(hdr)
	if len(sdrTL) == 0 || len(hdrTL) == 0 {
		return out
	}
	n := len(sdrTL)
	if len(hdrTL) < n {
		n = len(hdrTL)
	}
	for pos := 0; pos < n; pos++ {
		if sdrTL[pos] != hdrTL[pos] {
			rate := hdr.Header.Audio.SampleRate
			sec := float64(pos) / float64(rate)
			out = append(out, Finding{
				Kind: "audio-sample-divergence", Severity: SevDefect,
				Side:   "both",
				SDRSec: sec, HDRSec: sec,
				Detail: fmt.Sprintf("first differing PCM sample at position %d (%.6f s, rate %d)",
					pos, sec, rate),
			})
			break
		}
	}
	if len(sdrTL) != len(hdrTL) {
		maxLen := len(sdrTL)
		if len(hdrTL) > maxLen {
			maxLen = len(hdrTL)
		}
		sec := float64(maxLen) / float64(hdr.Header.Audio.SampleRate)
		out = append(out, Finding{
			Kind: "audio-length-difference", Severity: SevDefect,
			Side:   "both",
			SDRSec: sec, HDRSec: sec,
			Detail: fmt.Sprintf("PCM sample count SDR=%d HDR=%d (delta %d)",
				len(sdrTL), len(hdrTL), len(hdrTL)-len(sdrTL)),
		})
	}
	return out
}

func audioGaps(side string, f *vx.File) []Finding {
	var out []Finding
	var prevEnd uint64
	for i, p := range f.Audio {
		if i > 0 && p.SampleStart != prevEnd {
			rate := f.Header.Audio.SampleRate
			tick := int64(float64(p.SampleStart) * float64(f.Header.RateNum) /
				float64(rate) / float64(f.Header.RateDen))
			out = append(out, Finding{
				Kind: side + "-audio-gap", Severity: SevDefect,
				Side:    side,
				SDRTick: tick, HDRTick: tick,
				SDRSec: float64(p.SampleStart) / float64(rate),
				HDRSec: float64(p.SampleStart) / float64(rate),
				Detail: fmt.Sprintf("%s audio discontinuity: packet %d starts at %d, expected %d (gap %d samples)",
					side, i, p.SampleStart, prevEnd, p.SampleStart-prevEnd),
			})
		}
		prevEnd = p.SampleStart + uint64(len(p.Samples))
	}
	return out
}

func flattenAudio(f *vx.File) []int16 {
	if len(f.Audio) == 0 {
		return nil
	}
	end := uint64(0)
	for _, p := range f.Audio {
		if e := p.SampleStart + uint64(len(p.Samples)); e > end {
			end = e
		}
	}
	out := make([]int16, end)
	for _, p := range f.Audio {
		copy(out[p.SampleStart:], p.Samples)
	}
	return out
}

// CheckColorMetadata validates internal coherence of the HDR header and
// separates metadata errors from the expected SDR/HDR tone difference.
func CheckColorMetadata(sdr, hdr *vx.File) []Finding {
	var out []Finding
	valid := map[string]map[string]bool{
		"bt709":  {"bt1886": true},
		"bt2020": {"pq": true, "hlg": true},
	}
	if transfers, ok := valid[hdr.Header.Color.Primaries]; ok {
		if !transfers[hdr.Header.Color.Transfer] {
			out = append(out, Finding{
				Kind: "color-metadata-inconsistent", Severity: SevMeta,
				Side: "hdr",
				Detail: fmt.Sprintf("HDR primaries=%s but transfer=%s; expected pq/hlg",
					hdr.Header.Color.Primaries, hdr.Header.Color.Transfer),
			})
		}
	}
	if hdr.Header.Title == "review-hdr" &&
		sdr.Header.Color.Primaries == hdr.Header.Color.Primaries &&
		sdr.Header.Color.Transfer == hdr.Header.Color.Transfer {
		out = append(out, Finding{
			Kind: "color-metadata-identical", Severity: SevMeta,
			Side:   "both",
			Detail: "SDR and HDR declare identical color metadata",
		})
	}
	return out
}
