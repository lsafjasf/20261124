package main

import (
	"strings"
	"testing"
)

func testSpec(bitDepth int, full bool) Spec {
	return Spec{Width: 6, Height: 4, BitDepth: bitDepth, FullRange: full, Stride: 6*2 + 8}
}

func TestValidFramesPass(t *testing.T) {
	for _, bd := range []int{10, 12, 16} {
		for _, full := range []bool{false, true} {
			spec := testSpec(bd, full)
			data, err := Generate(spec)
			if err != nil {
				t.Fatalf("Generate %d-bit full=%v: %v", bd, full, err)
			}
			if fails := Check(data, spec); len(fails) != 0 {
				t.Errorf("%d-bit full=%v: want 0 failures, got %v", bd, full, fails[0])
			}
		}
	}
}

func TestCorruptorsAreDetected(t *testing.T) {
	spec := testSpec(10, false)
	cases := []struct {
		name string
		fn   func([]byte, Spec)
		want string // substring expected in at least one failure
	}{
		{"lsb-align", corruptLSBAlign, "MSB-aligned"},
		{"big-endian", corruptBigEndian, "big-endian"},
		{"uv-swap", corruptUVSwap, "UV interleave order swapped"},
		{"padding", corruptPadding, "padding"},
		{"range", corruptRange, "refusing to clip"},
	}
	for _, c := range cases {
		data, err := Generate(spec)
		if err != nil {
			t.Fatal(err)
		}
		c.fn(data, spec)
		fails := Check(data, spec)
		if len(fails) == 0 {
			t.Errorf("%s: corruption not detected", c.name)
			continue
		}
		found := false
		for _, f := range fails {
			if f.Expected == "" {
				t.Errorf("%s: failure missing expected value: %v", c.name, f)
			}
			if strings.Contains(f.Reason, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no failure mentions %q, first: %v", c.name, c.want, fails[0])
		}
	}
}

func TestStrideMismatch(t *testing.T) {
	spec := testSpec(10, false)
	data, _ := Generate(spec)
	bad := spec
	bad.Stride = spec.Width*2 - 2
	if fails := Check(data, bad); len(fails) == 0 {
		t.Error("undersized stride not detected")
	}
}
