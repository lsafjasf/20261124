package pgs

import (
	"encoding/binary"
	"fmt"
)

func (s *Segment) parsePayload(p []byte) error {
	switch s.Type {
	case TypePalette:
		return s.parsePalette(p)
	case TypeWindow:
		return s.parseWindow(p)
	case TypeObject:
		return s.parseObject(p)
	case TypeDisplay:
		return s.parseDisplay(p)
	case TypeClear:
		if len(p) != 0 {
			return fmt.Errorf("clear segment carries %d unexpected payload bytes", len(p))
		}
		return nil
	default:
		return fmt.Errorf("unknown segment type 0x%02x", s.Type)
	}
}

func (s *Segment) parsePalette(p []byte) error {
	if len(p) < 2 || (len(p)-2)%5 != 0 {
		return fmt.Errorf("malformed palette payload: %d bytes", len(p))
	}
	pal := &PaletteSegment{ID: p[0], Version: p[1]}
	for i := 2; i+5 <= len(p); i += 5 {
		pal.Entries = append(pal.Entries, PaletteEntry{
			ID: p[i], Y: p[i+1], Cr: p[i+2], Cb: p[i+3], Alpha: p[i+4],
		})
	}
	s.Palette = pal
	return nil
}

func (s *Segment) parseWindow(p []byte) error {
	if len(p) < 1 {
		return fmt.Errorf("malformed window payload: %d bytes", len(p))
	}
	n := int(p[0])
	if len(p) != 1+9*n {
		return fmt.Errorf("window payload holds %d defs but %d bytes", n, len(p))
	}
	win := &WindowSegment{}
	for i := 0; i < n; i++ {
		q := p[1+9*i:]
		win.Windows = append(win.Windows, WindowDef{
			ID: q[0],
			X:  binary.BigEndian.Uint16(q[1:]),
			Y:  binary.BigEndian.Uint16(q[3:]),
			W:  binary.BigEndian.Uint16(q[5:]),
			H:  binary.BigEndian.Uint16(q[7:]),
		})
	}
	s.Window = win
	return nil
}

func (s *Segment) parseObject(p []byte) error {
	if len(p) < 4 {
		return fmt.Errorf("malformed object payload: %d bytes", len(p))
	}
	obj := &ObjectSegment{
		ID:         binary.BigEndian.Uint16(p[0:]),
		Version:    p[2],
		Sequence:   p[3],
		DataLength: uint32(p[4])<<16 | uint32(p[5])<<8 | uint32(p[6]),
	}
	body := p[7:]
	if obj.Sequence&SeqFirst != 0 {
		if len(body) < 4 {
			return fmt.Errorf("first object fragment missing width/height")
		}
		obj.Width = binary.BigEndian.Uint16(body[0:])
		obj.Height = binary.BigEndian.Uint16(body[2:])
		obj.Data = body[4:]
	} else {
		obj.Data = body
	}
	s.Object = obj
	return nil
}

func (s *Segment) parseDisplay(p []byte) error {
	if len(p) < 11 {
		return fmt.Errorf("malformed display payload: %d bytes", len(p))
	}
	d := &DisplaySegment{
		Width:             binary.BigEndian.Uint16(p[0:]),
		Height:            binary.BigEndian.Uint16(p[2:]),
		FrameRate:         p[4],
		CompositionNumber: binary.BigEndian.Uint16(p[5:]),
		CompositionState:  p[7],
		PaletteUpdate:     p[8] == 0x80,
		PaletteID:         p[9],
	}
	n := int(p[10])
	q := p[11:]
	for i := 0; i < n; i++ {
		if len(q) < 8 {
			return fmt.Errorf("display payload truncated at composition object %d", i)
		}
		co := CompositionObject{
			ObjectID: binary.BigEndian.Uint16(q[0:]),
			WindowID: q[2],
			Cropped:  q[3] == 0x40,
			X:        binary.BigEndian.Uint16(q[4:]),
			Y:        binary.BigEndian.Uint16(q[6:]),
		}
		q = q[8:]
		if co.Cropped {
			if len(q) < 8 {
				return fmt.Errorf("display payload truncated at cropping info %d", i)
			}
			q = q[8:]
		}
		d.Objects = append(d.Objects, co)
	}
	s.Display = d
	return nil
}
