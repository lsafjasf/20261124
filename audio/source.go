package audio

import "math"

// GenerateSpeechLike builds a deterministic pseudo-speech mono signal.
// It mixes a vibrato fundamental, harmonics, a second voiced register and
// breath noise, shaped by a syllable-like amplitude envelope.
func GenerateSpeechLike(sampleRate, n int, seed uint64) []float64 {
	rng := newRNG(seed)
	out := make([]float64, n)
	phase := 0.0
	phase2 := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		f0 := 132.0 + 18.0*math.Sin(2*math.Pi*0.7*t) + 6.0*math.Sin(2*math.Pi*3.1*t)
		f2 := 198.0 + 10.0*math.Sin(2*math.Pi*0.5*t+1.7)
		phase += 2 * math.Pi * f0 / float64(sampleRate)
		phase2 += 2 * math.Pi * f2 / float64(sampleRate)

		env := 0.55 + 0.45*math.Pow(0.5+0.5*math.Sin(2*math.Pi*3.2*t-0.6), 1.5)
		voiced := 0.62*math.Sin(phase) +
			0.28*math.Sin(2*phase+0.4) +
			0.14*math.Sin(3*phase+1.1) +
			0.07*math.Sin(5*phase)
		second := 0.35 * math.Sin(phase2+0.8)
		noise := 0.05 * (rng.next()*2 - 1)
		s := env * (0.78*voiced + 0.22*second + noise)
		out[i] = Clamp(0.9*s, -1, 1)
	}
	return out
}

// LowPass is a one-pole leaky integrator. alpha in (0,1]: smaller -> duller.
func LowPass(x []float64, alpha float64) []float64 {
	y := make([]float64, len(x))
	acc := 0.0
	for i, v := range x {
		acc += alpha * (v - acc)
		y[i] = acc
	}
	return y
}

// Quantize8 simulates a coarse low-bitrate FEC payload. It stays entirely on
// the sender side: the receiver only ever sees this degraded representation.
func Quantize8(frame []float64) []float64 {
	const levels = 255.0
	q := make([]float64, len(frame))
	for i, v := range frame {
		q[i] = math.Round(Clamp(v, -1, 1)*levels) / levels
	}
	return q
}

// rng is a tiny deterministic xorshift generator (no math/rand global state).
type rng struct{ state uint64 }

func newRNG(seed uint64) *rng {
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	return &rng{state: seed}
}

func (r *rng) next() float64 {
	x := r.state
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	r.state = x
	return float64(x>>11) / float64(1<<53)
}
