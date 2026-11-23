package eac3sm

// Spec describes a synthetic E-AC-3 syncframe. It exists so tests and tools
// can build spec-conformant headers with a valid crc2 without a real encoder.
type Spec struct {
	FSCod   int  // 0=48kHz 1=44.1kHz 2=32kHz 3=half-rate (uses FSCod2)
	FSCod2  int  // half-rate selector when FSCod==3: 0=24kHz 1=22.05kHz 2=16kHz
	Acmod   int  // audio coding mode 0..7
	LFE     bool // lfeon
	Compr   *int // compression gain word; nil writes compre=0
	Corrupt bool // flip a payload byte after CRC computation
}

type bitWriter struct {
	buf []byte
	pos int
}

func (w *bitWriter) write(v uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.pos>>3 == len(w.buf) {
			w.buf = append(w.buf, 0)
		}
		if v>>uint(i)&1 == 1 {
			w.buf[w.pos>>3] |= 1 << (7 - uint(w.pos&7))
		}
		w.pos++
	}
}

// SynthFrame builds a 128-byte E-AC-3 syncframe with a correct crc2.
func SynthFrame(s Spec) []byte {
	const frameBytes = 128
	w := &bitWriter{}
	w.write(syncWord, 16)
	w.write(0, 2)               // frmtyp: independent frame
	w.write(0, 3)               // substreamid
	w.write(frameBytes/2-1, 11) // frmsiz
	w.write(uint32(s.FSCod), 2)
	if s.FSCod == 3 {
		w.write(uint32(s.FSCod2), 2)
	} else {
		w.write(3, 2) // numblkscod: 6 blocks
	}
	w.write(uint32(s.Acmod), 3)
	if s.LFE {
		w.write(1, 1)
	} else {
		w.write(0, 1)
	}
	w.write(16, 5) // bsid: E-AC-3
	w.write(27, 5) // dialnorm
	if s.Compr != nil {
		w.write(1, 1)
		w.write(uint32(*s.Compr), 8)
	} else {
		w.write(0, 1)
	}
	if s.Acmod == 0 { // dual mono second channel fields
		w.write(27, 5)
		w.write(0, 1)
	}
	frame := make([]byte, frameBytes)
	copy(frame, w.buf)
	crc := crc16(frame[:frameBytes-2])
	frame[frameBytes-2] = byte(crc >> 8)
	frame[frameBytes-1] = byte(crc)
	if s.Corrupt {
		frame[frameBytes/2] ^= 0xFF
	}
	return frame
}
