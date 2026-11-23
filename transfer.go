package aces709

import (
	"fmt"
	"math"
)

const (
	// BT709Alpha and BT709Beta are the continuous BT.709 OETF constants.
	BT709Alpha = 1.09929682680944
	BT709Beta  = 0.018053968510807

	bt709Gamma       = 0.45
	bt709InvGamma    = 1 / bt709Gamma
	bt709LinearSlope = 4.5
	bt709Knee        = bt709LinearSlope * BT709Beta
)

// EncodeBT709 converts display-linear light to a BT.709 encoded signal.
//
// Values in [0,1] use the BT.709 OETF. Negative values are handled with an
// odd-symmetric extension so out-of-gamut intermediate channels remain
// invertible. Values above 1 are encoded as extended-range highlights. The
// quantizer is the display-range boundary and is responsible for clipping.
func EncodeBT709(linear float64) (float64, error) {
	if !isFinite(linear) {
		return 0, fmt.Errorf("%w: BT.709 encode(%g)", ErrNonFiniteValue, linear)
	}
	sign := 1.0
	if linear < 0 {
		sign = -1
		linear = -linear
	}
	if linear < BT709Beta {
		return sign * bt709LinearSlope * linear, nil
	}
	return sign * (BT709Alpha*math.Pow(linear, bt709Gamma) - (BT709Alpha - 1)), nil
}

// DecodeBT709 converts a BT.709 encoded signal back to display-linear light.
// It is the double-precision inverse of EncodeBT709, including its extended
// negative and greater-than-one ranges.
func DecodeBT709(encoded float64) (float64, error) {
	if !isFinite(encoded) {
		return 0, fmt.Errorf("%w: BT.709 decode(%g)", ErrNonFiniteValue, encoded)
	}
	sign := 1.0
	if encoded < 0 {
		sign = -1
		encoded = -encoded
	}
	if encoded < bt709Knee {
		return sign * encoded / bt709LinearSlope, nil
	}
	return sign * math.Pow((encoded+(BT709Alpha-1))/BT709Alpha, bt709InvGamma), nil
}

func encodeRGB(rgb RGB) (RGB, error) {
	r, err := EncodeBT709(rgb.R)
	if err != nil {
		return RGB{}, err
	}
	g, err := EncodeBT709(rgb.G)
	if err != nil {
		return RGB{}, err
	}
	b, err := EncodeBT709(rgb.B)
	if err != nil {
		return RGB{}, err
	}
	return RGB{R: r, G: g, B: b}, nil
}

func decodeRGB(rgb RGB) (RGB, error) {
	r, err := DecodeBT709(rgb.R)
	if err != nil {
		return RGB{}, err
	}
	g, err := DecodeBT709(rgb.G)
	if err != nil {
		return RGB{}, err
	}
	b, err := DecodeBT709(rgb.B)
	if err != nil {
		return RGB{}, err
	}
	return RGB{R: r, G: g, B: b}, nil
}
