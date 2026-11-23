// Package sample builds small deterministic MP4/H.264 fixtures for the tagfix
// command and its tests. It intentionally implements only the boxes needed to
// exercise colr and avcC synchronization.
package sample

import "encoding/binary"

// SPSOptions controls the H.264 VUI fields written into the fixture.
type SPSOptions struct {
	HasVUI         bool
	HasSignal      bool
	HasDescription bool
	FullRange      bool
	Primaries      uint8
	Transfer       uint8
	Matrix         uint8
}

// Color is the container colr payload.
type Color struct {
	Primaries uint8
	Transfer  uint8
	Matrix    uint8
	FullRange bool
}

// Options describes one conflict fixture.
type Options struct {
	SPS       SPSOptions
	NoSPS     bool
	NoColr    bool
	Container Color
	MDAT      []byte
}

type bitWriter struct {
	bits []byte
}

func (w *bitWriter) bit(value int) {
	w.bits = append(w.bits, byte(value&1))
}

func (w *bitWriter) bitsN(value uint64, count int) {
	for i := count - 1; i >= 0; i-- {
		w.bit(int((value >> i) & 1))
	}
}

func (w *bitWriter) ue(value uint64) {
	code := value + 1
	width := 0
	for temp := code; temp > 1; temp >>= 1 {
		width++
	}
	for i := 0; i < width; i++ {
		w.bit(0)
	}
	w.bitsN(code, width+1)
}

func (w *bitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, bit := range w.bits {
		out[i/8] |= bit << (7 - i%8)
	}
	return out
}

// SPS creates an SPS NAL with optional VUI color signalling.
func SPS(options SPSOptions) []byte {
	var writer bitWriter
	writer.bitsN(66, 8)
	writer.bitsN(0, 8)
	writer.bitsN(30, 8)
	writer.ue(0)
	writer.ue(0)
	writer.ue(0)
	writer.ue(0)
	writer.ue(1)
	writer.bit(0)
	writer.ue(19)
	writer.ue(14)
	writer.bit(1)
	writer.bit(1)
	writer.bit(0)
	if options.HasVUI {
		writer.bit(1)
		writer.bit(0)
		writer.bit(0)
		if options.HasSignal {
			writer.bit(1)
			writer.bitsN(5, 3)
			if options.FullRange {
				writer.bit(1)
			} else {
				writer.bit(0)
			}
			if options.HasDescription {
				writer.bit(1)
				writer.bitsN(uint64(options.Primaries), 8)
				writer.bitsN(uint64(options.Transfer), 8)
				writer.bitsN(uint64(options.Matrix), 8)
			} else {
				writer.bit(0)
			}
		} else {
			writer.bit(0)
		}
		writer.bit(0)
		writer.bit(0)
		writer.bit(0)
		writer.bit(0)
		writer.bit(0)
		writer.bit(0)
	} else {
		writer.bit(0)
	}
	writer.bit(1)
	return append([]byte{0x67}, escape(writer.bytes())...)
}

func escape(rbsp []byte) []byte {
	out := make([]byte, 0, len(rbsp)+len(rbsp)/64+1)
	for i, value := range rbsp {
		if i >= 2 && rbsp[i-1] == 0 && rbsp[i-2] == 0 && value <= 3 {
			out = append(out, 3)
		}
		out = append(out, value)
	}
	return out
}

func avcc(sps []byte, includeSPS bool) []byte {
	payload := []byte{1, 66, 0, 30, 0xff}
	if includeSPS {
		payload = append(payload, 0xe1, 0, 0)
		binary.BigEndian.PutUint16(payload[len(payload)-2:], uint16(len(sps)))
		payload = append(payload, sps...)
	} else {
		payload = append(payload, 0xe0)
	}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	payload = append(payload, 1, 0, 0)
	binary.BigEndian.PutUint16(payload[len(payload)-2:], uint16(len(pps)))
	payload = append(payload, pps...)
	return payload
}

// MP4 builds an ftyp/moov/mdat file. moov is placed before mdat so offset
// changes are exercised as well as byte preservation.
func MP4(options Options) []byte {
	colrPayload := make([]byte, 11)
	copy(colrPayload, "nclx")
	binary.BigEndian.PutUint16(colrPayload[4:6], uint16(options.Container.Primaries))
	binary.BigEndian.PutUint16(colrPayload[6:8], uint16(options.Container.Transfer))
	binary.BigEndian.PutUint16(colrPayload[8:10], uint16(options.Container.Matrix))
	if options.Container.FullRange {
		colrPayload[10] = 0x80
	}
	sps := SPS(options.SPS)
	avcC := box("avcC", avcc(sps, !options.NoSPS))
	entryPayload := make([]byte, 78)
	entryPayload[7] = 1
	binary.BigEndian.PutUint16(entryPayload[24:26], 320)
	binary.BigEndian.PutUint16(entryPayload[26:28], 240)
	binary.BigEndian.PutUint32(entryPayload[28:32], 0x00480000)
	binary.BigEndian.PutUint32(entryPayload[32:36], 0x00480000)
	binary.BigEndian.PutUint16(entryPayload[40:42], 1)
	binary.BigEndian.PutUint16(entryPayload[74:76], 24)
	binary.BigEndian.PutUint16(entryPayload[76:78], 0xffff)
	children := avcC
	if !options.NoColr {
		children = append(box("colr", colrPayload), avcC...)
	}
	entry := box("avc1", append(entryPayload, children...))
	stsdPayload := []byte{0, 0, 0, 0, 0, 0, 0, 1}
	stsd := box("stsd", append(stsdPayload, entry...))
	stbl := box("stbl", stsd)
	minf := box("minf", stbl)
	hdlrPayload := []byte{0, 0, 0, 0, 0, 0, 0, 0, 'v', 'i', 'd', 'e', 0, 0, 0, 0}
	mdia := box("mdia", append(box("hdlr", hdlrPayload), minf...))
	moov := box("moov", box("trak", mdia))
	ftyp := box("ftyp", append([]byte("isom"), 0, 0, 0, 0, 'i', 's', 'o', 'm'))
	mdatPayload := options.MDAT
	if mdatPayload == nil {
		mdatPayload = []byte("MEDIA-PAYLOAD-MUST-NOT-CHANGE\x00\x01\x02\x03")
	}
	return append(append(ftyp, moov...), box("mdat", mdatPayload)...)
}

func box(typ string, payload []byte) []byte {
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], payload)
	return out
}
