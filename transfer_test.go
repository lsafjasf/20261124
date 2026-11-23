package aces709

import (
	"math"
	"testing"
)

func TestBT709KnownValues(t *testing.T) {
	tests := []struct {
		linear  float64
		encoded float64
	}{
		{0, 0},
		{BT709Beta, 4.5 * BT709Beta},
		{1, 1},
	}
	for _, test := range tests {
		got, err := EncodeBT709(test.linear)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-test.encoded) > 1e-14 {
			t.Errorf("EncodeBT709(%g) = %.17g, want %.17g", test.linear, got, test.encoded)
		}
		roundTrip, err := DecodeBT709(got)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(roundTrip-test.linear) > 2e-15 {
			t.Errorf("DecodeBT709 round trip = %.17g, want %.17g", roundTrip, test.linear)
		}
	}
}

func TestBT709Continuity(t *testing.T) {
	below, err := EncodeBT709(math.Nextafter(BT709Beta, 0))
	if err != nil {
		t.Fatal(err)
	}
	above, err := EncodeBT709(BT709Beta)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(above-below) > 1e-14 {
		t.Fatalf("BT.709 discontinuity across beta: below=%g above=%g", below, above)
	}
}

func TestBT709InverseAcrossExtendedRange(t *testing.T) {
	values := []float64{
		-1000, -32, -1, -0.18, -0.02, -1e-6, 0,
		1e-6, 0.001, 0.017, BT709Beta, 0.02, 0.18, 0.5,
		1, 2, 8, 32, 1000,
	}
	for _, value := range values {
		encoded, err := EncodeBT709(value)
		if err != nil {
			t.Fatalf("EncodeBT709(%g): %v", value, err)
		}
		decoded, err := DecodeBT709(encoded)
		if err != nil {
			t.Fatalf("DecodeBT709(%g): %v", encoded, err)
		}
		tolerance := 8e-15 * math.Max(1, math.Abs(value))
		if math.Abs(decoded-value) > tolerance {
			t.Errorf("BT.709 round trip %g -> %g -> %g, error %g", value, encoded, decoded, decoded-value)
		}
	}
}

func TestBT709RejectsNonFinite(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := EncodeBT709(value); err == nil {
			t.Errorf("EncodeBT709(%v) unexpectedly succeeded", value)
		}
		if _, err := DecodeBT709(value); err == nil {
			t.Errorf("DecodeBT709(%v) unexpectedly succeeded", value)
		}
	}
}
