package master

import (
	"fmt"
	"math"
)

type RegionMeasurement struct {
	MeanCode    float64 `json:"mean_code"`
	MinCode     uint16  `json:"min_code"`
	MaxCode     uint16  `json:"max_code"`
	MeanNits    float64 `json:"mean_nits"`
	MinNits     float64 `json:"min_nits"`
	MaxNits     float64 `json:"max_nits"`
	AtBlack     bool    `json:"at_black"`
	AtWhite     bool    `json:"at_white"`
	SampleCount int     `json:"sample_count"`
}

type PairMeasurement struct {
	SDR       RegionMeasurement `json:"sdr"`
	HDR10     RegionMeasurement `json:"hdr10"`
	DeltaNits float64           `json:"delta_nits"`
}

type HighlightMeasurement struct {
	Name           string            `json:"name"`
	ExpectedNits   float64           `json:"expected_nits,omitempty"`
	SDR            RegionMeasurement `json:"sdr"`
	HDR10          RegionMeasurement `json:"hdr10"`
	HDRSourceError float64           `json:"hdr_source_error_nits,omitempty"`
}

type HighlightResolution struct {
	HDRCodes    []uint16 `json:"hdr_codes"`
	SDRCodes    []uint16 `json:"sdr_codes"`
	HDRDistinct int      `json:"hdr_distinct_levels"`
	SDRDistinct int      `json:"sdr_distinct_levels"`
	HDRMinStep  int      `json:"hdr_min_step_codes"`
	SDRMinStep  int      `json:"sdr_min_step_codes"`
}

type FrameValidation struct {
	Shot          string                 `json:"shot"`
	Frame         int                    `json:"frame"`
	MiddleGray    *PairMeasurement       `json:"middle_gray,omitempty"`
	Skin          *PairMeasurement       `json:"skin,omitempty"`
	Highlights    []HighlightMeasurement `json:"highlights,omitempty"`
	HighlightBand *HighlightResolution   `json:"highlight_resolution,omitempty"`
	HDRMaxCLLNits float64                `json:"hdr_max_cll_nits"`
	HDRFALLNits   float64                `json:"hdr_fall_nits"`
	LegalRangeOK  bool                   `json:"legal_range_ok"`
}

type ValidationSummary struct {
	Frames                    int     `json:"frames"`
	LegalRangeOK              bool    `json:"legal_range_ok"`
	MaxMiddleGrayDeltaNits    float64 `json:"max_middle_gray_delta_nits"`
	MiddleGrayConsistent      bool    `json:"middle_gray_consistent"`
	HDRMaxCLLNits             float64 `json:"hdr_max_cll_nits"`
	HDRMaxFALLNits            float64 `json:"hdr_max_fall_nits"`
	BestHDRHighlightMinStep   int     `json:"best_hdr_highlight_min_step_codes"`
	SDRHighlightMinStep       int     `json:"sdr_highlight_min_step_codes"`
	HDRRetainsHighlightDetail bool    `json:"hdr_retains_highlight_detail"`
}

type ValidationReport struct {
	Version int               `json:"version"`
	Project string            `json:"project"`
	Source  string            `json:"source_manifest"`
	Summary ValidationSummary `json:"summary"`
	Frames  []FrameValidation `json:"frames"`
}

// MeasureRegion decodes master code words back to absolute display
// luminance. Averaging happens after transfer decoding, not in code space.
func MeasureRegion(m *Master, r Rect, profile string, cfg Config) (RegionMeasurement, error) {
	if !r.Valid(m.Width, m.Height) {
		return RegionMeasurement{}, fmt.Errorf("measurement rectangle outside %dx%d frame: %+v", m.Width, m.Height, r)
	}
	var sumCode [3]float64
	var sumNits float64
	first := true
	out := RegionMeasurement{SampleCount: r.Width * r.Height}
	for y := r.Y; y < r.Y+r.Height; y++ {
		for x := r.X; x < r.X+r.Width; x++ {
			codes := m.Pixel(x, y)
			var nits [3]float64
			for c := 0; c < 3; c++ {
				sumCode[c] += float64(codes[c])
				signal := DequantizeLegal10(codes[c], cfg)
				switch profile {
				case "SDR":
					nits[c] = BT1886DecodeGamma(signal, cfg.SDRGamma) * cfg.SDRPeakNits
				case "HDR10":
					nits[c] = PQDecode(signal)
				default:
					return RegionMeasurement{}, fmt.Errorf("unknown profile %q", profile)
				}
				if codes[c] == cfg.LegalBlack {
					out.AtBlack = true
				}
				if codes[c] == cfg.LegalWhite {
					out.AtWhite = true
				}
				if first {
					out.MinCode, out.MaxCode = codes[c], codes[c]
					out.MinNits, out.MaxNits = nits[c], nits[c]
				} else {
					out.MinCode = min(out.MinCode, codes[c])
					out.MaxCode = max(out.MaxCode, codes[c])
					out.MinNits = min(out.MinNits, nits[c])
					out.MaxNits = max(out.MaxNits, nits[c])
				}
			}
			yValue := Luminance2020(Vec3{nits[0], nits[1], nits[2]})
			if profile == "SDR" {
				yValue = Luminance709(Vec3{nits[0], nits[1], nits[2]})
			}
			sumNits += yValue
			out.MinNits = min(out.MinNits, yValue)
			out.MaxNits = max(out.MaxNits, yValue)
			first = false
		}
	}
	n := float64(out.SampleCount)
	out.MeanCode = (sumCode[0] + sumCode[1] + sumCode[2]) / (3 * n)
	out.MeanNits = sumNits / n
	return out, nil
}

