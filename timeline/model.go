// Package timeline implements a deterministic, replayable edit log for a
// simple non-linear video project.
package timeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
)

// Project is the complete editor state. Slices are kept in canonical order so
// that two states can be compared field by field without depending on the
// order in which independent operations happened to arrive.
type Project struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Clips   []Clip   `json:"clips"`
	Markers []Marker `json:"markers"`
}

// Clip is a timeline clip. SourceIn and SourceOut are source-media
// milliseconds; Start is the timeline position in milliseconds.
type Clip struct {
	ID        string   `json:"id"`
	Track     string   `json:"track"`
	Start     int64    `json:"start_ms"`
	SourceIn  int64    `json:"source_in_ms"`
	SourceOut int64    `json:"source_out_ms"`
	Speed     float64  `json:"speed"`
	Effects   []Effect `json:"effects"`
}

// Effect is an ordered-by-ID collection of named numeric parameters.
type Effect struct {
	ID     string             `json:"id"`
	Kind   string             `json:"kind"`
	Params map[string]float64 `json:"params"`
}

// Marker is a point annotation on the project timeline.
type Marker struct {
	ID    string `json:"id"`
	Time  int64  `json:"time_ms"`
	Label string `json:"label"`
}

// NewProject creates a project with canonical empty collections.
func NewProject(id, name string) Project {
	return Project{ID: id, Name: name, Clips: []Clip{}, Markers: []Marker{}}
}

// Normalize returns a deep copy in canonical order. The original value is not
// modified.
func (p Project) Normalize() Project {
	out := Project{ID: p.ID, Name: p.Name}
	out.Clips = make([]Clip, len(p.Clips))
	for i, clip := range p.Clips {
		out.Clips[i] = clip.normalized()
	}
	out.Markers = make([]Marker, len(p.Markers))
	copy(out.Markers, p.Markers)
	sort.Slice(out.Clips, func(i, j int) bool { return out.Clips[i].ID < out.Clips[j].ID })
	sort.Slice(out.Markers, func(i, j int) bool {
		if out.Markers[i].Time != out.Markers[j].Time {
			return out.Markers[i].Time < out.Markers[j].Time
		}
		return out.Markers[i].ID < out.Markers[j].ID
	})
	return out
}

func (c Clip) normalized() Clip {
	out := c
	out.Effects = make([]Effect, len(c.Effects))
	for i, effect := range c.Effects {
		out.Effects[i] = effect.normalized()
	}
	sort.Slice(out.Effects, func(i, j int) bool { return out.Effects[i].ID < out.Effects[j].ID })
	return out
}

func (e Effect) normalized() Effect {
	out := Effect{ID: e.ID, Kind: e.Kind, Params: make(map[string]float64, len(e.Params))}
	for key, value := range e.Params {
		out.Params[key] = value
	}
	if out.Params == nil {
		out.Params = map[string]float64{}
	}
	return out
}

// CanonicalJSON serializes the normalized project. encoding/json sorts map
// keys, while Normalize fixes the order of all slices.
func (p Project) CanonicalJSON() ([]byte, error) {
	return json.Marshal(p.Normalize())
}

// StateHash is the SHA-256 digest of CanonicalJSON. It is used after every
// journal record to locate the first divergent replay step.
func (p Project) StateHash() (string, error) {
	data, err := p.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Validate checks the invariants needed for deterministic replay.
func (p Project) Validate() error {
	if p.ID == "" {
		return errors.New("project ID is required")
	}
	clips := make(map[string]struct{}, len(p.Clips))
	markers := make(map[string]struct{}, len(p.Markers))
	for i, clip := range p.Clips {
		if _, exists := clips[clip.ID]; exists {
			return fmt.Errorf("clips[%d]: duplicate clip ID %q", i, clip.ID)
		}
		clips[clip.ID] = struct{}{}
		if err := clip.validate(); err != nil {
			return fmt.Errorf("clips[%d]: %w", i, err)
		}
	}
	for i, marker := range p.Markers {
		if _, exists := markers[marker.ID]; exists {
			return fmt.Errorf("markers[%d]: duplicate marker ID %q", i, marker.ID)
		}
		markers[marker.ID] = struct{}{}
		if err := marker.validate(); err != nil {
			return fmt.Errorf("markers[%d]: %w", i, err)
		}
	}
	return nil
}

func (c Clip) validate() error {
	if c.ID == "" {
		return errors.New("clip ID is required")
	}
	if c.Track == "" {
		return errors.New("clip track is required")
	}
	if c.Start < 0 {
		return errors.New("clip start must be non-negative")
	}
	if c.SourceIn < 0 || c.SourceOut <= c.SourceIn {
		return errors.New("clip source range is invalid")
	}
	if !validSpeed(c.Speed) {
		return errors.New("clip speed must be a finite positive number")
	}
	effects := make(map[string]struct{}, len(c.Effects))
	for i, effect := range c.Effects {
		if _, exists := effects[effect.ID]; exists {
			return fmt.Errorf("effects[%d]: duplicate effect ID %q", i, effect.ID)
		}
		effects[effect.ID] = struct{}{}
		if err := effect.validate(); err != nil {
			return fmt.Errorf("effects[%d]: %w", i, err)
		}
	}
	return nil
}

func (e Effect) validate() error {
	if e.ID == "" {
		return errors.New("effect ID is required")
	}
	if e.Kind == "" {
		return errors.New("effect kind is required")
	}
	for name, value := range e.Params {
		if name == "" {
			return errors.New("effect parameter name is required")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("effect parameter %q is not finite", name)
		}
	}
	return nil
}

func (m Marker) validate() error {
	if m.ID == "" {
		return errors.New("marker ID is required")
	}
	if m.Time < 0 {
		return errors.New("marker time must be non-negative")
	}
	return nil
}

func validSpeed(speed float64) bool {
	return speed > 0 && !math.IsNaN(speed) && !math.IsInf(speed, 0)
}
