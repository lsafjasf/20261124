package timemap

import (
	"math/big"
)

// FrameMapping is one row of the repeatable mapping table.
//
// OutputFrame is the frame index in the output sequence. SourceFrame is the
// source frame whose image covers the mapped source time, i.e.
// floor(sourceTime * SourceRate). AudioSample is the first audio sample at or
// before the mapped source time, i.e. floor(sourceTime * AudioRate).
// SourceTime and AudioPosition keep the exact unrounded positions.
type FrameMapping struct {
	OutputFrame   int64
	SourceFrame   int64
	AudioSample   int64
	SourceTime    *big.Rat // exact source time in seconds
	OutputTime    *big.Rat // exact output time in seconds
	AudioPosition *big.Rat // exact audio sample index (may be fractional)
}

// Table is the full, deterministic mapping for one spec.
type Table struct {
	Rows []FrameMapping
	spec Spec
}

// Spec returns an independent copy of the spec the table was built from.
func (t *Table) Spec() Spec {
	out := Spec{
		SourceRate:     cloneRat(t.spec.SourceRate),
		SourceFrames:   t.spec.SourceFrames,
		AudioRate:      cloneRat(t.spec.AudioRate),
		OutputRate:     cloneRat(t.spec.OutputRate),
		OutputFrames:   t.spec.OutputFrames,
		MaxTicksPerSec: t.spec.MaxTicksPerSec,
	}
	out.Keyframes = make([]Keyframe, len(t.spec.Keyframes))
	for i, k := range t.spec.Keyframes {
		out.Keyframes[i] = Keyframe{At: cloneRat(k.At), Source: cloneRat(k.Source)}
	}
	return out
}

// Row returns the mapping for output frame n.
func (t *Table) Row(n int64) (FrameMapping, error) {
	if n < 0 || n >= int64(len(t.Rows)) {
		return FrameMapping{}, ErrFrameNotFound
	}
	row := t.Rows[n]
	row.SourceTime = cloneRat(row.SourceTime)
	row.OutputTime = cloneRat(row.OutputTime)
	row.AudioPosition = cloneRat(row.AudioPosition)
	return row, nil
}

// SourceFrame returns the source frame number for output frame n.
func (t *Table) SourceFrame(n int64) (int64, error) {
	row, err := t.Row(n)
	if err != nil {
		return 0, err
	}
	return row.SourceFrame, nil
}

// AudioSample returns the integer audio sample position for output frame n.
func (t *Table) AudioSample(n int64) (int64, error) {
	row, err := t.Row(n)
	if err != nil {
		return 0, err
	}
	return row.AudioSample, nil
}

// Len returns the number of output frames in the table.
func (t *Table) Len() int { return len(t.Rows) }
