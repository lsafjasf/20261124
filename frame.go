package main

import "fmt"

// Spec describes a P010-family frame: 4:2:0, 16-bit little-endian words,
// valid bits MSB-aligned (high-bit aligned), a Y plane followed by an
// interleaved U/V plane (U first, then V, 4 bytes per chroma pair).
type Spec struct {
	Width     int
	Height    int
	BitDepth  int // 10, 12 or 16
	FullRange bool
	Stride    int // bytes per row, must be >= Width*2; tail is padding
}

func (s Spec) validate() error {
	if s.Width <= 0 || s.Height <= 0 || s.Width%2 != 0 || s.Height%2 != 0 {
		return fmt.Errorf("width/height must be positive even numbers, got %dx%d", s.Width, s.Height)
	}
	switch s.BitDepth {
	case 10, 12, 16:
	default:
		return fmt.Errorf("unsupported bit depth %d", s.BitDepth)
	}
	if s.Stride < s.Width*2 {
		return fmt.Errorf("stride %d smaller than row payload %d bytes", s.Stride, s.Width*2)
	}
	return nil
}

// Size is the total buffer size in bytes: Y rows plus Height/2 UV rows.
func (s Spec) Size() int {
	return s.Stride*s.Height + s.Stride*(s.Height/2)
}

// uvBase is the offset of the first UV row.
func (s Spec) uvBase() int {
	return s.Stride * s.Height
}

// sampleRange returns the legal code-value range in BitDepth scale.
// Limited range: Y [16,235], C [16,240] (MPEG), scaled to the bit depth.
// Full range: [0, 2^BitDepth-1].
func (s Spec) sampleRange(chroma bool) (lo, hi int) {
	if s.FullRange {
		return 0, 1<<s.BitDepth - 1
	}
	shift := s.BitDepth - 8
	if chroma {
		return 16 << shift, 240 << shift
	}
	return 16 << shift, 235 << shift
}

// Deterministic sample patterns. U and V use different steps so that a
// swapped UV plane is detectable at (almost) every chroma position.
func (s Spec) yPattern(x, y int) int {
	lo, hi := s.sampleRange(false)
	return lo + (x+y*s.Width)%(hi-lo+1)
}

func (s Spec) uPattern(cx, cy int) int {
	lo, hi := s.sampleRange(true)
	return lo + (1+2*cx+cy*(s.Width/2))%(hi-lo+1)
}

func (s Spec) vPattern(cx, cy int) int {
	lo, hi := s.sampleRange(true)
	return lo + (2+cx+3*cy*(s.Width/2))%(hi-lo+1)
}

// Generate builds a reference frame. Pattern values are in-range by
// construction; nothing is ever clipped.
func Generate(s Spec) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	buf := make([]byte, s.Size())
	sh := uint(16 - s.BitDepth)
	put := func(off, code int) {
		w := uint16(code) << sh
		buf[off] = byte(w)
		buf[off+1] = byte(w >> 8)
	}
	for y := 0; y < s.Height; y++ {
		for x := 0; x < s.Width; x++ {
			put(y*s.Stride+x*2, s.yPattern(x, y))
		}
	}
	for cy := 0; cy < s.Height/2; cy++ {
		for cx := 0; cx < s.Width/2; cx++ {
			off := s.uvBase() + cy*s.Stride + cx*4
			put(off, s.uPattern(cx, cy))
			put(off+2, s.vPattern(cx, cy))
		}
	}
	return buf, nil
}
