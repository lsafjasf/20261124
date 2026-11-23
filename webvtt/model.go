package webvtt

import (
	"fmt"
	"time"
)

// Rule is a stable identifier for a preview diagnostic.
type Rule string

const (
	RuleTimeSort       Rule = "TIME_SORT"
	RuleRegionClamp    Rule = "REGION_CLAMP"
	RuleLineLimit      Rule = "LINE_LIMIT"
	RuleConflict       Rule = "CONFLICT"
	RuleNegativeMargin Rule = "NEGATIVE_MARGIN"
	RuleMissingStyle   Rule = "MISSING_STYLE"
	RuleMissingRegion  Rule = "MISSING_REGION"
)

// Severity controls how a diagnostic is presented.
type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Cue is one WebVTT subtitle entry.
type Cue struct {
	ID          string
	GeneratedID bool
	Start       time.Duration
	End         time.Duration
	Text        []string
	Settings    CueSettings
	SourceIndex int
	Classes     []string
}

// CueSettings is the subset of WebVTT cue settings used by the layout.
type CueSettings struct {
	Region   string
	Line     string
	Position string
	Size     string
	Align    string
	Vertical string
}

// Point is a percentage anchor inside a region or the viewport.
type Point struct {
	X float64
	Y float64
}

// Region controls where cues are placed on the preview viewport.
type Region struct {
	ID             string
	Width          float64
	Lines          int
	RegionAnchor   Point
	ViewportAnchor Point
	Scroll         string
	Defined        bool
}

// StyleRule is the small CSS subset understood by the preview.
type StyleRule struct {
	Selector string
	Color    string
	Margin   Margin
}

// Margin is stored in CSS pixels. Negative values are diagnosed and clamped.
type Margin struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

// Document is a parsed WebVTT file.
type Document struct {
	Cues    []Cue
	Regions map[string]Region
	Styles  []StyleRule
}

// Issue identifies a rule and every cue ID involved in triggering it.
type Issue struct {
	Code     Rule
	Severity Severity
	CueIDs   []string
	RegionID string
	Message  string
	At       time.Duration
	Duration time.Duration
}

// Rect is a layout rectangle in viewport pixels.
type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

func (r Rect) Right() float64  { return r.X + r.Width }
func (r Rect) Bottom() float64 { return r.Y + r.Height }

// Intersects reports whether two rectangles share positive area.
func (r Rect) Intersects(other Rect) bool {
	return r.X < other.Right() && other.X < r.Right() &&
		r.Y < other.Bottom() && other.Y < r.Bottom()
}

// CueLayout is one cue after sorting, wrapping, and region placement.
type CueLayout struct {
	Cue              Cue
	RegionID         string
	Rect             Rect
	Lines            []string
	LineCount        int
	WrappedLineCount int
	Truncated        bool
	Rules            []Rule
	Style            ResolvedStyle
}

// ResolvedStyle is the effective, non-shrinking cue style.
type ResolvedStyle struct {
	Color  string
	Margin Margin
}

// Viewport describes the narrow preview surface.
type Viewport struct {
	Width      float64
	Height     float64
	FontSize   float64
	LineHeight float64
	Padding    float64
	SafeMargin float64
	MaxLines   int
}

// Options controls deterministic layout and diagnostics.
type Options struct {
	Viewport Viewport
	At       time.Duration
	AutoAt   bool
}

// DefaultOptions is a 320x180 narrow-screen preview.
func DefaultOptions() Options {
	return Options{
		Viewport: Viewport{
			Width:      320,
			Height:     180,
			FontSize:   16,
			LineHeight: 20,
			Padding:    6,
			SafeMargin: 8,
			MaxLines:   2,
		},
		AutoAt: true,
	}
}

// Validate rejects layouts that would hide problems by shrinking text.
func (o Options) Validate() error {
	v := o.Viewport
	if v.Width <= 0 || v.Height <= 0 {
		return fmt.Errorf("viewport dimensions must be positive")
	}
	if v.FontSize < 12 || v.LineHeight < v.FontSize {
		return fmt.Errorf("font size must be at least 12px and line height must not be smaller than font size")
	}
	if v.MaxLines < 1 {
		return fmt.Errorf("max lines must be at least 1")
	}
	if v.Padding < 0 || v.SafeMargin < 0 {
		return fmt.Errorf("padding and safe margin must not be negative")
	}
	return nil
}
