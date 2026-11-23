package master

import "math"

// BT.2020 RGB -> XYZ -> BT.709 RGB, with each row normalized so an
// achromatic input remains achromatic.
var rec2020ToRec709 = [3][3]float64{
	{1.660490931268, -0.587641089560, -0.072849841707},
	{-0.124550471457, 1.132899910500, -0.008349439043},
	{-0.018150779576, -0.100578893302, 1.118729672878},
}

func Luminance2020(c Vec3) float64 {
	return 0.2627*c[0] + 0.6780*c[1] + 0.0593*c[2]
}

func Luminance709(c Vec3) float64 {
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func MulMat3(m [3][3]float64, c Vec3) Vec3 {
	return Vec3{
		m[0][0]*c[0] + m[0][1]*c[1] + m[0][2]*c[2],
		m[1][0]*c[0] + m[1][1]*c[1] + m[1][2]*c[2],
		m[2][0]*c[0] + m[2][1]*c[1] + m[2][2]*c[2],
	}
}

// GamutMapToRec709 converts scene-linear Rec.2020 RGB into Rec.709 RGB and
// compresses only out-of-gamut chroma. The input luminance is preserved.
func GamutMapToRec709(c Vec3, targetLuminance float64) Vec3 {
	mapped := MulMat3(rec2020ToRec709, c)
	return FitGamut(mapped, targetLuminance, 0, 1)
}

// FitGamut desaturates toward the achromatic axis until RGB is inside
// [lo,hi]. This avoids independent channel clipping, which would shift hue and
// luminance. Values already in gamut are returned unchanged.
func FitGamut(c Vec3, targetLuminance, lo, hi float64) Vec3 {
	y := clamp(targetLuminance, lo, hi)
	if mn := c.Min(); mn < lo {
		den := y - mn
		if den > 1e-12 {
			t := (lo - mn) / den
			c = c.Add(Vec3{y, y, y}.Sub(c).Mul(t))
		}
	}
	if mx := c.Max(); mx > hi {
		den := mx - y
		if den > 1e-12 {
			t := (mx - hi) / den
			c = c.Add(Vec3{y, y, y}.Sub(c).Mul(t))
		}
	}
	return c.Clamp(lo, hi)
}

// GradeSDR produces BT.709 display-linear RGB normalized to the 100-nit SDR
// peak. The luminance shoulder is applied before the gamut conversion so skin
// hue and middle-gray luminance remain stable through highlight compression.
func GradeSDR(scene Vec3, cfg Config) Vec3 {
	yScene := max(Luminance2020(scene), 0) * cfg.ReferenceWhiteNits
	if yScene <= 1e-12 {
		return Vec3{}
	}
	mappedScene := FitGamut(scene, Luminance2020(scene), 0, math.Inf(1))
	yTarget := SDRToneLuminance(yScene, cfg)
	linear := mappedScene.Mul(yTarget / yScene)
	return GamutMapToRec709(linear, yTarget/cfg.SDRPeakNits)
}

// GradeHDR10 produces BT.2020 display-linear RGB in absolute cd/m^2.
func GradeHDR10(scene Vec3, cfg Config) Vec3 {
	yScene := max(Luminance2020(scene), 0) * cfg.ReferenceWhiteNits
	if yScene <= 1e-12 {
		return Vec3{}
	}
	clean := FitGamut(scene, Luminance2020(scene), 0, math.Inf(1))
	yTarget := HDRToneLuminance(yScene, cfg)
	sceneLum := max(Luminance2020(clean), 1e-12)
	linear := clean.Mul(yTarget / sceneLum)
	return FitGamut(linear, yTarget, 0, cfg.HDRPeakNits)
}
