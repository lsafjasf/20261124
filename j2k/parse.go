package j2k

import (
	"fmt"
	"sort"
)

type parser struct {
	data            []byte
	out             *Codestream
	mainCoding      *codingParams
	tileCoding      map[int]*codingParams
	parts           []TilePart
	skipPacketIndex bool
}

// Analyze parses a JPEG 2000 codestream and always returns the structure found
// so far. Structural defects are reported in Codestream.Issues with byte ranges.
func Analyze(data []byte) *Codestream {
	p := &parser{
		data: data,
		out: &Codestream{
			ByteLength: len(data),
			Markers:    make([]Marker, 0, 32),
			Tiles:      make([]Tile, 0, 8),
			Issues:     make([]Issue, 0),
		},
		tileCoding: make(map[int]*codingParams),
	}
	pos := p.parseMainHeader()
	if pos >= 0 && pos+1 < len(data) && data[pos] == 0xff && data[pos+1] == 0x90 {
		pos = p.parseTileParts(pos)
	}
	if pos >= 0 && pos < len(data) {
		p.finishTail(pos)
	}
	if p.out.EOC == nil {
		end := len(data)
		start := maxInt(0, end-2)
		p.out.addIssue("missing_eoc", SeverityError, ByteRange{start, end},
			"codestream ended without EOC")
	}
	p.buildTiles()
	return p.out
}

// Parse is the error-returning form of Analyze. Malformed codestreams are
// represented by Issues, so the returned error is currently always nil.
func Parse(data []byte) (*Codestream, error) {
	return Analyze(data), nil
}

func (p *parser) parseMainHeader() int {
	if len(p.data) < 2 || p.data[0] != 0xff || p.data[1] != 0x4f {
		p.out.addIssue("soc_missing", SeverityError, ByteRange{0, minInt(2, len(p.data))},
			"codestream does not begin with SOC")
		start := findByteMarker(p.data, markerSOC, 0, len(p.data))
		if start < 0 {
			return len(p.data)
		}
		p.out.addIssue("soc_not_at_start", SeverityWarning, ByteRange{0, start + 2},
			"SOC found at offset %d", start)
		p.out.SOC = ByteRange{start, start + 2}
		p.out.Markers = append(p.out.Markers, Marker{
			Name: "SOC", Code: markerSOC, Scope: "main",
			Range: ByteRange{start, start + 2}, MarkerRange: ByteRange{start, start + 2},
			PayloadRange: ByteRange{start + 2, start + 2}, Standalone: true,
		})
		return p.scanMainMarkers(start + 2)
	}
	p.out.SOC = ByteRange{0, 2}
	p.out.Markers = append(p.out.Markers, Marker{
		Name: "SOC", Code: markerSOC, Scope: "main",
		Range: ByteRange{0, 2}, MarkerRange: ByteRange{0, 2},
		PayloadRange: ByteRange{2, 2}, Standalone: true,
	})
	return p.scanMainMarkers(2)
}

func (p *parser) scanMainMarkers(pos int) int {
	for pos < len(p.data) {
		if pos+1 < len(p.data) && p.data[pos] == 0xff && p.data[pos+1] == 0xff {
			pos++
			continue
		}
		code, ok := markerAt(p.data, pos)
		if !ok {
			next := findNextFF(p.data, pos+1, len(p.data))
			if next < 0 {
				p.out.addIssue("truncated_marker", SeverityError, ByteRange{pos, len(p.data)},
					"expected marker, reached end of codestream")
				return len(p.data)
			}
			p.out.addIssue("expected_marker", SeverityWarning, ByteRange{pos, next},
				"expected marker prefix at offset %d", pos)
			pos = next
			continue
		}
		switch code {
		case markerEOC:
			p.recordEOC(pos, "main", 0)
			return pos + 2
		case markerSOT:
			return pos
		case markerSOC:
			p.out.addIssue("duplicate_soc", SeverityWarning, ByteRange{pos, pos + 2},
				"duplicate SOC marker")
			pos += 2
		case markerSOD, markerSOP, markerEPH:
			p.out.addIssue("unexpected_marker", SeverityError, ByteRange{pos, pos + 2},
				"%s outside a tile-part data section", markerName(code))
			pos += 2
		default:
			seg, _ := p.readMarkerAt(pos, len(p.data), "main", 0)
			if !seg.complete {
				return len(p.data)
			}
			p.handleMainSegment(seg)
			pos = seg.end
		}
	}
	return pos
}

