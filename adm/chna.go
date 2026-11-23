package adm

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ChnaEntry is one audioID row of the chna chunk (EBU Tech 3285).
// Layout per row: trackIndex(2) UID(12) trackFormatIDRef(14) packFormatIDRef(11).
type ChnaEntry struct {
	TrackIndex uint16
	UID        string // e.g. ATU_00000001
	TrackRef   string // e.g. AT_00031001_01
	PackRef    string // e.g. AP_00031001, may be empty
}

// Chna is the parsed chna chunk.
type Chna struct {
	NumTracks uint16
	NumUIDs   uint16
	Entries   []ChnaEntry
}

const chnaRowSize = 2 + 12 + 14 + 11

// ParseChna decodes a chna chunk payload.
func ParseChna(b []byte) (*Chna, error) {
	if len(b) < 4 {
		return nil, fmt.Errorf("chna chunk too short: %d bytes", len(b))
	}
	c := &Chna{
		NumTracks: binary.LittleEndian.Uint16(b[0:2]),
		NumUIDs:   binary.LittleEndian.Uint16(b[2:4]),
	}
	body := b[4:]
	if len(body)%chnaRowSize != 0 {
		return nil, fmt.Errorf("chna payload %d bytes is not a multiple of %d", len(body), chnaRowSize)
	}
	for off := 0; off+chnaRowSize <= len(body); off += chnaRowSize {
		row := body[off : off+chnaRowSize]
		c.Entries = append(c.Entries, ChnaEntry{
			TrackIndex: binary.LittleEndian.Uint16(row[0:2]),
			UID:        trimField(row[2:14]),
			TrackRef:   trimField(row[14:28]),
			PackRef:    trimField(row[28:39]),
		})
	}
	return c, nil
}

func trimField(b []byte) string {
	return strings.TrimRight(strings.TrimSpace(string(b)), "\x00")
}
