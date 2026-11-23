package main

// decodeRLE decodes the two interlaced RLE fields into pixels (w*h, values 0..3).
// Even lines come from offEven, odd lines from offOdd. Runs are nibble-coded:
// a 4/8/12/16-bit big-endian value v encodes run=v>>2, color=v&3; a 16-bit
// run of 0 means "fill to end of line".
func decodeRLE(e Entry, pkt []byte, offEven, offOdd int, pixels []uint8, w, h int) error {
	for y := 0; y < h; y++ {
		off := offEven
		if y%2 == 1 {
			off = offOdd
		}
		x := 0
		nib := 0 // nibble position within pkt[off], 0=high
		read4 := func() (int, bool) {
			if off >= len(pkt) {
				return 0, false
			}
			b := pkt[off]
			var v int
			if nib == 0 {
				v = int(b >> 4)
			} else {
				v = int(b & 0x0F)
				off++
			}
			nib ^= 1
			return v, true
		}
		for x < w {
			v, ok := read4()
			if !ok {
				return entryErr(e, "RLE data exhausted at line %d (offset +%d)", y, off)
			}
			if v < 0x4 {
				if v, ok = shiftIn(v, read4); !ok {
					return entryErr(e, "RLE data exhausted at line %d (offset +%d)", y, off)
				}
				if v < 0x10 {
					if v, ok = shiftIn(v, read4); !ok {
						return entryErr(e, "RLE data exhausted at line %d (offset +%d)", y, off)
					}
					if v < 0x40 {
						if v, ok = shiftIn(v, read4); !ok {
							return entryErr(e, "RLE data exhausted at line %d (offset +%d)", y, off)
						}
					}
				}
			}
			run, color := v>>2, uint8(v&3)
			if run == 0 {
				run = w - x // fill to end of line
			}
			if x+run > w {
				return entryErr(e, "RLE run of %d at line %d overflows width %d", run, y, w)
			}
			for i := 0; i < run; i++ {
				pixels[y*w+x+i] = color
			}
			x += run
		}
		// Lines start on a byte boundary.
		if nib == 1 {
			nib = 0
			off++
		}
		if y%2 == 1 {
			offOdd = off
		} else {
			offEven = off
		}
	}
	return nil
}

func shiftIn(v int, read4 func() (int, bool)) (int, bool) {
	n, ok := read4()
	if !ok {
		return 0, false
	}
	return v<<4 | n, true
}
