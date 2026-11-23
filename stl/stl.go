// Package stl reads and validates EBU Tech 3264 (STL) subtitle files.
//
// The package is deliberately read-only. Validation returns findings and repair
// suggestions; it never rewrites or truncates the input data.
package stl

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	// GSIBlockSize is the fixed size of the General Subtitle Information block.
	GSIBlockSize = 1024
	// TTIBlockSize is the fixed size of a Text and Timing Information block.
	TTIBlockSize = 128
	// TextFieldSize is the number of text bytes in a TTI block.
	TextFieldSize = 112

	ebnUserData      = 0xFE
	ebnLastBlock     = 0xFF
	ebnLastExtension = 0xEF
	unusedSpace      = 0x8F
	lineBreak        = 0x8A
)

var (
	// ErrEmptyFile reports that no bytes were available for a GSI block.
	ErrEmptyFile = errors.New("stl: empty file")
	// ErrTruncatedGSI reports that fewer than 1024 bytes were available.
	ErrTruncatedGSI = errors.New("stl: truncated GSI block")
)

// Timecode is a binary STL time code (hours, minutes, seconds, frames).
type Timecode struct {
	Hours   uint8 `json:"hours"`
	Minutes uint8 `json:"minutes"`
	Seconds uint8 `json:"seconds"`
	Frames  uint8 `json:"frames"`
}

// String returns the time code in HH:MM:SS:FF form.
func (t Timecode) String() string {
	return fmt.Sprintf("%02d:%02d:%02d:%02d", t.Hours, t.Minutes, t.Seconds, t.Frames)
}

// HHMMSSFF returns the eight-character GSI representation of the time code.
func (t Timecode) HHMMSSFF() string {
	return fmt.Sprintf("%02d%02d%02d%02d", t.Hours, t.Minutes, t.Seconds, t.Frames)
}

// Before reports whether t occurs before other in lexicographic time-code order.
func (t Timecode) Before(other Timecode) bool {
	if t.Hours != other.Hours {
		return t.Hours < other.Hours
	}
	if t.Minutes != other.Minutes {
		return t.Minutes < other.Minutes
	}
	if t.Seconds != other.Seconds {
		return t.Seconds < other.Seconds
	}
	return t.Frames < other.Frames
}

// GSI holds the declared fields of the General Subtitle Information block.
// String fields preserve the bytes from the file, except for trailing spaces.
type GSI struct {
	CPN          string
	DFC          string
	DSC          byte
	CCT          string
	LanguageCode string
	Programme    string
	Episode      string
	TNB          string
	TNS          string
	TNG          string
	MNC          string
	MNR          string
	TCS          byte
	TCP          string
	TCF          string
	TND          string
	DSN          string
	Country      string
}

// TTI is one 128-byte Text and Timing Information block.
type TTI struct {
	Index  int
	Offset int64
	SGN    uint8
	SN     uint16
	EBN    uint8
	CS     uint8
	TCI    Timecode
	TCO    Timecode
	VP     uint8
	JC     uint8
	CF     uint8
	Text   [TextFieldSize]byte
	raw    [TTIBlockSize]byte
}

// Document is a lenient in-memory view of an STL file.
type Document struct {
	Size        int
	GSIComplete bool
	GSI         GSI
	Blocks      []TTI
	Trailing    int
}

// Parse reads an STL file from disk. Structural problems are represented in the
// returned document and report; only I/O failures are returned as errors.
func ParseFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data), nil
}

// Parse interprets data leniently. A short GSI and a partial final TTI are
// retained so that Validate can describe every problem in the input.
func Parse(data []byte) *Document {
	d := &Document{Size: len(data)}
	if len(data) >= GSIBlockSize {
		d.GSIComplete = true
	}
	d.GSI = parseGSI(data)

	payload := data
	if len(payload) > GSIBlockSize {
		payload = payload[GSIBlockSize:]
	} else {
		payload = nil
	}
	d.Trailing = len(payload) % TTIBlockSize
	n := len(payload) / TTIBlockSize
	d.Blocks = make([]TTI, 0, n)
	for i := 0; i < n; i++ {
		start := i * TTIBlockSize
		d.Blocks = append(d.Blocks, parseTTI(payload[start:start+TTIBlockSize], i))
	}
	return d
}

// StructuralError reports whether the file is too short to contain a complete
// GSI block. Callers that only want to report can still use Parse and Validate.
func StructuralError(d *Document) error {
	if d == nil || d.Size == 0 {
		return ErrEmptyFile
	}
	if !d.GSIComplete {
		return ErrTruncatedGSI
	}
	return nil
}

func parseGSI(data []byte) GSI {
	field := func(start, end int) string {
		if start >= len(data) {
			return ""
		}
		if end > len(data) {
			end = len(data)
		}
		return trimField(data[start:end])
	}
	var dsc byte
	if len(data) > 11 {
		dsc = data[11]
	}
	var tcs byte
	if len(data) > 255 {
		tcs = data[255]
	}
	return GSI{
		CPN:          field(0, 3),
		DFC:          field(3, 11),
		DSC:          dsc,
		CCT:          field(12, 14),
		LanguageCode: field(14, 16),
		Programme:    field(16, 48),
		Episode:      field(48, 80),
		TNB:          field(238, 243),
		TNS:          field(243, 248),
		TNG:          field(248, 251),
		MNC:          field(251, 253),
		MNR:          field(253, 255),
		TCS:          tcs,
		TCP:          field(256, 264),
		TCF:          field(264, 272),
		TND:          field(272, 273),
		DSN:          field(273, 274),
		Country:      field(274, 277),
	}
}

func parseTTI(b []byte, index int) TTI {
	t := TTI{
		Index:  index,
		Offset: int64(GSIBlockSize + index*TTIBlockSize),
		SGN:    b[0],
		SN:     binary.LittleEndian.Uint16(b[1:3]),
		EBN:    b[3],
		CS:     b[4],
		TCI: Timecode{
			Hours: b[5], Minutes: b[6], Seconds: b[7], Frames: b[8],
		},
		TCO: Timecode{
			Hours: b[9], Minutes: b[10], Seconds: b[11], Frames: b[12],
		},
		VP: b[13],
		JC: b[14],
		CF: b[15],
	}
	copy(t.Text[:], b[16:128])
	copy(t.raw[:], b)
	return t
}

func trimField(b []byte) string {
	return strings.TrimRight(string(b), " \x00")
}

func (t TTI) isTextBlock() bool {
	return t.EBN <= ebnLastExtension || t.EBN == ebnLastBlock
}

func (t TTI) isUserData() bool {
	return t.EBN == ebnUserData
}

func (t TTI) isLastBlock() bool {
	return t.EBN == ebnLastBlock
}

func (t TTI) sameRaw(other TTI) bool {
	return t.raw == other.raw
}
