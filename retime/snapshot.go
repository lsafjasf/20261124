package main

import (
	"encoding/json"
	"fmt"
)

// deepCopy round-trips a value through JSON so the undo snapshot is fully
// detached from the working document.
func deepCopy[T any](in T) (T, error) {
	var out T
	data, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}

// ApplyProject runs the remap and returns a manifest containing the mapped
// events plus an untouched snapshot of the original curve and events. The
// original snapshot is the single source of truth for undo.
func ApplyProject(p Project) (Manifest, error) {
	curve, err := BuildCurve(p.SpeedCurve)
	if err != nil {
		return Manifest{}, err
	}
	origCurve, err := deepCopy(p.SpeedCurve)
	if err != nil {
		return Manifest{}, err
	}
	origEvents, err := deepCopy(p.Events)
	if err != nil {
		return Manifest{}, err
	}
	unique, pending, err := Remap(curve, p.Events)
	if err != nil {
		return Manifest{}, err
	}
	if unique == nil {
		unique = []MappedEvent{}
	}
	if pending == nil {
		pending = []MappedEvent{}
	}
	return Manifest{
		Sequence:       p.Sequence,
		NewDuration:    curve.DurationFrames(),
		UniqueMapped:   unique,
		Pending:        pending,
		AppliedCurve:   p.SpeedCurve,
		OriginalCurve:  origCurve,
		OriginalEvents: origEvents,
	}, nil
}

// Undo restores the exact original speed curve and all original events from
// the manifest snapshot.
func Undo(m Manifest) Project {
	return Project{
		Sequence:   m.Sequence,
		SpeedCurve: m.OriginalCurve,
		Events:     m.OriginalEvents,
	}
}

// Confirmation resolves a pending event to a manually chosen start frame and,
// for range events, an end frame.
type Confirmation struct {
	ID       string `json:"id"`
	NewStart int64  `json:"newStart"`
	NewEnd   *int64 `json:"newEnd,omitempty"`
}

// Confirm applies manual choices to pending events and moves them into the
// unique list. A choice outside that event's candidates is rejected.
func Confirm(m Manifest, choices []Confirmation) (Manifest, error) {
	byID := map[string]MappedEvent{}
	for _, pe := range m.Pending {
		byID[pe.ID] = pe
	}
	remaining := map[string]bool{}
	for id := range byID {
		remaining[id] = true
	}
	for _, ch := range choices {
		pe, ok := byID[ch.ID]
		if !ok {
			return m, fmt.Errorf("no pending event with id %q", ch.ID)
		}
		if !hasCandidate(pe, ch.NewStart) {
			return m, fmt.Errorf("event %q: frame %d is not among its candidates", ch.ID, ch.NewStart)
		}
		if ch.NewEnd != nil && !hasCandidate(pe, *ch.NewEnd) {
			return m, fmt.Errorf("event %q: end frame %d is not among its candidates", ch.ID, *ch.NewEnd)
		}
		start := ch.NewStart
		resolved := MappedEvent{
			ID:       pe.ID,
			Kind:     pe.Kind,
			Status:   "unique",
			NewStart: &start,
			NewEnd:   ch.NewEnd,
			Warnings: append(pe.Warnings, "resolved by manual confirmation"),
		}
		m.UniqueMapped = append(m.UniqueMapped, resolved)
		delete(remaining, ch.ID)
	}
	var stillPending []MappedEvent
	for _, pe := range m.Pending {
		if remaining[pe.ID] {
			stillPending = append(stillPending, pe)
		}
	}
	if stillPending == nil {
		stillPending = []MappedEvent{}
	}
	m.Pending = stillPending
	sortEvents(m.UniqueMapped)
	return m, nil
}

func hasCandidate(pe MappedEvent, frame int64) bool {
	for _, c := range pe.Candidates {
		if c.Frame == frame {
			return true
		}
	}
	return false
}