func (p *parser) handleMainSegment(seg markerSegment) {
	switch seg.code {
	case markerSIZ:
		if p.out.SIZ != nil {
			p.out.addIssue("duplicate_siz", SeverityWarning, ByteRange{seg.start, seg.end},
				"duplicate SIZ marker segment")
			return
		}
		p.out.SIZ = p.parseSIZ(seg)
	case markerCOD:
		if p.out.COD != nil {
			p.out.addIssue("duplicate_cod", SeverityWarning, ByteRange{seg.start, seg.end},
				"duplicate main-header COD marker segment")
			return
		}
		p.out.COD = p.parseCOD(seg)
		if p.out.COD != nil && p.out.SIZ != nil {
			p.mainCoding = codingFromCOD(p.out.COD, int(p.out.SIZ.Csiz))
		}
	case markerCOC:
		if p.out.SIZ == nil {
			p.out.addIssue("invalid_coc", SeverityError, ByteRange{seg.start, seg.end},
				"COC encountered before SIZ")
			return
		}
		idx, coc, ok := p.parseCOC(seg, int(p.out.SIZ.Csiz))
		if ok && p.mainCoding != nil && idx < len(p.mainCoding.components) {
			p.mainCoding.components[idx] = coc
		}
	case markerPOC:
		p.out.addIssue("unsupported_poc", SeverityWarning, ByteRange{seg.start, seg.end},
			"POC progression change is not applied to packet ordering")
		p.skipPacketIndex = true
	case markerPPM:
		p.out.addIssue("unsupported_packed_headers", SeverityWarning, ByteRange{seg.start, seg.end},
			"PPM packed packet headers are not parsed")
		p.skipPacketIndex = true
	}
}

func (p *parser) recordEOC(pos int, scope string, tilePart int) {
	if pos < 0 || pos+2 > len(p.data) || p.out.EOC != nil {
		return
	}
	r := ByteRange{pos, pos + 2}
	p.out.EOC = &r
	p.out.Markers = append(p.out.Markers, Marker{
		Name: "EOC", Code: markerEOC, Scope: scope, TilePart: tilePart,
		Range: r, MarkerRange: r, PayloadRange: ByteRange{pos + 2, pos + 2},
		Standalone: true,
	})
}

func (p *parser) finishTail(pos int) {
	if pos >= len(p.data) {
		return
	}
	if code, ok := markerAt(p.data, pos); ok {
		switch code {
		case markerEOC:
			p.recordEOC(pos, "main", 0)
			if pos+2 < len(p.data) {
				p.out.addIssue("trailing_data", SeverityWarning, ByteRange{pos + 2, len(p.data)},
					"%d bytes follow EOC", len(p.data)-pos-2)
			}
			return
		case markerSOT:
			p.out.addIssue("truncated_tile_part", SeverityError, ByteRange{pos, len(p.data)},
				"tile-part begins after previous data but was not parsed")
			return
		}
	}
	p.out.addIssue("trailing_data", SeverityError, ByteRange{pos, len(p.data)},
		"data remains after the last parsed tile-part")
}

func (p *parser) parseTileParts(pos int) int {
	for pos < len(p.data) {
		if pos+1 < len(p.data) && p.data[pos] == 0xff && p.data[pos+1] == 0xff {
			pos++
			continue
		}
		code, ok := markerAt(p.data, pos)
		if !ok || code != markerSOT {
			if ok && code == markerEOC {
				p.recordEOC(pos, "main", 0)
				return pos + 2
			}
			next, _ := findNextSOTOrEOC(p.data, pos+1, len(p.data))
			end := len(p.data)
			if next >= 0 {
				end = next
			}
			p.out.addIssue("truncated_tile_part", SeverityError, ByteRange{pos, end},
				"expected SOT marker at offset %d", pos)
			if next < 0 {
				return len(p.data)
			}
			pos = next
			continue
		}
		part, next := p.parseOneTilePart(pos, len(p.parts))
		p.parts = append(p.parts, part)
		if next <= pos {
			next = pos + 2
		}
		pos = next
		if pos < len(p.data) {
			if c, yes := markerAt(p.data, pos); yes && c == markerEOC {
				p.recordEOC(pos, "main", 0)
				return pos + 2
			}
		}
	}
	return pos
}

