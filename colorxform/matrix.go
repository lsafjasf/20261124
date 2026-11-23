package colorxform

// Mat3 is a row-major 3x3 color matrix.
type Mat3 [9]float64

// Mul returns a*b (apply b first, then a).
func (a Mat3) Mul(b Mat3) Mat3 {
	var r Mat3
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				r[i*3+j] += a[i*3+k] * b[k*3+j]
			}
		}
	}
	return r
}

// Apply transforms an RGB triplet.
func (m Mat3) Apply(r, g, b float64) (float64, float64, float64) {
	return m[0]*r + m[1]*g + m[2]*b,
		m[3]*r + m[4]*g + m[5]*b,
		m[6]*r + m[7]*g + m[8]*b
}

// Invert returns the matrix inverse.
func (m Mat3) Invert() Mat3 {
	a, b, c := m[0], m[1], m[2]
	d, e, f := m[3], m[4], m[5]
	g, h, i := m[6], m[7], m[8]
	A := e*i - f*h
	B := -(d*i - f*g)
	C := d*h - e*g
	det := a*A + b*B + c*C
	inv := Mat3{
		A, -(b*i - c*h), b*f - c*e,
		B, a*i - c*g, -(a*f - c*d),
		C, -(a*h - b*g), a*e - b*d,
	}
	for k := range inv {
		inv[k] /= det
	}
	return inv
}

// sRGB / Rec.709 primaries (D65) to CIE XYZ.
var srgbToXYZ = Mat3{
	0.41239079926595934, 0.357584339383878, 0.1804807884018343,
	0.21263900587151027, 0.715168678767756, 0.07219231536073371,
	0.01933081871559182, 0.11919477979462598, 0.9505321522496607,
}

// ACES AP1 primaries (D60) to CIE XYZ, per the ACEScg specification
// (ACES white point from SMPTE ST 2065-1, AP1 primaries from ACES
// TB-2014-004).
var ap1ToXYZ = Mat3{
	0.6624541811085053, 0.1340042064564331, 0.1561876870049078,
	0.27222871678091454, 0.6740817658081484, 0.05368951740793705,
	-0.005574649490394108, 0.004060733528982568, 1.0103391003129971,
}

// bradford is the Bradford cone-response matrix used for chromatic
// adaptation.
var bradford = Mat3{
	0.8951, 0.2664, -0.1614,
	-0.7502, 1.7135, 0.0367,
	0.0389, -0.0685, 1.0296,
}

// whiteXYZ returns the XYZ tristimulus of a white point given as
// chromaticity coordinates, with Y normalized to 1.
func whiteXYZ(x, y float64) (float64, float64, float64) {
	return x / y, 1, (1 - x - y) / y
}

// catMatrix builds a Bradford chromatic-adaptation transform that maps
// the source white (xy) onto the destination white (xy).
func catMatrix(srcX, srcY, dstX, dstY float64) Mat3 {
	sx, sy, sz := whiteXYZ(srcX, srcY)
	dx, dy, dz := whiteXYZ(dstX, dstY)
	sr, sg, sb := bradford.Apply(sx, sy, sz)
	dr, dg, db := bradford.Apply(dx, dy, dz)
	scale := Mat3{
		dr / sr, 0, 0,
		0, dg / sg, 0,
		0, 0, db / sb,
	}
	return bradford.Invert().Mul(scale).Mul(bradford)
}

// catD65toD60 adapts sRGB/Rec.709 white (D65, xy 0.3127/0.3290) to the
// ACES white point (D60, xy 0.32168/0.33767 per SMPTE ST 2065-1).
var catD65toD60 = catMatrix(0.3127, 0.3290, 0.32168, 0.33767)

var (
	xyzToSRGB   = srgbToXYZ.Invert()
	xyzToAP1    = ap1ToXYZ.Invert()
	catD60toD65 = catD65toD60.Invert()

	// SRGBToACEScg converts linear sRGB (D65) to ACEScg (AP1, D60).
	SRGBToACEScg = xyzToAP1.Mul(catD65toD60).Mul(srgbToXYZ)
	// ACEScgToSRGB converts ACEScg back to linear sRGB/Rec.709 (D65).
	ACEScgToSRGB = xyzToSRGB.Mul(catD60toD65).Mul(ap1ToXYZ)
)
