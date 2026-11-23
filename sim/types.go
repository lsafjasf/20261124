// Package sim models a one-way audio channel and three receiver-side
// packet-loss recovery strategies without any external dependency.
package sim

// FrameSize is the Opus 20 ms frame at 48 kHz.
const FrameSize = 960

// SampleRate is the signal sample rate used throughout the experiment.
const SampleRate = 48000

// Packet is the only object crossing the channel. Frame is the coded primary
// payload; FEC is an independent, intentionally degraded copy of the previous
// frame produced by the sender. The receiver never receives the clean source.
type Packet struct {
	Index int
	Frame []float64
	FEC   []float64 // nil when the packet carries no in-band FEC
	Lost  bool      // set by the channel, not the sender
}

// Mask marks lost frame indices.
type Mask map[int]bool

// IsLost reports whether frame i was dropped.
func (m Mask) IsLost(i int) bool { return m[i] }

// Count returns the number of lost frames.
func (m Mask) Count() int { return len(m) }

// Events groups consecutive lost indices into [start,end] inclusive runs.
func (m Mask) Events(nFrames int) [][2]int {
	var events [][2]int
	start := -1
	for i := 0; i < nFrames; i++ {
		if m[i] && start < 0 {
			start = i
		}
		if start >= 0 && (!m[i] || i == nFrames-1) {
			end := i - 1
			if m[i] {
				end = i
			}
			events = append(events, [2]int{start, end})
			start = -1
		}
	}
	return events
}

// Mode identifies a receiver recovery strategy.
type Mode string

const (
	ModeSilence Mode = "silence" // zero insertion
	ModePLC     Mode = "plc"     // independent receiver-only PLC
	ModeFEC     Mode = "fec"     // Opus-style built-in in-band FEC
)

// State labels recorded in the receiver state chain, one per output frame.
const (
	StateOK      = "OK"               // good frame decoded normally
	StateSilence = "SILENCE"          // lost -> zero insertion
	StatePLC     = "PLC"              // lost -> independent packet loss concealment
	StatePLCCold = "PLC_COLD"         // PLC with no usable history yet
	StateFEC     = "FEC"              // lost -> reconstructed from in-band FEC
	StateFECPlc  = "FEC_FALLBACK_PLC" // FEC requested but none available
	StateFECSil  = "FEC_FALLBACK_SILENCE"
	StateFadeIn  = "FADE_IN" // first good frame crossfaded after a loss
)

// FrameResult is one row of the receiver state chain.
type FrameResult struct {
	Index    int
	State    string
	Lost     bool
	Samples  int // concealed/recovered samples on lost frames, 0 on good frames
	Gain     float64
	PitchLag int
}
