package vobsub

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// DecodeSPU parses the display-control sequences in one SPU. The returned
// value is usable even when err != nil, which lets the caller preserve the raw
// packet and its original bitmap for review.
func DecodeSPU(buf []byte) (*SPU, error) {
	if len(buf) < 4 {
		return nil, errors.New("SPU shorter than 4-byte header")
	}
	size := int(binary.BigEndian.Uint16(buf[:2]))
	if size < 4 {
		return nil, fmt.Errorf("invalid SPU size %d", size)
	}
	if size > len(buf) {
		return &SPU{Size: size, Raw: append([]byte(nil), buf...)}, fmt.Errorf("SPU declares %d bytes, have %d", size, len(buf))
	}
	buf = buf[:size]
	controlOffset := int(binary.BigEndian.Uint16(buf[2:4]))
	if controlOffset < 4 || controlOffset >= size {
		return &SPU{Size: size, Raw: append([]byte(nil), buf...)}, fmt.Errorf("invalid control offset %d", controlOffset)
	}

	out := &SPU{
		Size:          size,
		ControlOffset: controlOffset,
		Raw:           append([]byte(nil), buf...),
	}
	var state State
	active := false
	var startState State
	visibleStates := 0
	firstState := true

	offset := controlOffset
	lastDate := int64(-1)
	seen := make(map[int]bool)
	var parseErr error
	for seqNo := 0; ; seqNo++ {
		if seqNo > 4096 {
			parseErr = errors.New("too many control sequences")
			break
		}
		if offset < controlOffset || offset+4 > size {
			parseErr = fmt.Errorf("control sequence offset %d out of range", offset)
			break
		}
		if seen[offset] {
			parseErr = fmt.Errorf("control sequence loop at %d", offset)
			break
		}
		seen[offset] = true

		date := int64(binary.BigEndian.Uint16(buf[offset : offset+2]))
		next := int(binary.BigEndian.Uint16(buf[offset+2 : offset+4]))
		if date < lastDate {
			parseErr = fmt.Errorf("control sequence time goes backwards: %d then %d", lastDate, date)
			break
		}
		lastDate = date
		pos := offset + 4
		seqChangedWhileVisible := false

		for {
			if pos >= size {
				parseErr = errors.New("control sequence missing 0xff terminator")
				break
			}
			cmd := buf[pos]
			switch cmd {
			case 0x00: // FSTA_DSP
				out.Forced = true
				pos++
			case 0x01: // STA_DSP; start date comes from the sequence header
				if active {
					out.Animation = true
					if len(out.Snapshots) > 0 {
						out.Snapshots[len(out.Snapshots)-1].Active = true
					}
				}
				active = true
				out.HasStart = true
				out.StartTick = date
				if firstState {
					startState = state
					firstState = false
				}
				visibleStates++
				pos++
			case 0x02: // STP_DSP; end date comes from the sequence header
				active = false
				out.HasStop = true
				out.EndTick = date
				pos++
			case 0x03:
				if pos+3 > size {
					parseErr = errors.New("truncated palette command")
					break
				}
				before := state
				state.PaletteMap[3] = buf[pos+1] >> 4
				state.PaletteMap[2] = buf[pos+1] & 0x0f
				state.PaletteMap[1] = buf[pos+2] >> 4
				state.PaletteMap[0] = buf[pos+2] & 0x0f
				pos += 3
				seqChangedWhileVisible = seqChangedWhileVisible || (active && state != before)
			case 0x04:
				if pos+3 > size {
					parseErr = errors.New("truncated alpha command")
					break
				}
				before := state
				state.Alpha[3] = buf[pos+1] >> 4
				state.Alpha[2] = buf[pos+1] & 0x0f
				state.Alpha[1] = buf[pos+2] >> 4
				state.Alpha[0] = buf[pos+2] & 0x0f
				pos += 3
				seqChangedWhileVisible = seqChangedWhileVisible || (active && state != before)
			case 0x05, 0x85:
				if pos+7 > size {
					parseErr = errors.New("truncated display-area command")
					break
				}
				before := state
				x1 := int(buf[pos+1])<<4 | int(buf[pos+2])>>4
				x2 := int(buf[pos+2]&0x0f)<<8 | int(buf[pos+3])
				y1 := int(buf[pos+4])<<4 | int(buf[pos+5])>>4
				y2 := int(buf[pos+5]&0x0f)<<8 | int(buf[pos+6])
				if x2 < x1 || y2 < y1 {
					parseErr = fmt.Errorf("invalid display area (%d,%d)-(%d,%d)", x1, y1, x2, y2)
					break
				}
				state.Rect = Rect{X: x1, Y: y1, Width: x2 - x1 + 1, Height: y2 - y1 + 1}
				pos += 7
				seqChangedWhileVisible = seqChangedWhileVisible || (active && state != before)
			case 0x06:
				if pos+5 > size {
					parseErr = errors.New("truncated pixel-data command")
					break
				}
				before := state
				state.TopOffset = int(binary.BigEndian.Uint16(buf[pos+1 : pos+3]))
				state.BotOffset = int(binary.BigEndian.Uint16(buf[pos+3 : pos+5]))
				pos += 5
				seqChangedWhileVisible = seqChangedWhileVisible || (active && state != before)
			case 0x86:
				if pos+9 > size {
					parseErr = errors.New("truncated extended pixel-data command")
					break
				}
				before := state
				state.TopOffset = int(binary.BigEndian.Uint16(buf[pos+1 : pos+3]))
				state.BotOffset = int(binary.BigEndian.Uint16(buf[pos+5 : pos+7]))
				pos += 9
				seqChangedWhileVisible = seqChangedWhileVisible || (active && state != before)
			case 0x07, 0x87:
				if pos+3 > size {
					parseErr = errors.New("truncated colour-change command")
					break
				}
				n := int(binary.BigEndian.Uint16(buf[pos+1 : pos+3]))
				if n < 2 || pos+1+n > size {
					parseErr = fmt.Errorf("invalid colour-change size %d", n)
					break
				}
				pos += 1 + n
				seqChangedWhileVisible = seqChangedWhileVisible || active
				parseErr = errors.New("CHG_COLCON is not migrated")
			case 0xff:
				pos++
				goto sequenceDone
			default:
				parseErr = fmt.Errorf("unsupported control command 0x%02x", cmd)
			}
			if parseErr != nil {
				break
			}
		}

	sequenceDone:
		if seqChangedWhileVisible || (active && visibleStates > 1) {
			out.Animation = true
		}
		out.Snapshots = append(out.Snapshots, Snapshot{TickMS: ticksToMS(date), State: state, Active: active})
		if parseErr != nil {
			break
		}
		if next == offset || next == 0 {
			break
		}
		if next < controlOffset || next >= size {
			parseErr = fmt.Errorf("next control sequence offset %d out of range", next)
			break
		}
		offset = next
	}
	out.MainState = startState
	if !out.HasStart {
		out.MainState = state
	}
	if out.HasStart && out.HasStop && out.EndTick == out.StartTick {
		parseErr = errors.New("zero-length display interval")
	}
	return out, parseErr
}

func ticksToMS(ticks int64) int64 {
	return (ticks*1024 + 45) / 90
}
