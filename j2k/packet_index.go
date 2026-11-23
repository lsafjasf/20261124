package j2k

func (p *parser) indexPackets(tile *Tile, cp *codingParams) {
	sequence := packetSequence(tile, cp)
	if len(sequence) == 0 {
		return
	}
	state := newPacketScanState(tile, cp)
	nextPacket := 0
	for i := range tile.TileParts {
		part := &tile.TileParts[i]
		if part.DuplicateOf != nil || part.SOD == nil {
			continue
		}
		pos := part.Data.Start
		end := part.Data.End
		for pos < end {
			if nextPacket >= len(sequence) {
				p.out.addIssue("packet_count_overflow", SeverityError, ByteRange{pos, end},
					"tile %d has packet data after all %d packets were indexed",
					tile.Index, len(sequence))
				pos = end
				break
			}
			id := sequence[nextPacket]
			packet, next, issue := p.parsePacketAt(pos, end, id, nextPacket, state)
			part.Packets = append(part.Packets, packet)
			tile.Packets = append(tile.Packets, packet)
			if issue != nil {
				part.Complete = false
				p.out.Issues = append(p.out.Issues, Issue{
					Code: issue.code, Severity: SeverityError, Message: issue.message,
					Range: issue.r, Related: issue.related,
				})
				pos = end
				break
			}
			if next <= pos {
				p.out.addIssue("packet_progress_stalled", SeverityError, ByteRange{pos, end},
					"packet %d did not advance the input", nextPacket)
				pos = end
				break
			}
			nextPacket++
			pos = next
		}
		if pos != end && nextPacket < len(sequence) {
			p.out.addIssue("truncated_packet", SeverityError, ByteRange{pos, end},
				"tile-part %d packet data ended early", part.Index)
		}
	}
	if nextPacket < len(sequence) {
		for i := range tile.TileParts {
			if tile.TileParts[i].DuplicateOf == nil {
				tile.TileParts[i].Complete = false
			}
		}
		r := tileRange(tile.TileParts)
		p.out.addIssue("missing_packets", SeverityError, r,
			"tile %d indexed %d of %d packets", tile.Index, nextPacket, len(sequence))
	}
}
