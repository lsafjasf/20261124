package main

import "math"

// ST 2084 (PQ) constants. PQ is an ABSOLUTE curve: the signal always
// refers to 0..10000 nit, independent of any display peak.
const (
	m1 = 2610.0 / 16384.0
	m2 = 2523.0 / 4096.0 * 128.0
	c1 = 3424.0 / 4096.0
	c2 = 2413.0 / 4096.0 * 32.0
	c3 = 2392.0 / 4096.0 * 32.0

	PQMaxNits  = 10000.0
	PQCodeBits = 10
	PQCodeMax  = 1<<PQCodeBits - 1 // 1023

	// Mid-gray reference: 18% of BT.2408 graphics white (203 nit).
	MidGrayNits = 0.18 * 203.0 // 36.54 nit
)

// Display peaks under test.
var DisplayPeaks = []float64{1000, 2000, 4000}

// PQEncode converts absolute linear nits (float64, unquantized) to a
// PQ signal in [0,1] (still float64, unquantized).
func PQEncode(nits float64) float64 {
	if nits < 0 {
		nits = 0
	}
	y := nits / PQMaxNits
	ym1 := math.Pow(y, m1)
	return math.Pow((c1+c2*ym1)/(1+c3*ym1), m2)
}

// PQDecode converts a PQ signal in [0,1] back to absolute nits (float64).
func PQDecode(signal float64) float64 {
	if signal <= 0 {
		return 0
	}
	if signal > 1 {
		signal = 1
	}
	em2 := math.Pow(signal, 1/m2)
	num := em2 - c1
	if num < 0 {
		num = 0
	}
	return PQMaxNits * math.Pow(num/(c2-c3*em2), 1/m1)
}

// QuantizePQ is THE quantization point of the pipeline:
// float64 PQ signal -> 10-bit integer code. Everything before it
// (linear nits, PQ signal) and the decoded nits after it stay float64.
func QuantizePQ(signal float64) uint16 {
	if signal < 0 {
		signal = 0
	}
	if signal > 1 {
		signal = 1
	}
	return uint16(math.Round(signal * PQCodeMax))
}

// DequantizePQ maps a 10-bit code back to a float64 PQ signal.
func DequantizePQ(code uint16) float64 {
	return float64(code) / PQCodeMax
}

// EncodePQ10: linear nits -> 10-bit PQ code (peak-independent).
func EncodePQ10(nits float64) uint16 { return QuantizePQ(PQEncode(nits)) }

// DecodePQ10: 10-bit PQ code -> absolute nits (peak-independent).
func DecodePQ10(code uint16) float64 { return PQDecode(DequantizePQ(code)) }

// DisplayMap converts absolute nits to an 8-bit output for a display
// with the given peak. Values above the peak are out of range: they are
// clipped and flagged. This 8-bit step is display-only quantization and
// never feeds back into the PQ code path.
func DisplayMap(nits, peak float64) (level uint8, clipped bool) {
	if nits > peak {
		return 255, true
	}
	if nits < 0 {
		nits = 0
	}
	return uint8(math.Round(255 * nits / peak)), false
}
