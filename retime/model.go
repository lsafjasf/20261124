package main

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Speed is an exact signed playback rate. Value 0 means a freeze (hold frame).
type Speed struct {
	rat *big.Rat
}

func parseSpeed(s string) (Speed, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Speed{}, errors.New("empty speed")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return Speed{}, fmt.Errorf("invalid speed %q", s)
	}
	return Speed{rat: r}, nil
}

func speedInt(n int64) Speed { return Speed{rat: big.NewRat(n, 1)} }

func (s Speed) isZero() bool     { return s.rat == nil || s.rat.Sign() == 0 }
func (s Speed) isNeg() bool      { return s.rat != nil && s.rat.Sign() < 0 }
func (s Speed) String() string   { return s.rat.RatString() }
func (s Speed) ratVal() *big.Rat { return new(big.Rat).Set(s.rat) }

// Segment is one piece of a timeline-origin speed curve.
// Frames and durations are integer frame counts; speeds are exact rationals.
type Segment struct {
	Name     string `json:"name"`
	SrcStart int64  `json:"srcStart"`
	Hold     int64  `json:"hold"`
	Speed    string `json:"speed"`
	speed    Speed
}

// Event is a subtitle or marker anchored at an old (source) frame position.
type Event struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Start int64  `json:"start"`
	End   *int64 `json:"end,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Project is the JSON document handled by the CLI.
type Project struct {
	Sequence      string    `json:"sequence"`
	FrameRate     string    `json:"frameRate,omitempty"`
	SpeedCurve    []Segment `json:"speedCurve"`
	OriginalCurve []Segment `json:"originalCurve,omitempty"`
	Events        []Event   `json:"events,omitempty"`
}

// Candidate is one possible new position for an event that is not uniquely mappable.
type Candidate struct {
	Frame int64  `json:"frame"`
	Why   string `json:"why"`
}

// MappedEvent is the per-event result of a remap.
type MappedEvent struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Status     string      `json:"status"`
	NewStart   *int64      `json:"newStart,omitempty"`
	NewEnd     *int64      `json:"newEnd,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Warnings   []string    `json:"warnings,omitempty"`
}

// Manifest is the full result of applying a speed curve to a project.
type Manifest struct {
	Sequence       string        `json:"sequence"`
	NewDuration    int64         `json:"newDurationFrames"`
	UniqueMapped   []MappedEvent `json:"uniqueMapped"`
	Pending        []MappedEvent `json:"pending"`
	AppliedCurve   []Segment     `json:"appliedCurve"`
	OriginalCurve  []Segment     `json:"originalCurve"`
	OriginalEvents []Event       `json:"originalEvents"`
}
