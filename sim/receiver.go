package sim

// Receiver consumes channel output packets. It has no access to the sender's
// clean source: good frames come from packets, FEC from the next packet's
// redundancy payload, and PLC output from the receiver's own history only.
type Receiver struct {
	mode      Mode
	plc       *PLC
	lastLost  bool
	chain     []FrameResult
	recovered int
}

func NewReceiver(mode Mode) *Receiver {
	return &Receiver{mode: mode, plc: NewPLC()}
}

// Run decodes all frames in order and returns the reconstructed signal.
func (r *Receiver) Run(pkts []*Packet) [][]float64 {
	n := len(pkts)
	out := make([][]float64, n)
	for i := 0; i < n; i++ {
		p := pkts[i]
		switch {
		case !p.Lost:
			out[i] = r.goodFrame(p)
		case r.mode == ModeSilence:
			out[i] = r.silenceFrame(i)
		case r.mode == ModePLC:
			out[i] = r.plcFrame(i)
		case r.mode == ModeFEC:
			out[i] = r.fecFrame(i, pkts)
		default:
			panic("unknown receiver mode: " + r.mode)
		}
		r.lastLost = p.Lost
	}
	return out
}

func (r *Receiver) goodFrame(p *Packet) []float64 {
	var frame []float64
	state := StateOK
	if r.lastLost && r.plc.consecLost > 0 {
		frame = r.plc.CrossfadeResume(p.Frame)
		state = StateFadeIn
	} else {
		frame = copyFrame(p.Frame)
		r.plc.Remember(frame)
		r.plc.ResetLossState()
	}
	r.chain = append(r.chain, FrameResult{Index: p.Index, State: state, Samples: 0})
	return frame
}

func (r *Receiver) silenceFrame(i int) []float64 {
	zero := make([]float64, FrameSize)
	r.plc.Remember(zero) // model the receiver output history faithfully
	r.plc.ResetLossState()
	r.recovered += FrameSize
	r.chain = append(r.chain, FrameResult{
		Index: i, State: StateSilence, Lost: true, Samples: FrameSize,
	})
	return zero
}

func (r *Receiver) plcFrame(i int) []float64 {
	frame, state := r.plc.Synthesize()
	r.recovered += FrameSize
	r.chain = append(r.chain, FrameResult{
		Index:    i,
		State:    state,
		Lost:     true,
		Samples:  FrameSize,
		Gain:     r.plc.gain,
		PitchLag: r.plc.pitchLag,
	})
	return frame
}

func (r *Receiver) fecFrame(i int, pkts []*Packet) []float64 {
	// Opus in-band FEC: the redundancy for lost frame i arrives inside packet
	// i+1. Only the first frame of a burst can be repaired this way.
	if i+1 < len(pkts) && !pkts[i+1].Lost && pkts[i+1].FEC != nil {
		frame := r.fecFadeIn(pkts[i+1].FEC)
		r.plc.Remember(frame)
		r.plc.ResetLossState()
		r.recovered += FrameSize
		r.chain = append(r.chain, FrameResult{
			Index: i, State: StateFEC, Lost: true, Samples: FrameSize,
		})
		return frame
	}
	// Burst tail: no future redundancy exists. Fall back transparently and
	// record which secondary strategy the receiver selected.
	if r.plc.Cold() {
		zero := make([]float64, FrameSize)
		r.plc.Remember(zero)
		r.plc.ResetLossState()
		r.recovered += FrameSize
		r.chain = append(r.chain, FrameResult{
			Index: i, State: StateFECSil, Lost: true, Samples: FrameSize,
		})
		return zero
	}
	frame, _ := r.plc.Synthesize()
	r.recovered += FrameSize
	r.chain = append(r.chain, FrameResult{
		Index:    i,
		State:    StateFECPlc,
		Lost:     true,
		Samples:  FrameSize,
		Gain:     r.plc.gain,
		PitchLag: r.plc.pitchLag,
	})
	return frame
}

// fecFadeIn ramps the degraded payload in and crossfades against the prior
// output tail to avoid a click at the lost-frame boundary.
func (r *Receiver) fecFadeIn(fec []float64) []float64 {
	out := copyFrame(fec)
	n := fadeInLen
	if n > len(out) {
		n = len(out)
	}
	for i := 0; i < n; i++ {
		w := float64(i) / float64(n)
		var prev float64
		if k := len(r.plc.history) - n + i; k >= 0 {
			prev = r.plc.history[k]
		}
		out[i] = (1-w)*prev + w*out[i]
	}
	return out
}

// Chain exposes the per-frame receiver state chain.
func (r *Receiver) Chain() []FrameResult { return r.chain }

// RecoveredSamples counts samples the receiver synthesized or inserted on
// lost frames (FEC reconstruction counts as recovery).
func (r *Receiver) RecoveredSamples() int { return r.recovered }
