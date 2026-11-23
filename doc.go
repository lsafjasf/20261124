// Package aces709 converts scene-linear ACEScg/AP1 samples with a D60 white
// point to BT.709/D65 and back.
//
// The conversion is split into independently testable stages:
//
//	ACEScg RGB -> XYZ(D60)          ColorSpace.RGBToXYZ
//	XYZ(D60) -> XYZ(D65)            BradfordAdaptation
//	XYZ(D65) -> linear Rec.709 RGB  ColorSpace.XYZToRGB
//	linear -> BT.709 encoded        EncodeBT709 / DecodeBT709
//	encoded -> integer codes        Quantizer.Encode / Decode
//
// Pipeline performs the stage composition with float64 and precomputes an
// explicit inverse matrix chain. NewPipeline rejects transposed, inverted, or
// swapped matrices and mismatched white points instead of silently producing
// wrong colors.
//
// BT.709 encoding above 1 and below 0 uses an odd-symmetric extended range so
// wide-gamut and HDR intermediates remain invertible. Display quantization is
// the point where values are clipped and a Clip flag is reported.
package aces709
