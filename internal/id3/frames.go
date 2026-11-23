package id3

import "encoding/binary"

func parseFrames(ver Version, body []byte) ([]*Frame, error) {
	var frames []*Frame
	pos := 0
	order := 0
	for pos+10 <= len(body) {
		if body[pos] == 0 { // padding begins
			break
		}
		idBytes := body[pos : pos+4]
		if !isFrameID(idBytes) {
			break
		}
		id := string(idBytes)
		var size uint32
		if ver.Major == 4 {
			size = Syncsafe(body[pos+4 : pos+8])
		} else {
			size = binary.BigEndian.Uint32(body[pos+4 : pos+8])
		}
		if size == 0 || pos+10+int(size) > len(body) {
			break
		}
		flags := binary.BigEndian.Uint16(body[pos+8 : pos+10])
		raw := append([]byte(nil), body[pos+10:pos+10+int(size)]...)
		f := &Frame{ID: id, Version: ver, Flags: flags, RawData: raw, Order: order}
		order++
		pos += 10 + int(size)

		data, err := decodeFramePayload(ver, f)
		if err != nil {
			return nil, err
		}
		f.Data = data
		switch {
		case id[0] == 'T':
			f.Text = DecodeText(data)
			if id == "TIT2" {
				f.Title = f.Text
			}
		case id == "CHAP":
			parseCHAP(f)
		case id == "CTOC":
			parseCTOC(ver, f)
		}
		frames = append(frames, f)
	}
	return frames, nil
}

// decodeFramePayload strips group/encryption/length prefixes and reverses
// frame-level unsynchronisation. Compressed or encrypted payloads are kept
// opaque (f.Compressed / f.Encrypted) and returned as-is for preservation.
func decodeFramePayload(ver Version, f *Frame) ([]byte, error) {
	data := f.RawData
	pos := 0
	if ver.Major == 4 {
		// v2.4 status flags: 0x40 group, 0x08 compression, 0x04 encryption,
		// 0x02 unsync, 0x01 data-length indicator.
		status := byte(f.Flags)
		if status&0x40 != 0 {
			if pos >= len(data) {
				return nil, ErrTruncated
			}
			f.HasGroup, f.GroupID = true, data[pos]
			pos++
		}
		if status&0x04 != 0 {
			if pos >= len(data) {
				return nil, ErrTruncated
			}
			f.Encrypted = true
			pos++
		}
		if status&0x01 != 0 {
			if pos+4 > len(data) {
				return nil, ErrTruncated
			}
			pos += 4 // data length indicator
		}
		if status&0x08 != 0 {
			f.Compressed = true
		}
		if f.Encrypted || f.Compressed {
			return data, nil
		}
		if status&0x02 != 0 {
			return Unsync(data[pos:]), nil
		}
		return append([]byte(nil), data[pos:]...), nil
	}

	// v2.3 format descriptions: 0x80 compression, 0x40 encryption,
	// 0x20 grouping identity.
	fmtDesc := byte(f.Flags >> 8)
	if fmtDesc&0x20 != 0 {
		if pos >= len(data) {
			return nil, ErrTruncated
		}
		f.HasGroup, f.GroupID = true, data[pos]
		pos++
	}
	if fmtDesc&0x40 != 0 {
		if pos >= len(data) {
			return nil, ErrTruncated
		}
		f.Encrypted = true
		pos++
	}
	if fmtDesc&0x80 != 0 {
		if pos+4 > len(data) {
			return nil, ErrTruncated
		}
		f.Compressed = true
		pos += 4
	}
	if f.Encrypted || f.Compressed {
		return data, nil
	}
	return append([]byte(nil), data[pos:]...), nil
}
