package sim

import "math"

// PLC is a receiver-only packet loss concealer. It never imports the source
// signal: every output sample is synthesized from previously decoded frames.
type PLC struct {
	history    []float64 // recent good/reconstructed output samples
	consecLost int
	gain       float64
	pitchLag   int
	phase      int // offset inside the repeated pitch period
}

const (
	minLag     = 48   // 1 ms at 48 kHz
	maxLag     = 960  // one full frame (~20 ms)
	historyCap = 1920 // two frames of context
	crossLen   = 192  // 4 ms equal-power-ish linear crossfade
	fadeInLen  = 128  // concealment onset ramp to avoid clicks
)

func NewPLC() *PLC {
	return &PLC{gain: 1, pitchLag: 0}
}

// Remember stores output samples that the receiver actually produced.
func (p *PLC) Remember(frame []float64) {
	p.history = append(p.history, frame...)
	if len(p.history) > historyCap {
		p.history = p.history[len(p.history)-historyCap:]
	}
}

// ResetLossState is called once a real frame resumes cleanly.
func (p *PLC) ResetLossState() {
	p.consecLost = 0
	p.gain = 1
	p.phase = 0
}

// Cold reports whether enough history exists for periodic synthesis.
func (p *PLC) Cold() bool { return len(p.history) < minLag*2 }

// Synthesize fills one lost frame and returns its per-frame state label.
func (p *PLC) Synthesize() ([]float64, string) {
	out := make([]float64, FrameSize)
	if p.Cold() {
		// No usable history: deterministic zero output, same length contract.
		p.rememberOutput(out)
		p.consecLost++
		return out, StatePLCCold
	}
	if p.consecLost == 0 || p.pitchLag == 0 {
		p.pitchLag = p.estimatePitch()
		p.phase = 0
	}
	base := p.pitchLag
	for i := 0; i < FrameSize; i++ {
		src := len(p.history) - base + ((p.phase + i) % base)
		out[i] = p.history[src] * p.gain
		if p.consecLost == 0 && i < fadeInLen {
			out[i] *= float64(i) / float64(fadeInLen)
		}
	}
	p.phase = (p.phase + FrameSize) % base
	// 2 dB attenuation per additional lost frame, floor to avoid silence decay
	// hiding the state chain during long bursts.
	p.gain = math.Max(0.05, p.gain*math.Pow(10, -2.0/20.0))
	p.consecLost++
	p.rememberOutput(out)
	return out, StatePLC
}

func (p *PLC) rememberOutput(f []float64) {
	p.Remember(f)
}

// CrossfadeResume blends the first good frame with what PLC would have
// produced, preventing a discontinuity at the concealment boundary.
func (p *PLC) CrossfadeResume(good []float64) []float64 {
	if p.consecLost == 0 {
		return copyFrame(good)
	}
	ghost, _ := p.Synthesize()
	out := copyFrame(good)
	n := crossLen
	if n > len(out) {
		n = len(out)
	}
	for i := 0; i < n; i++ {
		w := float64(i) / float64(n)
		out[i] = (1-w)*ghost[i] + w*good[i]
	}
	// Synthesize pushed ghost into history and incremented the loss counter;
	// replace that bookkeeping with the real resumed frame.
	p.history = p.history[:len(p.history)-FrameSize]
	p.consecLost = 0
	p.ResetLossState()
	p.Remember(out)
	return out
}

// estimatePitch finds the lag with the largest normalized autocorrelation.
func (p *PLC) estimatePitch() int {
	h := p.history
	n := len(h)
	lagMax := maxLag
	if lagMax > n/2 {
		lagMax = n / 2
	}
	energy := 0.0
	for _, v := range h[n-lagMax:] {
		energy += v * v
	}
	if energy < 1e-9 {
		return lagMax
	}
	bestLag, bestScore := lagMax, -1.0
	for lag := minLag; lag <= lagMax; lag++ {
		num, den := 0.0, 0.0
		for i := 0; i < lagMax; i++ {
			a := h[n-1-i]
			b := h[n-1-i-lag]
			num += a * b
			den += b * b
		}
		if den < 1e-12 {
			continue
		}
		score := num / math.Sqrt(energy*den)
		if score > bestScore {
			bestScore, bestLag = score, lag
		}
	}
	return bestLag
}
