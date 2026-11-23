package webvtt

import (
	"fmt"
	"time"
)

// Report is the deterministic, serializable result consumed by SVG and JSON.
type Report struct {
	Viewport     ViewportReport `json:"viewport"`
	SampleTimeMS int64          `json:"sampleTimeMs"`
	AutoSample   bool           `json:"autoSample"`
	Cues         []CueReport    `json:"cues"`
	Regions      []RegionReport `json:"regions"`
	ActiveCueIDs []string       `json:"activeCueIds"`
	Issues       []IssueReport  `json:"issues"`
}

type ViewportReport struct {
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	FontSize   float64 `json:"fontSize"`
	LineHeight float64 `json:"lineHeight"`
	Padding    float64 `json:"padding"`
	SafeMargin float64 `json:"safeMargin"`
	MaxLines   int     `json:"maxLines"`
}

type CueReport struct {
	ID               string   `json:"id"`
	GeneratedID      bool     `json:"generatedId"`
	SourceIndex      int      `json:"sourceIndex"`
	StartMS          int64    `json:"startMs"`
	EndMS            int64    `json:"endMs"`
	RegionID         string   `json:"regionId"`
	Rect             Rect     `json:"rect"`
	Lines            []string `json:"lines"`
	LineCount        int      `json:"lineCount"`
	WrappedLineCount int      `json:"wrappedLineCount"`
	Truncated        bool     `json:"truncated"`
	Color            string   `json:"color"`
	Margin           Margin   `json:"margin"`
	Rules            []Rule   `json:"rules,omitempty"`
}

type RegionReport struct {
	ID             string   `json:"id"`
	WidthPercent   float64  `json:"widthPercent"`
	Lines          int      `json:"lines"`
	RegionAnchor   Point    `json:"regionAnchor"`
	ViewportAnchor Point    `json:"viewportAnchor"`
	CueIDs         []string `json:"cueIds"`
	Rect           Rect     `json:"rect"`
	OriginalRect   Rect     `json:"originalRect"`
	Clamped        bool     `json:"clamped"`
}

type IssueReport struct {
	Code       Rule     `json:"code"`
	Severity   Severity `json:"severity"`
	CueIDs     []string `json:"cueIds"`
	RegionID   string   `json:"regionId,omitempty"`
	Message    string   `json:"message"`
	AtMS       int64    `json:"atMs"`
	DurationMS int64    `json:"durationMs,omitempty"`
}

func durationMS(value time.Duration) int64 {
	return value.Milliseconds()
}

// FormatTimestamp renders a compact hh:mm:ss.mmm value.
func FormatTimestamp(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	hours := value / time.Hour
	value -= hours * time.Hour
	minutes := value / time.Minute
	value -= minutes * time.Minute
	seconds := value / time.Second
	value -= seconds * time.Second
	millis := value / time.Millisecond
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}
