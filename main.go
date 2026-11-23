package main

import "fmt"

// forEachSample walks every payload word, skipping row-end padding.
func forEachSample(s Spec, fn func(plane string, line, off int)) {
	for y := 0; y < s.Height; y++ {
		for x := 0; x < s.Width; x++ {
			fn("Y", y, y*s.Stride+x*2)
		}
	}
	for cy := 0; cy < s.Height/2; cy++ {
		for cx := 0; cx < s.Width/2; cx++ {
			off := s.uvBase() + cy*s.Stride + cx*4
			fn("U", cy, off)
			fn("V", cy, off+2)
		}
	}
}

func word(data []byte, off int) uint16 {
	return uint16(data[off]) | uint16(data[off+1])<<8
}

func putWord(data []byte, off int, w uint16) {
	data[off] = byte(w)
	data[off+1] = byte(w >> 8)
}

// Corruptors: each produces one classic P010/P016 mix-up.

func corruptLSBAlign(data []byte, s Spec) {
	sh := uint(16 - s.BitDepth)
	forEachSample(s, func(_ string, _, off int) {
		putWord(data, off, word(data, off)>>sh) // valid bits moved to the bottom
	})
}

func corruptBigEndian(data []byte, s Spec) {
	forEachSample(s, func(_ string, _, off int) {
		data[off], data[off+1] = data[off+1], data[off]
	})
}

func corruptUVSwap(data []byte, s Spec) {
	for cy := 0; cy < s.Height/2; cy++ {
		for cx := 0; cx < s.Width/2; cx++ {
			off := s.uvBase() + cy*s.Stride + cx*4
			u, v := word(data, off), word(data, off+2)
			putWord(data, off, v)
			putWord(data, off+2, u)
		}
	}
}

func corruptPadding(data []byte, s Spec) {
	if s.Stride > s.Width*2 {
		data[2*s.Stride+s.Width*2] = 0xFF // Y plane, line 2, first pad byte
	}
}

func corruptRange(data []byte, s Spec) {
	_, hi := s.sampleRange(false)
	putWord(data, 0, uint16(hi+1)<<uint(16-s.BitDepth)) // Y(0,0) above limited-range max
}

func report(title string, fails []Failure) {
	if len(fails) == 0 {
		fmt.Printf("%-34s OK\n", title)
		return
	}
	trunc := ""
	if len(fails) == maxFailures {
		trunc = " (truncated)"
	}
	fmt.Printf("%-34s %d failure(s)%s\n", title, len(fails), trunc)
	for _, f := range fails {
		fmt.Printf("    %s\n", f)
	}
}

func main() {
	base := Spec{Width: 6, Height: 4, Stride: 6*2 + 8} // 8 bytes of row padding

	fmt.Println("== valid reference frames (must pass) ==")
	for _, bd := range []int{10, 12, 16} {
		for _, full := range []bool{false, true} {
			spec := base
			spec.BitDepth, spec.FullRange = bd, full
			data, err := Generate(spec)
			if err != nil {
				fmt.Println("generate:", err)
				continue
			}
			report(fmt.Sprintf("P%03d %s-range", bd, rangeName(full)), Check(data, spec))
		}
	}

	fmt.Println("\n== corrupted frames (must fail with exact locations) ==")
	spec := base
	spec.BitDepth, spec.FullRange = 10, false
	cases := []struct {
		name string
		fn   func([]byte, Spec)
	}{
		{"LSB-aligned instead of MSB-aligned", corruptLSBAlign},
		{"big-endian instead of little-endian", corruptBigEndian},
		{"UV plane swapped (VU order)", corruptUVSwap},
		{"garbage in row-end padding", corruptPadding},
		{"code above limited-range max", corruptRange},
	}
	for _, c := range cases {
		data, err := Generate(spec)
		if err != nil {
			fmt.Println("generate:", err)
			continue
		}
		c.fn(data, spec)
		report(c.name, Check(data, spec))
	}

	fmt.Println("\n== stride mismatch ==")
	strideData, _ := Generate(spec)
	report("stride too small for payload", Check(strideData, Spec{
		Width: spec.Width, Height: spec.Height, BitDepth: spec.BitDepth, Stride: spec.Width*2 - 2,
	}))
}
