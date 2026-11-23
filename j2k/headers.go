package j2k

import "fmt"

type codingParams struct {
	progression int
	layers      int
	mct         uint8
	sop         bool
	eph         bool
	components  []ComponentCoding
	hasCOD      bool
}

func progressionName(progression int) string {
	switch progression {
	case 0:
		return "LRCP"
	case 1:
		return "RLCP"
	case 2:
		return "RPCL"
	case 3:
		return "PCRL"
	case 4:
		return "CPRL"
	default:
		return fmt.Sprintf("unknown(%d)", progression)
	}
}

func (p *parser) parseSIZ(seg markerSegment) *SIZ {
	payload := p.data[seg.payloadStart:seg.payloadEnd]
	if len(payload) < 36 {
		p.out.addIssue("invalid_marker_length", SeverityError,
			ByteRange{seg.payloadStart, seg.payloadEnd},
			"SIZ payload has %d bytes, want at least 36", len(payload))
		return nil
	}
	siz := &SIZ{
		Range:  ByteRange{seg.start, seg.end},
		Rsiz:   u16(payload, 0),
		Xsiz:   u32(payload, 2),
		Ysiz:   u32(payload, 6),
		XOsiz:  u32(payload, 10),
		YOsiz:  u32(payload, 14),
		XTsiz:  u32(payload, 18),
		YTsiz:  u32(payload, 22),
		XTOsiz: u32(payload, 26),
		YTOsiz: u32(payload, 30),
		Csiz:   u16(payload, 34),
	}
	need := 36 + 3*int(siz.Csiz)
	if len(payload) < need {
		p.out.addIssue("invalid_marker_length", SeverityError,
			ByteRange{seg.payloadStart + len(payload), seg.payloadStart + need},
			"SIZ declares %d components but its payload is truncated", siz.Csiz)
		return nil
	}
	for i := 0; i < int(siz.Csiz); i++ {
		off := 36 + 3*i
		comp := Component{
			Index: i,
			Ssiz:  payload[off],
			XRsiz: payload[off+1],
			YRsiz: payload[off+2],
			Range: ByteRange{seg.payloadStart + off, seg.payloadStart + off + 3},
		}
		if comp.XRsiz == 0 || comp.YRsiz == 0 {
			p.out.addIssue("invalid_siz_component", SeverityError, comp.Range,
				"component %d has zero subsampling", i)
		}
		siz.Components = append(siz.Components, comp)
	}
	if siz.Csiz == 0 {
		p.out.addIssue("invalid_siz", SeverityError, siz.Range, "SIZ declares zero components")
	}
	if siz.Xsiz <= siz.XOsiz || siz.Ysiz <= siz.YOsiz {
		p.out.addIssue("invalid_siz", SeverityError, ByteRange{seg.payloadStart, seg.payloadStart + 36},
			"invalid image extent Xsiz/Ysiz")
	}
	if siz.XTsiz == 0 || siz.YTsiz == 0 {
		p.out.addIssue("invalid_siz", SeverityError, ByteRange{seg.payloadStart + 18, seg.payloadStart + 26},
			"tile size must be non-zero")
	} else if siz.Xsiz < siz.XTOsiz || siz.Ysiz < siz.YTOsiz {
		p.out.addIssue("invalid_siz", SeverityError, ByteRange{seg.payloadStart, seg.payloadStart + 34},
			"image extent lies before the tile grid origin")
	} else {
		tx := ceilDivU64(uint64(siz.Xsiz)-uint64(siz.XTOsiz), uint64(siz.XTsiz))
		ty := ceilDivU64(uint64(siz.Ysiz)-uint64(siz.YTOsiz), uint64(siz.YTsiz))
		if tx > uint64(^uint(0)>>1) || ty > uint64(^uint(0)>>1) ||
			(tx > 0 && ty > uint64(^uint(0)>>1)/tx) {
			p.out.addIssue("length_overflow", SeverityError, siz.Range,
				"tile grid dimensions overflow")
		} else if tx*ty > 10_000_000 {
			p.out.addIssue("resource_limit", SeverityError, siz.Range,
				"tile grid contains too many tiles (%d)", tx*ty)
		} else {
			siz.TilesX, siz.TilesY = int(tx), int(ty)
		}
	}
	return siz
}

