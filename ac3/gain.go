package ac3

// Gain math for dialnorm metadata edits and loudness normalization.
// An AC-3 decoder attenuates the program by dialnorm dB, so program
// loudness is approximately -dialnorm LKFS. Changing the dialnorm field
// without re-encoding changes decoder attenuation; these helpers compute
// the PCM gain needed to compensate or to hit a loudness target.

// DialnormEditGainDB returns the PCM gain (dB) required to keep the
// reproduced loudness identical after rewriting dialnorm from oldD to
// newD. Positive means the audio must be amplified.
func DialnormEditGainDB(oldD, newD int) float64 {
	return float64(newD - oldD)
}

// NormalizeGainDB returns the PCM gain (dB) that brings a program whose
// dialnorm is d to the target loudness (LKFS).
func NormalizeGainDB(d int, targetLKFS float64) float64 {
	return targetLKFS + float64(d)
}

// FrameGain is the per-frame gain plan.
type FrameGain struct {
	Frame            int
	Dialnorm         int
	LoudnessLKFS     float64
	NormalizeGainDB  float64
	DialnormEditGain float64 // only valid when newDialnorm >= 0
}

// TargetGainDB computes per-frame gains: normalization to targetLKFS and,
// if newDialnorm >= 0, compensation for rewriting dialnorm to newDialnorm.
func TargetGainDB(frames []*Frame, targetLKFS float64, newDialnorm int) []FrameGain {
	out := make([]FrameGain, 0, len(frames))
	for _, f := range frames {
		fg := FrameGain{
			Frame:           f.Index,
			Dialnorm:        f.Dialnorm,
			LoudnessLKFS:    f.LoudnessLKFS(),
			NormalizeGainDB: NormalizeGainDB(f.Dialnorm, targetLKFS),
		}
		if newDialnorm >= 0 {
			fg.DialnormEditGain = DialnormEditGainDB(f.Dialnorm, newDialnorm)
		}
		out = append(out, fg)
	}
	return out
}
