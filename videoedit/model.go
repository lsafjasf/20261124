package videoedit

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a time.Duration that marshals as milliseconds in JSON so that
// project and change-order files stay human-readable.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Milliseconds())
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var millis int64
	if err := json.Unmarshal(data, &millis); err == nil {
		d.Duration = time.Duration(millis) * time.Millisecond
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("duration must be milliseconds (number) or a Go duration string: %w", err)
	}
	parsed, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", text, err)
	}
	d.Duration = parsed
	return nil
}

// Clip is a single media clip placed on a track.
type Clip struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Start    Duration `json:"start"`
	Length   Duration `json:"length"`
	SourceIn Duration `json:"sourceIn"`
	// Detached marks a child clip whose parent stopped containing it and the
	// user chose to keep it detached instead of clipping or deleting it.
	Detached bool `json:"detached,omitempty"`
}

// End returns the exclusive timeline end of the clip.
func (c Clip) End() time.Duration { return c.Start.Duration + c.Length.Duration }

// Track is a timeline lane. A non-empty ParentID makes it a child track whose
// clips are expected to stay inside the matching parent track clip.
type Track struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	ParentID string `json:"parentId,omitempty"`
	Clips    []Clip `json:"clips"`
}

// Project is the full editing project.
type Project struct {
	ID     string  `json:"id"`
	Name   string  `json:"name,omitempty"`
	Tracks []Track `json:"tracks"`
}

// ClipRef locates a clip inside the project.
type ClipRef struct {
	TrackIndex int
	ClipIndex  int
	Clip       Clip
}

// FindClip returns the reference of the clip with the given ID, or false.
func (p *Project) FindClip(id string) (ClipRef, bool) {
	for ti := range p.Tracks {
		for ci := range p.Tracks[ti].Clips {
			if p.Tracks[ti].Clips[ci].ID == id {
				return ClipRef{TrackIndex: ti, ClipIndex: ci, Clip: p.Tracks[ti].Clips[ci]}, true
			}
		}
	}
	return ClipRef{}, false
}

// DeepClone returns an independent copy of the project.
func (p *Project) DeepClone() *Project {
	data, err := json.Marshal(p)
	if err != nil {
		panic(fmt.Errorf("clone project: %w", err))
	}
	var clone Project
	if err := json.Unmarshal(data, &clone); err != nil {
		panic(fmt.Errorf("clone project: %w", err))
	}
	return &clone
}

// Equal reports whether two projects hold identical data.
func (p *Project) Equal(other *Project) bool {
	left, err := json.Marshal(p)
	if err != nil {
		return false
	}
	right, err := json.Marshal(other)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
