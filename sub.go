package main

import (
	"encoding/binary"
	"fmt"
	"io"
)

// EntryError ties a failure to one specific idx entry.
type EntryError struct {
	Entry Entry
	Msg   string
}

func (e *EntryError) Error() string {
	return fmt.Sprintf("entry #%d [%s lang=%d(%s) filepos=0x%09X]: %s",
		e.Entry.Index, e.Entry.Timestamp, e.Entry.LangIdx, e.Entry.LangID, e.Entry.FilePos, e.Msg)
}

func entryErr(e Entry, format string, args ...any) *EntryError {
	return &EntryError{Entry: e, Msg: fmt.Sprintf(format, args...)}
}

// Subtitle is a decoded SPU packet plus its location in the .sub file.
type Subtitle struct {
	Entry     Entry
	ByteStart int64 // first byte of the (pack/)PES data
	ByteEnd   int64 // one past the last byte of the SPU packet
	Width     int
	Height    int
	X, Y      int
	Pixels    []uint8  // Width*Height, values 0..3 (color slots)
	Colors    [4]uint8 // slot -> palette index (into idx 16-color palette)
	Alpha     [4]uint8 // slot -> alpha 0..255
}

// ByteRange returns the [start,end) byte range in the .sub file.
func (s *Subtitle) ByteRange() (int64, int64) { return s.ByteStart, s.ByteEnd }

// readSPU reads the SPU packet for one idx entry from the .sub file.
// r must be positioned at entry.FilePos. subSize is the total .sub file size,
// used to detect truncation. Returns the raw SPU packet and the byte range.
func readSPU(r io.ReaderAt, e Entry, subSize int64) (spu []byte, start, end int64, err error) {
	if e.FilePos < 0 || e.FilePos >= subSize {
		return nil, 0, 0, entryErr(e, "filepos 0x%X beyond .sub size 0x%X (truncated .sub?)", e.FilePos, subSize)
	}
	pos := e.FilePos
	start = pos

	readAt := func(buf []byte, off int64) error {
		n, rerr := r.ReadAt(buf, off)
		if rerr != nil && rerr != io.EOF {
			return rerr
		}
		if n < len(buf) {
			return io.ErrUnexpectedEOF
		}
		return nil
	}

	// Skip MPEG pack / system headers until the private_stream_1 PES.
	var hdr [6]byte
	for {
		if err := readAt(hdr[:], pos); err != nil {
			return nil, 0, 0, entryErr(e, "reading header at 0x%X: %v (truncated .sub)", pos, err)
		}
		if hdr[0] != 0x00 || hdr[1] != 0x00 || hdr[2] != 0x01 {
			return nil, 0, 0, entryErr(e, "no MPEG start code at 0x%X (got %02X %02X %02X)", pos, hdr[0], hdr[1], hdr[2])
		}
		switch hdr[3] {
		case 0xBA: // pack header: 12 bytes + stuffing length
			var rest [10]byte
			if err := readAt(rest[:], pos+4); err != nil {
				return nil, 0, 0, entryErr(e, "reading pack header at 0x%X: %v", pos, err)
			}
			stuffing := int64(rest[9] & 0x07)
			pos += 14 + stuffing
		case 0xBB: // system header: length-prefixed
			if err := readAt(hdr[4:6], pos+4); err != nil {
				return nil, 0, 0, entryErr(e, "reading system header at 0x%X: %v", pos, err)
			}
			pos += 6 + int64(binary.BigEndian.Uint16(hdr[4:6]))
		case 0xBD: // private_stream_1: subtitle PES
			goto pesFound
		default:
			return nil, 0, 0, entryErr(e, "unexpected stream id 0x%02X at 0x%X (not a subtitle PES)", hdr[3], pos)
		}
	}

pesFound:
	pesStart := pos
	if err := readAt(hdr[4:6], pos+4); err != nil {
		return nil, 0, 0, entryErr(e, "reading PES length at 0x%X: %v (truncated .sub)", pos, err)
	}
	pesLen := int64(binary.BigEndian.Uint16(hdr[4:6]))
	pesEnd := pos + 6 + pesLen
	if pesEnd > subSize {
		return nil, 0, 0, entryErr(e, "PES at 0x%X declares %d bytes but .sub ends at 0x%X (truncated .sub)",
			pesStart, 6+pesLen, subSize)
	}
	pos += 6

	// PES header: 2 flag bytes + header_data_length.
	var pesHdr [3]byte
	if err := readAt(pesHdr[:], pos); err != nil {
		return nil, 0, 0, entryErr(e, "reading PES header at 0x%X: %v", pos, err)
	}
	pos += 3 + int64(pesHdr[2])

	// Sub-stream id byte: must be 0x20+langIdx.
	var sid [1]byte
	if err := readAt(sid[:], pos); err != nil {
		return nil, 0, 0, entryErr(e, "reading sub-stream id at 0x%X: %v", pos, err)
	}
	pos++
	if sid[0] < 0x20 || sid[0] > 0x3F {
		return nil, 0, 0, entryErr(e, "byte 0x%02X at 0x%X is not a subtitle sub-stream id", sid[0], pos-1)
	}
	if got, want := int(sid[0]-0x20), e.LangIdx; got != want {
		return nil, 0, 0, entryErr(e,
			"sub-stream id 0x%02X is language stream %d, but idx entry expects stream %d (multi-language .sub: wrong filepos for this entry?)",
			sid[0], got, want)
	}

	spuLen := pesEnd - pos
	spu = make([]byte, spuLen)
	if err := readAt(spu, pos); err != nil {
		return nil, 0, 0, entryErr(e, "reading SPU packet at 0x%X: %v (truncated .sub)", pos, err)
	}
	end = pesEnd
	return spu, start, end, nil
}
