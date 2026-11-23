package ac3

import (
	"fmt"
	"io"
)

// EventKind classifies stream-level findings, all located by frame number.
type EventKind int

const (
	EventResync      EventKind = iota // garbage skipped between frames
	EventCRCError                     // crc1 and/or crc2 mismatch
	EventParamChange                  // encoding parameter discontinuity
	EventTruncated                    // stream ends mid-frame
)

type Event struct {
	Kind    EventKind
	Frame   int   // frame number the event belongs to
	Offset  int64 // byte offset in the stream
	Message string
}

func (e Event) String() string {
	return fmt.Sprintf("frame %d (offset %d): %s", e.Frame, e.Offset, e.Message)
}

// Result is the outcome of scanning a (possibly concatenated) AC-3 stream.
type Result struct {
	Frames []*Frame
	Events []Event
}

// Duration returns the total decoded duration of all parsed frames.
func (r *Result) Duration() float64 {
	var sec float64
	for _, f := range r.Frames {
		sec += 1536.0 / float64(f.SampleRate)
	}
	return sec
}

// keyParams are the encoding parameters whose change mid-stream is
// reported as a discontinuity (typical for concatenated programs).
func keyParams(f *Frame) string {
	return fmt.Sprintf("fscod=%d frmsizecod=%d bsid=%d bsmod=%d acmod=%d lfeon=%v dialnorm=%d",
		f.FSCod, f.FrmSizeCod, f.BSID, f.BSMod, f.ACMod, f.LFEOn, f.Dialnorm)
}

// Scan reads AC-3 syncframes from r until EOF, tolerating garbage between
// frames (resync) and reporting CRC failures and parameter changes with
// frame numbers.
func Scan(r io.Reader) (*Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	var prev *Frame
	pos := 0
	for pos < len(data) {
		if pos+2 > len(data) || data[pos] != 0x0B || data[pos+1] != 0x77 {
			next := nextSync(data, pos)
			skipped := next - pos
			if next >= len(data) {
				res.Events = append(res.Events, Event{EventResync, len(res.Frames), int64(pos),
					fmt.Sprintf("trailing %d non-AC-3 bytes at end of stream", skipped)})
				break
			}
			res.Events = append(res.Events, Event{EventResync, len(res.Frames), int64(pos),
				fmt.Sprintf("skipped %d non-AC-3 byte(s), resynced", skipped)})
			pos = next
		}
		size, err := FrameSize(data[pos:])
		if err != nil {
			res.Events = append(res.Events, Event{EventResync, len(res.Frames), int64(pos),
				fmt.Sprintf("false sync (%v), advancing", err)})
			pos += 2
			continue
		}
		if pos+size > len(data) {
			res.Events = append(res.Events, Event{EventTruncated, len(res.Frames), int64(pos),
				fmt.Sprintf("truncated frame: %d of %d bytes present", len(data)-pos, size)})
			break
		}
		f, err := ParseFrame(data[pos:pos+size], len(res.Frames), int64(pos))
		if err != nil {
			res.Events = append(res.Events, Event{EventResync, len(res.Frames), int64(pos),
				fmt.Sprintf("unparseable frame (%v), advancing", err)})
			pos += 2
			continue
		}
		if !f.CRC1OK || !f.CRC2OK {
			res.Events = append(res.Events, Event{EventCRCError, f.Index, f.Offset,
				fmt.Sprintf("CRC error (crc1_ok=%v crc2_ok=%v)", f.CRC1OK, f.CRC2OK)})
		}
		if prev != nil && keyParams(prev) != keyParams(f) {
			res.Events = append(res.Events, Event{EventParamChange, f.Index, f.Offset,
				fmt.Sprintf("parameter change: [%s] -> [%s]", keyParams(prev), keyParams(f))})
		}
		res.Frames = append(res.Frames, f)
		prev = f
		pos += size
	}
	return res, nil
}

func nextSync(data []byte, from int) int {
	for i := from + 1; i+1 < len(data); i++ {
		if data[i] == 0x0B && data[i+1] == 0x77 {
			return i
		}
	}
	return len(data)
}
