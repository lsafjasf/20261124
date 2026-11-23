package timeline

import "math/big"

const DocumentVersion = 1

type Document struct {
	Version  int      `json:"version"`
	Sequence Sequence `json:"sequence"`
	Media    []Media  `json:"media"`
	Events   []Event  `json:"events"`
}

type Sequence struct {
	ID  string `json:"id"`
	FPS string `json:"fps"`
}

type Media struct {
	ID     string `json:"id"`
	FPS    string `json:"fps"`
	Frames int64  `json:"frames"`
}

type Event struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Media          string `json:"media,omitempty"`
	MediaInFrame   int64  `json:"mediaInFrame,omitempty"`
	MediaOutFrame  int64  `json:"mediaOutFrame,omitempty"`
	Speed          string `json:"speed,omitempty"`
	SequenceFrames int64  `json:"sequenceFrames,omitempty"`
}

type Source struct {
	ID     string
	FPS    *big.Rat
	Frames int64
}

type TimedEvent struct {
	Definition Event
	Kind       string
	Source     *Source

	Speed *big.Rat

	// Media time is absolute time on the source asset.
	MediaIn       *big.Rat
	MediaOut      *big.Rat
	MediaDuration *big.Rat

	// Source time is clip-local elapsed time, starting at zero at media in.
	SourceIn  *big.Rat
	SourceOut *big.Rat

	// Sequence time includes speed changes and gaps.
	SequenceIn        *big.Rat
	SequenceOut       *big.Rat
	SequenceDuration  *big.Rat
	SequenceInFrames  *big.Rat
	SequenceOutFrames *big.Rat
	SequenceInRound   *big.Int
	SequenceOutRound  *big.Int
}

type Timeline struct {
	SequenceID  string
	SequenceFPS *big.Rat
	Media       map[string]Source
	Events      []TimedEvent
	Duration    *big.Rat
}

func (e TimedEvent) IsClip() bool {
	return e.Kind == "clip"
}

func (e TimedEvent) IsGap() bool {
	return e.Kind == "gap"
}
