package main

import (
	"encoding/binary"
)

// decodeSPU parses the SPU packet header, control sequences and RLE data.
func decodeSPU(e Entry, spu []byte, start, end int64) (*Subtitle, error) {
	if len(spu) < 4 {
		return nil, entryErr(e, "SPU packet too short: %d bytes", len(spu))
	}
	pktSize := int(binary.BigEndian.Uint16(spu[0:2]))
	ctrlOff := int(binary.BigEndian.Uint16(spu[2:4]))
	if pktSize > len(spu) {
		return nil, entryErr(e, "SPU declares %d bytes but PES holds %d (truncated .sub)", pktSize, len(spu))
	}
	if pktSize < 4 {
		return nil, entryErr(e, "SPU declares invalid size %d", pktSize)
	}
	if ctrlOff >= pktSize {
		return nil, entryErr(e, "control offset %d beyond packet size %d", ctrlOff, pktSize)
	}
	pkt := spu[:pktSize]

	sub := &Subtitle{Entry: e, ByteStart: start, ByteEnd: end}
	sub.Alpha = [4]uint8{0, 255, 255, 255}
	rleEven, rleOdd := -1, -1

	pos := ctrlOff
	for pos < pktSize {
		if allZero(pkt[pos:pktSize]) {
			break // trailing padding after the last 0xFF
		}
		if pos+3 > pktSize {
			return nil, entryErr(e, "control sequence at +%d truncated (packet size %d)", pos, pktSize)
		}
		pos += 2 // delay, not needed for a static preview
		for {
			if pos >= pktSize {
				return nil, entryErr(e, "control commands run past packet end (offset +%d)", pos)
			}
			cmd := pkt[pos]
			pos++
			switch cmd {
			case 0x00, 0x01, 0x02:
			case 0x03:
				if pos+2 > pktSize {
					return nil, entryErr(e, "palette command truncated at +%d", pos)
				}
				sub.Colors = nibbles4(pkt[pos], pkt[pos+1])
				pos += 2
			case 0x04:
				if pos+2 > pktSize {
					return nil, entryErr(e, "alpha command truncated at +%d", pos)
				}
				a := nibbles4(pkt[pos], pkt[pos+1])
				for i, v := range a {
					sub.Alpha[i] = v * 17
				}
				pos += 2
			case 0x05:
				if pos+6 > pktSize {
					return nil, entryErr(e, "coordinate command truncated at +%d", pos)
				}
				c := pkt[pos : pos+6]
				x1 := int(c[0])<<4 | int(c[1])>>4
				x2 := int(c[1]&0x0F)<<8 | int(c[2])
				y1 := int(c[3])<<4 | int(c[4])>>4
				y2 := int(c[4]&0x0F)<<8 | int(c[5])
				pos += 6
				if x2 < x1 || y2 < y1 {
					return nil, entryErr(e, "invalid coordinates (%d,%d)-(%d,%d)", x1, y1, x2, y2)
				}
				sub.X, sub.Y = x1, y1
				sub.Width, sub.Height = x2-x1+1, y2-y1+1
			case 0x06:
				if pos+4 > pktSize {
					return nil, entryErr(e, "RLE-offset command truncated at +%d", pos)
				}
				rleEven = int(binary.BigEndian.Uint16(pkt[pos : pos+2]))
				rleOdd = int(binary.BigEndian.Uint16(pkt[pos+2 : pos+4]))
				pos += 4
			case 0xFF:
				goto seqDone
			default:
				return nil, entryErr(e, "unknown control command 0x%02X at +%d", cmd, pos-1)
			}
		}
	seqDone:
		if pos%2 == 1 {
			pos++
		}
	}

	if sub.Width == 0 || sub.Height == 0 {
		return nil, entryErr(e, "missing coordinate command in control sequence")
	}
	if rleEven < 0 || rleOdd < 0 {
		return nil, entryErr(e, "missing RLE-offset command in control sequence")
	}
	if rleEven >= pktSize || rleOdd >= pktSize {
		return nil, entryErr(e, "RLE offsets %d/%d beyond packet size %d", rleEven, rleOdd, pktSize)
	}

	sub.Pixels = make([]uint8, sub.Width*sub.Height)
	if err := decodeRLE(e, pkt, rleEven, rleOdd, sub.Pixels, sub.Width, sub.Height); err != nil {
		return nil, err
	}
	return sub, nil
}

func nibbles4(b0, b1 byte) [4]uint8 {
	return [4]uint8{b0 >> 4, b0 & 0x0F, b1 >> 4, b1 & 0x0F}
}

func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
