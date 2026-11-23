package main

import "fmt"

// SMPTE ST 337 / IEC 61937 burst preamble sync words.
const (
	syncPa uint16 = 0xF872
	syncPb uint16 = 0x4E1F
	// Byte-swapped variants indicate a byte-order error in the stream.
	syncPaSwapped uint16 = 0x72F8
	syncPbSwapped uint16 = 0x1F4E
)

// Pc is the decoded burst-preamble "Pc" word (burst info).
type Pc struct {
	DataType      uint8 // bits 0-4
	TypeDependent uint8 // bits 5-6, data-type-dependent info
	ErrorFlag     bool  // bit 7
	TypeNumber    uint8 // bits 8-12, data type number (stream number)
	BurstMode     uint8 // bits 13-15
}

func parsePc(w uint16) Pc {
	return Pc{
		DataType:      uint8(w & 0x1F),
		TypeDependent: uint8((w >> 5) & 0x3),
		ErrorFlag:     w&0x80 != 0,
		TypeNumber:    uint8((w >> 8) & 0x1F),
		BurstMode:     uint8((w >> 13) & 0x7),
	}
}

var dataTypeNames = map[uint8]string{
	0x00: "Null",
	0x01: "AC-3",
	0x02: "Pause",
	0x03: "MPEG-1 Layer 1",
	0x04: "MPEG-1 Layer 2/3",
	0x05: "MPEG-2",
	0x06: "MPEG-2 AAC",
	0x07: "MPEG-2 Layer 1 LSF",
	0x08: "MPEG-2 Layer 2 LSF",
	0x09: "MPEG-2 Layer 3 LSF",
	0x0A: "DTS-I",
	0x0B: "DTS-II",
	0x0C: "DTS-III",
	0x0D: "ATRAC",
	0x0E: "ATRAC-2/3",
	0x0F: "ATRAC-X",
	0x15: "E-AC-3",
	0x16: "DTS-HD",
	0x17: "MLP/TrueHD",
	0x18: "DST",
}

func dataTypeName(t uint8) (string, bool) {
	n, ok := dataTypeNames[t]
	if !ok {
		return fmt.Sprintf("Unknown(0x%02X)", t), false
	}
	return n, true
}

type SegKind int

const (
	SegPCM SegKind = iota
	SegBurst
	SegError
)

func (k SegKind) String() string {
	switch k {
	case SegPCM:
		return "PCM"
	case SegBurst:
		return "BURST"
	default:
		return "ERROR"
	}
}

// Segment is one contiguous region of the AES3 word stream.
// Word indices are 16-bit-word offsets; one word = one subframe sample slot.
type Segment struct {
	Kind         SegKind
	StartWord    int
	EndWord      int // exclusive
	Pc           Pc
	TypeName     string
	KnownType    bool
	LengthBits   uint32   // Pd: burst-payload length in bits
	PayloadWords int      // ceil(LengthBits/16)
	PadBits      int      // zero padding bits in the final payload word
	StuffWords   int      // zero stuffing words after the payload
	Payload      []uint16 // raw burst-payload words (kept for unknown types)
	Errors       []string
}

func (s Segment) frameRange(channels int) (int, int) {
	if channels < 1 {
		channels = 1
	}
	return s.StartWord / channels, (s.EndWord + channels - 1) / channels
}

// layout describes how a burst preamble is embedded in the word stream.
type layout int

const (
	layoutNone layout = iota
	layoutMono          // Pa Pb Pc Pd (single/deinterleaved channel)
	layoutStereo        // Pa Pa Pb Pb Pc Pc Pd Pd (interleaved AES3, both subframes)
)

// detectLayout checks for a valid burst preamble starting at words[i].
func detectLayout(words []uint16, i int) layout {
	n := len(words)
	if i+3 < n && words[i+1] == syncPa && words[i+2] == syncPb && words[i+3] == syncPb {
		return layoutStereo
	}
	if i+1 < n && words[i+1] == syncPb {
		return layoutMono
	}
	return layoutNone
}

