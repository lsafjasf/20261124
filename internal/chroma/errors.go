package chroma

import "errors"

var (
	// ErrNilPlane indicates a missing plane argument.
	ErrNilPlane = errors.New("chroma: nil plane")
	// ErrPlaneSize indicates mismatched plane dimensions.
	ErrPlaneSize = errors.New("chroma: plane size mismatch")
)
