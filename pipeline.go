package aces709

import (
	"fmt"
	"math"
)

const validationTolerance = 1e-9

// Config contains every matrix in the ACEScg -> Rec.709 path. Each field is
// directional and is validated against the canonical ACEScg and Rec.709
// definitions before a Pipeline is built.
type Config struct {
	ACEScgToXYZ Matrix3
	XYZToRec709 Matrix3
	D60ToD65    Matrix3
	ACEScgWhite Chromaticity
	Rec709White Chromaticity
}

// DefaultConfig returns the canonical AP1/D60 -> XYZ -> Bradford D60/D65 ->
// Rec.709 matrix chain.
func DefaultConfig() Config {
	config, err := canonicalConfig()
	if err != nil {
		panic(fmt.Sprintf("aces709: invalid built-in color science constants: %v", err))
	}
	return config
}

func canonicalConfig() (Config, error) {
	acescgToXYZ, err := acescgSpace.RGBToXYZ()
	if err != nil {
		return Config{}, err
	}
	xyzToRec709, err := rec709Space.XYZToRGB()
	if err != nil {
		return Config{}, err
	}
	adaptation, err := BradfordAdaptation(acescgSpace.WhitePoint, rec709Space.WhitePoint)
	if err != nil {
		return Config{}, err
	}
	return Config{
		ACEScgToXYZ: acescgToXYZ,
		XYZToRec709: xyzToRec709,
		D60ToD65:    adaptation,
		ACEScgWhite: acescgSpace.WhitePoint,
		Rec709White: rec709Space.WhitePoint,
	}, nil
}

// Pipeline converts scene-linear ACEScg samples to BT.709 and back. All matrix
// arithmetic uses float64.
type Pipeline struct {
	config       Config
	sourceToDest Matrix3
	destToSource Matrix3
}

// NewPipeline validates config and precomputes the forward and reverse matrix
// chain. Transposed matrices, inverted matrices, swapped spaces, and wrong
// white points fail construction instead of producing silently incorrect
// images.
func NewPipeline(config Config) (*Pipeline, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}

	sourceToDest := config.XYZToRec709.Mul(config.D60ToD65).Mul(config.ACEScgToXYZ)
	if err := sourceToDest.validate("ACEScg to Rec.709"); err != nil {
		return nil, err
	}
	destToSource, err := sourceToDest.Inverse()
	if err != nil {
		return nil, err
	}
	return &Pipeline{
		config:       config,
		sourceToDest: sourceToDest,
		destToSource: destToSource,
	}, nil
}

// ValidateConfig checks that every matrix and white point is canonical for
// ACEScg/AP1/D60 and Rec.709/BT.709/D65. It returns ErrWhitePoint or
// ErrMatrixOrder for the two most common wiring mistakes.
func ValidateConfig(config Config) error {
	canonical, err := canonicalConfig()
	if err != nil {
		return err
	}
	return validateConfig(config, canonical)
}

// NewDefaultPipeline builds the canonical ACEScg -> Rec.709 pipeline.
func NewDefaultPipeline() (*Pipeline, error) {
	return NewPipeline(DefaultConfig())
}

func validateConfig(got, want Config) error {
	if !closeChromaticity(got.ACEScgWhite, want.ACEScgWhite, validationTolerance) {
		return fmt.Errorf("%w: ACEScg white is (%g,%g), want D60 (%g,%g)",
			ErrWhitePoint, got.ACEScgWhite.X, got.ACEScgWhite.Y,
			want.ACEScgWhite.X, want.ACEScgWhite.Y)
	}
	if !closeChromaticity(got.Rec709White, want.Rec709White, validationTolerance) {
		return fmt.Errorf("%w: Rec.709 white is (%g,%g), want D65 (%g,%g)",
			ErrWhitePoint, got.Rec709White.X, got.Rec709White.Y,
			want.Rec709White.X, want.Rec709White.Y)
	}

	if err := validateMatrixField("ACEScgToXYZ", got.ACEScgToXYZ, want.ACEScgToXYZ,
		[]Matrix3{want.XYZToRec709, want.D60ToD65}); err != nil {
		return err
	}
	if err := validateMatrixField("XYZToRec709", got.XYZToRec709, want.XYZToRec709,
		[]Matrix3{want.ACEScgToXYZ, want.D60ToD65}); err != nil {
		return err
	}
	if err := validateMatrixField("D60ToD65", got.D60ToD65, want.D60ToD65,
		[]Matrix3{want.ACEScgToXYZ, want.XYZToRec709}); err != nil {
		return err
	}

	sourceWhite, err := got.ACEScgWhite.XYZ()
	if err != nil {
		return err
	}
	destWhite, err := got.Rec709White.XYZ()
	if err != nil {
		return err
	}
	if mapped := got.ACEScgToXYZ.ApplyVec(Vec3{1, 1, 1}); !closeVec(mapped, sourceWhite, 1e-8) {
		return fmt.Errorf("%w: ACEScg matrix does not map (1,1,1) to D60 white", ErrWhitePoint)
	}
	if mapped := got.D60ToD65.ApplyVec(sourceWhite); !closeVec(mapped, destWhite, 1e-8) {
		return fmt.Errorf("%w: adaptation does not map D60 white to D65", ErrWhitePoint)
	}
	if mapped := got.XYZToRec709.ApplyVec(destWhite); !closeVec(mapped, Vec3{1, 1, 1}, 1e-8) {
		return fmt.Errorf("%w: Rec.709 matrix does not map D65 white to (1,1,1)", ErrWhitePoint)
	}
	return nil
}

