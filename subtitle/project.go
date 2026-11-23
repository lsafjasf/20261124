// Package subtitle defines the frame-based project model and SRT serialization
// used by the timebase drift diagnostic.
package subtitle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"subdrift/timebase"
)

// Cue is a subtitle event on the editorial timeline. Frame numbers are
// half-open: StartFrame is included and EndFrame is the first frame after the
// cue.
type Cue struct {
	ID         string `json:"id"`
	StartFrame int64  `json:"start_frame"`
	EndFrame   int64  `json:"end_frame"`
	Text       string `json:"text"`
}

// Project describes the exact editorial rate and the lossy delivery path that
// is being audited.
type Project struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	VideoRate    timebase.Rate `json:"video_rate"`
	NominalRate  timebase.Rate `json:"nominal_rate"`
	PTSTimebase  timebase.Rate `json:"pts_timebase"`
	Intermediate string        `json:"intermediate"`
	Cues         []Cue         `json:"cues"`
}

// UseCentiseconds reports whether the legacy path round-trips through an ASS
// centisecond representation before writing SRT.
func (p *Project) UseCentiseconds() bool {
	return p.Intermediate == "ass-cs" || p.Intermediate == "ass" || p.Intermediate == "centiseconds"
}

// Validate checks the invariants required by the diagnostic.
func (p *Project) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("project name is empty")
	}
	for label, rate := range map[string]timebase.Rate{
		"video_rate":   p.VideoRate,
		"nominal_rate": p.NominalRate,
		"pts_timebase": p.PTSTimebase,
	} {
		if !rate.Valid() {
			return fmt.Errorf("%s must be positive", label)
		}
	}
	if len(p.Cues) == 0 {
		return fmt.Errorf("project has no cues")
	}
	for i, cue := range p.Cues {
		if cue.ID == "" {
			return fmt.Errorf("cue %d has no id", i)
		}
		if cue.StartFrame < 0 || cue.EndFrame <= cue.StartFrame {
			return fmt.Errorf("cue %q has invalid frame range [%d,%d)", cue.ID, cue.StartFrame, cue.EndFrame)
		}
	}
	return nil
}

// DurationFrames is the maximum cue end frame.
func (p *Project) DurationFrames() int64 {
	var end int64
	for _, cue := range p.Cues {
		if cue.EndFrame > end {
			end = cue.EndFrame
		}
	}
	return end
}

// ParseProject decodes a project JSON document.
func ParseProject(r io.Reader) (*Project, error) {
	dec := json.NewDecoder(r)
	var p Project
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode project: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// LoadProject reads and validates a project from disk.
func LoadProject(path string) (*Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseProject(f)
}
