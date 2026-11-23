// Command mkfixture writes the synthetic PGS scenario used by the tests and
// README. The stream contains two palette updates and several clipping cases.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type entry struct {
	index, y, cr, cb, t byte
}

type window struct {
	id   byte
	x, y int
	w, h int
}

type objectRef struct {
	id, win int
	x, y    int
}

func main() {
	out := flag.String("out", "testdata/scenario.sup", "output SUP path")
	flag.Parse()
	data := scenario()
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkfixture:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkfixture:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, len(data))
}

func scenario() []byte {
	var b builder
	objects := []objectRef{
		{id: 1, win: 0, x: 580, y: 170},
		{id: 2, win: 0, x: 620, y: 170},
		{id: 3, win: 1, x: 1850, y: 920},
	}

	b.segment(0, 0x16, pcs(1920, 1080, 1, 2, false, 0, objects))
	b.segment(0, 0x17, wds(
		window{id: 0, x: 600, y: 150, w: 300, h: 300},
		window{id: 1, x: 1800, y: 900, w: 300, h: 300},
	))
	b.segment(0, 0x14, pds(0, 0,
		entry{1, 235, 128, 128, 128},
		entry{2, 128, 128, 128, 128},
		entry{3, 180, 128, 128, 64},
	))
	b.segment(0, 0x15, ods(1, 0, 0xc0, 50, 20, 1))
	b.segment(0, 0x15, ods(2, 0, 0xc0, 20, 20, 2))
	b.segment(0, 0x15, ods(3, 0, 0xc0, 100, 100, 3))
	b.end(0)

	b.segment(90000, 0x16, pcs(1920, 1080, 2, 0, true, 0, nil))
	b.segment(90000, 0x14, pds(0, 1, entry{2, 128, 128, 128, 255}))
	b.end(90000)

	b.segment(180000, 0x16, pcs(1920, 1080, 3, 0, true, 0, nil))
	b.segment(180000, 0x14, pds(0, 2, entry{2, 128, 128, 128, 0}))
	b.end(180000)
	return b.bytes()
}

type builder struct{ buf bytes.Buffer }

func (b *builder) segment(pts uint32, typ byte, payload []byte) {
	var h [13]byte
	h[0], h[1] = 'P', 'G'
	binary.BigEndian.PutUint32(h[2:6], pts)
	binary.BigEndian.PutUint32(h[6:10], pts)
	h[10] = typ
	binary.BigEndian.PutUint16(h[11:13], uint16(len(payload)))
	b.buf.Write(h[:])
	b.buf.Write(payload)
}

func (b *builder) end(pts uint32) { b.segment(pts, 0x80, nil) }
func (b *builder) bytes() []byte  { return b.buf.Bytes() }

func pcs(w, h int, number uint16, state byte, paletteUpdate bool, paletteID byte, refs []objectRef) []byte {
	p := make([]byte, 11)
	binary.BigEndian.PutUint16(p[0:2], uint16(w))
	binary.BigEndian.PutUint16(p[2:4], uint16(h))
	p[4] = 0x10
	binary.BigEndian.PutUint16(p[5:7], number)
	p[7] = state << 6
	if paletteUpdate {
		p[8] = 0x80
	}
	p[9] = paletteID
	p[10] = byte(len(refs))
	for _, ref := range refs {
		o := []byte{byte(ref.id >> 8), byte(ref.id), byte(ref.win), 0}
		o = append(o, byte(ref.x>>8), byte(ref.x), byte(ref.y>>8), byte(ref.y))
		p = append(p, o...)
	}
	return p
}

func wds(windows ...window) []byte {
	p := []byte{byte(len(windows))}
	for _, w := range windows {
		p = append(p, w.id,
			byte(w.x>>8), byte(w.x), byte(w.y>>8), byte(w.y),
			byte(w.w>>8), byte(w.w), byte(w.h>>8), byte(w.h))
	}
	return p
}

func pds(id, version byte, entries ...entry) []byte {
	p := []byte{id, version}
	for _, e := range entries {
		p = append(p, e.index, e.y, e.cr, e.cb, e.t)
	}
	return p
}

func ods(id uint16, version, flags byte, w, h int, index byte) []byte {
	pixels := bytes.Repeat([]byte{index}, w*h)
	rle := rleEncode(pixels, w, h)
	dataLen := 4 + len(rle)
	p := []byte{byte(id >> 8), byte(id), version, flags,
		byte(dataLen >> 16), byte(dataLen >> 8), byte(dataLen),
		byte(w >> 8), byte(w), byte(h >> 8), byte(h)}
	return append(p, rle...)
}

func rleEncode(pixels []byte, w, h int) []byte {
	var out []byte
	for y := 0; y < h; y++ {
		for x := 0; x < w; {
			color := pixels[y*w+x]
			run := 1
			for x+run < w && pixels[y*w+x+run] == color {
				run++
			}
			switch {
			case color == 0 && run <= 63:
				out = append(out, 0, byte(run))
			case color == 0:
				out = append(out, 0, 0x40|byte(run>>8), byte(run))
			case run == 1:
				out = append(out, color)
			case run == 2:
				out = append(out, color, color)
			case run <= 63:
				out = append(out, 0, 0x80|byte(run), color)
			default:
				out = append(out, 0, 0xc0|byte(run>>8), byte(run), color)
			}
			x += run
		}
		out = append(out, 0, 0)
	}
	return out
}
