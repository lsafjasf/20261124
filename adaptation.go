package aces709

import (
	"fmt"
	"math"
)

var bradfordMatrix = Matrix3{
	{0.8951, 0.2664, -0.1614},
	{-0.7502, 1.7135, 0.0367},
	{0.0389, -0.0685, 1.0296},
}

// BradfordResponse returns the Bradford cone-response matrix. The value is a
// copy of the canonical definition.
func BradfordResponse() Matrix3 {
	return bradfordMatrix
}

// BradfordAdaptation returns the matrix that adapts XYZ from srcWhite to
// dstWhite using the Bradford transform. Both white points are normalized to
// Y=1.
func BradfordAdaptation(srcWhite, dstWhite Chromaticity) (Matrix3, error) {
	src, err := srcWhite.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("source white: %w", err)
	}
	dst, err := dstWhite.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("destination white: %w", err)
	}
	if err := bradfordMatrix.validate("Bradford"); err != nil {
		return Matrix3{}, err
	}
	invBradford, err := bradfordMatrix.Inverse()
	if err != nil {
		return Matrix3{}, err
	}

	srcCone := bradfordMatrix.ApplyVec(src)
	dstCone := bradfordMatrix.ApplyVec(dst)
	if math.Abs(srcCone.X) < 1e-15 || math.Abs(srcCone.Y) < 1e-15 || math.Abs(srcCone.Z) < 1e-15 {
		return Matrix3{}, fmt.Errorf("%w: source white maps to a zero Bradford cone response", ErrWhitePoint)
	}
	coneScale := Matrix3{
		{dstCone.X / srcCone.X, 0, 0},
		{0, dstCone.Y / srcCone.Y, 0},
		{0, 0, dstCone.Z / srcCone.Z},
	}
	return invBradford.Mul(coneScale).Mul(bradfordMatrix), nil
}
