package j2k

import "fmt"

type packetIssue struct {
	code    string
	message string
	r       ByteRange
	related []ByteRange
}

func packetProblem(code string, r ByteRange, format string, args ...any) *packetIssue {
	return &packetIssue{code: code, message: fmt.Sprintf(format, args...), r: r}
}

func (p *parser) parsePacketAt(start, end int, id packetID, packetIndex int,
	state *packetScanState) (Packet, int, *packetIssue) {
	pkt := Packet{
		Index: packetIndex, Layer: id.Layer, Resolution: id.Resolution,
		Component: id.Component, Precinct: id.Precinct,
	}
	pos := start
	if state.coding.sop && pos+2 <= end && p.data[pos] == 0xff && p.data[pos+1] == 0x91 {
		if pos+6 > end {
			return pkt, end, packetProblem("truncated_marker", ByteRange{pos, end},
				"SOP at offset %d is truncated", pos)
		}
		sop := ByteRange{pos, pos + 6}
		pkt.SOP = &sop
		if u16(p.data, pos+2) != 4 {
			p.out.addIssue("invalid_sop", SeverityWarning, ByteRange{pos + 2, pos + 4},
				"SOP at offset %d should have Lsop=4", pos)
		}
		pos += 6
	}
	headerStart := pos
	reader := newBitReader(p.data, pos, end)
	present, err := reader.readBit()
	if err != nil {
		pkt.Header = ByteRange{headerStart, end}
		pkt.Data = ByteRange{end, end}
		return pkt, end, packetProblem("truncated_packet_header", ByteRange{headerStart, end},
			"packet %d header is truncated", packetIndex)
	}
	if present == 0 {
		if err := reader.align(); err != nil {
			pkt.Header = ByteRange{headerStart, end}
			pkt.Data = ByteRange{end, end}
			return pkt, end, packetProblem("truncated_packet_header", ByteRange{headerStart, end},
				"empty packet %d header is truncated", packetIndex)
		}
		headerEnd := headerStart + reader.consumed()
		if eph, issue := p.finishEPH(headerEnd, end, state.coding.eph); issue != nil {
			pkt.Header = ByteRange{headerStart, headerEnd}
			pkt.Data = ByteRange{headerEnd, end}
			return pkt, end, issue
		} else if eph != nil {
			pkt.EPH = eph
			headerEnd = eph.End
		}
		pkt.Empty = true
		pkt.Header = ByteRange{headerStart, headerEnd}
		pkt.Data = ByteRange{headerEnd, headerEnd}
		return pkt, headerEnd, nil
	}

	var bodyBytes uint64
	bands := numBands(id.Resolution)
	for band := 0; band < bands; band++ {
		bs, ok := p.getBandPrecinctState(state, id, band)
		if !ok {
			pkt.Header = ByteRange{headerStart, end}
			pkt.Data = ByteRange{end, end}
			return pkt, end, packetProblem("resource_limit", pkt.Header,
				"packet %d has an oversized precinct", packetIndex)
		}
		if bs == nil {
			continue
		}
		style := uint8(0)
		if id.Component < len(state.coding.components) {
			style = state.coding.components[id.Component].CodeBlockStyle
		}
		if style&0x40 != 0 {
			pkt.Header = ByteRange{headerStart, end}
			pkt.Data = ByteRange{end, end}
			return pkt, end, packetProblem("unsupported_coding_style", ByteRange{headerStart, end},
				"HT code-block style is not supported by this structural scanner")
		}
		for leaf := 0; leaf < len(bs.blocks); leaf++ {
			block := &bs.blocks[leaf]
			var included bool
			if len(block.segments) == 0 {
				included, err = bs.incl.decode(reader, leaf, id.Layer+1)
			} else {
				var bit int
				bit, err = reader.readBit()
				included = bit != 0
			}
			if err != nil {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
					"packet %d inclusion bits are truncated", packetIndex)
			}
			if !included {
				continue
			}
			if len(block.segments) == 0 {
				found := false
				for threshold := 0; threshold <= 256; threshold++ {
					found, err = bs.imsb.decode(reader, leaf, threshold)
					if err != nil || found {
						break
					}
				}
				if err != nil || !found {
					pkt.Header = ByteRange{headerStart, end}
					pkt.Data = ByteRange{end, end}
					return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
						"packet %d zero-bitplane tag-tree is truncated", packetIndex)
				}
				block.numLenBits = 3
			}
			passes, err := readNumPasses(reader)
			if err != nil {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
					"packet %d coding-pass count is truncated", packetIndex)
			}
			increment, err := readCommaCode(reader)
			if err != nil {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
					"packet %d length increment is truncated", packetIndex)
			}
			block.numLenBits += increment
			if block.numLenBits > 32 {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				return pkt, end, packetProblem("packet_header_overflow", ByteRange{headerStart, end},
					"packet %d code-block length width exceeds 32 bits", packetIndex)
			}
			added, err := readBlockLengths(reader, block, style, passes)
			if err != nil {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
					"packet %d code-block lengths are truncated", packetIndex)
			}
			bodyBytes += added
			if bodyBytes > uint64(end-start) {
				pkt.Header = ByteRange{headerStart, end}
				pkt.Data = ByteRange{end, end}
				pkt.CodeBlockBytes = saturatingInt64(bodyBytes)
				issue := packetProblem("length_overflow", ByteRange{headerStart, end},
					"packet %d declares more code-block bytes than remain", packetIndex)
				issue.related = []ByteRange{{headerStart, end}}
				return pkt, end, issue
			}
		}
	}
	if err := reader.align(); err != nil {
		pkt.Header = ByteRange{headerStart, end}
		pkt.Data = ByteRange{end, end}
		return pkt, end, packetProblem("truncated_packet_header", pkt.Header,
			"packet %d header alignment is truncated", packetIndex)
	}
	headerEnd := headerStart + reader.consumed()
	if eph, issue := p.finishEPH(headerEnd, end, state.coding.eph); issue != nil {
		pkt.Header = ByteRange{headerStart, headerEnd}
		pkt.Data = ByteRange{headerEnd, end}
		return pkt, end, issue
	} else if eph != nil {
		pkt.EPH = eph
		headerEnd = eph.End
	}
	pkt.Header = ByteRange{headerStart, headerEnd}
	pkt.CodeBlockBytes = saturatingInt64(bodyBytes)
	next64 := uint64(headerEnd) + bodyBytes
	if next64 > uint64(end) {
		pkt.Data = ByteRange{headerEnd, end}
		issue := packetProblem("length_overflow", ByteRange{headerStart, headerEnd},
			"packet %d body ends beyond tile-part data", packetIndex)
		issue.related = []ByteRange{{headerEnd, end}}
		return pkt, end, issue
	}
	pkt.Data = ByteRange{headerEnd, int(next64)}
	return pkt, int(next64), nil
}

