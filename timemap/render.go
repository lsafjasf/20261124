package timemap

import (
	"math/big"
)

func mul64(a, b int64) (int64, bool) {
	p := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	if !p.IsInt64() {
		return 0, false
	}
	return p.Int64(), true
}

// renderRows evaluates the piecewise-linear mapping at every output frame and
// enforces row-level bounds and monotonicity, so no invalid row can reach the
// returned table.
func renderRows(s Spec, d tickDomain, legs []leg, durationTicks int64) (*Table, error) {
	outPeriod, err := d.toTicks(new(big.Rat).Inv(s.OutputRate))
	if err != nil {
		return nil, ErrTickOverflow
	}
	framePeriod, err := d.toTicks(new(big.Rat).Inv(s.SourceRate))
	if err != nil {
		return nil, ErrTickOverflow
	}

	rows := make([]FrameMapping, 0, s.OutputFrames)
	legIdx := 0
	firstAt, lastAt := legs[0].at0, legs[len(legs)-1].at1
	var legPrev *big.Rat
	legPrevIdx := -1

	lastOutAt, ok := mul64(s.OutputFrames-1, outPeriod)
	if !ok || lastOutAt > lastAt {
		return nil, frameError(s.OutputFrames-1, ErrOutOfBounds)
	}

	for n := int64(0); n < s.OutputFrames; n++ {
		at, ok := mul64(n, outPeriod)
		if !ok {
			return nil, frameError(n, ErrTickOverflow)
		}
		if at < firstAt {
			return nil, frameError(n, ErrOutOfBounds)
		}
		for legIdx < len(legs)-1 && at > legs[legIdx].at1 {
			legIdx++
		}
		lg := legs[legIdx]

		srcTicks := interpolate(lg, at)
		if srcTicks.Sign() < 0 || cmpInt64(srcTicks, durationTicks) > 0 {
			return nil, frameError(n, ErrOutOfBounds)
		}
		if legPrevIdx != legIdx {
			legPrev, legPrevIdx = srcTicks, legIdx
		} else {
			sg := sign(lg.src1 - lg.src0)
			if sg != 0 && srcTicks.Cmp(legPrev)*sg < 0 {
				return nil, frameError(n, ErrNonMonotonic)
			}
			legPrev = new(big.Rat).Set(srcTicks)
		}

		sourceTime := new(big.Rat).Quo(srcTicks, big.NewRat(d.ticksPerSec, 1))
		sourceFrameRat := new(big.Rat).Quo(srcTicks, big.NewRat(framePeriod, 1))
		sourceFrame, ok := floorRat(sourceFrameRat)
		if !ok {
			return nil, frameError(n, ErrTickOverflow)
		}
		if sourceFrame == s.SourceFrames {
			sourceFrame--
		}
		if sourceFrame < 0 || sourceFrame >= s.SourceFrames {
			return nil, frameError(n, ErrOutOfBounds)
		}

		audioPosition := new(big.Rat).Mul(sourceTime, s.AudioRate)
		audioSample, ok := floorRat(audioPosition)
		if !ok {
			return nil, frameError(n, ErrTickOverflow)
		}
		maxSampleRat := new(big.Rat).Mul(s.Duration(), s.AudioRate)
		maxSample, ok := floorRat(maxSampleRat)
		if !ok {
			return nil, frameError(n, ErrTickOverflow)
		}
		if maxSampleRat.IsInt() {
			maxSample--
		}
		if audioSample > maxSample {
			audioSample = maxSample
		}
		if audioSample < 0 {
			return nil, frameError(n, ErrOutOfBounds)
		}

		rows = append(rows, FrameMapping{
			OutputFrame:   n,
			SourceFrame:   sourceFrame,
			AudioSample:   audioSample,
			SourceTime:    sourceTime,
			OutputTime:    new(big.Rat).Quo(big.NewRat(n, 1), s.OutputRate),
			AudioPosition: audioPosition,
		})
	}

	return &Table{Rows: rows, spec: cloneSpec(s)}, nil
}

// interpolate returns src0 + (src1-src0)*(at-at0)/(at1-at0) in ticks.
func interpolate(lg leg, at int64) *big.Rat {
	span := big.NewRat(lg.at1-lg.at0, 1)
	part := big.NewRat(at-lg.at0, 1)
	delta := big.NewRat(lg.src1-lg.src0, 1)
	rel := new(big.Rat).Quo(part, span)
	rel.Mul(rel, delta)
	return rel.Add(rel, big.NewRat(lg.src0, 1))
}

func floorRat(r *big.Rat) (int64, bool) {
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}

func cmpInt64(r *big.Rat, v int64) int {
	return r.Cmp(big.NewRat(v, 1))
}

func cloneSpec(s Spec) Spec {
	c := s
	c.SourceRate = cloneRat(s.SourceRate)
	c.OutputRate = cloneRat(s.OutputRate)
	c.AudioRate = cloneRat(s.AudioRate)
	c.Keyframes = make([]Keyframe, len(s.Keyframes))
	for i, k := range s.Keyframes {
		c.Keyframes[i] = Keyframe{At: cloneRat(k.At), Source: cloneRat(k.Source)}
	}
	return c
}
