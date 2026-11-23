package sim

import "opusloss/audio"

// BuildPackets runs on the sender. Each packet contains its own coded frame
// and, as Opus in-band FEC does, a degraded redundant copy of the previous
// frame piggy-backed on packet i. Nothing here is exposed to the receiver
// beyond the packet object.
func BuildPackets(frames [][]float64) []*Packet {
	pkts := make([]*Packet, len(frames))
	for i, f := range frames {
		coded := copyFrame(f) // coded primary payload (lossless stand-in)
		var fec []float64
		if i > 0 {
			dulled := audio.LowPass(frames[i-1], 0.35)
			fec = audio.Quantize8(dulled)
		}
		pkts[i] = &Packet{Index: i, Frame: coded, FEC: fec}
	}
	return pkts
}

// ApplyChannel drops every packet whose index is in mask. The mask models the
// network; neither sender nor receiver can rewrite it.
func ApplyChannel(pkts []*Packet, mask Mask) []*Packet {
	out := make([]*Packet, len(pkts))
	for i, p := range pkts {
		if mask[i] {
			lost := *p
			lost.Lost = true
			lost.Frame = nil
			lost.FEC = nil
			out[i] = &lost
		} else {
			out[i] = p
		}
	}
	return out
}

// IsolatedMask creates isolated single-frame losses at the given indices.
// Neighbors are asserted present so each event has FEC available.
func IsolatedMask(indices ...int) Mask {
	m := Mask{}
	for _, i := range indices {
		m[i] = true
	}
	return m
}

// BurstMask creates runs of consecutive lost frames [start, start+length).
func BurstMask(start, length int) Mask {
	m := Mask{}
	for i := start; i < start+length; i++ {
		m[i] = true
	}
	return m
}

func copyFrame(f []float64) []float64 {
	c := make([]float64, len(f))
	copy(c, f)
	return c
}