func (p *parser) parseOneTilePart(pos, ordinal int) (TilePart, int) {
	part := TilePart{
		Index: ordinal, Range: ByteRange{pos, pos}, Complete: true,
		Packets: make([]Packet, 0),
	}
	seg, _ := p.readMarkerAt(pos, len(p.data), "tile", ordinal)
	part.SOT.Range = ByteRange{pos, minInt(pos+12, len(p.data))}
	part.SOT.LengthFieldRange = ByteRange{pos + 2, minInt(pos+4, len(p.data))}
	if !seg.complete || seg.length != 10 || seg.payloadEnd-seg.payloadStart < 8 {
		end := len(p.data)
		if seg.complete {
			end = seg.end
		}
		part.Range.End = end
		part.Header = ByteRange{end, end}
		part.Data = ByteRange{end, end}
		part.Complete = false
		p.out.addIssue("invalid_sot", SeverityError, ByteRange{pos, end},
			"SOT at offset %d must have Lsot=10 and eight payload bytes", pos)
		return part, end
	}
	part.SOT.Isot = int(u16(p.data, seg.payloadStart))
	part.SOT.Psot = u32(p.data, seg.payloadStart+2)
	part.SOT.TPsot = int(p.data[seg.payloadStart+6])
	part.SOT.TNsot = int(p.data[seg.payloadStart+7])
	part.Isot = part.SOT.Isot
	part.TPsot = part.SOT.TPsot
	part.TNsot = part.SOT.TNsot
	part.DeclaredLength = part.SOT.Psot

	if p.out.SIZ != nil {
		total := p.out.SIZ.TilesX * p.out.SIZ.TilesY
		if total > 0 && (part.Isot < 0 || part.Isot >= total) {
			p.out.addIssue("invalid_sot", SeverityError, ByteRange{seg.payloadStart, seg.payloadStart + 2},
				"SOT tile index %d is outside 0..%d", part.Isot, total-1)
		}
	}
	if part.TNsot != 0 && part.TPsot >= part.TNsot {
		p.out.addIssue("invalid_sot", SeverityError, ByteRange{seg.payloadStart + 6, seg.payloadStart + 8},
			"TPsot %d is not below TNsot %d", part.TPsot, part.TNsot)
	}

	end := len(p.data)
	if part.SOT.Psot != 0 {
		declared := int64(pos) + int64(part.SOT.Psot)
		psotOverflow := false
		if declared < int64(seg.end) || declared > int64(len(p.data)) {
			psotOverflow = true
			part.Complete = false
			p.out.addIssue("length_overflow", SeverityError, ByteRange{pos + 6, pos + 10},
				"Psot %d places tile-part end at %d outside [%d,%d]",
				part.SOT.Psot, declared, seg.end, len(p.data))
		} else {
			end = int(declared)
		}
		if end < seg.end {
			end = seg.end
		}
		if boundary, boundaryCode := findNextSOTOrEOC(p.data, seg.end, end); boundary >= 0 {
			if !psotOverflow {
				part.Complete = false
				p.out.addIssue("length_overflow", SeverityError, ByteRange{pos + 6, pos + 10},
					"Psot %d overlaps %s at offset %d", part.SOT.Psot,
					markerName(boundaryCode), boundary)
			}
			for i := len(p.out.Issues) - 1; i >= 0; i-- {
				if p.out.Issues[i].Code == "length_overflow" &&
					p.out.Issues[i].Range.Start == pos+6 {
					p.out.Issues[i].Related = append(p.out.Issues[i].Related,
						ByteRange{boundary, boundary + 2})
					break
				}
			}
			end = boundary
		}
	} else {
		eoc := findByteMarker(p.data, markerEOC, seg.end, len(p.data))
		if eoc >= 0 {
			end = eoc
		} else {
			part.Complete = false
			p.out.addIssue("truncated_tile_part", SeverityError, ByteRange{seg.end, len(p.data)},
				"Psot=0 tile-part has no EOC terminator")
		}
	}
	part.Range.End = end
	part.Complete = part.Complete && end <= len(p.data) && end >= seg.end

	headerEnd, sod, dataEnd, ok := p.parseTileHeader(seg.end, end, part.Isot, ordinal)
	part.Header = ByteRange{seg.end, headerEnd}
	part.SOD = sod
	part.Data = ByteRange{headerEnd, dataEnd}
	if !ok {
		part.Complete = false
	}
	if part.SOD != nil {
		part.Data = ByteRange{part.SOD.End, end}
	}
	return part, end
}

