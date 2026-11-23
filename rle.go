package vobsub

import (
	"errors"
	"fmt"
)

// DecodeRaster decodes the two interlaced DVD subtitle RLE fields.
func DecodeRaster(spu []byte, st State) (*Raster, error) {
	if st.Rect.Width <= 0 || st.Rect.Height <= 0 {
		return nil, errors.New("missing display area")
	}
	if st.Rect.Width > 4096 || st.Rect.Height > 4096 {
		return nil, fmt.Errorf("display area %dx%d is too large", st.Rect.Width, st.Rect.Height)
	}
	if len(spu) < 4 {
		return nil, errors.New("SPU too short")
	}
	control := int(spu[2])<<8 | int(spu[3])
	if control <= 4 || control > len(spu) {
		return nil, fmt.Errorf("invalid control offset %d", control)
	}
	topRows := (st.Rect.Height + 1) / 2
	botRows := st.Rect.Height / 2
	top, err := fieldBytes(spu, st.TopOffset, topRows, control)
	if err != nil {
		return nil, fmt.Errorf("top field: %w", err)
	}
	var bottom []byte
	if botRows > 0 {
		bottom, err = fieldBytes(spu, st.BotOffset, botRows, control)
		if err != nil {
			return nil, fmt.Errorf("bottom field: %w", err)
		}
	}

	r := &Raster{
		Width:  st.Rect.Width,
		Height: st.Rect.Height,
		Pixels: make([]uint8, st.Rect.Width*st.Rect.Height),
	}
	if err := decodeField(top, r, 0); err != nil {
		return r, fmt.Errorf("top field: %w", err)
	}
	if botRows > 0 {
		if err := decodeField(bottom, r, 1); err != nil {
			return r, fmt.Errorf("bottom field: %w", err)
		}
	}
	for _, p := range r.Pixels {
		if p < 4 {
			r.Used[p] = true
		}
	}
	return r, nil
}

func fieldBytes(spu []byte, off, rows, control int) ([]byte, error) {
	if off < 4 || off >= control {
		return nil, fmt.Errorf("offset %d outside pixel area 4..%d", off, control)
	}
	if rows <= 0 {
		return nil, errors.New("zero rows")
	}
	return spu[off:control], nil
}

func decodeField(src []byte, r *Raster, parity int) error {
	br := nibbleReader{data: src}
	rows := (r.Height + 1 - parity) / 2
	for row := 0; row < rows; row++ {
		y := row*2 + parity
		x := 0
		for x < r.Width {
			run, color, err := decodeRun(&br)
			if err != nil {
				return fmt.Errorf("row %d at x=%d: %w", y, x, err)
			}
			if run == fillRest {
				run = r.Width - x
			}
			if run > r.Width-x {
				return fmt.Errorf("row %d at x=%d: run %d exceeds width", y, x, run)
			}
			for i := 0; i < run; i++ {
				r.Pixels[y*r.Width+x+i] = color
			}
			x += run
		}
	}
	return nil
}

const fillRest = 1 << 30

func decodeRun(br *nibbleReader) (int, uint8, error) {
	var v uint32
	for t := uint32(1); v < t && t <= 0x40; t <<= 2 {
		n, err := br.readNibble()
		if err != nil {
			return 0, 0, err
		}
		v = v<<4 | uint32(n)
	}
	color := uint8(v & 3)
	if v < 4 {
		return fillRest, color, nil
	}
	return int(v >> 2), color, nil
}

type nibbleReader struct {
	data []byte
	bit  int
}

func (r *nibbleReader) readNibble() (uint8, error) {
	if r.bit+4 > len(r.data)*8 {
		return 0, errors.New("RLE data exhausted")
	}
	b := r.data[r.bit/8]
	var nib uint8
	if r.bit%8 == 0 {
		nib = b >> 4
	} else {
		nib = b & 0x0f
	}
	r.bit += 4
	return nib, nil
}
