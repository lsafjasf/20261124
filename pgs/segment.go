package pgs

import (
	"encoding/binary"
	"fmt"
)

// PGS segment types, named after the role they play when rebuilding
// epoch state in arrival order.
const (
	TypePalette = 0x14 // PDS: palette definition
	TypeObject  = 0x15 // ODS: object (bitmap) definition
	TypeDisplay = 0x16 // PCS: presentation composition ("display")
	TypeWindow  = 0x17 // WDS: window definition
	TypeClear   = 0x80 // END: ends a display set ("clear")
)

// PCS composition_state values.
const (
	StateNormal      = 0x00
	StateAcquisition = 0x40
	StateEpochStart  = 0x80
)

// ODS last_in_sequence_flag values.
const (
	SeqLast      = 0x40
	SeqFirst     = 0x80
	SeqFirstLast = 0xC0
)

// TypeName maps a segment type byte to its rebuild-stage name.
func TypeName(t byte) string {
	switch t {
	case TypePalette:
		return "palette"
	case TypeObject:
		return "object"
	case TypeDisplay:
		return "display"
	case TypeWindow:
		return "window"
	case TypeClear:
		return "clear"
	default:
		return fmt.Sprintf("unknown(0x%02x)", t)
	}
}

type PaletteEntry struct {
	ID, Y, Cr, Cb, Alpha byte
}

type PaletteSegment struct {
	ID      byte
	Version byte
	Entries []PaletteEntry
}

type WindowDef struct {
	ID         byte
	X, Y, W, H uint16
}

type WindowSegment struct {
	Windows []WindowDef
}

type ObjectSegment struct {
	ID         uint16
	Version    byte
	Sequence   byte
	DataLength uint32 // 24-bit, includes the 4 width/height bytes in first fragment
	Width      uint16 // valid only when Sequence has the "first" bit
	Height     uint16
	Data       []byte
}

type CompositionObject struct {
	ObjectID uint16
	WindowID byte
	Cropped  bool
	X, Y     uint16
}

type DisplaySegment struct {
	Width, Height     uint16
	FrameRate         byte
	CompositionNumber uint16
	CompositionState  byte
	PaletteUpdate     bool
	PaletteID         byte
	Objects           []CompositionObject
}

// Segment is one parsed PGS segment with its timing and typed payload.
type Segment struct {
	Index  int
	Offset int
	Type   byte
	PTS    uint32 // 90 kHz clock
	DTS    uint32
	Size   uint16 // payload size

	Palette *PaletteSegment
	Window  *WindowSegment
	Object  *ObjectSegment
	Display *DisplaySegment
}

// Parse decodes a whole .sup stream into segments, in arrival order.
func Parse(data []byte) ([]Segment, error) {
	var out []Segment
	off := 0
	for off < len(data) {
		if len(data)-off < 13 {
			return nil, fmt.Errorf("truncated segment header at offset %d", off)
		}
		if data[off] != 0x50 || data[off+1] != 0x47 { // "PG"
			return nil, fmt.Errorf("bad magic at offset %d: %02x %02x", off, data[off], data[off+1])
		}
		seg := Segment{
			Index:  len(out),
			Offset: off,
			PTS:    binary.BigEndian.Uint32(data[off+2:]),
			DTS:    binary.BigEndian.Uint32(data[off+6:]),
			Type:   data[off+10],
			Size:   binary.BigEndian.Uint16(data[off+11:]),
		}
		payload := data[off+13:]
		if len(payload) < int(seg.Size) {
			return nil, fmt.Errorf("truncated payload for segment %d (%s)", seg.Index, TypeName(seg.Type))
		}
		payload = payload[:seg.Size]
		if err := seg.parsePayload(payload); err != nil {
			return nil, fmt.Errorf("segment %d (%s): %w", seg.Index, TypeName(seg.Type), err)
		}
		out = append(out, seg)
		off += 13 + int(seg.Size)
	}
	return out, nil
}
