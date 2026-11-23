package aces709

import (
	"errors"
	"math"
	"testing"
)

func TestRec709QuantizerLevels(t *testing.T) {
	tests := []struct {
		bits  int
		black uint16
		white uint16
	}{
		{8, 16, 235},
		{10, 64, 940},
		{12, 256, 3760},
		{16, 4096, 60160},
	}
	for _, test := range tests {
		q, err := Rec709Quantizer(test.bits)
		if err != nil {
			t.Fatal(err)
		}
		if q.BlackCode() != test.black || q.WhiteCode() != test.white {
			t.Errorf("%d-bit levels = %d/%d, want %d/%d",
				test.bits, q.BlackCode(), q.WhiteCode(), test.black, test.white)
		}
		low, err := q.Encode(0)
		if err != nil {
			t.Fatal(err)
		}
		high, err := q.Encode(1)
		if err != nil {
			t.Fatal(err)
		}
		if low.Code != test.black || high.Code != test.white {
			t.Errorf("%d-bit endpoints = %d/%d, want %d/%d",
				test.bits, low.Code, high.Code, test.black, test.white)
		}
	}
}

func TestQuantizerRoundTripIsWithinHalfStep(t *testing.T) {
	for _, bits := range []int{8, 10, 12, 16} {
		for _, fullRange := range []bool{false, true} {
			q, err := NewQuantizer(bits, fullRange)
			if err != nil {
				t.Fatal(err)
			}
			maxError := 0.0
			for i := 0; i <= 4096; i++ {
				value := float64(i) / 4096
				encoded, err := q.Encode(value)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := q.Decode(encoded.Code)
				if err != nil {
					t.Fatal(err)
				}
				maxError = math.Max(maxError, math.Abs(decoded-value))
			}
			limit := q.Step()/2 + 1e-15
			if maxError > limit {
				t.Errorf("%d-bit fullRange=%v max error %.12g > half step %.12g",
					bits, fullRange, maxError, limit)
			}
		}
	}
}

func TestQuantizerClipsAndReports(t *testing.T) {
	q, err := Rec709Quantizer(10)
	if err != nil {
		t.Fatal(err)
	}
	low, err := q.Encode(-0.25)
	if err != nil {
		t.Fatal(err)
	}
	if low.Code != q.BlackCode() || low.Clip != ClipLow {
		t.Fatalf("negative sample = %+v, want code %d ClipLow", low, q.BlackCode())
	}
	high, err := q.Encode(8)
	if err != nil {
		t.Fatal(err)
	}
	if high.Code != q.WhiteCode() || high.Clip != ClipHigh {
		t.Fatalf("HDR sample = %+v, want code %d ClipHigh", high, q.WhiteCode())
	}
}

func TestQuantizerErrors(t *testing.T) {
	if _, err := NewQuantizer(7, false); !errors.Is(err, ErrUnsupportedBitDepth) {
		t.Fatalf("7-bit error = %v", err)
	}
	if _, err := NewQuantizer(17, false); !errors.Is(err, ErrUnsupportedBitDepth) {
		t.Fatalf("17-bit error = %v", err)
	}
	q, err := NewQuantizer(8, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Decode(256); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("out-of-range decode error = %v", err)
	}
	if _, err := q.Encode(math.NaN()); !errors.Is(err, ErrNonFiniteValue) {
		t.Fatalf("NaN encode error = %v", err)
	}
}
