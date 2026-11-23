package master

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
)

const (
	ToolVersion          = "1.0.0"
	DefaultReference     = 100.0
	DefaultSDRPeak       = 100.0
	DefaultHDRPeak       = 1000.0
	DefaultHDRKnee       = 900.0
	DefaultSDRGamma      = 2.4
	DefaultLegalBlack    = 64
	DefaultLegalWhite    = 940
	DefaultMasterBits    = 10
	MiddleGrayScene      = 0.18
	MiddleGrayTargetNits = MiddleGrayScene * DefaultReference
)

type Vec3 [3]float64

func (v Vec3) Add(o Vec3) Vec3    { return Vec3{v[0] + o[0], v[1] + o[1], v[2] + o[2]} }
func (v Vec3) Sub(o Vec3) Vec3    { return Vec3{v[0] - o[0], v[1] - o[1], v[2] - o[2]} }
func (v Vec3) Mul(s float64) Vec3 { return Vec3{v[0] * s, v[1] * s, v[2] * s} }
func (v Vec3) Min() float64       { return min(v[0], min(v[1], v[2])) }
func (v Vec3) Max() float64       { return max(v[0], max(v[1], v[2])) }
func (v Vec3) Clamp(lo, hi float64) Vec3 {
	return Vec3{clamp(v[0], lo, hi), clamp(v[1], lo, hi), clamp(v[2], lo, hi)}
}

// Rect uses a top-left origin and integer pixel coordinates.
type Rect struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Anchor string `json:"anchor,omitempty"`
}

func (r Rect) Valid(w, h int) bool {
	return r.Width > 0 && r.Height > 0 &&
		r.X >= 0 && r.Y >= 0 && r.X+r.Width <= w && r.Y+r.Height <= h
}

// Frame is one scene-linear source frame. Pixels are stored in top-down,
// left-to-right order and use Rec.2020 primaries with a D65 white point.
type Frame struct {
	Width       int
	Height      int
	Shot        string
	FrameNumber int
	Subtitle    Rect
	Pixels      []Vec3
}

func (f *Frame) Validate() error {
	if f.Width <= 0 || f.Height <= 0 {
		return fmt.Errorf("invalid frame dimensions %dx%d", f.Width, f.Height)
	}
	if len(f.Pixels) != f.Width*f.Height {
		return fmt.Errorf("pixel count %d does not match %dx%d", len(f.Pixels), f.Width, f.Height)
	}
	if err := ValidateShotID(f.Shot); err != nil {
		return err
	}
	if f.FrameNumber <= 0 {
		return fmt.Errorf("frame number must be positive")
	}
	if f.Subtitle.Width == 0 && f.Subtitle.Height == 0 {
		return nil
	}
	if !f.Subtitle.Valid(f.Width, f.Height) {
		return fmt.Errorf("subtitle rectangle is outside the frame: %+v", f.Subtitle)
	}
	return nil
}

func (f *Frame) At(x, y int) Vec3 {
	return f.Pixels[y*f.Width+x]
}

var shotIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func ValidateShotID(shot string) error {
	if !shotIDPattern.MatchString(shot) {
		return errors.New("shot id must contain only letters, digits, dot, underscore or dash")
	}
	if filepath.Base(shot) != shot {
		return errors.New("shot id must not contain a path")
	}
	return nil
}

type Config struct {
	ReferenceWhiteNits float64 `json:"reference_white_nits"`
	SDRPeakNits        float64 `json:"sdr_peak_nits"`
	HDRPeakNits        float64 `json:"hdr_peak_nits"`
	HDRKneeNits        float64 `json:"hdr_knee_nits"`
	SDRGamma           float64 `json:"sdr_gamma"`
	LegalBlack         uint16  `json:"legal_black"`
	LegalWhite         uint16  `json:"legal_white"`
	Bits               int     `json:"bits"`
	Dither             bool    `json:"dither"`
	ditherSet          bool
}

func DefaultConfig() Config {
	return Config{
		ReferenceWhiteNits: DefaultReference,
		SDRPeakNits:        DefaultSDRPeak,
		HDRPeakNits:        DefaultHDRPeak,
		HDRKneeNits:        DefaultHDRKnee,
		SDRGamma:           DefaultSDRGamma,
		LegalBlack:         DefaultLegalBlack,
		LegalWhite:         DefaultLegalWhite,
		Bits:               DefaultMasterBits,
		Dither:             true,
	}
}

// UnmarshalJSON distinguishes an omitted dither flag (keep the default) from
// an explicit false value while preserving the simple bool API.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var presence struct {
		Dither *bool `json:"dither"`
	}
	if err := json.Unmarshal(data, &presence); err != nil {
		return err
	}
	*c = Config(decoded)
	if presence.Dither != nil {
		c.Dither = *presence.Dither
		c.ditherSet = true
	}
	return nil
}

func (c Config) Validate() error {
	if c.ReferenceWhiteNits <= 0 || c.SDRPeakNits <= 0 || c.HDRPeakNits <= 0 {
		return errors.New("peak and reference luminance values must be positive")
	}
	middleGray := MiddleGrayScene * c.ReferenceWhiteNits
	if c.SDRPeakNits < middleGray {
		return errors.New("SDR peak must not be below middle gray")
	}
	if c.HDRKneeNits < middleGray || c.HDRKneeNits >= c.HDRPeakNits {
		return errors.New("HDR knee must be between middle gray and HDR peak")
	}
	if c.SDRGamma <= 0 {
		return errors.New("SDR gamma must be positive")
	}
	if c.Bits != 10 {
		return errors.New("this delivery profile requires 10-bit code words")
	}
	if c.LegalBlack >= c.LegalWhite || c.LegalWhite > (1<<c.Bits)-1 {
		return errors.New("invalid legal range")
	}
	return nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
