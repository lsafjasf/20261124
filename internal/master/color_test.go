package master

import (
	"math"
	"testing"
)

func TestPQAndBT1886RoundTrip(t *testing.T) {
	for _, nits := range []float64{0.01, 1, 18, 100, 800, 1000} {
		code := PQEncode(nits)
		got := PQDecode(code)
		if math.Abs(got-nits) > nits*1e-9+1e-6 {
			t.Fatalf("PQ round trip %.6f -> %.8f -> %.6f", nits, code, got)
		}
	}
	if got := PQEncode(100); math.Abs(got-0.508078) > 5e-7 {
		t.Fatalf("PQ(100 nit) = %.9f, want about 0.508078", got)
	}
	for _, signal := range []float64{0, 0.01, 0.18, 0.5, 1} {
		got := BT1886Decode(BT1886Encode(signal))
		if math.Abs(got-signal) > 1e-12 {
			t.Fatalf("BT.1886 round trip %f -> %f", signal, got)
		}
	}
}

func TestMiddleGrayMatchesAcrossDeliverables(t *testing.T) {
	f := solidFrame(16, 16, Vec3{0.18, 0.18, 0.18})
	cfg := DefaultConfig()
	sdr, hdr, _, err := EncodeFrame(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := Rect{X: 0, Y: 0, Width: 16, Height: 16}
	sm, err := MeasureRegion(sdr, r, "SDR", cfg)
	if err != nil {
		t.Fatal(err)
	}
	hm, err := MeasureRegion(hdr, r, "HDR10", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sm.MeanNits-MiddleGrayTargetNits) > 0.01 {
		t.Fatalf("SDR middle gray %.4f nit", sm.MeanNits)
	}
	if math.Abs(hm.MeanNits-MiddleGrayTargetNits) > 0.01 {
		t.Fatalf("HDR middle gray %.4f nit", hm.MeanNits)
	}
	if math.Abs(sm.MeanNits-hm.MeanNits) > 0.01 {
		t.Fatalf("middle gray delta %.4f nit", math.Abs(sm.MeanNits-hm.MeanNits))
	}
}

func TestHDRKeepsHighlightSteps(t *testing.T) {
	const w, h = 16, 8
	f := &Frame{Width: w, Height: h, Shot: "S001", FrameNumber: 1, Pixels: make([]Vec3, w*h)}
	levels := []float64{1, 2, 4, 8}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := levels[x/4]
			f.Pixels[y*w+x] = Vec3{v, v, v}
		}
	}
	analysis := &FrameAnalysis{}
	for i, level := range levels {
		analysis.Highlights = append(analysis.Highlights, HighlightRegion{
			Name: "step", ExpectedNits: level * 100,
			Rect: Rect{X: i * 4, Y: 0, Width: 4, Height: h},
		})
	}
	cfg := DefaultConfig()
	sdr, hdr, stats, err := EncodeFrame(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	report, err := AnalyzeFrame(f.Shot, f.FrameNumber, analysis, sdr, hdr, stats, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range report.Highlights {
		want := levels[i] * 100
		if math.Abs(got.HDR10.MeanNits-want) > want*0.01 {
			t.Fatalf("HDR step %d = %.2f nit, want %.2f", i, got.HDR10.MeanNits, want)
		}
	}
	band := report.HighlightBand
	if band == nil || band.HDRDistinct != 4 || band.SDRDistinct != 4 {
		t.Fatalf("distinct levels: %+v", band)
	}
	if band.HDRMinStep <= band.SDRMinStep {
		t.Fatalf("HDR step %d is not wider than SDR step %d", band.HDRMinStep, band.SDRMinStep)
	}
}

func TestAllCodesStayInLegalRange(t *testing.T) {
	f := solidFrame(8, 8, Vec3{0.18, 0.18, 0.18})
	values := []Vec3{
		{-0.2, 0.5, 0.3}, {0, 0, 0}, {1, 1, 1},
		{8, 8, 8}, {100, 50, 25}, {0.2, 0.01, 0.001},
		{0.18, 0.18, 0.18}, {2, 0.1, 0.05}, {10, 10, 10},
		{0.01, 0.3, 0.01}, {0.5, 0.5, 0.01}, {0.001, 0.001, 1},
	}
	for i, v := range values {
		f.Pixels[i] = v
	}
	cfg := DefaultConfig()
	sdr, hdr, _, err := EncodeFrame(f, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []*Master{sdr, hdr} {
		for i, code := range m.Codes {
			if code < cfg.LegalBlack || code > cfg.LegalWhite {
				t.Fatalf("word %d code %d outside %d..%d", i, code, cfg.LegalBlack, cfg.LegalWhite)
			}
		}
	}
}

func TestGamutMapPreservesNeutralAndLuminance(t *testing.T) {
	neutral := GamutMapToRec709(Vec3{0.18, 0.18, 0.18}, 0.18)
	for c, v := range neutral {
		if math.Abs(v-0.18) > 1e-9 {
			t.Fatalf("neutral channel %d = %.12f", c, v)
		}
	}
	in := Vec3{-0.1, 0.6, 0.2}
	target := Luminance709(in)
	out := FitGamut(in, target, 0, 1)
	if out.Min() < 0 {
		t.Fatalf("negative channel survived: %+v", out)
	}
	if math.Abs(Luminance709(out)-target) > 1e-9 {
		t.Fatalf("luminance changed: %.12f -> %.12f", target, Luminance709(out))
	}
}

func solidFrame(w, h int, v Vec3) *Frame {
	f := &Frame{Width: w, Height: h, Shot: "S001", FrameNumber: 1, Pixels: make([]Vec3, w*h)}
	for i := range f.Pixels {
		f.Pixels[i] = v
	}
	return f
}