func validateMatrixField(name string, got, want Matrix3, otherCanonical []Matrix3) error {
	if err := got.validate(name); err != nil {
		return err
	}
	if closeMatrix(got, want, validationTolerance) {
		return nil
	}
	if closeMatrix(got, want.Transpose(), validationTolerance) {
		return fmt.Errorf("%w: %s is transposed; this API uses column vectors", ErrMatrixOrder, name)
	}
	inverse, err := want.Inverse()
	if err == nil && closeMatrix(got, inverse, validationTolerance) {
		return fmt.Errorf("%w: %s is inverted; use the documented direction", ErrMatrixOrder, name)
	}
	for _, other := range otherCanonical {
		if closeMatrix(got, other, validationTolerance) {
			return fmt.Errorf("%w: %s contains a matrix from another stage", ErrMatrixOrder, name)
		}
		if inverse, err := other.Inverse(); err == nil && closeMatrix(got, inverse, validationTolerance) {
			return fmt.Errorf("%w: %s contains an inverted matrix from another stage", ErrMatrixOrder, name)
		}
	}
	return fmt.Errorf("%w: %s does not match its canonical color space (max delta %.3g)",
		ErrInvalidMatrix, name, maxMatrixDelta(got, want))
}

func closeChromaticity(a, b Chromaticity, tolerance float64) bool {
	return math.Abs(a.X-b.X) <= tolerance && math.Abs(a.Y-b.Y) <= tolerance
}

func closeVec(a, b Vec3, tolerance float64) bool {
	return math.Abs(a.X-b.X) <= tolerance &&
		math.Abs(a.Y-b.Y) <= tolerance &&
		math.Abs(a.Z-b.Z) <= tolerance
}

// ACEScgToRec709Linear converts scene-linear ACEScg to display-linear Rec.709.
func (p *Pipeline) ACEScgToRec709Linear(rgb RGB) (RGB, error) {
	if err := validateRGB(rgb); err != nil {
		return RGB{}, err
	}
	out := p.sourceToDest.ApplyVec(Vec3{rgb.R, rgb.G, rgb.B})
	if !isFinite(out.X) || !isFinite(out.Y) || !isFinite(out.Z) {
		return RGB{}, fmt.Errorf("%w: matrix product produced a non-finite value", ErrNonFiniteValue)
	}
	return RGB{out.X, out.Y, out.Z}, nil
}

// Rec709LinearToACEScg is the double-precision inverse of
// ACEScgToRec709Linear.
func (p *Pipeline) Rec709LinearToACEScg(rgb RGB) (RGB, error) {
	if err := validateRGB(rgb); err != nil {
		return RGB{}, err
	}
	out := p.destToSource.ApplyVec(Vec3{rgb.R, rgb.G, rgb.B})
	if !isFinite(out.X) || !isFinite(out.Y) || !isFinite(out.Z) {
		return RGB{}, fmt.Errorf("%w: inverse matrix product produced a non-finite value", ErrNonFiniteValue)
	}
	return RGB{out.X, out.Y, out.Z}, nil
}

// ACEScgToRec709 converts ACEScg to BT.709-encoded RGB. The result may contain
// negative channels or values above one for out-of-gamut or HDR inputs.
func (p *Pipeline) ACEScgToRec709(rgb RGB) (RGB, error) {
	linear, err := p.ACEScgToRec709Linear(rgb)
	if err != nil {
		return RGB{}, err
	}
	return encodeRGB(linear)
}

// Rec709ToACEScg inverts ACEScgToRec709 in float64.
func (p *Pipeline) Rec709ToACEScg(encoded RGB) (RGB, error) {
	linear, err := decodeRGB(encoded)
	if err != nil {
		return RGB{}, err
	}
	return p.Rec709LinearToACEScg(linear)
}

// ACEScgToRec709Codes converts ACEScg through BT.709 and then to integer code
// values. Clip flags identify display-range clipping.
func (p *Pipeline) ACEScgToRec709Codes(rgb RGB, q Quantizer) ([3]Quantized, error) {
	encoded, err := p.ACEScgToRec709(rgb)
	if err != nil {
		return [3]Quantized{}, err
	}
	return q.EncodeRGB(encoded)
}

// Rec709CodesToACEScg dequantizes integer code values and applies the inverse
// BT.709 and color transforms.
func (p *Pipeline) Rec709CodesToACEScg(codes [3]uint16, q Quantizer) (RGB, error) {
	encoded, err := q.DecodeRGB(codes)
	if err != nil {
		return RGB{}, err
	}
	return p.Rec709ToACEScg(encoded)
}

// ConvertFrame converts a frame of scene-linear ACEScg samples to BT.709-encoded
// RGB without modifying the input.
func (p *Pipeline) ConvertFrame(src []RGB) ([]RGB, error) {
	out := make([]RGB, len(src))
	for i, rgb := range src {
		converted, err := p.ACEScgToRec709(rgb)
		if err != nil {
			return nil, fmt.Errorf("pixel %d: %w", i, err)
		}
		out[i] = converted
	}
	return out, nil
}

// InvertFrame converts BT.709-encoded RGB back to scene-linear ACEScg.
func (p *Pipeline) InvertFrame(src []RGB) ([]RGB, error) {
	out := make([]RGB, len(src))
	for i, rgb := range src {
		converted, err := p.Rec709ToACEScg(rgb)
		if err != nil {
			return nil, fmt.Errorf("pixel %d: %w", i, err)
		}
		out[i] = converted
	}
	return out, nil
}

func validateRGB(rgb RGB) error {
	if !isFinite(rgb.R) || !isFinite(rgb.G) || !isFinite(rgb.B) {
		return fmt.Errorf("%w: (%g,%g,%g)", ErrNonFiniteValue, rgb.R, rgb.G, rgb.B)
	}
	return nil
}
