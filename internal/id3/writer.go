package id3

import (
	"encoding/binary"
	"sort"
)

// WriteOptions controls regeneration of a tag block.
type WriteOptions struct {
	DropFrames map[string]bool // frame ids to omit from the new tag
	Padding    int             // zero bytes appended inside the tag
}

// WriteBlock serializes a new ID3 tag block. Frame order is preserved, unknown
// frames are re-emitted with their exact wire payload, and CHAP/CTOC/TIT2
// frames are rebuilt from their decoded representation. The audio bytes are
// not touched by this function.
func WriteBlock(tag *Tag, opt WriteOptions) []byte {
	frames := make([]*Frame, 0, len(tag.Frames))
	for _, f := range tag.Frames {
		if opt.DropFrames != nil && opt.DropFrames[f.ID] {
			continue
		}
		frames = append(frames, f)
	}
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].Order < frames[j].Order })

	var body []byte
	if tag.HasExtHeader {
		body = append(body, tag.ExtHeader...)
	}
	for _, f := range frames {
		body = append(body, encodeFrame(tag.Version, f)...)
	}
	body = append(body, make([]byte, opt.Padding)...)

	flags := byte(0)
	if tag.Version.Major == 3 {
		// v2.3 only supports tag-level unsynchronisation.
		body = Sync(body)
		flags = 0x80
	}

	header := make([]byte, 10)
	header[0], header[1], header[2] = 'I', 'D', '3'
	header[3] = tag.Version.Major
	header[4] = tag.Version.Minor
	header[5] = flags
	putSyncsafe(uint32(len(body)), header[6:10])

	out := make([]byte, 0, 10+len(body))
	out = append(out, header...)
	out = append(out, body...)
	return out
}

func encodeFrame(ver Version, f *Frame) []byte {
	payload := framePayload(f)
	header := make([]byte, 10)
	copy(header[0:4], f.ID)
	if ver.Major == 4 {
		putSyncsafe(uint32(len(payload)), header[4:8])
	} else {
		binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	}
	binary.BigEndian.PutUint16(header[8:10], f.Flags)
	return append(header, payload...)
}

func framePayload(f *Frame) []byte {
	// Opaque frames (encrypted/compressed) or anything other than the frame
	// types we understand are preserved on the wire exactly as parsed.
	if f.Compressed || f.Encrypted || (f.ID != "CHAP" && f.ID != "CTOC") {
		return append([]byte(nil), f.RawData...)
	}

	var payload []byte
	payload = append(payload, f.ElementID...)
	payload = append(payload, 0)
	if f.ID == "CHAP" {
		payload = appendU32(payload, f.Start)
		payload = appendU32(payload, f.End)
		payload = appendU32(payload, f.StOff)
		payload = appendU32(payload, f.EnOff)
	} else {
		payload = append(payload, f.encodedCTOCFlags(), byte(len(f.ChildIDs)))
		for _, c := range f.ChildIDs {
			payload = append(payload, c...)
			payload = append(payload, 0)
		}
	}
	for _, sub := range f.SubFrames {
		payload = append(payload, encodeFrame(f.Version, sub)...)
	}

	// Re-apply any parsed prefixes (group id) and frame-level unsync so the
	// wire representation stays self-consistent with the frame flags.
	return rewrapPayload(f, payload)
}

func appendU32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (f *Frame) encodedCTOCFlags() byte {
	// Recover the flags byte from the raw payload (element id + NUL + flags).
	for i := 0; i+1 < len(f.RawData); i++ {
		if f.RawData[i] == 0 {
			return f.RawData[i+1]
		}
	}
	return 0x03
}

func rewrapPayload(f *Frame, core []byte) []byte {
	if f.Version.Major == 4 {
		status := byte(f.Flags)
		var prefix []byte
		if status&0x40 != 0 && f.HasGroup {
			prefix = append(prefix, f.GroupID)
		}
		out := append(prefix, core...)
		if status&0x02 != 0 {
			out = Sync(out)
		}
		return out
	}
	fmtDesc := byte(f.Flags >> 8)
	var prefix []byte
	if fmtDesc&0x20 != 0 && f.HasGroup {
		prefix = append(prefix, f.GroupID)
	}
	return append(prefix, core...)
}
