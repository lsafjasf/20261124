package j2k

import "fmt"

const (
	markerSOC uint16 = 0xff4f
	markerSIZ uint16 = 0xff51
	markerCOD uint16 = 0xff52
	markerCOC uint16 = 0xff53
	markerTLM uint16 = 0xff55
	markerPLM uint16 = 0xff57
	markerPLT uint16 = 0xff58
	markerQCD uint16 = 0xff5c
	markerQCC uint16 = 0xff5d
	markerRGN uint16 = 0xff5e
	markerPOC uint16 = 0xff5f
	markerPPM uint16 = 0xff60
	markerPPT uint16 = 0xff61
	markerCRG uint16 = 0xff63
	markerCOM uint16 = 0xff64
	markerSOT uint16 = 0xff90
	markerSOP uint16 = 0xff91
	markerEPH uint16 = 0xff92
	markerSOD uint16 = 0xff93
	markerEOC uint16 = 0xffd9
)

var markerNames = map[uint16]string{
	markerSOC: "SOC", markerSIZ: "SIZ", markerCOD: "COD", markerCOC: "COC",
	markerTLM: "TLM", markerPLM: "PLM", markerPLT: "PLT", markerQCD: "QCD",
	markerQCC: "QCC", markerRGN: "RGN", markerPOC: "POC", markerPPM: "PPM",
	markerPPT: "PPT", markerCRG: "CRG", markerCOM: "COM", markerSOT: "SOT",
	markerSOP: "SOP", markerEPH: "EPH", markerSOD: "SOD", markerEOC: "EOC",
}

func markerName(code uint16) string {
	if name, ok := markerNames[code]; ok {
		return name
	}
	return fmt.Sprintf("0xFF%02X", uint8(code))
}

func markerAt(data []byte, pos int) (uint16, bool) {
	if pos < 0 || pos+2 > len(data) || data[pos] != 0xff {
		return 0, false
	}
	return uint16(data[pos])<<8 | uint16(data[pos+1]), true
}

func standaloneMarker(code uint16) bool {
	switch code {
	case markerSOC, markerSOD, markerEOC:
		return true
	default:
		return false
	}
}

type markerSegment struct {
	code         uint16
	start        int
	end          int
	payloadStart int
	payloadEnd   int
	length       int
	standalone   bool
	complete     bool
}

// readMarkerAt parses one marker or marker segment. It records the marker and
// emits a byte-range diagnostic when the declared segment is not contained in
// [pos, limit).
func (p *parser) readMarkerAt(pos, limit int, scope string, tilePart int) (markerSegment, bool) {
	code, ok := markerAt(p.data, pos)
	if !ok {
		return markerSegment{}, false
	}
	seg := markerSegment{
		code:         code,
		start:        pos,
		end:          pos + 2,
		payloadStart: pos + 2,
		payloadEnd:   pos + 2,
		standalone:   standaloneMarker(code),
		complete:     true,
	}
	if seg.standalone {
		p.addMarker(seg, scope, tilePart)
		return seg, true
	}
	if pos+4 > limit {
		seg.complete = false
		seg.end = limit
		seg.payloadEnd = limit
		p.out.addIssue("truncated_marker", SeverityError, ByteRange{pos, limit},
			"%s at offset %d has no complete length field", markerName(code), pos)
		p.addMarker(seg, scope, tilePart)
		return seg, true
	}
	seg.length = int(uint16(p.data[pos+2])<<8 | uint16(p.data[pos+3]))
	if seg.length < 2 {
		seg.complete = false
		seg.payloadStart, seg.payloadEnd = pos+4, pos+4
		p.out.addIssue("invalid_marker_length", SeverityError, ByteRange{pos + 2, pos + 4},
			"%s at offset %d declares invalid length %d", markerName(code), pos, seg.length)
		p.addMarker(seg, scope, tilePart)
		return seg, true
	}
	declaredEnd := pos + 2 + seg.length
	seg.payloadStart = pos + 4
	if declaredEnd > limit {
		seg.complete = false
		seg.end = limit
		seg.payloadEnd = limit
		p.out.addIssue("length_overflow", SeverityError, ByteRange{pos + 2, pos + 4},
			"%s at offset %d declares end %d beyond limit %d",
			markerName(code), pos, declaredEnd, limit)
		p.addMarker(seg, scope, tilePart)
		return seg, true
	}
	seg.end = declaredEnd
	seg.payloadEnd = declaredEnd
	p.addMarker(seg, scope, tilePart)
	return seg, true
}

func (p *parser) addMarker(seg markerSegment, scope string, tilePart int) {
	p.out.Markers = append(p.out.Markers, Marker{
		Name:         markerName(seg.code),
		Code:         seg.code,
		Scope:        scope,
		TilePart:     tilePart,
		Range:        ByteRange{seg.start, seg.end},
		MarkerRange:  ByteRange{seg.start, minInt(seg.start+2, len(p.data))},
		PayloadRange: ByteRange{seg.payloadStart, seg.payloadEnd},
		Length:       seg.length,
		Standalone:   seg.standalone,
	})
}

func u16(data []byte, off int) uint16 {
	return uint16(data[off])<<8 | uint16(data[off+1])
}

func u32(data []byte, off int) uint32 {
	return uint32(data[off])<<24 | uint32(data[off+1])<<16 |
		uint32(data[off+2])<<8 | uint32(data[off+3])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
