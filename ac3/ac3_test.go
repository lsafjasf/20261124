package ac3

import (
	"bytes"
	"testing"
)

type bitWriter struct {
	buf   []byte
	nbits int
}

// GF(2)[x] helpers mirroring the reference AC-3 encoder, poly x^16+x^15+x^2+1.
const crcPoly = 0x18005

func mulPoly(a, b uint32) uint32 {
	var c uint32
	for a != 0 {
		if a&1 != 0 {
			c ^= b
		}
		a >>= 1
		b <<= 1
		if b&0x10000 != 0 {
			b ^= crcPoly
		}
	}
	return c
}

func powPoly(a, n uint32) uint32 {
	r := uint32(1)
	for n != 0 {
		if n&1 != 0 {
			r = mulPoly(r, a)
		}
		a = mulPoly(a, a)
		n >>= 1
	}
	return r
}

func (w *bitWriter) put(v, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.nbits%8 == 0 {
			w.buf = append(w.buf, 0)
		}
		if v>>uint(i)&1 == 1 {
			w.buf[len(w.buf)-1] |= 1 << (7 - uint(w.nbits%8))
		}
		w.nbits++
	}
}

type frameOpts struct {
	fscod, frmsizecod int
	bsmod, acmod      int
	lfeon             bool
	dialnorm          int
	compr, dynrng     int // signed; compr >= 0x100 disables
	comprOn, dynrngOn bool
}

// buildFrame synthesizes a standards-shaped AC-3 syncframe with valid
// CRCs. Payload past the first audio block header is zero-filled.
func buildFrame(o frameOpts) []byte {
	w := &bitWriter{}
	w.put(SyncWord, 16)
	w.put(0, 16) // crc1 placeholder
	w.put(o.fscod, 2)
	w.put(o.frmsizecod, 6)
	w.put(8, 5) // bsid
	w.put(o.bsmod, 3)
	w.put(o.acmod, 3)
	if o.acmod&1 != 0 && o.acmod != 1 {
		w.put(1, 2) // cmixlev
	}
	if o.acmod&4 != 0 {
		w.put(1, 2) // surmixlev
	}
	if o.acmod == 2 {
		w.put(0, 2) // dsurmod
	}
	if o.lfeon {
		w.put(1, 1)
	} else {
		w.put(0, 1)
	}
	w.put(o.dialnorm, 5)
	if o.comprOn {
		w.put(1, 1)
		w.put(o.compr&0xFF, 8)
	} else {
		w.put(0, 1)
	}
	if o.acmod == 0 {
		w.put(0, 1) // langcode
	}
	w.put(0, 1) // audprodie
	if o.acmod == 0 {
		w.put(0, 1) // audprodi2e
	}
	w.put(1, 1) // copyrightb
	w.put(1, 1) // origbs
	w.put(0, 1) // timecod1e
	w.put(0, 1) // timecod2e
	w.put(0, 1) // addbsie
	nfch := acmodTable[o.acmod].nfchans
	w.put(0, nfch) // blksw
	w.put(0, nfch) // dithflag
	if o.dynrngOn {
		w.put(1, 1)
		w.put(o.dynrng&0xFF, 8)
	} else {
		w.put(0, 1)
	}

	size := frameSizeTable[o.frmsizecod][o.fscod] * 2
	frame := make([]byte, size)
	copy(frame, w.buf)
	n58 := crc1Len(size)
	// crc1: CRC over bytes [4, n58), corrected so that the CRC of the
	// whole first 5/8 of the frame becomes zero (reference encoder math).
	c := CRC16(frame[4:n58])
	crcInv := powPoly(crcPoly>>1, uint32(16*(n58/2)-16))
	crc1 := uint16(mulPoly(crcInv, uint32(c)))
	frame[2], frame[3] = byte(crc1>>8), byte(crc1)
	crc2 := CRC16(frame[n58 : size-2])
	frame[size-2], frame[size-1] = byte(crc2>>8), byte(crc2)
	return frame
}

func stereoOpts() frameOpts {
	return frameOpts{fscod: 0, frmsizecod: 30, bsmod: 0, acmod: 2,
		dialnorm: 27, comprOn: true, compr: 32, dynrngOn: true, dynrng: -16}
}

