// Package fixture builds binary PGS (.sup) streams for tests and tools.
package fixture

import "encoding/binary"

// Seg is one segment to serialize: type byte, PTS/DTS in 90 kHz ticks,
// and an already-encoded payload.
type Seg struct {
	Type    byte
	PTS     uint32
	DTS     uint32
	Payload []byte
}

// Build serializes segments into a binary .sup stream.
func Build(segs ...Seg) []byte {
	var out []byte
	for _, s := range segs {
		hdr := make([]byte, 13)
		hdr[0] = 0x50 // 'P'
		hdr[1] = 0x47 // 'G'
		binary.BigEndian.PutUint32(hdr[2:], s.PTS)
		binary.BigEndian.PutUint32(hdr[6:], s.DTS)
		hdr[10] = s.Type
		binary.BigEndian.PutUint16(hdr[11:], uint16(len(s.Payload)))
		out = append(out, hdr...)
		out = append(out, s.Payload...)
	}
	return out
}

// Palette encodes a PDS payload. Each entry is {id, Y, Cr, Cb, alpha}.
func Palette(id, version byte, entries ...[5]byte) []byte {
	p := []byte{id, version}
	for _, e := range entries {
		p = append(p, e[:]...)
	}
	return p
}

// Win describes one window for the Window payload builder.
type Win struct {
	ID         byte
	X, Y, W, H uint16
}

// Window encodes a WDS payload.
func Window(wins ...Win) []byte {
	p := []byte{byte(len(wins))}
	for _, w := range wins {
		q := make([]byte, 9)
		q[0] = w.ID
		binary.BigEndian.PutUint16(q[1:], w.X)
		binary.BigEndian.PutUint16(q[3:], w.Y)
		binary.BigEndian.PutUint16(q[5:], w.W)
		binary.BigEndian.PutUint16(q[7:], w.H)
		p = append(p, q...)
	}
	return p
}

// Object encodes a single-fragment (first+last) ODS payload.
func Object(id uint16, version byte, w, h uint16, rle []byte) []byte {
	p := make([]byte, 0, 11+len(rle))
	p = binary.BigEndian.AppendUint16(p, id)
	p = append(p, version, 0xC0) // first + last
	dl := uint32(4 + len(rle))
	p = append(p, byte(dl>>16), byte(dl>>8), byte(dl))
	p = binary.BigEndian.AppendUint16(p, w)
	p = binary.BigEndian.AppendUint16(p, h)
	return append(p, rle...)
}

// CompObj describes one composition object for the Display builder.
type CompObj struct {
	ObjectID uint16
	WindowID byte
	X, Y     uint16
}

// Display encodes a PCS payload.
func Display(compState byte, paletteUpdate bool, paletteID byte, objs ...CompObj) []byte {
	p := make([]byte, 0, 11+8*len(objs))
	p = binary.BigEndian.AppendUint16(p, 1920) // video width
	p = binary.BigEndian.AppendUint16(p, 1080) // video height
	p = append(p, 0x10)                        // frame rate
	p = binary.BigEndian.AppendUint16(p, 0)    // composition number
	p = append(p, compState)
	if paletteUpdate {
		p = append(p, 0x80)
	} else {
		p = append(p, 0x00)
	}
	p = append(p, paletteID, byte(len(objs)))
	for _, o := range objs {
		p = binary.BigEndian.AppendUint16(p, o.ObjectID)
		p = append(p, o.WindowID, 0x00) // not cropped
		p = binary.BigEndian.AppendUint16(p, o.X)
		p = binary.BigEndian.AppendUint16(p, o.Y)
	}
	return p
}
