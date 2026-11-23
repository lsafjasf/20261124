package colorxform

import (
	"errors"
	"image"
	"math"
)

// Options controls the conversion pipeline.
type Options struct {
	// ExposureStops is a photographic exposure compensation applied in
	// the scene-linear ACEScg working domain as a multiply by
	// 2^ExposureStops. It is a creative control only; it must never be
	// used to compensate for transfer-function mistakes.
	ExposureStops float64
	// OutputDepth is the output bit depth: 8 or 16.
	OutputDepth int
	// ClampOutput clamps the final display-referred Rec.709 code values
	// to [0,1] before quantization. Intermediate (scene-referred)
	// values are never clamped. Default true.
	ClampOutput bool
}

// DefaultOptions returns the standard conversion options.
func DefaultOptions() Options {
	return Options{OutputDepth: 16, ClampOutput: true}
}

// ConvertColor runs one normalized sRGB code-value triplet through the
// full pipeline: sRGB EOTF -> linear sRGB -> ACEScg -> exposure ->
// linear Rec.709 -> Rec.709 OETF. Negative and >1 values are preserved
// through every stage; clamping only happens at quantization time when
// ClampOutput is set.
func ConvertColor(r, g, b, exposureStops float64) (float64, float64, float64) {
	lr, lg, lb := SRGBDecode(r), SRGBDecode(g), SRGBDecode(b)
	ar, ag, ab := SRGBToACEScg.Apply(lr, lg, lb)
	return ConvertFromACEScg(ar, ag, ab, exposureStops)
}

// ConvertFromACEScg runs the second half of the pipeline on a
// scene-linear ACEScg triplet: exposure compensation, conversion to
// linear Rec.709, and the Rec.709 OETF. Wide-gamut inputs may produce
// negative (or >1) code values; they are preserved, not clipped.
func ConvertFromACEScg(ar, ag, ab, exposureStops float64) (float64, float64, float64) {
	if exposureStops != 0 {
		gain := math.Exp2(exposureStops)
		ar, ag, ab = ar*gain, ag*gain, ab*gain
	}
	rr, rg, rb := ACEScgToSRGB.Apply(ar, ag, ab)
	return Rec709Encode(rr), Rec709Encode(rg), Rec709Encode(rb)
}

// quantize maps a normalized code value to an n-bit integer.
func quantize(v float64, max uint32, clamp bool) uint32 {
	if clamp {
		v = math.Min(math.Max(v, 0), 1)
	}
	return uint32(math.Round(v * float64(max)))
}

// ConvertImage converts an sRGB-encoded source image to a Rec.709
// encoded image. 8-bit sources are normalized by 255, 16-bit sources
// by 65535; alpha is passed through unchanged.
func ConvertImage(src image.Image, opts Options) (image.Image, error) {
	if opts.OutputDepth != 8 && opts.OutputDepth != 16 {
		return nil, errors.New("colorxform: OutputDepth must be 8 or 16")
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if opts.OutputDepth == 8 {
		dst := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
				cr, cg, cb := ConvertColor(
					float64(r)/65535, float64(g)/65535, float64(bl)/65535,
					opts.ExposureStops)
				i := dst.PixOffset(x, y)
				dst.Pix[i+0] = uint8(quantize(cr, 255, opts.ClampOutput))
				dst.Pix[i+1] = uint8(quantize(cg, 255, opts.ClampOutput))
				dst.Pix[i+2] = uint8(quantize(cb, 255, opts.ClampOutput))
				dst.Pix[i+3] = uint8(a >> 8)
			}
		}
		return dst, nil
	}
	dst := image.NewNRGBA64(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			cr, cg, cb := ConvertColor(
				float64(r)/65535, float64(g)/65535, float64(bl)/65535,
				opts.ExposureStops)
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(quantize(cr, 65535, opts.ClampOutput) >> 8)
			dst.Pix[i+1] = uint8(quantize(cr, 65535, opts.ClampOutput))
			dst.Pix[i+2] = uint8(quantize(cg, 65535, opts.ClampOutput) >> 8)
			dst.Pix[i+3] = uint8(quantize(cg, 65535, opts.ClampOutput))
			dst.Pix[i+4] = uint8(quantize(cb, 65535, opts.ClampOutput) >> 8)
			dst.Pix[i+5] = uint8(quantize(cb, 65535, opts.ClampOutput))
			dst.Pix[i+6] = uint8(a >> 8)
			dst.Pix[i+7] = uint8(a)
		}
	}
	return dst, nil
}

// SourceDepth reports the effective bit depth of a decoded image:
// 16 for 64-bit-per-pixel types, 8 otherwise.
func SourceDepth(img image.Image) int {
	switch img.(type) {
	case *image.NRGBA64, *image.RGBA64, *image.Gray16, *image.Alpha16:
		return 16
	}
	return 8
}
