package master

import "math"

const (
	pqM1 = 2610.0 / 16384.0
	pqM2 = 2523.0 / 32.0
	pqC1 = 3424.0 / 4096.0
	pqC2 = 2413.0 / 128.0
	pqC3 = 2392.0 / 128.0
)

// BT1886Encode maps display-linear light in [0,1] to a BT.1886 code value.
// BT.1886 uses a 2.4 display gamma around a black floor; the delivery uses
// black level 0 after legal-range mapping.
func BT1886Encode(linear float64) float64 {
	return BT1886EncodeGamma(linear, DefaultSDRGamma)
}

func BT1886Decode(code float64) float64 {
	return BT1886DecodeGamma(code, DefaultSDRGamma)
}

func BT1886EncodeGamma(linear, gamma float64) float64 {
	return math.Pow(clamp(linear, 0, 1), 1/gamma)
}

func BT1886DecodeGamma(code, gamma float64) float64 {
	return math.Pow(clamp(code, 0, 1), gamma)
}

// PQEncode implements the SMPTE ST 2084 inverse EOTF. The input is absolute
// luminance in cd/m^2 and the output is normalized to [0,1].
func PQEncode(luminanceNits float64) float64 {
	l := clamp(luminanceNits, 0, 10000) / 10000.0
	lp := math.Pow(l, pqM1)
	return math.Pow((pqC1+pqC2*lp)/(1+pqC3*lp), pqM2)
}

// PQDecode implements the SMPTE ST 2084 EOTF and returns cd/m^2.
func PQDecode(code float64) float64 {
	p := math.Pow(clamp(code, 0, 1), 1/pqM2)
	num := math.Max(p-pqC1, 0)
	den := pqC2 - pqC3*p
	if den <= 0 {
		return 10000
	}
	return 10000 * math.Pow(num/den, 1/pqM1)
}

// softShoulder is continuous at knee and approaches peak asymptotically.
// It is used only above the knee; values below it are returned unchanged.
func softShoulder(value, knee, peak float64) float64 {
	if value <= knee {
		return value
	}
	span := peak - knee
	if span <= 0 {
		return peak
	}
	return knee + span*(1-math.Exp(-(value-knee)/span))
}

// SDRToneLuminance maps absolute scene luminance to a 100-nit SDR display.
// Everything through 18 nits is linear, so 18% scene gray remains 18 nits.
func SDRToneLuminance(nits float64, cfg Config) float64 {
	middleGray := MiddleGrayScene * cfg.ReferenceWhiteNits
	if nits <= middleGray {
		return max(nits, 0)
	}
	return softShoulder(nits, middleGray, cfg.SDRPeakNits)
}

// HDRToneLuminance preserves highlights through the HDR knee and rolls off
// only near the 1000-nit delivery peak.
func HDRToneLuminance(nits float64, cfg Config) float64 {
	if nits <= cfg.HDRKneeNits {
		return max(nits, 0)
	}
	return softShoulder(nits, cfg.HDRKneeNits, cfg.HDRPeakNits)
}

// QuantizeLegal10 applies a deterministic half-code ordered dither, then maps
// normalized signal to 10-bit video-limited code words (64..940).
func QuantizeLegal10(v float64, x, y int, cfg Config) uint16 {
	v = clamp(v, 0, 1)
	if cfg.Dither {
		v += bayer8(x, y) / float64(cfg.LegalWhite-cfg.LegalBlack)
		v = clamp(v, 0, 1)
	}
	span := float64(cfg.LegalWhite - cfg.LegalBlack)
	code := float64(cfg.LegalBlack) + math.Round(v*span)
	return uint16(clamp(code, float64(cfg.LegalBlack), float64(cfg.LegalWhite)))
}

// DequantizeLegal10 returns the center of a legal code-word interval.
func DequantizeLegal10(code uint16, cfg Config) float64 {
	span := float64(cfg.LegalWhite - cfg.LegalBlack)
	return clamp((float64(code)-float64(cfg.LegalBlack))/span, 0, 1)
}

var bayer8Matrix = [8][8]float64{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}

func bayer8(x, y int) float64 {
	// Center the ordered pattern around zero and keep it within one code step.
	return (bayer8Matrix[y&7][x&7]+0.5)/64.0 - 0.5
}
