package aces709

import (
	"errors"
	"fmt"
	"math"
)

// Matrix3 is a row-major 3x3 matrix. Mul applies the right-hand operand
// first: (a.Mul(b)) * v == a * (b * v).
type Matrix3 [3][3]float64

// Vec3 is a three-component column vector.
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// RGB is a three-channel color sample. Values are scene-linear in an ACEScg
// frame and display-linear in a linear Rec.709 frame.
type RGB struct {
	R float64
	G float64
	B float64
}

// Identity3 returns the 3x3 identity matrix.
func Identity3() Matrix3 {
	return Matrix3{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
}

// Mul returns a*b. See Matrix3 for multiplication order.
func (a Matrix3) Mul(b Matrix3) Matrix3 {
	var out Matrix3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = a[row][0]*b[0][col] +
				a[row][1]*b[1][col] +
				a[row][2]*b[2][col]
		}
	}
	return out
}

// ApplyVec multiplies m by v.
func (m Matrix3) ApplyVec(v Vec3) Vec3 {
	return Vec3{
		X: m[0][0]*v.X + m[0][1]*v.Y + m[0][2]*v.Z,
		Y: m[1][0]*v.X + m[1][1]*v.Y + m[1][2]*v.Z,
		Z: m[2][0]*v.X + m[2][1]*v.Y + m[2][2]*v.Z,
	}
}

// Transpose returns the matrix transpose.
func (m Matrix3) Transpose() Matrix3 {
	var out Matrix3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = m[col][row]
		}
	}
	return out
}

// Inverse returns the inverse of m using a double-precision cofactor
// expansion. Singular or non-finite matrices return ErrInvalidMatrix.
func (m Matrix3) Inverse() (Matrix3, error) {
	cof := Matrix3{
		{
			m[1][1]*m[2][2] - m[1][2]*m[2][1],
			m[0][2]*m[2][1] - m[0][1]*m[2][2],
			m[0][1]*m[1][2] - m[0][2]*m[1][1],
		},
		{
			m[1][2]*m[2][0] - m[1][0]*m[2][2],
			m[0][0]*m[2][2] - m[0][2]*m[2][0],
			m[0][2]*m[1][0] - m[0][0]*m[1][2],
		},
		{
			m[1][0]*m[2][1] - m[1][1]*m[2][0],
			m[0][1]*m[2][0] - m[0][0]*m[2][1],
			m[0][0]*m[1][1] - m[0][1]*m[1][0],
		},
	}
	det := m[0][0]*cof[0][0] + m[0][1]*cof[1][0] + m[0][2]*cof[2][0]
	if !isFinite(det) || math.Abs(det) < 1e-15 {
		return Matrix3{}, fmt.Errorf("%w: determinant is zero or non-finite", ErrInvalidMatrix)
	}
	invDet := 1 / det
	var out Matrix3
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			out[row][col] = cof[row][col] * invDet
		}
	}
	return out, nil
}

func (m Matrix3) validate(name string) error {
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			if !isFinite(m[row][col]) {
				return fmt.Errorf("%w: %s[%d][%d] is not finite", ErrInvalidMatrix, name, row, col)
			}
		}
	}
	if _, err := m.Inverse(); err != nil {
		return fmt.Errorf("%w: %s is singular", ErrInvalidMatrix, name)
	}
	return nil
}

func closeMatrix(a, b Matrix3, tolerance float64) bool {
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			if math.Abs(a[row][col]-b[row][col]) > tolerance {
				return false
			}
		}
	}
	return true
}

func maxMatrixDelta(a, b Matrix3) float64 {
	var maxDelta float64
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			maxDelta = math.Max(maxDelta, math.Abs(a[row][col]-b[row][col]))
		}
	}
	return maxDelta
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// ErrInvalidMatrix reports a non-finite, singular, or otherwise unsupported
// matrix.
var ErrInvalidMatrix = errors.New("aces709: invalid matrix")
