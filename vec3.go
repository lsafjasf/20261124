package spatial

import "math"

// Vec3 is a position or direction in listener-local meters.
// +X points right, +Y up, and +Z forward.
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// IsFinite reports whether every component is a finite number.
func (v Vec3) IsFinite() bool {
	return !math.IsNaN(v.X) && !math.IsInf(v.X, 0) &&
		!math.IsNaN(v.Y) && !math.IsInf(v.Y, 0) &&
		!math.IsNaN(v.Z) && !math.IsInf(v.Z, 0)
}

// Length returns the Euclidean length of v.
func (v Vec3) Length() float64 {
	return math.Hypot(v.X, math.Hypot(v.Y, v.Z))
}

// Scale returns v multiplied by k.
func (v Vec3) Scale(k float64) Vec3 {
	return Vec3{X: v.X * k, Y: v.Y * k, Z: v.Z * k}
}

// Add returns v plus other.
func (v Vec3) Add(other Vec3) Vec3 {
	return Vec3{X: v.X + other.X, Y: v.Y + other.Y, Z: v.Z + other.Z}
}

// Dot returns the dot product of v and other.
func (v Vec3) Dot(other Vec3) float64 {
	return v.X*other.X + v.Y*other.Y + v.Z*other.Z
}

// Unit returns a unit vector in the direction of v.
func (v Vec3) Unit() (Vec3, bool) {
	n := v.Length()
	if n < directionEpsilon {
		return Vec3{}, false
	}
	return v.Scale(1 / n), true
}

// Horizontal returns v with its vertical component removed.
func (v Vec3) Horizontal() Vec3 {
	return Vec3{X: v.X, Z: v.Z}
}

const directionEpsilon = 1e-12
