package mp4

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Box struct {
	Type         string
	Start        int64
	Size         int64
	HeaderSize   int64
	PayloadStart int64
}

func (b Box) End() int64 { return b.Start + b.Size }

type File struct {
	r    io.ReaderAt
	Size int64
}

func NewFile(r io.ReaderAt, size int64) *File {
	return &File{r: r, Size: size}
}

func (f *File) Boxes(start, limit int64) ([]Box, error) {
	if start < 0 || limit < start || limit > f.Size {
		return nil, fmt.Errorf("invalid box range [%d,%d), file size %d", start, limit, f.Size)
	}
	var boxes []Box
	for off := start; off < limit; {
		if limit-off < 8 {
			return nil, fmt.Errorf("truncated box header at %d", off)
		}
		var head [16]byte
		if _, err := f.r.ReadAt(head[:8], off); err != nil {
			return nil, fmt.Errorf("read box header at %d: %w", off, err)
		}
		size := int64(binary.BigEndian.Uint32(head[0:4]))
		typ := string(head[4:8])
		header := int64(8)
		switch size {
		case 0:
			size = limit - off
		case 1:
			if limit-off < 16 {
				return nil, fmt.Errorf("truncated large box header at %d", off)
			}
			if _, err := f.r.ReadAt(head[8:16], off+8); err != nil {
				return nil, fmt.Errorf("read large box header at %d: %w", off, err)
			}
			large := binary.BigEndian.Uint64(head[8:16])
			if large > uint64(limit-off) {
				return nil, fmt.Errorf("box %q at %d has size %d beyond parent end %d",
					typ, off, large, limit)
			}
			size = int64(large)
			header = 16
		}
		if size < header {
			return nil, fmt.Errorf("box %q at %d has invalid size %d", typ, off, size)
		}
		if size > limit-off {
			return nil, fmt.Errorf("box %q at %d has size %d beyond parent end %d",
				typ, off, size, limit)
		}
		if typ == "uuid" {
			if size < header+16 {
				return nil, fmt.Errorf("truncated uuid box at %d", off)
			}
			header += 16
		}
		boxes = append(boxes, Box{
			Type: typ, Start: off, Size: size,
			HeaderSize: header, PayloadStart: off + header,
		})
		off += size
	}
	return boxes, nil
}

func (f *File) TopLevel() ([]Box, error) {
	return f.Boxes(0, f.Size)
}

func (f *File) ReadAt(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off+n > f.Size {
		return nil, fmt.Errorf("read [%d,%d) outside file size %d", off, off+n, f.Size)
	}
	buf := make([]byte, n)
	if n == 0 {
		return buf, nil
	}
	if _, err := f.r.ReadAt(buf, off); err != nil {
		return nil, err
	}
	return buf, nil
}

func (f *File) Payload(b Box, max int64) ([]byte, error) {
	n := b.Size - b.HeaderSize
	if max > 0 && n > max {
		n = max
	}
	return f.ReadAt(b.PayloadStart, n)
}

func find(boxes []Box, typ string) *Box {
	for i := range boxes {
		if boxes[i].Type == typ {
			return &boxes[i]
		}
	}
	return nil
}

func findAll(boxes []Box, typ string) []Box {
	var out []Box
	for _, b := range boxes {
		if b.Type == typ {
			out = append(out, b)
		}
	}
	return out
}

func (f *File) Children(parent Box) ([]Box, error) {
	return f.Boxes(parent.PayloadStart, parent.End())
}

func (f *File) ContainerBoxes() ([]Box, error) {
	boxes, err := f.Boxes(0, f.Size)
	if err != nil {
		return nil, err
	}
	var found []Box
	for _, t := range []string{"moov", "moof", "meta"} {
		for _, b := range findAll(boxes, t) {
			found = append(found, b)
		}
	}
	return found, nil
}
