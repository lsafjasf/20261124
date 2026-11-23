package aces709

import "errors"

var (
	// ErrWhitePoint reports an invalid white point or a white point that does
	// not match the color space / adaptation matrix it accompanies.
	ErrWhitePoint = errors.New("aces709: white point mismatch")

	// ErrMatrixOrder reports a matrix that is valid in isolation but uses the
	// wrong direction, is transposed, or is otherwise ordered incorrectly.
	ErrMatrixOrder = errors.New("aces709: matrix order mismatch")

	// ErrNonFiniteValue reports NaN or an infinity in a color sample.
	ErrNonFiniteValue = errors.New("aces709: non-finite color value")

	// ErrOutOfRange reports a code value outside the quantizer's bit depth.
	ErrOutOfRange = errors.New("aces709: code value out of range")

	// ErrUnsupportedBitDepth reports a bit depth outside 8..16.
	ErrUnsupportedBitDepth = errors.New("aces709: unsupported bit depth")
)
