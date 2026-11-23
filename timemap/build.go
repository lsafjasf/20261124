package timemap

import (
	"math/big"
)

type leg struct {
	at0, at1   int64 // output ticks, at1 > at0
	src0, src1 int64 // source ticks
}

// Build validates the spec and returns the per-output-frame mapping table.
//
// It rejects invalid specs with ErrNonMonotonic, out-of-bounds source times
// with ErrOutOfBounds and direction changes whose excursion is shorter than
// one source frame with ErrSubFrameLoop.
func Build(s Spec) (*Table, error) {
	if err := s.validateFields(); err != nil {
		return nil, err
	}

	domain, err := buildTickDomain(s)
	if err != nil {
		return nil, err
	}

	kfTicks, err := keyframeTicks(s, domain)
	if err != nil {
		return nil, err
	}

	durationTicks, err := domain.toTicks(s.Duration())
	if err != nil {
		return nil, ErrTickOverflow
	}
	for _, k := range kfTicks {
		if k.src < 0 || k.src > durationTicks {
			return nil, frameError(0, ErrOutOfBounds)
		}
	}

	legs := buildLegs(kfTicks)
	if err := rejectSubFrameLoops(legs, domain, s.SourceRate); err != nil {
		return nil, err
	}

	return renderRows(s, domain, legs, durationTicks)
}

type tickPoint struct {
	at  int64
	src int64
}

func keyframeTicks(s Spec, d tickDomain) ([]tickPoint, error) {
	out := make([]tickPoint, len(s.Keyframes))
	for i, k := range s.Keyframes {
		at, err := d.toTicks(k.At)
		if err != nil {
			return nil, err
		}
		src, err := d.toTicks(k.Source)
		if err != nil {
			return nil, err
		}
		out[i] = tickPoint{at: at, src: src}
	}
	return out, nil
}

func buildLegs(points []tickPoint) []leg {
	legs := make([]leg, len(points)-1)
	for i := 0; i < len(points)-1; i++ {
		legs[i] = leg{
			at0:  points[i].at,
			at1:  points[i+1].at,
			src0: points[i].src,
			src1: points[i+1].src,
		}
	}
	return legs
}

func sign(delta int64) int {
	switch {
	case delta > 0:
		return 1
	case delta < 0:
		return -1
	default:
		return 0
	}
}

// rejectSubFrameLoops rejects interior direction changes whose excursion is
// shorter than one source frame. Such a turn re-reads identical source frames
// without observable progress. Legs touching the start or end of the profile
// are open-ended runs and are never loops.
func rejectSubFrameLoops(legs []leg, d tickDomain, rate *big.Rat) error {
	frameTicks, err := d.toTicks(new(big.Rat).Inv(rate))
	if err != nil {
		return ErrTickOverflow
	}

	last := len(legs) - 1
	for i, lg := range legs {
		delta := lg.src1 - lg.src0
		if delta == 0 {
			// An interior freeze is a zero-excursion turn and must be
			// rejected; freezes at either open end stay legal.
			if i != 0 && i != last {
				return ErrSubFrameLoop
			}
			continue
		}
		prevSig := 0
		if i > 0 {
			prevSig = sign(legs[i-1].src1 - legs[i-1].src0)
		}
		nextSig := 0
		if i < last {
			nextSig = sign(legs[i+1].src1 - legs[i+1].src0)
		}
		curSig := sign(delta)
		turnBefore := prevSig != 0 && prevSig != curSig
		turnAfter := nextSig != 0 && nextSig != curSig
		if turnBefore && turnAfter && abs64(delta) < frameTicks {
			return ErrSubFrameLoop
		}
	}
	return nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