func (p *parser) finishEPH(pos, end int, required bool) (*ByteRange, *packetIssue) {
	if !required {
		return nil, nil
	}
	if pos+2 > end {
		return nil, packetProblem("truncated_marker", ByteRange{pos, end},
			"required EPH marker at offset %d is truncated", pos)
	}
	if p.data[pos] != 0xff || p.data[pos+1] != 0x92 {
		return nil, packetProblem("missing_eph", ByteRange{pos, pos + 2},
			"required EPH marker is missing at offset %d", pos)
	}
	r := ByteRange{pos, pos + 2}
	return &r, nil
}

func readNumPasses(r *bitReader) (int, error) {
	bit, err := r.readBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return 1, nil
	}
	bit, err = r.readBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return 2, nil
	}
	v, err := r.readBits(2)
	if err != nil {
		return 0, err
	}
	if v != 3 {
		return 3 + int(v), nil
	}
	v, err = r.readBits(5)
	if err != nil {
		return 0, err
	}
	if v != 31 {
		return 6 + int(v), nil
	}
	v, err = r.readBits(7)
	if err != nil {
		return 0, err
	}
	return 37 + int(v), nil
}

func readCommaCode(r *bitReader) (int, error) {
	n := 0
	for n <= 32 {
		bit, err := r.readBit()
		if err != nil {
			return 0, err
		}
		if bit == 0 {
			return n, nil
		}
		n++
	}
	return 0, fmt.Errorf("comma code exceeds 32 ones")
}

func readBlockLengths(r *bitReader, block *codeBlockState, style uint8, passes int) (uint64, error) {
	if len(block.segments) == 0 {
		appendSegment(block, style)
	} else if last := &block.segments[len(block.segments)-1]; last.passes == last.maxPasses {
		appendSegment(block, style)
	}
	var total uint64
	remaining := passes
	for remaining > 0 {
		segment := &block.segments[len(block.segments)-1]
		take := segment.maxPasses - segment.passes
		if take > remaining {
			take = remaining
		}
		bits := block.numLenBits + floorLog2Int(take)
		if bits > 32 {
			return total, fmt.Errorf("code-block length field exceeds 32 bits")
		}
		length, err := r.readBits(bits)
		if err != nil {
			return total, err
		}
		total += length
		segment.passes += take
		remaining -= take
		if remaining > 0 {
			appendSegment(block, style)
		}
	}
	return total, nil
}

func floorLog2Int(v int) int {
	n := 0
	for v > 1 {
		v >>= 1
		n++
	}
	return n
}

func saturatingInt64(v uint64) int64 {
	const maxInt64 = uint64(1<<63 - 1)
	if v > maxInt64 {
		return int64(maxInt64)
	}
	return int64(v)
}
