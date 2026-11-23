// Package timemap builds repeatable mappings between the timeline of a
// source clip and the timeline of an output sequence, including variable
// speed, reverse playback and frozen sections.
//
// All time arithmetic is exact rational arithmetic (math/big). When a common
// integer tick domain exists the builder works in integer ticks, so the same
// inputs always produce byte-identical tables across machines and runs.
package timemap

import (
	"errors"
	"math/big"
)

// Common frame rates expressed as exact rational numbers.
var (
	Rate23976 = big.NewRat(24000, 1001) // 23.976 fps
	Rate24    = big.NewRat(24, 1)
	Rate25    = big.NewRat(25, 1)
	Rate2997  = big.NewRat(30000, 1001) // 29.97 fps
	Rate30    = big.NewRat(30, 1)
	Rate48000 = big.NewRat(48000, 1)
	Rate96000 = big.NewRat(96000, 1)
)

// Keyframe fixes where an output time lands on the source timeline.
//
// The slope between two consecutive keyframes is the local speed:
// positive slope plays forward, negative slope plays backward and a zero
// slope freezes the image. Consecutive keyframes may have different slopes,
// which models instantaneous speed changes at the boundary.
type Keyframe struct {
	At     *big.Rat // output time in seconds
	Source *big.Rat // source time in seconds
}

// Spec describes a source clip and the output sequence derived from it.
type Spec struct {
	SourceRate     *big.Rat // source video frames per second
	SourceFrames   int64    // number of source frames, frame i spans [i/rate,(i+1)/rate)
	AudioRate      *big.Rat // source audio samples per second
	OutputRate     *big.Rat // output video frames per second
	OutputFrames   int64    // number of output frames to render
	Keyframes      []Keyframe
	MaxTicksPerSec int64 // optional cap for the integer tick domain; 0 uses default
}

// Rat is a convenience constructor for an exact rational second value.
func Rat(num, den int64) *big.Rat {
	if den == 0 {
		panic("timemap: zero denominator")
	}
	return big.NewRat(num, den)
}

// KF is a convenience constructor for a keyframe.
func KF(atNum, atDen, srcNum, srcDen int64) Keyframe {
	return Keyframe{At: Rat(atNum, atDen), Source: Rat(srcNum, srcDen)}
}

// cloneRat returns an independent copy so callers cannot mutate internals.
func cloneRat(r *big.Rat) *big.Rat {
	return new(big.Rat).Set(r)
}

// positiveRat reports whether r is a finite, strictly positive number.
func positiveRat(r *big.Rat) bool {
	return r != nil && r.Sign() > 0
}

var (
	errBadSourceRate   = errors.New("timemap: SourceRate must be positive")
	errBadOutputRate   = errors.New("timemap: OutputRate must be positive")
	errBadAudioRate    = errors.New("timemap: AudioRate must be positive")
	errBadFrameCount   = errors.New("timemap: frame counts must be positive")
	errKeyframeCount   = errors.New("timemap: at least two keyframes are required")
	errKeyframeOrder   = errors.New("timemap: keyframe output times must be strictly increasing")
	errNilKeyframeTime = errors.New("timemap: keyframe times must not be nil")
)

func (s Spec) validateFields() error {
	if !positiveRat(s.SourceRate) {
		return errBadSourceRate
	}
	if !positiveRat(s.OutputRate) {
		return errBadOutputRate
	}
	if !positiveRat(s.AudioRate) {
		return errBadAudioRate
	}
	if s.SourceFrames <= 0 || s.OutputFrames <= 0 {
		return errBadFrameCount
	}
	if len(s.Keyframes) < 2 {
		return errKeyframeCount
	}
	prev := new(big.Rat)
	for i, k := range s.Keyframes {
		if k.At == nil || k.Source == nil {
			return errNilKeyframeTime
		}
		if i > 0 && k.At.Cmp(prev) <= 0 {
			return errKeyframeOrder
		}
		prev.Set(k.At)
	}
	return nil
}

// Duration returns the source clip duration in seconds: SourceFrames / SourceRate.
func (s Spec) Duration() *big.Rat {
	d := new(big.Rat).SetFrac(big.NewInt(s.SourceFrames), big.NewInt(1))
	return d.Quo(d, s.SourceRate)
}

// tickDomain holds integer scaling factors used for exact interpolation.
type tickDomain struct {
	ticksPerSec int64 // T
}

// defaultTicksPerSec is the LCM of the denominators of the two frame rates,
// the audio rate and every keyframe time, capped to keep tick products small.
const defaultTicksPerSecCap = int64(2_000_000_000)

func lcmBig(a, b *big.Int) *big.Int {
	g := new(big.Int).GCD(nil, nil, a, b)
	q := new(big.Int).Quo(b, g)
	return new(big.Int).Mul(a, q)
}

// buildTickDomain finds T such that every time value used by the spec is an
// integer number of ticks. A second x = n/d becomes x*T ticks, which is
// integral when d divides T. Frame and sample periods (1/rate) require T to
// also be a multiple of each rate numerator.
func buildTickDomain(s Spec) (tickDomain, error) {
	t := big.NewInt(1)
	for _, r := range []*big.Rat{s.SourceRate, s.OutputRate, s.AudioRate} {
		t = lcmBig(t, new(big.Int).Set(r.Num()))
		t = lcmBig(t, new(big.Int).Set(r.Denom()))
	}
	for _, k := range s.Keyframes {
		t = lcmBig(t, new(big.Int).Set(k.At.Denom()))
		t = lcmBig(t, new(big.Int).Set(k.Source.Denom()))
	}
	cap := s.MaxTicksPerSec
	if cap <= 0 {
		cap = defaultTicksPerSecCap
	}
	if !t.IsInt64() || t.Int64() > cap {
		return tickDomain{}, ErrTickOverflow
	}
	return tickDomain{ticksPerSec: t.Int64()}, nil
}

// toTicks converts an exact rational second value to integer ticks and
// verifies the conversion really is integral.
func (d tickDomain) toTicks(r *big.Rat) (int64, error) {
	p := new(big.Int).Mul(r.Num(), big.NewInt(d.ticksPerSec))
	q := new(big.Int).Quo(p, r.Denom())
	rem := new(big.Int).Rem(p, r.Denom())
	if rem.Sign() != 0 || !q.IsInt64() {
		return 0, ErrTickOverflow
	}
	return q.Int64(), nil
}
