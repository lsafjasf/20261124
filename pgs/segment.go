// Package pgs decodes and renders Presentation Graphic Stream (PGS)
// subtitles without any cgo or third-party dependency.
package pgs

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// Segment types used by PGS streams.
const (
	SegmentPalette      byte = 0x14
	SegmentObject       byte = 0x15
	SegmentPresentation byte = 0x16
	SegmentWindow       byte = 0x17
	SegmentEnd          byte = 0x80
)

// Clock is the PGS timestamp frequency: 90 kHz.
const Clock = 90000

// Segment is one length-prefixed PGS segment. PTS and DTS are 90 kHz ticks.
type Segment struct {
	Offset  int64
	PTS     uint32
	DTS     uint32
	Type    byte
	Payload []byte
}

// Stream is an immutable parsed SUP stream.
type Stream struct {
	Segments []Segment
	sets     []displaySet
}

type displaySet struct {
	pts      uint32
	segments []Segment
	offset   int64
}

// ReadSUP parses a PGS .sup stream from r.
func ReadSUP(r io.Reader) (*Stream, error) {
	br := bufio.NewReader(r)
	var segments []Segment
	var offset int64

	for {
		var header [13]byte
		n, err := io.ReadFull(br, header[:])
		offset += int64(n)
		if errors.Is(err, io.EOF) && n == 0 {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("pgs: truncated 13-byte segment header at offset %d: %w", offset-int64(n), err)
		}
		if header[0] != 'P' || header[1] != 'G' {
			return nil, fmt.Errorf("pgs: bad segment magic %q at offset %d", header[:2], offset-13)
		}

		size := int(header[11])<<8 | int(header[12])
		payload := make([]byte, size)
		n, err = io.ReadFull(br, payload)
		offset += int64(n)
		if err != nil {
			return nil, fmt.Errorf("pgs: truncated payload for segment type 0x%02x at offset %d: %w",
				header[10], offset-int64(n), err)
		}

		segments = append(segments, Segment{
			Offset:  offset - int64(size) - 13,
			PTS:     binary.BigEndian.Uint32(header[2:6]),
			DTS:     binary.BigEndian.Uint32(header[6:10]),
			Type:    header[10],
			Payload: payload,
		})
	}

	stream := &Stream{Segments: segments}
	stream.sets = groupDisplaySets(segments)
	return stream, nil
}

// OpenSUP opens and parses a PGS .sup file.
func OpenSUP(path string) (*Stream, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ReadSUP(f)
}

func groupDisplaySets(segments []Segment) []displaySet {
	var sets []displaySet
	var current []Segment
	var offset int64

	flush := func() {
		if len(current) == 0 {
			return
		}
		pts := uint32(0)
		foundPCS := false
		for _, seg := range current {
			if seg.Type == SegmentPresentation {
				pts = seg.PTS
				foundPCS = true
				break
			}
		}
		if !foundPCS {
			pts = current[0].PTS
		}
		set := displaySet{pts: pts, segments: append([]Segment(nil), current...), offset: offset}
		sets = append(sets, set)
		current = current[:0]
	}

	for _, seg := range segments {
		if len(current) == 0 {
			offset = seg.Offset
		}
		current = append(current, seg)
		if seg.Type == SegmentEnd {
			flush()
		}
	}
	flush()
	return sets
}