func TestParseValidFrame(t *testing.T) {
	raw := buildFrame(stereoOpts())
	f, err := ParseFrame(raw, 0, 0)
	if err != nil {
		t.Fatalf("ParseFrame: %v", err)
	}
	if !f.CRC1OK || !f.CRC2OK {
		t.Errorf("CRC checks failed: crc1=%v crc2=%v", f.CRC1OK, f.CRC2OK)
	}
	if f.SampleRate != 48000 || f.BitRate != 448 || f.FrameBytes != 1792 {
		t.Errorf("sampling params: %d Hz %d kbps %d B", f.SampleRate, f.BitRate, f.FrameBytes)
	}
	if f.Dialnorm != 27 || f.BSMod != 0 || f.ACMod != 2 {
		t.Errorf("dialnorm=%d bsmod=%d acmod=%d", f.Dialnorm, f.BSMod, f.ACMod)
	}
	if !f.ComprPresent || f.Compr != 32 || f.ComprGainDB() != 1.0 {
		t.Errorf("compr=%d gain=%v", f.Compr, f.ComprGainDB())
	}
	if !f.DynrngSet || f.Dynrng != -16 || f.DynrngGainDB() != -0.5 {
		t.Errorf("dynrng=%d gain=%v", f.Dynrng, f.DynrngGainDB())
	}
}

func eventsOfKind(res *Result, k EventKind) []Event {
	var out []Event
	for _, e := range res.Events {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

func TestCRCErrorLocatedByFrame(t *testing.T) {
	var stream bytes.Buffer
	for i := 0; i < 3; i++ {
		stream.Write(buildFrame(stereoOpts()))
	}
	data := stream.Bytes()
	data[1792+100] ^= 0xFF // corrupt payload of frame 1
	res, err := Scan(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 3 {
		t.Fatalf("frames=%d", len(res.Frames))
	}
	crcErrs := eventsOfKind(res, EventCRCError)
	if len(crcErrs) != 1 || crcErrs[0].Frame != 1 {
		t.Fatalf("CRC events: %v", crcErrs)
	}
}

func TestParamChangeLocatedByFrame(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(buildFrame(stereoOpts()))
	o := stereoOpts()
	o.dialnorm = 24
	stream.Write(buildFrame(o))
	o.acmod = 7
	o.lfeon = true
	stream.Write(buildFrame(o))
	res, err := Scan(bytes.NewReader(stream.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	changes := eventsOfKind(res, EventParamChange)
	if len(changes) != 2 || changes[0].Frame != 1 || changes[1].Frame != 2 {
		t.Fatalf("param change events: %v", changes)
	}
}

func TestConcatenationAndResync(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(buildFrame(stereoOpts()))
	stream.Write([]byte{0x00, 0x11, 0x22}) // garbage between programs
	stream.Write(buildFrame(stereoOpts()))
	res, err := Scan(bytes.NewReader(stream.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Frames) != 2 {
		t.Fatalf("frames=%d", len(res.Frames))
	}
	rs := eventsOfKind(res, EventResync)
	if len(rs) != 1 || rs[0].Frame != 1 || rs[0].Offset != 1792 {
		t.Fatalf("resync events: %v", rs)
	}
	if res.Frames[1].Offset != 1795 {
		t.Fatalf("frame 1 offset=%d", res.Frames[1].Offset)
	}
}

func TestTruncatedFrame(t *testing.T) {
	raw := buildFrame(stereoOpts())
	res, err := Scan(bytes.NewReader(raw[:1000]))
	if err != nil {
		t.Fatal(err)
	}
	tr := eventsOfKind(res, EventTruncated)
	if len(tr) != 1 || tr[0].Frame != 0 {
		t.Fatalf("truncated events: %v", tr)
	}
}

func TestGainMath(t *testing.T) {
	if g := DialnormEditGainDB(27, 24); g != -3 {
		t.Errorf("edit gain=%v", g)
	}
	if g := NormalizeGainDB(27, -24); g != 3 {
		t.Errorf("normalize gain=%v", g)
	}
	f, _ := ParseFrame(buildFrame(stereoOpts()), 0, 0)
	if f.LoudnessLKFS() != -27 {
		t.Errorf("loudness=%v", f.LoudnessLKFS())
	}
}
