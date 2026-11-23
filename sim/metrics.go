package sim

import (
	"errors"
	"math"
)

// Metrics summarizes waveform error over the full signal.
type Metrics struct {
	Frames           int
	LostFrames       int
	RecoveredFrames  int
	RecoveredSamples int
	MAE              float64
	RMSE             float64
	PeakAbs          float64
	SegSNRDB         float64 // segmental SNR over lost-frame regions only
}

// ErrorMetrics compares reconstructed frames against the clean reference.
// The reference is used only for offline scoring, never inside a receiver.
func ErrorMetrics(reconstructed, clean [][]float64, chain []FrameResult) Metrics {
	m := Metrics{Frames: len(clean)}
	var absSum, sqSum, sigPow, errPow float64
	scored := 0
	for _, row := range chain {
		if !row.Lost {
			continue
		}
		m.LostFrames++
		m.RecoveredFrames++
		m.RecoveredSamples += row.Samples
		rec := reconstructed[row.Index]
		ref := clean[row.Index]
		for i := range ref {
			d := rec[i] - ref[i]
			ad := math.Abs(d)
			absSum += ad
			sqSum += d * d
			if ad > m.PeakAbs {
				m.PeakAbs = ad
			}
			sigPow += ref[i] * ref[i]
			errPow += d * d
			scored++
		}
	}
	if scored > 0 {
		m.MAE = absSum / float64(scored)
		m.RMSE = math.Sqrt(sqSum / float64(scored))
	}
	if errPow > 1e-12 && sigPow > 0 {
		m.SegSNRDB = 10 * math.Log10(sigPow/errPow)
	} else {
		m.SegSNRDB = math.Inf(1)
	}
	return m
}

// AuditNoBackfill is the explicit anti-cheating guard. Every frame the
// receiver produced while the packet was lost must differ from the sender's
// clean frame; an exact copy would prove source backfilling.
func AuditNoBackfill(reconstructed, clean [][]float64, chain []FrameResult) error {
	for _, row := range chain {
		if !row.Lost {
			continue
		}
		rec := reconstructed[row.Index]
		ref := clean[row.Index]
		if len(rec) != len(ref) {
			return errors.New("length mismatch on recovered frame")
		}
		identical := true
		for i := range ref {
			if rec[i] != ref[i] {
				identical = false
				break
			}
		}
		if identical {
			return errors.New("recovered frame is a verbatim copy of the source: backfill forbidden")
		}
	}
	return nil
}

// StateSequence renders the state chain as a compact per-frame token string.
func StateSequence(chain []FrameResult) string {
	b := make([]byte, 0, len(chain)*4)
	tokens := map[string]byte{
		StateOK:      '.',
		StateFadeIn:  'R',
		StateSilence: 'S',
		StatePLC:     'P',
		StatePLCCold: 'C',
		StateFEC:     'F',
		StateFECPlc:  'p',
		StateFECSil:  's',
	}
	for i, row := range chain {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, tokens[row.State])
	}
	return string(b)
}
