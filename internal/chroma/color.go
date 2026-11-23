package chroma

import "math"

// YCbCrToRGB converts BT.601 full-range YCbCr to RGB.
func YCbCrToRGB(p YCbCr) RGB {
	cb := p.Cb - 128
	cr := p.Cr - 128
	return RGB{
		R: p.Y + 1.402*cr,
		G: p.Y - 0.344136*cb - 0.714136*cr,
		B: p.Y + 1.772*cb,
	}
}

// RGBToYCbCr converts BT.601 full-range RGB to YCbCr.
func RGBToYCbCr(p RGB) YCbCr {
	return YCbCr{
		Y:  0.299*p.R + 0.587*p.G + 0.114*p.B,
		Cb: -0.168736*p.R - 0.331264*p.G + 0.5*p.B + 128,
		Cr: 0.5*p.R - 0.418688*p.G - 0.081312*p.B + 128,
	}
}

// YUVToRGB converts a full-resolution image to RGB.
func YUVToRGB(src *YUVImage) *RGBImage {
	dst := &RGBImage{W: src.W, H: src.H, Pix: make([]RGB, len(src.Pix))}
	for i, p := range src.Pix {
		dst.Pix[i] = YCbCrToRGB(p)
	}
	return dst
}

// Combine rebuilds a YUV image from a luma plane and upsampled chroma planes.
func Combine(y, cb, cr *Plane) (*YUVImage, error) {
	if y == nil || cb == nil || cr == nil {
		return nil, ErrNilPlane
	}
	if y.W != cb.W || y.W != cr.W || y.H != cb.H || y.H != cr.H {
		return nil, ErrPlaneSize
	}
	dst := &YUVImage{W: y.W, H: y.H, Pix: make([]YCbCr, len(y.Pix))}
	for i, luma := range y.Pix {
		dst.Pix[i] = YCbCr{Y: luma, Cb: cb.Pix[i], Cr: cr.Pix[i]}
	}
	return dst, nil
}

// Clamp8 clamps and rounds a display component.
func Clamp8(v float64) uint8 {
	return uint8(math.Round(math.Max(0, math.Min(255, v))))
}