func (p *parser) parseTileHeader(start, limit, tile int, ordinal int) (int, *ByteRange, int, bool) {
	pos := start
	for pos < limit {
		if pos+1 < limit && p.data[pos] == 0xff && p.data[pos+1] == 0xff {
			pos++
			continue
		}
		code, ok := markerAt(p.data, pos)
		if !ok {
			next := findNextFF(p.data, pos+1, limit)
			if next < 0 {
				p.out.addIssue("truncated_marker", SeverityError, ByteRange{pos, limit},
					"tile-part header has no SOD marker")
				return limit, nil, limit, false
			}
			p.out.addIssue("expected_marker", SeverityWarning, ByteRange{pos, next},
				"expected marker in tile-part header")
			pos = next
			continue
		}
		switch code {
		case markerSOD:
			r := ByteRange{pos, pos + 2}
			p.out.Markers = append(p.out.Markers, Marker{
				Name: "SOD", Code: markerSOD, Scope: "tile", TilePart: ordinal,
				Range: r, MarkerRange: r, PayloadRange: ByteRange{pos + 2, pos + 2},
				Standalone: true,
			})
			return pos, &r, pos + 2, true
		case markerSOT, markerEOC:
			p.out.addIssue("truncated_marker", SeverityError, ByteRange{pos, limit},
				"tile-part header ended before SOD")
			return pos, nil, pos, false
		case markerSOC, markerSOP, markerEPH:
			p.out.addIssue("unexpected_marker", SeverityWarning, ByteRange{pos, pos + 2},
				"unexpected %s in tile-part header", markerName(code))
			pos += 2
		default:
			seg, _ := p.readMarkerAt(pos, limit, "tile", ordinal)
			if !seg.complete {
				return limit, nil, limit, false
			}
			p.applyTileSegment(tile, seg)
			pos = seg.end
		}
	}
	p.out.addIssue("truncated_marker", SeverityError, ByteRange{start, limit},
		"tile-part header has no SOD marker")
	return limit, nil, limit, false
}

func (p *parser) applyTileSegment(tile int, seg markerSegment) {
	if p.out.SIZ == nil {
		return
	}
	cp := p.tileCoding[tile]
	if cp == nil {
		cp = cloneCoding(p.mainCoding)
		if cp == nil {
			cp = &codingParams{components: make([]ComponentCoding, int(p.out.SIZ.Csiz))}
		}
		p.tileCoding[tile] = cp
	}
	switch seg.code {
	case markerCOD:
		cod := p.parseCOD(seg)
		if cod == nil {
			return
		}
		cp.progression, cp.layers, cp.mct = cod.Progression, cod.Layers, cod.MCT
		cp.sop, cp.eph = cod.Scod&0x02 != 0, cod.Scod&0x04 != 0
		cp.hasCOD = true
		for i := 0; i < len(cp.components); i++ {
			cp.components[i] = ComponentCoding{
				Index: i, Decompositions: cod.Decompositions, Resolutions: cod.Resolutions,
				CodeBlockWidthExp: cod.CodeBlockWidthExp, CodeBlockHeightExp: cod.CodeBlockHeightExp,
				CodeBlockStyle: cod.CodeBlockStyle, Transform: cod.Transform,
				Precincts: append([]PrecinctSize(nil), cod.Precincts...),
			}
		}
	case markerCOC:
		idx, coc, ok := p.parseCOC(seg, int(p.out.SIZ.Csiz))
		if ok && idx >= 0 && idx < len(cp.components) {
			cp.components[idx] = coc
		}
	case markerPOC:
		p.out.addIssue("unsupported_poc", SeverityWarning, ByteRange{seg.start, seg.end},
			"POC progression change is not applied to packet ordering")
		p.skipPacketIndex = true
	case markerPPT:
		p.out.addIssue("unsupported_packed_headers", SeverityWarning, ByteRange{seg.start, seg.end},
			"PPT packed packet headers are not parsed")
		p.skipPacketIndex = true
	}
}

func findNextFF(data []byte, start, limit int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i+1 < limit; i++ {
		if data[i] == 0xff {
			return i
		}
	}
	return -1
}

func findByteMarker(data []byte, marker uint16, start, limit int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i+1 < limit; i++ {
		if data[i] == 0xff && data[i+1] == byte(marker) {
			return i
		}
	}
	return -1
}

func findNextSOTOrEOC(data []byte, start, limit int) (int, uint16) {
	for i := start; i+1 < limit; i++ {
		if data[i] != 0xff {
			continue
		}
		code := uint16(data[i])<<8 | uint16(data[i+1])
		if code == markerSOT || code == markerEOC {
			return i, code
		}
	}
	return -1, 0
}

func (t TilePart) String() string {
	return fmt.Sprintf("tile-part tile=%d index=%d %s", t.Isot, t.TPsot, t.Range)
}

func sortTileParts(parts []TilePart) {
	sort.SliceStable(parts, func(i, j int) bool {
		if parts[i].TPsot != parts[j].TPsot {
			return parts[i].TPsot < parts[j].TPsot
		}
		return parts[i].Index < parts[j].Index
	})
}
