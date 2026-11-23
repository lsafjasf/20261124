package aces709

import (
	"errors"
	"math"
	"testing"
)

type generatedSample struct {
	name string
	rgb  RGB
}

func generatedSamples() []generatedSample {
	samples := make([]generatedSample, 0, 128)
	for i := 0; i <= 32; i++ {
		exponent := -4 + 7*float64(i)/32
		value := math.Pow(10, exponent)
		samples = append(samples, generatedSample{
			name: "gray",
			rgb:  RGB{value, value, value},
		})
	}
	pureValues := []float64{1e-4, 0.01, 0.18, 1, 4, 16}
	for _, value := range pureValues {
		samples = append(samples,
			generatedSample{name: "red", rgb: RGB{value, 0, 0}},
			generatedSample{name: "green", rgb: RGB{0, value, 0}},
			generatedSample{name: "blue", rgb: RGB{0, 0, value}},
			generatedSample{name: "cyan", rgb: RGB{0, value, value}},
			generatedSample{name: "magenta", rgb: RGB{value, 0, value}},
			generatedSample{name: "yellow", rgb: RGB{value, value, 0}},
		)
	}
	highlights := []RGB{
		{2.5, 1.5, 0.75},
		{8, 4, 2},
		{32, 16, 8},
		{100, 50, 20},
		{1000, 500, 250},
	}
	for _, rgb := range highlights {
		samples = append(samples, generatedSample{name: "HDR highlight", rgb: rgb})
	}
	return samples
}

func TestPipelineFloatRoundTripGeneratedSamples(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	var maxError, maxRelative float64
	for _, sample := range generatedSamples() {
		encoded, err := pipeline.ACEScgToRec709(sample.rgb)
		if err != nil {
			t.Fatalf("%s %v encode: %v", sample.name, sample.rgb, err)
		}
		decoded, err := pipeline.Rec709ToACEScg(encoded)
		if err != nil {
			t.Fatalf("%s %v decode: %v", sample.name, sample.rgb, err)
		}
		absolute := maxRGBError(sample.rgb, decoded)
		scale := math.Max(1, maxAbsRGB(sample.rgb))
		relative := absolute / scale
		maxError = math.Max(maxError, absolute)
		maxRelative = math.Max(maxRelative, relative)
		if relative > 2e-13 {
			t.Errorf("%s %v round trip error %g (encoded %v)", sample.name, sample.rgb, absolute, encoded)
		}
	}
	t.Logf("float round trip: max absolute error %.3g, max scaled error %.3g", maxError, maxRelative)
}

func TestPipelineLinearInverseGeneratedSamples(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range generatedSamples() {
		linear, err := pipeline.ACEScgToRec709Linear(sample.rgb)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := pipeline.Rec709LinearToACEScg(linear)
		if err != nil {
			t.Fatal(err)
		}
		scale := math.Max(1, maxAbsRGB(sample.rgb))
		if error := maxRGBError(sample.rgb, decoded) / scale; error > 5e-14 {
			t.Errorf("%s linear round trip scaled error %g", sample.name, error)
		}
	}
}

