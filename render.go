package tagfix

import (
	"encoding/binary"
	"fmt"
	"math"
)

type boxEdits struct {
	replace map[*mp4Box][]byte
	insert  map[*mp4Box][][]byte
}

func newBoxEdits() *boxEdits {
	return &boxEdits{
		replace: make(map[*mp4Box][]byte),
		insert:  make(map[*mp4Box][][]byte),
	}
}

func renderFile(data []byte, top []*mp4Box, edits *boxEdits) ([]byte, error) {
	out := make([]byte, 0, len(data)+32)
	for _, box := range top {
		rendered, err := renderBox(data, box, edits)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered...)
	}
	return out, nil
}

func renderBox(data []byte, box *mp4Box, edits *boxEdits) ([]byte, error) {
	if replacement, ok := edits.replace[box]; ok {
		return append([]byte(nil), replacement...), nil
	}
	if box.kind == boxLeaf {
		return append([]byte(nil), data[box.start:box.end]...), nil
	}
	if box.childrenStart < box.payload || box.childrenStart > box.end {
		return nil, fmt.Errorf("%w: invalid child range for %s", ErrUnsupportedMedia, box.typ)
	}
	payload := make([]byte, 0, box.end-box.payload)
	payload = append(payload, data[box.payload:box.childrenStart]...)
	for _, child := range box.children {
		rendered, err := renderBox(data, child, edits)
		if err != nil {
			return nil, err
		}
		payload = append(payload, rendered...)
	}
	for _, child := range edits.insert[box] {
		payload = append(payload, child...)
	}
	return serializeBox(box, payload)
}

func serializeBox(box *mp4Box, payload []byte) ([]byte, error) {
	total := uint64(8 + len(payload))
	useLarge := box.large || total > math.MaxUint32
	if useLarge {
		total += 8
		if total > math.MaxUint64 {
			return nil, fmt.Errorf("%w: box %s is too large", ErrCannotAutoFix, box.typ)
		}
		out := make([]byte, 16+len(payload))
		binary.BigEndian.PutUint32(out[0:4], 1)
		copy(out[4:8], box.typ)
		binary.BigEndian.PutUint64(out[8:16], total)
		copy(out[16:], payload)
		return out, nil
	}
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(total))
	copy(out[4:8], box.typ)
	copy(out[8:], payload)
	return out, nil
}

func replacementBox(box *mp4Box, payload []byte) ([]byte, error) {
	return serializeBox(box, payload)
}

func makeColrBox(tags ColorTags) []byte {
	payload := make([]byte, 11)
	copy(payload[0:4], "nclx")
	binary.BigEndian.PutUint16(payload[4:6], uint16(tags.Primaries))
	binary.BigEndian.PutUint16(payload[6:8], uint16(tags.Transfer))
	binary.BigEndian.PutUint16(payload[8:10], uint16(tags.Matrix))
	if tags.FullRange {
		payload[10] = 0x80
	}
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], "colr")
	copy(out[8:], payload)
	return out
}
