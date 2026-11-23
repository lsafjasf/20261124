package inspect

import "vxcheck/internal/vx"

// Severity classifies findings.
type Severity string

const (
	SevNormal Severity = "normal-tone" // expected SDR vs HDR color difference
	SevDefect Severity = "defect"      // dropped frame, gap, wrong boundary
	SevMeta   Severity = "metadata-error"
)

// Finding is one review result row, ordered by timeline position.
type Finding struct {
	Kind     string   `json:"kind"`
	Severity Severity `json:"severity"`
	Side     string   `json:"side,omitempty"` // sdr | hdr | both
	SDRTick  int64    `json:"sdr_tick"`
	HDRTick  int64    `json:"hdr_tick"`
	SDRSec   float64  `json:"sdr_seconds"`
	HDRSec   float64  `json:"hdr_seconds"`
	Detail   string   `json:"detail"`
	SDRFrame int      `json:"sdr_frame_index"`
	HDRFrame int      `json:"hdr_frame_index"`
}

// ToneStat summarizes the expected grade difference for matched frames.
type ToneStat struct {
	MatchedPairs      int     `json:"matched_pairs"`
	MeanLumaDeltaMean int     `json:"mean_luma_delta_mean"`
	MaxEdgeDiff       float64 `json:"max_edge_diff"`
	Note              string  `json:"note"`
}

// Report is the complete inspection result.
type Report struct {
	SDR             vx.Header        `json:"sdr_header"`
	HDR             vx.Header        `json:"hdr_header"`
	Findings        []Finding        `json:"findings"`
	Tone            ToneStat         `json:"tone"`
	FirstAnomaly    *Finding         `json:"first_anomaly,omitempty"`
	FirstTimeline   *Finding         `json:"first_timeline_anomaly,omitempty"`
	HeaderAnomalies []Finding        `json:"header_anomalies,omitempty"`
	Streams         [2]StreamSummary `json:"streams"`
}

// StreamSummary is the per-version structural inventory.
type StreamSummary struct {
	Title        string        `json:"title"`
	FrameCount   int           `json:"frame_count"`
	DeclaredNums []uint32      `json:"declared_nums"`
	PTS          []int64       `json:"pts"`
	CutTicks     []int64       `json:"cut_ticks"`
	Subtitles    []vx.Subtitle `json:"subtitles"`
	AudioPackets int           `json:"audio_packets"`
	AudioFirst   uint64        `json:"audio_first_sample"`
	AudioLastEnd uint64        `json:"audio_last_sample_end"`
}

// CutPoint records a declared scene cut.
type CutPoint struct {
	Index int
	Tick  int64
}

// Cuts extracts declared scene cuts in stream order.
func Cuts(f *vx.File) []CutPoint {
	var out []CutPoint
	for _, fr := range f.Frames {
		if fr.Cut {
			out = append(out, CutPoint{Index: fr.Index, Tick: fr.PTS})
		}
	}
	return out
}