func (p *parser) parseCOD(seg markerSegment) *COD {
	payload := p.data[seg.payloadStart:seg.payloadEnd]
	if len(payload) < 10 {
		p.out.addIssue("invalid_marker_length", SeverityError,
			ByteRange{seg.payloadStart, seg.payloadEnd},
			"COD payload has %d bytes, want at least 10", len(payload))
		return nil
	}
	cod := &COD{
		Range:              ByteRange{seg.start, seg.end},
		Scod:               payload[0],
		Progression:        int(payload[1]),
		Layers:             int(u16(payload, 2)),
		MCT:                payload[4],
		Decompositions:     int(payload[5]),
		CodeBlockWidthExp:  int(payload[6]) + 2,
		CodeBlockHeightExp: int(payload[7]) + 2,
		CodeBlockStyle:     payload[8],
		Transform:          payload[9],
	}
	cod.ProgressionName = progressionName(cod.Progression)
	cod.Resolutions = cod.Decompositions + 1
	if cod.Progression < 0 || cod.Progression > 4 {
		p.out.addIssue("invalid_cod", SeverityError,
			ByteRange{seg.payloadStart + 1, seg.payloadStart + 2},
			"unknown progression order %d", cod.Progression)
	}
	if cod.Layers < 1 {
		p.out.addIssue("invalid_cod", SeverityError,
			ByteRange{seg.payloadStart + 2, seg.payloadStart + 4},
			"COD declares zero quality layers")
	}
	if cod.Decompositions > 32 || cod.Resolutions > 33 {
		p.out.addIssue("resource_limit", SeverityError,
			ByteRange{seg.payloadStart + 5, seg.payloadStart + 6},
			"COD declares too many resolution levels (%d)", cod.Resolutions)
	}
	if cod.CodeBlockWidthExp < 2 || cod.CodeBlockWidthExp > 10 ||
		cod.CodeBlockHeightExp < 2 || cod.CodeBlockHeightExp > 10 {
		p.out.addIssue("invalid_cod", SeverityWarning,
			ByteRange{seg.payloadStart + 6, seg.payloadStart + 8},
			"unusual code-block dimensions %dx%d",
			1<<cod.CodeBlockWidthExp, 1<<cod.CodeBlockHeightExp)
	}
	if cod.Scod&0x01 != 0 {
		off := seg.payloadStart + 10
		for i := 0; i < cod.Resolutions; i++ {
			if off+i >= seg.payloadEnd {
				p.out.addIssue("invalid_marker_length", SeverityError,
					ByteRange{off + i, seg.end},
					"COD precinct table is truncated at resolution %d", i)
				cod.Precincts = append(cod.Precincts, PrecinctSize{15, 15})
				continue
			}
			b := p.data[off+i]
			ps := PrecinctSize{WidthExp: int(b & 0x0f), HeightExp: int(b >> 4)}
			if i > 0 && (ps.WidthExp == 0 || ps.HeightExp == 0) {
				p.out.addIssue("invalid_cod", SeverityWarning,
					ByteRange{off + i, off + i + 1},
					"zero precinct exponent at resolution %d", i)
			}
			cod.Precincts = append(cod.Precincts, ps)
		}
	} else {
		for i := 0; i < cod.Resolutions; i++ {
			cod.Precincts = append(cod.Precincts, PrecinctSize{15, 15})
		}
	}
	return cod
}

func (p *parser) parseCOC(seg markerSegment, csiz int) (int, ComponentCoding, bool) {
	payload := p.data[seg.payloadStart:seg.payloadEnd]
	compBytes := 1
	if csiz > 256 {
		compBytes = 2
	}
	if len(payload) < compBytes+6 {
		p.out.addIssue("invalid_marker_length", SeverityError,
			ByteRange{seg.payloadStart, seg.payloadEnd},
			"COC payload is too short")
		return 0, ComponentCoding{}, false
	}
	index := int(payload[0])
	if compBytes == 2 {
		index = int(u16(payload, 0))
	}
	off := compBytes
	coc := ComponentCoding{
		Index:              index,
		Decompositions:     int(payload[off+1]),
		CodeBlockWidthExp:  int(payload[off+2]) + 2,
		CodeBlockHeightExp: int(payload[off+3]) + 2,
		CodeBlockStyle:     payload[off+4],
		Transform:          payload[off+5],
	}
	coc.Resolutions = coc.Decompositions + 1
	if index < 0 || index >= csiz {
		p.out.addIssue("invalid_coc", SeverityError,
			ByteRange{seg.payloadStart, seg.payloadStart + compBytes},
			"COC component index %d is outside 0..%d", index, csiz-1)
		return index, coc, false
	}
	if payload[off]&0x01 != 0 {
		for i := 0; i < coc.Resolutions; i++ {
			pos := off + 6 + i
			if pos >= len(payload) {
				p.out.addIssue("invalid_marker_length", SeverityError,
					ByteRange{seg.payloadStart + pos, seg.payloadEnd},
					"COC precinct table is truncated at resolution %d", i)
				coc.Precincts = append(coc.Precincts, PrecinctSize{15, 15})
				continue
			}
			coc.Precincts = append(coc.Precincts, PrecinctSize{
				WidthExp: int(payload[pos] & 0x0f), HeightExp: int(payload[pos] >> 4),
			})
		}
	} else {
		for i := 0; i < coc.Resolutions; i++ {
			coc.Precincts = append(coc.Precincts, PrecinctSize{15, 15})
		}
	}
	return index, coc, true
}

func codingFromCOD(cod *COD, csiz int) *codingParams {
	cp := &codingParams{
		progression: cod.Progression,
		layers:      cod.Layers,
		mct:         cod.MCT,
		sop:         cod.Scod&0x02 != 0,
		eph:         cod.Scod&0x04 != 0,
		hasCOD:      true,
	}
	for i := 0; i < csiz; i++ {
		cp.components = append(cp.components, ComponentCoding{
			Index:              i,
			Decompositions:     cod.Decompositions,
			Resolutions:        cod.Resolutions,
			CodeBlockWidthExp:  cod.CodeBlockWidthExp,
			CodeBlockHeightExp: cod.CodeBlockHeightExp,
			CodeBlockStyle:     cod.CodeBlockStyle,
			Transform:          cod.Transform,
			Precincts:          append([]PrecinctSize(nil), cod.Precincts...),
		})
	}
	return cp
}

func cloneCoding(cp *codingParams) *codingParams {
	if cp == nil {
		return nil
	}
	out := *cp
	out.components = make([]ComponentCoding, len(cp.components))
	for i := range cp.components {
		out.components[i] = cp.components[i]
		out.components[i].Precincts = append([]PrecinctSize(nil), cp.components[i].Precincts...)
	}
	return &out
}

func ceilDivU64(a, b uint64) uint64 {
	if b == 0 {
		return 0
	}
	return a/b + boolToU64(a%b != 0)
}

func boolToU64(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}