// Parse scans an AES3 word stream and splits it into PCM, burst and error
// segments. Normal PCM is never reported as a burst: a burst requires the
// full Pa/Pb sync-word pair (duplicated per channel for interleaved stereo).
func Parse(words []uint16) []Segment {
	var segs []Segment
	n := len(words)
	pcmStart, i := 0, 0

	flushPCM := func(upTo int) {
		if upTo > pcmStart {
			segs = append(segs, Segment{Kind: SegPCM, StartWord: pcmStart, EndWord: upTo})
		}
	}

	for i < n {
		w := words[i]

		// Byte-swapped sync words: byte-order error in the stream.
		if w == syncPaSwapped && i+1 < n && (words[i+1] == syncPbSwapped || words[i+1] == syncPaSwapped) {
			flushPCM(i)
			end := i + 4
			if i+3 < n && words[i+1] == syncPaSwapped && words[i+3] == syncPbSwapped {
				end = i + 8
			}
			if end > n {
				end = n
			}
			segs = append(segs, Segment{
				Kind:      SegError,
				StartWord: i,
				EndWord:   end,
				Errors:    []string{"byte-order error: byte-swapped burst preamble (0x72F8/0x1F4E)"},
			})
			i, pcmStart = end, end
			continue
		}

		if w != syncPa {
			i++
			continue
		}

		lay := detectLayout(words, i)
		if lay == layoutNone {
			// Lone Pa without a following Pb: illegal sync word, not a burst.
			flushPCM(i)
			segs = append(segs, Segment{
				Kind:      SegError,
				StartWord: i,
				EndWord:   i + 1,
				Errors:    []string{"illegal sync word: Pa (0xF872) not followed by Pb (0x4E1F)"},
			})
			i++
			pcmStart = i
			continue
		}

		flushPCM(i)
		seg, next := parseBurst(words, i, lay)
		segs = append(segs, seg)
		i, pcmStart = next, next
	}
	flushPCM(n)
	return segs
}

// parseBurst parses one data burst starting at the preamble at words[i].
// It returns the burst segment and the word index just past it.
func parseBurst(words []uint16, i int, lay layout) (Segment, int) {
	n := len(words)
	seg := Segment{Kind: SegBurst, StartWord: i}

	headerWords := 4
	pcIdx, pdIdx := i+2, i+3
	if lay == layoutStereo {
		headerWords = 8
		pcIdx, pdIdx = i+4, i+6
	}
	if pdIdx >= n {
		seg.EndWord = n
		seg.Errors = append(seg.Errors, "truncated burst: preamble incomplete at end of stream")
		return seg, n
	}

	pc := parsePc(words[pcIdx])
	pd := uint32(words[pdIdx])
	if lay == layoutStereo {
		if words[pcIdx] != words[pcIdx+1] || words[pdIdx] != words[pdIdx+1] {
			seg.Errors = append(seg.Errors, "preamble channel mismatch: Pc/Pd differ between subframes")
		}
	}
	seg.Pc = pc
	seg.TypeName, seg.KnownType = dataTypeName(pc.DataType)
	seg.LengthBits = pd
	seg.PayloadWords = int((pd + 15) / 16)
	seg.PadBits = seg.PayloadWords*16 - int(pd)
	if pc.ErrorFlag {
		seg.Errors = append(seg.Errors, "Pc error flag set: burst-payload data may be invalid")
	}

	payloadStart := i + headerWords
	payloadEnd := payloadStart + seg.PayloadWords
	truncated := false
	availEnd := payloadEnd
	if availEnd > n {
		availEnd = n
	}
	// A new preamble inside the payload means the burst was truncated.
	for j := payloadStart; j+1 < availEnd; j++ {
		if words[j] == syncPa && detectLayout(words, j) != layoutNone {
			payloadEnd = j
			truncated = true
			seg.Errors = append(seg.Errors,
				fmt.Sprintf("burst truncated by new preamble at word %d (expected %d payload words)", j, seg.PayloadWords))
			break
		}
	}
	if !truncated && payloadEnd > n {
		// Burst payload cut off by end of stream (cross-frame truncation).
		payloadEnd = n
		truncated = true
		seg.Errors = append(seg.Errors,
			fmt.Sprintf("truncated burst: payload cut off at end of stream (got %d of %d words)",
				payloadEnd-payloadStart, seg.PayloadWords))
	}

	seg.Payload = append(seg.Payload, words[payloadStart:payloadEnd]...)

	// Zero stuffing fills the gap up to the next burst (or stream end).
	stuffEnd := payloadEnd
	for stuffEnd < n && words[stuffEnd] == 0 {
		stuffEnd++
	}
	seg.StuffWords = stuffEnd - payloadEnd
	seg.EndWord = stuffEnd
	return seg, stuffEnd
}