func AnalyzeFrame(shot string, frameNo int, analysis *FrameAnalysis, sdr, hdr *Master, stats HDRStats, cfg Config) (*FrameValidation, error) {
	if analysis == nil {
		return &FrameValidation{
			Shot: shot, Frame: frameNo, HDRMaxCLLNits: stats.MaxCLLNits,
			HDRFALLNits: stats.FALLNits, LegalRangeOK: true,
		}, nil
	}
	out := &FrameValidation{
		Shot: shot, Frame: frameNo, HDRMaxCLLNits: stats.MaxCLLNits,
		HDRFALLNits: stats.FALLNits, LegalRangeOK: true,
	}
	if analysis.MiddleGray != nil {
		pair, err := measurePair(sdr, hdr, *analysis.MiddleGray, cfg)
		if err != nil {
			return nil, err
		}
		out.MiddleGray = &pair
	}
	if analysis.Skin != nil {
		pair, err := measurePair(sdr, hdr, *analysis.Skin, cfg)
		if err != nil {
			return nil, err
		}
		out.Skin = &pair
	}
	for _, region := range analysis.Highlights {
		sdrM, err := MeasureRegion(sdr, region.Rect, "SDR", cfg)
		if err != nil {
			return nil, err
		}
		hdrM, err := MeasureRegion(hdr, region.Rect, "HDR10", cfg)
		if err != nil {
			return nil, err
		}
		hm := HighlightMeasurement{Name: region.Name, ExpectedNits: region.ExpectedNits, SDR: sdrM, HDR10: hdrM}
		if region.ExpectedNits > 0 {
			hm.HDRSourceError = math.Abs(hdrM.MeanNits - region.ExpectedNits)
		}
		out.Highlights = append(out.Highlights, hm)
	}
	if len(out.Highlights) > 1 {
		out.HighlightBand = highlightResolution(out.Highlights)
	}
	return out, nil
}

func measurePair(sdr, hdr *Master, r Rect, cfg Config) (PairMeasurement, error) {
	sm, err := MeasureRegion(sdr, r, "SDR", cfg)
	if err != nil {
		return PairMeasurement{}, err
	}
	hm, err := MeasureRegion(hdr, r, "HDR10", cfg)
	if err != nil {
		return PairMeasurement{}, err
	}
	return PairMeasurement{SDR: sm, HDR10: hm, DeltaNits: math.Abs(sm.MeanNits - hm.MeanNits)}, nil
}

func highlightResolution(hs []HighlightMeasurement) *HighlightResolution {
	out := &HighlightResolution{
		HDRMinStep: 1 << 30,
		SDRMinStep: 1 << 30,
	}
	prevHDR := -1
	prevSDR := -1
	for _, h := range hs {
		hdrCode := int(math.Round(h.HDR10.MeanCode))
		sdrCode := int(math.Round(h.SDR.MeanCode))
		out.HDRCodes = append(out.HDRCodes, uint16(hdrCode))
		out.SDRCodes = append(out.SDRCodes, uint16(sdrCode))
		if prevHDR >= 0 {
			step := hdrCode - prevHDR
			out.HDRMinStep = min(out.HDRMinStep, step)
			if hdrCode != prevHDR {
				out.HDRDistinct++
			}
		} else {
			out.HDRDistinct = 1
		}
		if prevSDR >= 0 {
			step := sdrCode - prevSDR
			out.SDRMinStep = min(out.SDRMinStep, step)
			if sdrCode != prevSDR {
				out.SDRDistinct++
			}
		} else {
			out.SDRDistinct = 1
		}
		prevHDR, prevSDR = hdrCode, sdrCode
	}
	return out
}

func SummarizeReport(report *ValidationReport) {
	s := &report.Summary
	s.Frames = len(report.Frames)
	s.LegalRangeOK = true
	s.BestHDRHighlightMinStep = 0
	s.SDRHighlightMinStep = 1 << 30
	for _, f := range report.Frames {
		if !f.LegalRangeOK {
			s.LegalRangeOK = false
		}
		if f.MiddleGray != nil {
			s.MaxMiddleGrayDeltaNits = max(s.MaxMiddleGrayDeltaNits, f.MiddleGray.DeltaNits)
		}
		s.HDRMaxCLLNits = max(s.HDRMaxCLLNits, f.HDRMaxCLLNits)
		s.HDRMaxFALLNits = max(s.HDRMaxFALLNits, f.HDRFALLNits)
		if f.HighlightBand != nil {
			s.BestHDRHighlightMinStep = max(s.BestHDRHighlightMinStep, f.HighlightBand.HDRMinStep)
			s.SDRHighlightMinStep = min(s.SDRHighlightMinStep, f.HighlightBand.SDRMinStep)
		}
	}
	if s.SDRHighlightMinStep == 1<<30 {
		s.SDRHighlightMinStep = 0
	}
	s.MiddleGrayConsistent = s.MaxMiddleGrayDeltaNits <= 0.1
	s.HDRRetainsHighlightDetail = s.BestHDRHighlightMinStep > s.SDRHighlightMinStep && s.BestHDRHighlightMinStep >= 2
}
