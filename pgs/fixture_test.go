package pgs

import (
	"bytes"
	"encoding/binary"
)

type supBuilder struct {
	buf bytes.Buffer
}

func (b *supBuilder) segment(pts uint32, typ byte, payload []byte) {
	var header [13]byte
	header[0], header[1] = 'P', 'G'
	binary.BigEndian.PutUint32(header[2:6], pts)
	binary.BigEndian.PutUint32(header[6:10], pts)
	header[10] = typ
	binary.BigEndian.PutUint16(header[11:13], uint16(len(payload)))
	b.buf.Write(header[:])
	b.buf.Write(payload)
}

func (b *supBuilder) end(pts uint32) {
	b.segment(pts, SegmentEnd, nil)
}

func (b *supBuilder) bytes() []byte { return b.buf.Bytes() }

func pcsPayload(width, height int, number uint16, state byte, paletteUpdate bool, paletteID byte, objects []compositionObject) []byte {
	p := make([]byte, 11)
	binary.BigEndian.PutUint16(p[0:2], uint16(width))
	binary.BigEndian.PutUint16(p[2:4], uint16(height))
	p[4] = 0x10
	binary.BigEndian.PutUint16(p[5:7], number)
	p[7] = state << 6
	if paletteUpdate {
		p[8] = 0x80
	}
	p[9] = paletteID
	p[10] = byte(len(objects))
	for _, obj := range objects {
		entry := make([]byte, 8)
		binary.BigEndian.PutUint16(entry[0:2], obj.id)
		entry[2] = obj.windowID
		if obj.cropped {
			entry[3] |= 0x80
		}
		if obj.forced {
			entry[3] |= 0x40
		}
		binary.BigEndian.PutUint16(entry[4:6], uint16(obj.x))
		binary.BigEndian.PutUint16(entry[6:8], uint16(obj.y))
		if obj.cropped {
			crop := make([]byte, 8)
			binary.BigEndian.PutUint16(crop[0:2], uint16(obj.crop.X))
			binary.BigEndian.PutUint16(crop[2:4], uint16(obj.crop.Y))
			binary.BigEndian.PutUint16(crop[4:6], uint16(obj.crop.Width))
			binary.BigEndian.PutUint16(crop[6:8], uint16(obj.crop.Height))
			entry = append(entry, crop...)
		}
		p = append(p, entry...)
	}
	return p
}

func wdsPayload(windows ...parsedWindow) []byte {
	p := []byte{byte(len(windows))}
	for _, w := range windows {
		entry := make([]byte, 9)
		entry[0] = w.id
		binary.BigEndian.PutUint16(entry[1:3], uint16(w.rect.X))
		binary.BigEndian.PutUint16(entry[3:5], uint16(w.rect.Y))
		binary.BigEndian.PutUint16(entry[5:7], uint16(w.rect.Width))
		binary.BigEndian.PutUint16(entry[7:9], uint16(w.rect.Height))
		p = append(p, entry...)
	}
	return p
}

type testPaletteEntry struct {
	index        byte
	y, cr, cb, t byte
}

func pdsPayload(id, version byte, entries ...testPaletteEntry) []byte {
	p := []byte{id, version}
	for _, e := range entries {
		p = append(p, e.index, e.y, e.cr, e.cb, e.t)
	}
	return p
}

func odsPayload(id uint16, version, flags byte, width, height int, pixels []byte) []byte {
	rle := encodeTestRLE(pixels, width, height)
	dataLen := 4 + len(rle)
	p := make([]byte, 7)
	binary.BigEndian.PutUint16(p[0:2], id)
	p[2] = version
	p[3] = flags
	p[4] = byte(dataLen >> 16)
	p[5] = byte(dataLen >> 8)
	p[6] = byte(dataLen)
	dim := make([]byte, 4)
	binary.BigEndian.PutUint16(dim[0:2], uint16(width))
	binary.BigEndian.PutUint16(dim[2:4], uint16(height))
	p = append(p, dim...)
	p = append(p, rle...)
	return p
}

func encodeTestRLE(pixels []byte, width, height int) []byte {
	var out []byte
	for y := 0; y < height; y++ {
		for x := 0; x < width; {
			color := pixels[y*width+x]
			run := 1
			for x+run < width && pixels[y*width+x+run] == color {
				run++
			}
			out = appendTestRun(out, color, run)
			x += run
		}
		out = append(out, 0, 0)
	}
	return out
}

func appendTestRun(out []byte, color byte, run int) []byte {
	if color == 0 {
		if run <= 63 {
			return append(out, 0, byte(run))
		}
		return append(out, 0, 0x40|byte(run>>8), byte(run))
	}
	if run == 1 {
		return append(out, color)
	}
	if run == 2 {
		return append(out, color, color)
	}
	if run <= 63 {
		return append(out, 0, 0x80|byte(run), color)
	}
	return append(out, 0, 0xc0|byte(run>>8), byte(run), color)
}

func solidPixels(width, height int, index byte) []byte {
	pixels := make([]byte, width*height)
	for i := range pixels {
		pixels[i] = index
	}
	return pixels
}
