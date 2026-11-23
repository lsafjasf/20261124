package id3

import (
	"bytes"
	"encoding/binary"
)

// Syncsafe decodes a 28-bit synchsafe integer (each byte uses only 7 bits),
// as used by ID3v2 tag sizes and ID3v2.4 frame sizes.
func Syncsafe(b []byte) uint32 {
	var v uint32
	for _, x := range b {
		v = (v << 7) | uint32(x&0x7f)
	}
	return v
}

func putSyncsafe(v uint32, b []byte) {
	for i := len(b) - 1; i >= 0; i-- {
		b[i] = byte(v & 0x7f)
		v >>= 7
	}
}

// Unsync reverses ID3v2 unsynchronisation: every FF 00 becomes FF.
func Unsync(data []byte) []byte {
	if bytes.IndexByte(data, 0xff) < 0 {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		if data[i] == 0xff && i+1 < len(data) && data[i+1] == 0x00 {
			out = append(out, 0xff)
			i++
			continue
		}
		out = append(out, data[i])
	}
	return out
}

// Sync applies unsynchronisation encoding (FF 00 -> FF 00) and pads a final
// FF byte with 00, per ID3v2.3 tag-level unsynchronisation rules.
func Sync(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/255+1)
	for i := 0; i < len(data); i++ {
		out = append(out, data[i])
		if data[i] == 0xff {
			out = append(out, 0)
		}
	}
	return out
}

func be32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

func isFrameID(b []byte) bool {
	for _, c := range b {
		if c < 'A' || c > 'Z' {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// Parse reads an ID3v2.3/2.4 tag from the start of buf and retains the
// remaining bytes as Audio. It returns ErrNoTag when no tag is present.
func Parse(buf []byte) (*Tag, error) {
	if len(buf) < 10 || string(buf[0:3]) != "ID3" {
		return nil, ErrNoTag
	}
	major, minor := buf[3], buf[4]
	if major != 3 && major != 4 {
		return nil, ErrVersion
	}
	flags := buf[5]
	size := int(Syncsafe(buf[6:10]))
	tagEnd := 10 + size
	if len(buf) < tagEnd {
		return nil, ErrTruncated
	}

	t := &Tag{
		Version:     Version{Major: major, Minor: minor},
		HeaderFlags: flags,
		TagSize:     tagEnd,
		BodyStart:   10,
		BodyEnd:     tagEnd,
	}

	body := buf[10:tagEnd]
	// v2.4 footer flag (bit 4): a 10-byte footer sits at the end.
	if major == 4 && flags&0x10 != 0 {
		t.HasFooter = true
		body = body[:len(body)-10]
	}

	pos := 0
	if flags&0x40 != 0 { // extended header
		var extLen int
		if major == 4 {
			if len(body) < 4 {
				return nil, ErrBadHeader
			}
			extLen = int(Syncsafe(body[0:4]))
		} else {
			if len(body) < 4 {
				return nil, ErrBadHeader
			}
			extLen = int(binary.BigEndian.Uint32(body[0:4])) + 4
		}
		if extLen > len(body) {
			return nil, ErrBadHeader
		}
		t.HasExtHeader = true
		t.ExtHeader = append([]byte(nil), body[:extLen]...)
		pos = extLen
	}

	t.BodyStart = pos
	// Tag-level unsynchronisation applies to the frame section only; the
	// extended header was already captured in its original (wire) form.
	frameSection := body[pos:]
	if flags&0x80 != 0 {
		frameSection = Unsync(frameSection)
	}
	frames, err := parseFrames(t.Version, frameSection)
	if err != nil {
		return nil, err
	}
	t.Frames = frames
	t.Audio = buf[tagEnd:]
	return t, nil
}
