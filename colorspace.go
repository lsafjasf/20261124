package aces709

import (
	"fmt"
	"math"
)

// Chromaticity is a CIE 1931 xy chromaticity coordinate.
type Chromaticity struct {
	X float64
	Y float64
}

// XYZ returns the Y=1 normalized CIE XYZ vector for a chromaticity.
func (c Chromaticity) XYZ() (Vec3, error) {
	if !isFinite(c.X) || !isFinite(c.Y) || math.Abs(c.Y) < 1e-15 {
		return Vec3{}, fmt.Errorf("%w: invalid chromaticity (%g, %g)", ErrWhitePoint, c.X, c.Y)
	}
	return Vec3{
		X: c.X / c.Y,
		Y: 1,
		Z: (1 - c.X - c.Y) / c.Y,
	}, nil
}

// Primaries describes the red, green, and blue chromaticities of an RGB
// color space.
type Primaries struct {
	Red   Chromaticity
	Green Chromaticity
	Blue  Chromaticity
}

// ColorSpace is an RGB color space defined by its primaries and white point.
type ColorSpace struct {
	Name       string
	Primaries  Primaries
	WhitePoint Chromaticity
}

var (
	acescgSpace = ColorSpace{
		Name: "ACEScg",
		Primaries: Primaries{
			Red:   Chromaticity{X: 0.713, Y: 0.293},
			Green: Chromaticity{X: 0.165, Y: 0.830},
			Blue:  Chromaticity{X: 0.128, Y: 0.044},
		},
		WhitePoint: Chromaticity{X: 0.32168, Y: 0.33767},
	}
	rec709Space = ColorSpace{
		Name: "Rec.709",
		Primaries: Primaries{
			Red:   Chromaticity{X: 0.640, Y: 0.330},
			Green: Chromaticity{X: 0.300, Y: 0.600},
			Blue:  Chromaticity{X: 0.150, Y: 0.060},
		},
		WhitePoint: Chromaticity{X: 0.3127, Y: 0.3290},
	}
)

// ACEScgColorSpace returns the ACEScg working space: AP1 primaries with the
// ACES D60 white point. The value is a copy of the canonical definition.
func ACEScgColorSpace() ColorSpace {
	return acescgSpace
}

// Rec709ColorSpace returns the Rec.709 RGB space with a D65 white point. The
// value is a copy of the canonical definition.
func Rec709ColorSpace() ColorSpace {
	return rec709Space
}

// RGBToXYZ returns the matrix that converts linear RGB in cs to CIE XYZ.
// Columns are scaled by the RGB weights that map the white point to Y=1.
func (cs ColorSpace) RGBToXYZ() (Matrix3, error) {
	r, err := cs.Primaries.Red.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("%s red primary: %w", cs.Name, err)
	}
	g, err := cs.Primaries.Green.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("%s green primary: %w", cs.Name, err)
	}
	b, err := cs.Primaries.Blue.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("%s blue primary: %w", cs.Name, err)
	}
	white, err := cs.WhitePoint.XYZ()
	if err != nil {
		return Matrix3{}, fmt.Errorf("%s white point: %w", cs.Name, err)
	}

	primaries := Matrix3{
		{r.X, g.X, b.X},
		{r.Y, g.Y, b.Y},
		{r.Z, g.Z, b.Z},
	}
	if err := primaries.validate("primaries"); err != nil {
		return Matrix3{}, err
	}
	inverse, err := primaries.Inverse()
	if err != nil {
		return Matrix3{}, err
	}
	scale := inverse.ApplyVec(white)
	if !isFinite(scale.X) || !isFinite(scale.Y) || !isFinite(scale.Z) ||
		scale.X <= 0 || scale.Y <= 0 || scale.Z <= 0 {
		return Matrix3{}, fmt.Errorf("%w: %s primaries do not enclose its white point", ErrWhitePoint, cs.Name)
	}

	return Matrix3{
		{r.X * scale.X, g.X * scale.Y, b.X * scale.Z},
		{r.Y * scale.X, g.Y * scale.Y, b.Y * scale.Z},
		{r.Z * scale.X, g.Z * scale.Y, b.Z * scale.Z},
	}, nil
}

// XYZToRGB returns the inverse of cs.RGBToXYZ.
func (cs ColorSpace) XYZToRGB() (Matrix3, error) {
	toXYZ, err := cs.RGBToXYZ()
	if err != nil {
		return Matrix3{}, err
	}
	return toXYZ.Inverse()
}

// RGBToXYZ converts a linear RGB sample in cs to CIE XYZ.
func (cs ColorSpace) RGBToXYZSample(rgb RGB) (Vec3, error) {
	m, err := cs.RGBToXYZ()
	if err != nil {
		return Vec3{}, err
	}
	return m.ApplyVec(Vec3{rgb.R, rgb.G, rgb.B}), nil
}

// XYZToRGB converts a CIE XYZ sample to linear RGB in cs.
func (cs ColorSpace) XYZToRGBSample(xyz Vec3) (RGB, error) {
	m, err := cs.XYZToRGB()
	if err != nil {
		return RGB{}, err
	}
	v := m.ApplyVec(xyz)
	return RGB{v.X, v.Y, v.Z}, nil
}
