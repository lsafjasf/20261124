package pgs

import "fmt"

// decodeRLE decodes the PGS run-length format. A non-zero byte is a single
// palette index. A zero byte introduces an escape: bit 7 adds an explicit
// palette index, bit 6 extends the run length to 14 bits, and the low six bits
// hold the run length (or its high bits when bit 6 is set). A zero run is an
// end-of-line marker.
func decodeRLE(data []byte, width, height int) ([]uint8, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("pgs: invalid object size %dx%d", width, height)
	}
	if width > 16384 || height > 16384 {
		return nil, fmt.Errorf("pgs: object too large %dx%d", width, height)
	}

	out := make([]uint8, width*height)
	pos := 0
	line := 0
	x := 0

	for pos < len(data) && line < height {
		control := data[pos]
		pos++
		color := control
		run := 1

		if control == 0 {
			if pos >= len(data) {
				return nil, fmt.Errorf("pgs: truncated RLE escape on line %d", line)
			}
			flags := data[pos]
			pos++
			run = int(flags & 0x3f)
			if flags&0x40 != 0 {
				if pos >= len(data) {
					return nil, fmt.Errorf("pgs: truncated long RLE run on line %d", line)
				}
				run = run<<8 | int(data[pos])
				pos++
			}
			if flags&0x80 != 0 {
				if pos >= len(data) {
					return nil, fmt.Errorf("pgs: missing RLE palette index on line %d", line)
				}
				color = data[pos]
				pos++
			} else {
				color = 0
			}
		}

		if run == 0 {
			if x != width {
				return nil, fmt.Errorf("pgs: RLE line %d has %d pixels, want %d", line, x, width)
			}
			line++
			x = 0
			continue
		}
		if x+run > width {
			return nil, fmt.Errorf("pgs: RLE line %d overruns width %d by %d pixels", line, width, x+run-width)
		}

		start := line*width + x
		for i := 0; i < run; i++ {
			out[start+i] = color
		}
		x += run
	}

	if line == height-1 && x == width {
		line++
	}
	if line < height {
		return nil, fmt.Errorf("pgs: RLE contains %d of %d lines", line, height)
	}
	return out, nil
}
