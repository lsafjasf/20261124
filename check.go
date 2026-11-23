package main

import "fmt"

// Failure pinpoints one mismatch: plane, row within the plane, absolute
// byte offset, and the expected vs actual value. Values are reported
// verbatim; nothing is clipped to make a mismatch look valid.
type Failure struct {
	Plane      string // "Y", "U", "V" or "PAD"
	Line       int    // row index within the plane
	ByteOffset int    // absolute offset in the buffer
	Expected   string
	Actual     string
	Reason     string
}

func (f Failure) String() string {
	if f.Expected == "" {
		return fmt.Sprintf("%s: %s", f.Plane, f.Reason)
	}
	return fmt.Sprintf("%s line=%d byte=0x%06X expected=%s actual=%s: %s",
		f.Plane, f.Line, f.ByteOffset, f.Expected, f.Actual, f.Reason)
}

// maxFailures caps the report so a fully corrupted frame stays readable.
const maxFailures = 64

// Check validates data against spec and returns every deviation found
// (up to maxFailures). It never normalizes or clips sample values.
func Check(data []byte, s Spec) []Failure {
	var fails []Failure
	add := func(f Failure) bool {
		fails = append(fails, f)
		return len(fails) < maxFailures
	}
	if err := s.validate(); err != nil {
		return []Failure{{Plane: "-", Reason: "invalid spec: " + err.Error()}}
	}
	if len(data) != s.Size() {
		return []Failure{{
			Plane: "-",
			Reason: fmt.Sprintf("buffer size %d, expected %d (stride=%d, %d Y rows + %d UV rows)",
				len(data), s.Size(), s.Stride, s.Height, s.Height/2),
		}}
	}

	sh := uint(16 - s.BitDepth)
	lowMask := uint16(1<<sh - 1) // bits that must be zero in MSB-aligned data
	yLo, yHi := s.sampleRange(false)
	cLo, cHi := s.sampleRange(true)

	checkSample := func(plane string, line, off int, wantCode, altCode, lo, hi int) bool {
		got := uint16(data[off]) | uint16(data[off+1])<<8
		want := uint16(wantCode) << sh
		if got == want {
			return true
		}
		hex := func(v uint16) string { return fmt.Sprintf("0x%04X", v) }
		switch {
		case got&lowMask != 0:
			reason := fmt.Sprintf("valid bits not MSB-aligned: low %d bits must be zero", 16-s.BitDepth)
			if swapped := got<<8 | got>>8; swapped == want {
				reason = "byte order looks big-endian; P010/P016 are little-endian"
			}
			return add(Failure{plane, line, off, hex(want), hex(got), reason})
		default:
			code := int(got >> sh)
			switch {
			case code < lo || code > hi:
				return add(Failure{plane, line, off, hex(want), hex(got),
					fmt.Sprintf("code %d outside %s range [%d,%d]; refusing to clip", code, rangeName(s.FullRange), lo, hi)})
			case code == altCode:
				return add(Failure{plane, line, off, hex(want), hex(got),
					"UV interleave order swapped (found the other chroma channel here)"})
			default:
				return add(Failure{plane, line, off, hex(want), hex(got), "sample value mismatch"})
			}
		}
	}

	ok := true
	for y := 0; y < s.Height && ok; y++ {
		for x := 0; x < s.Width && ok; x++ {
			ok = checkSample("Y", y, y*s.Stride+x*2, s.yPattern(x, y), -1, yLo, yHi)
		}
	}
	for cy := 0; cy < s.Height/2 && ok; cy++ {
		for cx := 0; cx < s.Width/2 && ok; cx++ {
			off := s.uvBase() + cy*s.Stride + cx*4
			if ok = checkSample("U", cy, off, s.uPattern(cx, cy), s.vPattern(cx, cy), cLo, cHi); !ok {
				break
			}
			ok = checkSample("V", cy, off+2, s.vPattern(cx, cy), s.uPattern(cx, cy), cLo, cHi)
		}
	}

	// Row-end padding must be zero. Payload is Width*2 bytes in both
	// planes (Y: Width words; UV: Width/2 pairs of U,V words).
	rows := s.Height + s.Height/2
	for line := 0; line < rows && ok; line++ {
		for off := s.Width * 2; off < s.Stride && ok; off++ {
			abs := line*s.Stride + off
			if data[abs] != 0 {
				ok = add(Failure{"PAD", line, abs, "0x00",
					fmt.Sprintf("0x%02X", data[abs]), "non-zero row-end padding"})
			}
		}
	}
	return fails
}

func rangeName(full bool) string {
	if full {
		return "full"
	}
	return "limited"
}
