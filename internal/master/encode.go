package master

import "fmt"

type HDRStats struct {
	MaxCLLNits float64 `json:"max_cll_nits"`
	FALLNits   float64 `json:"fall_nits"`
}

// EncodeFrame derives both deliverables in one pass over the same source
// samples. The color paths remain independent after the shared scene sample is
// read.
func EncodeFrame(src *Frame, cfg Config) (sdr, hdr10 *Master, stats HDRStats, err error) {
	if err := src.Validate(); err != nil {
		return nil, nil, stats, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, stats, err
	}
	sdr = NewMaster(src.Width, src.Height)
	hdr10 = NewMaster(src.Width, src.Height)
	var lumSum float64
	for y := 0; y < src.Height; y++ {
		for x := 0; x < src.Width; x++ {
			scene := src.At(x, y)

			sdrLinear := GradeSDR(scene, cfg)
			var sdrCodes [3]uint16
			for c := 0; c < 3; c++ {
				signal := BT1886EncodeGamma(sdrLinear[c], cfg.SDRGamma)
				sdrCodes[c] = QuantizeLegal10(signal, x, y, cfg)
			}
			sdr.SetPixel(x, y, sdrCodes)

			hdrLinear := GradeHDR10(scene, cfg)
			var hdrCodes [3]uint16
			for c := 0; c < 3; c++ {
				signal := PQEncode(hdrLinear[c])
				hdrCodes[c] = QuantizeLegal10(signal, x, y, cfg)
			}
			hdr10.SetPixel(x, y, hdrCodes)

			nits := Luminance2020(hdrLinear)
			lumSum += nits
			stats.MaxCLLNits = max(stats.MaxCLLNits, nits)
		}
	}
	stats.FALLNits = lumSum / float64(src.Width*src.Height)
	if err := checkCodeRange(sdr, cfg); err != nil {
		return nil, nil, stats, fmt.Errorf("SDR legal range: %w", err)
	}
	if err := checkCodeRange(hdr10, cfg); err != nil {
		return nil, nil, stats, fmt.Errorf("HDR10 legal range: %w", err)
	}
	return sdr, hdr10, stats, nil
}

func checkCodeRange(m *Master, cfg Config) error {
	for i, code := range m.Codes {
		if code < cfg.LegalBlack || code > cfg.LegalWhite {
			return fmt.Errorf("code %d at word %d is outside %d..%d", code, i, cfg.LegalBlack, cfg.LegalWhite)
		}
	}
	return nil
}