func TestFrameConversionDoesNotMutateInput(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	src := []RGB{{0.18, 0.18, 0.18}, {2, 1, 0.5}, {0, 1, 0}}
	want := append([]RGB(nil), src...)
	encoded, err := pipeline.ConvertFrame(src)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := pipeline.InvertFrame(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for i := range src {
		if src[i] != want[i] {
			t.Fatalf("input pixel %d was mutated: %v", i, src[i])
		}
		if error := maxRGBError(want[i], decoded[i]); error > 1e-14 {
			t.Errorf("frame pixel %d inverse error %g", i, error)
		}
	}
}

func TestPipelineQuantizedGrayHalfStep(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Rec709Quantizer(10)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 100; i++ {
		value := 0.95 * float64(i) / 100
		sample := RGB{value, value, value}
		encoded, err := pipeline.ACEScgToRec709(sample)
		if err != nil {
			t.Fatal(err)
		}
		codes, err := q.EncodeRGB(encoded)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := q.DecodeRGB([3]uint16{codes[0].Code, codes[1].Code, codes[2].Code})
		if err != nil {
			t.Fatal(err)
		}
		if error := maxRGBError(encoded, decoded); error > q.Step()/2+1e-15 {
			t.Errorf("gray %g encoded quantization error %g > half step %g", value, error, q.Step()/2)
		}
	}
}

func TestPipelineCodeRoundTripGray(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Rec709Quantizer(10)
	if err != nil {
		t.Fatal(err)
	}
	maxLinearError := 0.0
	for i := 0; i <= 100; i++ {
		value := 0.95 * float64(i) / 100
		sample := RGB{value, value, value}
		codes, err := pipeline.ACEScgToRec709Codes(sample, q)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := pipeline.Rec709CodesToACEScg(
			[3]uint16{codes[0].Code, codes[1].Code, codes[2].Code}, q)
		if err != nil {
			t.Fatal(err)
		}
		maxLinearError = math.Max(maxLinearError, maxRGBError(sample, restored))
	}
	// 10-bit OETF quantization can be amplified by the inverse slope near one.
	if maxLinearError > 0.0012 {
		t.Fatalf("10-bit gray code round trip max linear error %g > 0.0012", maxLinearError)
	}
	t.Logf("10-bit gray code round trip max linear error %.3g", maxLinearError)
}

func TestPipelineQuantizationGeneratedSamples(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Rec709Quantizer(10)
	if err != nil {
		t.Fatal(err)
	}
	clippedSamples := 0
	for _, sample := range generatedSamples() {
		encoded, err := pipeline.ACEScgToRec709(sample.rgb)
		if err != nil {
			t.Fatal(err)
		}
		codes, err := q.EncodeRGB(encoded)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := q.DecodeRGB([3]uint16{codes[0].Code, codes[1].Code, codes[2].Code})
		if err != nil {
			t.Fatal(err)
		}
		if maxRGBError(encoded, decoded) > q.Step()/2+1e-15 {
			// Clipped values are checked against their clamped expectation below.
			if codes[0].Clip == ClipNone && codes[1].Clip == ClipNone && codes[2].Clip == ClipNone {
				t.Errorf("%s %v unclipped quantization error %g",
					sample.name, sample.rgb, maxRGBError(encoded, decoded))
			}
		}

		raw := [3]float64{encoded.R, encoded.G, encoded.B}
		got := [3]float64{decoded.R, decoded.G, decoded.B}
		sampleClipped := false
		for channel := 0; channel < 3; channel++ {
			wantClip := ClipNone
			want := raw[channel]
			switch {
			case raw[channel] < 0:
				wantClip = ClipLow
				want = 0
			case raw[channel] > 1:
				wantClip = ClipHigh
				want = 1
			}
			if codes[channel].Clip != wantClip {
				t.Errorf("%s %v channel %d clip = %v, want %v",
					sample.name, sample.rgb, channel, codes[channel].Clip, wantClip)
			}
			if math.Abs(got[channel]-want) > q.Step()/2+1e-15 {
				t.Errorf("%s %v channel %d decoded %g, want %g within half step",
					sample.name, sample.rgb, channel, got[channel], want)
			}
			sampleClipped = sampleClipped || wantClip != ClipNone
		}
		if sampleClipped {
			clippedSamples++
		}
	}
	if clippedSamples < 20 {
		t.Fatalf("only %d generated samples exercised display clipping", clippedSamples)
	}
	t.Logf("quantization check covered %d samples, %d with display clipping",
		len(generatedSamples()), clippedSamples)
}

func TestPipelineReportsDisplayClipping(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	q, err := Rec709Quantizer(10)
	if err != nil {
		t.Fatal(err)
	}
	tests := []generatedSample{
		{name: "ACEScg red", rgb: RGB{1, 0, 0}},
		{name: "HDR neutral", rgb: RGB{8, 8, 8}},
		{name: "HDR color", rgb: RGB{32, 16, 8}},
	}
	for _, test := range tests {
		codes, err := pipeline.ACEScgToRec709Codes(test.rgb, q)
		if err != nil {
			t.Fatal(err)
		}
		clipped := false
		for _, channel := range codes {
			clipped = clipped || channel.Clip != ClipNone
		}
		if !clipped {
			t.Errorf("%s did not report display clipping: %v", test.name, codes)
		}
	}
}

func TestPipelineRejectsWrongWhitePoint(t *testing.T) {
	config := DefaultConfig()
	config.ACEScgWhite = Rec709ColorSpace().WhitePoint
	if _, err := NewPipeline(config); !errors.Is(err, ErrWhitePoint) {
		t.Fatalf("wrong source white error = %v, want ErrWhitePoint", err)
	}

	config = DefaultConfig()
	config.Rec709White = ACEScgColorSpace().WhitePoint
	if _, err := NewPipeline(config); !errors.Is(err, ErrWhitePoint) {
		t.Fatalf("wrong destination white error = %v, want ErrWhitePoint", err)
	}
}

func TestPipelineRejectsWrongMatrixOrder(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "transposed ACEScg matrix",
			mutate: func(c *Config) {
				c.ACEScgToXYZ = c.ACEScgToXYZ.Transpose()
			},
		},
		{
			name: "inverted Rec.709 matrix",
			mutate: func(c *Config) {
				c.XYZToRec709, _ = c.XYZToRec709.Inverse()
			},
		},
		{
			name: "transposed adaptation",
			mutate: func(c *Config) {
				c.D60ToD65 = c.D60ToD65.Transpose()
			},
		},
		{
			name: "swapped space matrices",
			mutate: func(c *Config) {
				c.ACEScgToXYZ, c.XYZToRec709 = c.XYZToRec709, c.ACEScgToXYZ
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig()
			test.mutate(&config)
			if _, err := NewPipeline(config); !errors.Is(err, ErrMatrixOrder) {
				t.Fatalf("error = %v, want ErrMatrixOrder", err)
			}
		})
	}
}

func TestPipelineRejectsNonFiniteSamples(t *testing.T) {
	pipeline, err := NewDefaultPipeline()
	if err != nil {
		t.Fatal(err)
	}
	bad := RGB{R: math.NaN(), G: 0, B: 0}
	if _, err := pipeline.ACEScgToRec709(bad); !errors.Is(err, ErrNonFiniteValue) {
		t.Fatalf("non-finite input error = %v", err)
	}
}

func maxRGBError(a, b RGB) float64 {
	return math.Max(math.Abs(a.R-b.R), math.Max(math.Abs(a.G-b.G), math.Abs(a.B-b.B)))
}

func maxAbsRGB(v RGB) float64 {
	return math.Max(math.Abs(v.R), math.Max(math.Abs(v.G), math.Abs(v.B)))
}
