package main

import "fmt"

// Result is the complete conversion output for one input.
type Result struct {
	FPS        float64 `json:"fps"`
	CueCount   int     `json:"cue_count"`
	IssueCount int     `json:"issue_count"`
	Cues       []Cue   `json:"cues"`
	Issues     []Issue `json:"issues"`
}

// Cue is one MicroDVD subtitle entry.
type Cue struct {
	Number      int        `json:"number"`
	Line        int        `json:"line"`
	StartFrame  int64      `json:"start_frame"`
	EndFrame    int64      `json:"end_frame"`
	StartMS     int64      `json:"start_ms"`
	EndMS       int64      `json:"end_ms"`
	DurationMS  int64      `json:"duration_ms"`
	StylePrefix string     `json:"style_prefix,omitempty"`
	StyleTags   []StyleTag `json:"style_tags,omitempty"`
	RawText     string     `json:"raw_text"`
	Text        string     `json:"text"`
	RawLine     string     `json:"raw_line"`
	Issues      []Issue    `json:"issues,omitempty"`
}

// StyleTag is a parsed leading MicroDVD style directive, such as {Y:i}.
type StyleTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Raw   string `json:"raw"`
}

// Issue describes a recoverable input or timing problem.
type Issue struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Line       int    `json:"line,omitempty"`
	Cue        int    `json:"cue,omitempty"`
	RelatedCue int    `json:"related_cue,omitempty"`
}

// EncodingError reports a byte offset that is not valid UTF-8.
type EncodingError struct {
	Offset int
	Line   int
}

func (e *EncodingError) Error() string {
	return fmt.Sprintf("invalid UTF-8 encoding at byte offset %d (line %d)", e.Offset, e.Line)
}
