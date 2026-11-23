package main

import (
	"fmt"
	"math/big"
	"sort"
)

type frameRat struct{ r *big.Rat }

func fr(n int64) frameRat                  { return frameRat{big.NewRat(n, 1)} }
func (f frameRat) add(o frameRat) frameRat { return frameRat{new(big.Rat).Add(f.r, o.r)} }
func (f frameRat) sub(o frameRat) frameRat { return frameRat{new(big.Rat).Sub(f.r, o.r)} }
func (f frameRat) mul(o frameRat) frameRat { return frameRat{new(big.Rat).Mul(f.r, o.r)} }
func (f frameRat) quo(o frameRat) frameRat { return frameRat{new(big.Rat).Quo(f.r, o.r)} }

// roundNearest converts an exact rational frame position to the nearest integer
// frame. Half values round away from zero, matching NLE frame snapping.
func (f frameRat) roundNearest() int64 {
	q := new(big.Rat).Mul(f.r, big.NewRat(2, 1))
	whole := new(big.Int).Quo(q.Num(), q.Denom())
	rounded := new(big.Int).Quo(new(big.Int).Add(whole, big.NewInt(1)), big.NewInt(2))
	if f.r.Sign() < 0 {
		rounded = new(big.Int).Quo(new(big.Int).Sub(whole, big.NewInt(1)), big.NewInt(2))
	}
	return rounded.Int64()
}

type placedSeg struct {
	seg      Segment
	speed    Speed
	tlStart  frameRat // timeline start, exact
	tlEnd    frameRat // timeline end, exclusive, exact
	srcStart int64
}

// Curve is a validated, positioned speed curve.
type Curve struct {
	segs    []placedSeg
	srcMax  int64
	tlTotal frameRat
}

// BuildCurve validates a speed curve and computes each segment's exact
// timeline extent. Segments are expressed in source frames and placed
// back to back on the new sequence timeline.
func BuildCurve(segs []Segment) (*Curve, error) {
	if len(segs) == 0 {
		return nil, fmt.Errorf("speed curve has no segments")
	}
	out := make([]placedSeg, 0, len(segs))
	cursor := fr(0)
	var srcMax int64
	for i := range segs {
		s := segs[i]
		sp, err := parseSpeed(s.Speed)
		if err != nil {
			return nil, fmt.Errorf("segment %d (%s): %v", i, s.Name, err)
		}
		if s.Hold < 0 {
			return nil, fmt.Errorf("segment %d (%s): hold must be >= 0", i, s.Name)
		}
		if s.SrcStart < 0 {
			return nil, fmt.Errorf("segment %d (%s): srcStart must be >= 0", i, s.Name)
		}
		s.speed = sp
		var dur frameRat
		switch {
		case sp.isZero():
			// Freeze: hold length equals its timeline duration (no source advance).
			if s.Hold == 0 {
				return nil, fmt.Errorf("segment %d (%s): freeze hold must be > 0", i, s.Name)
			}
			dur = fr(s.Hold)
		default:
			if s.Hold == 0 {
				return nil, fmt.Errorf("segment %d (%s): source hold must be > 0", i, s.Name)
			}
			// duration = sourceFrames / |speed|
			dur = fr(s.Hold).quo(frameRat{new(big.Rat).Abs(sp.ratVal())})
		}
		end := cursor.add(dur)
		out = append(out, placedSeg{
			seg:      s,
			speed:    sp,
			tlStart:  cursor,
			tlEnd:    end,
			srcStart: s.SrcStart,
		})
		cursor = end
		if end := s.SrcStart + s.Hold; end > srcMax {
			srcMax = end
		}
	}
	return &Curve{segs: out, srcMax: srcMax, tlTotal: cursor}, nil
}

// DurationFrames is the rounded total new sequence length in frames.
func (c *Curve) DurationFrames() int64 { return c.tlTotal.roundNearest() }

// mapPoint returns the exact timeline position(s) for a source frame.
// A freeze returns the whole hold interval; other segments return one point.
// Multiple results mean the old position is not uniquely mappable.
func (c *Curve) mapPoint(src int64) ([]frameRat, string, error) {
	type hit struct {
		pos frameRat
		why string
	}
	var hits []hit
	for idx, ps := range c.segs {
		lo, hi := ps.srcStart, ps.srcStart+ps.seg.Hold
		if src < lo || src >= hi {
			continue
		}
		// Half-open intervals [lo, hi); only the final segment keeps its right edge.
		isLast := idx == len(c.segs)-1
		if src == hi && !isLast {
			continue
		}
		switch {
		case ps.speed.isZero():
			// Every source position covered by a freeze collapses to its interval.
			hits = append(hits, hit{ps.tlStart, "freeze-hold-start"})
			hits = append(hits, hit{ps.tlEnd, "freeze-hold-end"})
		case ps.speed.isNeg():
			// Reverse: source advances from hi back to lo as time moves forward.
			rel := fr(hi - src).quo(frameRat{new(big.Rat).Abs(ps.speed.ratVal())})
			hits = append(hits, hit{ps.tlStart.add(rel), "negative-speed"})
		default:
			rel := fr(src - lo).quo(frameRat{ps.speed.ratVal()})
			hits = append(hits, hit{ps.tlStart.add(rel), "variable-speed"})
		}
	}
	if len(hits) == 0 {
		return nil, "", fmt.Errorf("source frame %d is outside the speed curve", src)
	}
	pts := make([]frameRat, 0, len(hits))
	why := ""
	for _, h := range hits {
		pts = append(pts, h.pos)
		if why == "" {
			why = h.why
		}
	}
	return pts, why, nil
}

func dedupInt64(xs []int64) []int64 {
	seen := map[int64]bool{}
	out := xs[:0]
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
