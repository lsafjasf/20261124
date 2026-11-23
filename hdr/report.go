package hdr

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Region is a connected group of pixels with the same unrecoverable reason.
// Width and Height describe the bounding box; Pixels is the actual number of
// pixels in the group (a connected region need not fill its bounding box).
type Region struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Pixels int    `json:"pixels"`
	Reason Reason `json:"reason"`
}

// FrameReport describes one compression pass. Negative recovery and highlight
// compression are reported separately so callers can distinguish wide-gamut
// colors from genuinely lost detail.
type FrameReport struct {
	Width               int      `json:"width"`
	Height              int      `json:"height"`
	Preset              string   `json:"preset"`
	Pixels              int      `json:"pixels"`
	NegativeRecovered   int      `json:"negative_recovered"`
	HighlightCompressed int      `json:"highlight_compressed"`
	Unrecoverable       int      `json:"unrecoverable"`
	NonFinite           int      `json:"non_finite"`
	NegativeLuminance   int      `json:"negative_luminance"`
	HighlightDetailLost int      `json:"highlight_detail_lost"`
	Regions             []Region `json:"regions,omitempty"`
}

func (r FrameReport) HasLoss() bool {
	return r.Unrecoverable > 0
}

// Summary returns a compact human-readable description of the report.
func (r FrameReport) Summary() string {
	return fmt.Sprintf(
		"frame %dx%d preset=%s negative_recovered=%d highlight_compressed=%d unrecoverable=%d regions=%d",
		r.Width, r.Height, r.Preset, r.NegativeRecovered,
		r.HighlightCompressed, r.Unrecoverable, len(r.Regions),
	)
}

// ReportLog writes one JSON object per frame (JSON Lines). A single log can be
// shared by concurrent workers; writes are serialized.
type ReportLog struct {
	mu      sync.Mutex
	encoder *json.Encoder
	next    int
}

func NewReportLog(w io.Writer) *ReportLog {
	return &ReportLog{encoder: json.NewEncoder(w)}
}

// Write appends a frame report. Frame indices start at zero and increase
// monotonically; they are assigned here so every record is traceable even when
// frames are processed concurrently.
func (l *ReportLog) Write(report FrameReport) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.next
	l.next++
	record := struct {
		Frame int `json:"frame"`
		FrameReport
	}{Frame: index, FrameReport: report}
	if err := l.encoder.Encode(record); err != nil {
		return index, err
	}
	return index, nil
}
