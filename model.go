package timeline

import (
	"errors"
	"fmt"
)

// Timeline is an editable sequence of tracks, clips, and markers.
//
// Clips and markers carry stable IDs because array indexes are not identities:
// two editors may insert different entities at the same index.
type Timeline struct {
	Tracks  []Track  `json:"tracks"`
	Clips   []Clip   `json:"clips"`
	Markers []Marker `json:"markers,omitempty"`
}

type Track struct {
	ID     string         `json:"id"`
	Name   string         `json:"name,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

type Clip struct {
	ID          string         `json:"id"`
	TrackID     string         `json:"trackId"`
	Name        string         `json:"name,omitempty"`
	Start       float64        `json:"start"`
	Duration    float64        `json:"duration"`
	SourceIn    float64        `json:"sourceIn,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
	Transitions []Transition   `json:"transitions,omitempty"`
}

type Transition struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Params map[string]any `json:"params,omitempty"`
}

type Marker struct {
	ID     string         `json:"id"`
	Time   float64        `json:"time"`
	Label  string         `json:"label,omitempty"`
	Color  string         `json:"color,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

// Validate rejects inputs whose stable IDs are not usable for three-way merge.
func (t Timeline) Validate() error {
	trackIDs := make(map[string]struct{}, len(t.Tracks))
	for i, track := range t.Tracks {
		if track.ID == "" {
			return fmt.Errorf("track[%d]: id is required", i)
		}
		if _, exists := trackIDs[track.ID]; exists {
			return fmt.Errorf("track id %q is duplicated", track.ID)
		}
		trackIDs[track.ID] = struct{}{}
	}

	clipIDs := make(map[string]struct{}, len(t.Clips))
	for i, clip := range t.Clips {
		if clip.ID == "" {
			return fmt.Errorf("clip[%d]: id is required", i)
		}
		if _, exists := clipIDs[clip.ID]; exists {
			return fmt.Errorf("clip id %q is duplicated", clip.ID)
		}
		clipIDs[clip.ID] = struct{}{}
		if clip.TrackID == "" {
			return fmt.Errorf("clip %q: trackId is required", clip.ID)
		}
		if _, exists := trackIDs[clip.TrackID]; !exists {
			return fmt.Errorf("clip %q: unknown trackId %q", clip.ID, clip.TrackID)
		}
		if err := validateTransitionIDs(clip); err != nil {
			return err
		}
	}

	markerIDs := make(map[string]struct{}, len(t.Markers))
	for i, marker := range t.Markers {
		if marker.ID == "" {
			return fmt.Errorf("marker[%d]: id is required", i)
		}
		if _, exists := markerIDs[marker.ID]; exists {
			return fmt.Errorf("marker id %q is duplicated", marker.ID)
		}
		markerIDs[marker.ID] = struct{}{}
	}
	return nil
}

func validateTransitionIDs(clip Clip) error {
	seen := make(map[string]struct{}, len(clip.Transitions))
	for i, transition := range clip.Transitions {
		if transition.ID == "" {
			return fmt.Errorf("clip %q transition[%d]: id is required", clip.ID, i)
		}
		if _, exists := seen[transition.ID]; exists {
			return fmt.Errorf("clip %q transition id %q is duplicated", clip.ID, transition.ID)
		}
		seen[transition.ID] = struct{}{}
	}
	return nil
}

// MergeError wraps validation or serialization failures.
type MergeError struct {
	Err error
}

func (e *MergeError) Error() string { return "timeline merge: " + e.Err.Error() }
func (e *MergeError) Unwrap() error { return e.Err }

func wrapMergeError(err error) error {
	if err == nil {
		return nil
	}
	var mergeErr *MergeError
	if errors.As(err, &mergeErr) {
		return err
	}
	return &MergeError{Err: err}
}
