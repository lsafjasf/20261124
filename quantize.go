package aces709

import (
	"fmt"
	"math"
)

// Clip reports whether a value had to be clipped before quantization.
type Clip uint8

const (
	ClipNone Clip = iota
	ClipLow
	ClipHigh
)

// Quantized is one normalized sample mapped to an integer code value.
type Quantized struct {
	Code uint16
	Clip Clip
}

// Quantizer maps normalized [0,1] samples to unsigned integer codes.
//
// Rec.709 studio/limited range uses black and white levels of 16/235 for
// 8-bit, 64/940 for 10-bit, and so on. Full range uses 0/maxCode.
type Quantizer struct {
	bits      int
	fullRange bool
	maxCode   uint16
	black     uint16
	white     uint16
}

// NewQuantizer constructs a quantizer with a bit depth in [8,16].
func NewQuantizer(bits int, fullRange bool) (Quantizer, error) {
	if bits < 8 || bits > 16 {
		return Quantizer{}, fmt.Errorf("%w: %d (want 8..16)", ErrUnsupportedBitDepth, bits)
	}
	shift := uint(bits - 8)
	q := Quantizer{
		bits:      bits,
		fullRange: fullRange,
		maxCode:   uint16((1 << bits) - 1),
		black:     uint16(16) << shift,
		white:     uint16(235) << shift,
	}
	if fullRange {
		q.black = 0
		q.white = q.maxCode
	}
	if q.white <= q.black {
		return Quantizer{}, fmt.Errorf("%w: empty quantization range", ErrInvalidMatrix)
	}
	return q, nil
}

// Rec709Quantizer constructs the standard limited-range Rec.709 quantizer.
func Rec709Quantizer(bits int) (Quantizer, error) {
	return NewQuantizer(bits, false)
}

// Bits returns the quantizer bit depth.
func (q Quantizer) Bits() int {
	return q.bits
}

// FullRange reports whether 0..maxCode is used instead of Rec.709 studio
// levels.
func (q Quantizer) FullRange() bool {
	return q.fullRange
}

// BlackCode returns the normalized-zero code value.
func (q Quantizer) BlackCode() uint16 {
	return q.black
}

// WhiteCode returns the normalized-one code value.
func (q Quantizer) WhiteCode() uint16 {
	return q.white
}

// Encode maps a normalized encoded sample to a code value. Values below zero
// or above one are clipped to the quantization range and reported via Clip.
func (q Quantizer) Encode(value float64) (Quantized, error) {
	if q.bits == 0 {
		return Quantized{}, fmt.Errorf("%w: uninitialized quantizer", ErrUnsupportedBitDepth)
	}
	if !isFinite(value) {
		return Quantized{}, fmt.Errorf("%w: quantize(%g)", ErrNonFiniteValue, value)
	}
	clip := ClipNone
	if value < 0 {
		value = 0
		clip = ClipLow
	} else if value > 1 {
		value = 1
		clip = ClipHigh
	}
	span := float64(q.white - q.black)
	code := uint16(math.Floor(float64(q.black) + value*span + 0.5))
	return Quantized{Code: code, Clip: clip}, nil
}

// Decode maps a code value back to a normalized sample. Every code representable
// by the bit depth is accepted; codes outside the studio range decode outside
// [0,1], which is useful for inspecting footroom and headroom.
func (q Quantizer) Decode(code uint16) (float64, error) {
	if q.bits == 0 {
		return 0, fmt.Errorf("%w: uninitialized quantizer", ErrUnsupportedBitDepth)
	}
	if code > q.maxCode {
		return 0, fmt.Errorf("%w: %d exceeds %d-bit maximum %d", ErrOutOfRange, code, q.bits, q.maxCode)
	}
	span := float64(q.white - q.black)
	return (float64(code) - float64(q.black)) / span, nil
}

// EncodeRGB quantizes the three channels of an encoded RGB sample.
func (q Quantizer) EncodeRGB(rgb RGB) ([3]Quantized, error) {
	r, err := q.Encode(rgb.R)
	if err != nil {
		return [3]Quantized{}, err
	}
	g, err := q.Encode(rgb.G)
	if err != nil {
		return [3]Quantized{}, err
	}
	b, err := q.Encode(rgb.B)
	if err != nil {
		return [3]Quantized{}, err
	}
	return [3]Quantized{r, g, b}, nil
}

// DecodeRGB dequantizes three code values.
func (q Quantizer) DecodeRGB(codes [3]uint16) (RGB, error) {
	r, err := q.Decode(codes[0])
	if err != nil {
		return RGB{}, err
	}
	g, err := q.Decode(codes[1])
	if err != nil {
		return RGB{}, err
	}
	b, err := q.Decode(codes[2])
	if err != nil {
		return RGB{}, err
	}
	return RGB{R: r, G: g, B: b}, nil
}

// Step returns the normalized spacing between adjacent codes.
func (q Quantizer) Step() float64 {
	if q.bits == 0 {
		return math.NaN()
	}
	return 1 / float64(q.white-q.black)
}
