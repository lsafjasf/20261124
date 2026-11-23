package tagfix

import (
	"encoding/binary"
	"fmt"
)

type boxKind uint8

const (
	boxLeaf boxKind = iota
	boxContainer
	boxSTSD
	boxSampleEntry
)

type mp4Box struct {
	typ           string
	kind          boxKind
	start         int
	payload       int
	end           int
	childrenStart int
	children      []*mp4Box
	large         bool
	sizeZero      bool
}

func parseTopLevel(data []byte) ([]*mp4Box, error) {
	return parseBoxRange(data, 0, len(data))
}

func parseBoxRange(data []byte, start, end int) ([]*mp4Box, error) {
	var boxes []*mp4Box
	for pos := start; pos < end; {
		if end-pos < 8 {
			return nil, fmt.Errorf("%w: trailing %d bytes at offset %d",
				ErrUnsupportedMedia, end-pos, pos)
		}
		size32 := binary.BigEndian.Uint32(data[pos:])
		typ := string(data[pos+4 : pos+8])
		header := 8
		large := false
		sizeZero := false
		var size uint64 = uint64(size32)
		switch size32 {
		case 1:
			if end-pos < 16 {
				return nil, fmt.Errorf("%w: truncated 64-bit box at offset %d",
					ErrUnsupportedMedia, pos)
			}
			size = binary.BigEndian.Uint64(data[pos+8:])
			header = 16
			large = true
		case 0:
			size = uint64(end - pos)
			sizeZero = true
		}
		if size < uint64(header) || size > uint64(end-pos) {
			return nil, fmt.Errorf("%w: box %q at offset %d has invalid size %d",
				ErrUnsupportedMedia, typ, pos, size)
		}
		box := &mp4Box{
			typ:           typ,
			start:         pos,
			payload:       pos + header,
			end:           pos + int(size),
			childrenStart: pos + header,
			large:         large,
			sizeZero:      sizeZero,
		}
		if err := populateBox(data, box); err != nil {
			return nil, err
		}
		boxes = append(boxes, box)
		pos += int(size)
	}
	return boxes, nil
}

func populateBox(data []byte, box *mp4Box) error {
	switch box.typ {
	case "moov", "trak", "mdia", "minf", "stbl", "edts", "dinf",
		"mvex", "moof", "traf", "mfra":
		box.kind = boxContainer
	case "stsd":
		box.kind = boxSTSD
		if box.payload+8 > box.end {
			return fmt.Errorf("%w: truncated stsd at offset %d", ErrUnsupportedMedia, box.start)
		}
		box.childrenStart = box.payload + 8
	case "avc1", "avc2", "avc3", "avc4":
		box.kind = boxSampleEntry
		if box.payload+78 > box.end {
			return fmt.Errorf("%w: truncated %s sample entry at offset %d",
				ErrUnsupportedMedia, box.typ, box.start)
		}
		box.childrenStart = box.payload + 78
	default:
		return nil
	}
	children, err := parseBoxRange(data, box.childrenStart, box.end)
	if err != nil {
		return err
	}
	box.children = children
	return nil
}

func findChild(box *mp4Box, typ string) *mp4Box {
	for _, child := range box.children {
		if child.typ == typ {
			return child
		}
	}
	return nil
}

type trackInfo struct {
	index      int
	sampleType string
	sample     *mp4Box
	colr       *mp4Box
	colrs      []*mp4Box
	avcc       *mp4Box
}

// findVideoTracks returns only AVC visual tracks. Ambiguous or unsupported
// tracks are left out rather than guessed.
func findVideoTracks(data []byte, top []*mp4Box) ([]trackInfo, error) {
	var tracks []trackInfo
	for _, topBox := range top {
		if topBox.typ != "moov" {
			continue
		}
		for _, trak := range topBox.children {
			if trak.typ != "trak" {
				continue
			}
			mdia := findChild(trak, "mdia")
			if mdia == nil {
				continue
			}
			hdlr := findChild(mdia, "hdlr")
			if hdlr == nil || hdlr.payload+12 > hdlr.end ||
				string(data[hdlr.payload+8:hdlr.payload+12]) != "vide" {
				continue
			}
			minf := findChild(mdia, "minf")
			if minf == nil {
				continue
			}
			stbl := findChild(minf, "stbl")
			if stbl == nil {
				continue
			}
			stsd := findChild(stbl, "stsd")
			if stsd == nil {
				continue
			}
			for _, entry := range stsd.children {
				switch entry.typ {
				case "avc1", "avc2", "avc3", "avc4":
					track := trackInfo{
						index:      len(tracks),
						sampleType: entry.typ,
						sample:     entry,
						avcc:       findChild(entry, "avcC"),
					}
					for _, child := range entry.children {
						if child.typ == "colr" {
							track.colrs = append(track.colrs, child)
						}
					}
					var nclx []*mp4Box
					for _, box := range track.colrs {
						if box.payload+4 <= box.end &&
							string(data[box.payload:box.payload+4]) == "nclx" {
							nclx = append(nclx, box)
						}
					}
					switch {
					case len(nclx) == 1:
						track.colr = nclx[0]
					case len(nclx) > 1:
						return nil, fmt.Errorf("%w: track %d has %d nclx colr boxes",
							ErrCannotAutoFix, track.index, len(nclx))
					}
					tracks = append(tracks, track)
				}
			}
		}
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("%w: no AVC video sample entry", ErrUnsupportedMedia)
	}
	return tracks, nil
}

type colrInfo struct {
	typ             string
	primariesOffset int
	transferOffset  int
	matrixOffset    int
	rangeOffset     int
	tags            ColorTags
	valid           bool
}

func parseColr(data []byte, box *mp4Box) (*colrInfo, error) {
	if box.payload+11 > box.end {
		return nil, fmt.Errorf("%w: truncated colr at offset %d", ErrUnsupportedMedia, box.start)
	}
	info := &colrInfo{
		typ:             string(data[box.payload : box.payload+4]),
		primariesOffset: box.payload + 4,
		transferOffset:  box.payload + 6,
		matrixOffset:    box.payload + 8,
		rangeOffset:     box.payload + 10,
	}
	if info.typ != "nclx" {
		return info, nil
	}
	info.tags = ColorTags{
		Primaries: data[info.primariesOffset+1],
		Transfer:  data[info.transferOffset+1],
		Matrix:    data[info.matrixOffset+1],
		FullRange: data[info.rangeOffset]&0x80 != 0,
	}
	info.valid = true
	return info, nil
}
