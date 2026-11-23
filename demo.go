package vobsub

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const demoBlockSize = 4096

// WriteDemoPairs writes a deterministic .idx/.sub pair for tests and demos.
func WriteDemoPairs(dir string) (string, string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	pal := demoPalette()
	simple := makeDemoBitmap(64, 16, false)
	outlined := makeDemoBitmap(72, 18, true)
	animated := makeDemoBitmap(56, 16, true)
	missingStop := makeDemoBitmap(60, 14, false)

	type spec struct {
		start, end  int64
		bitmap      []uint8
		rect        Rect
		animated    bool
		missingStop bool
	}
	specs := []spec{
		{start: 1000, end: 2500, bitmap: simple, rect: Rect{X: 100, Y: 390, Width: 64, Height: 16}},
		{start: 3000, end: 4500, bitmap: outlined, rect: Rect{X: 260, Y: 382, Width: 72, Height: 18}},
		{start: 5000, end: 7000, bitmap: animated, rect: Rect{X: 430, Y: 370, Width: 56, Height: 16}, animated: true},
		{start: 8000, end: 0, bitmap: missingStop, rect: Rect{X: 590, Y: 390, Width: 60, Height: 14}, missingStop: true},
	}

	var sub bytes.Buffer
	var idx bytes.Buffer
	fmt.Fprintf(&idx, "# VobSub index file, v6 (do not modify this line!)\n")
	fmt.Fprintf(&idx, "size: 720x480\n")
	fmt.Fprintf(&idx, "palette: ")
	for i, c := range pal {
		if i > 0 {
			idx.WriteString(", ")
		}
		fmt.Fprintf(&idx, "%02x%02x%02x", c.R, c.G, c.B)
	}
	idx.WriteString("\nlangidx: 0\nid: und, index: 0\n")
	for i, item := range specs {
		end := item.end
		if end <= item.start {
			end = item.start + 1
		}
		spu, err := buildDemoSPU(item.bitmap, item.rect, end-item.start, item.animated, item.missingStop)
		if err != nil {
			return "", "", err
		}
		block, err := buildDemoBlock(spu)
		if err != nil {
			return "", "", err
		}
		filepos := int64(i * demoBlockSize)
		fmt.Fprintf(&idx, "timestamp: %s, filepos: %09x\n", formatIDXTime(item.start), filepos)
		sub.Write(block)
	}
	idxPath := filepath.Join(dir, "demo.idx")
	subPath := filepath.Join(dir, "demo.sub")
	if err := os.WriteFile(idxPath, idx.Bytes(), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(subPath, sub.Bytes(), 0o644); err != nil {
		return "", "", err
	}
	return idxPath, subPath, nil
}

func demoPalette() [16]RGB {
	var p [16]RGB
	p[0] = RGB{0, 0, 0}
	p[1] = RGB{255, 255, 255}
	p[2] = RGB{220, 32, 32}
	p[3] = RGB{40, 180, 90}
	for i := 4; i < 16; i++ {
		v := uint8(0x20 + i*8)
		p[i] = RGB{v, v, v}
	}
	return p
}

func makeDemoBitmap(w, h int, outline bool) []uint8 {
	pixels := make([]uint8, w*h)
	left, right, top, bottom := 3, w-4, 3, h-4
	if outline {
		left, right, top, bottom = 2, w-3, 2, h-3
		for y := top; y <= bottom; y++ {
			for x := left; x <= right; x++ {
				if x == left || x == right || y == top || y == bottom {
					pixels[y*w+x] = 2
				}
			}
		}
		left++
		right--
		top++
		bottom--
	}
	for y := top; y <= bottom; y++ {
		for x := left; x <= right; x++ {
			pixels[y*w+x] = 1
		}
	}
	// Carve a few non-text bars so the crop is visibly not a guessed glyph.
	for x := left + 4; x+4 <= right-2; x += 12 {
		for y := top + 3; y+2 <= bottom-2; y += 7 {
			pixels[y*w+x] = 0
			pixels[y*w+x+1] = 0
			pixels[(y+1)*w+x] = 0
		}
	}
	return pixels
}

func buildDemoSPU(pixels []uint8, rect Rect, durationMS int64, animated, missingStop bool) ([]byte, error) {
	if rect.Width <= 0 || rect.Height <= 0 || len(pixels) != rect.Width*rect.Height {
		return nil, fmt.Errorf("invalid demo bitmap")
	}
	raster := &Raster{Width: rect.Width, Height: rect.Height, Pixels: pixels}
	top := encodeDemoField(raster, 0)
	bottom := encodeDemoField(raster, 1)
	pixelData := append(append([]byte(nil), top...), bottom...)
	controlOffset := 4 + len(pixelData)
	if controlOffset > 65535 {
		return nil, fmt.Errorf("demo pixel data too large")
	}

	var sequences [][]byte
	endTick := msToTicks(durationMS)
	if endTick < 1 {
		endTick = 1
	}
	state := append([]byte(nil), demoStateCommands(rect, len(top))...)
	seq := append(append([]byte(nil), state...), 0x01, 0xff)
	sequences = append(sequences, seq)
	if animated {
		moved := rect
		moved.X += 24
		seq = append(append([]byte(nil), demoStateCommands(moved, len(top))...), 0xff)
		sequences = append(sequences, seq)
	}
	if !missingStop {
		sequences = append(sequences, []byte{0x02, 0xff})
	}

	var control bytes.Buffer
	seqOffsets := make([]int, len(sequences))
	for i, seq := range sequences {
		seqOffsets[i] = control.Len()
		control.Write([]byte{0, 0, 0, 0})
		control.Write(seq)
	}
	final := make([]byte, control.Len())
	copy(final, control.Bytes())
	for i := range sequences {
		var date int
		switch {
		case i == 0:
			date = 0
		case animated && len(sequences) == 3 && i == 1:
			date = int(endTick / 2)
		default:
			date = int(endTick)
		}
		if date > 65535 {
			return nil, fmt.Errorf("demo duration too long")
		}
		binary.BigEndian.PutUint16(final[seqOffsets[i]:], uint16(date))
		next := seqOffsets[i]
		if i+1 < len(sequences) {
			next = seqOffsets[i+1]
		}
		binary.BigEndian.PutUint16(final[seqOffsets[i]+2:], uint16(controlOffset+next))
	}

	total := controlOffset + len(final)
	if total > 65535 {
		return nil, fmt.Errorf("demo SPU too large")
	}
	spu := make([]byte, total)
	binary.BigEndian.PutUint16(spu[0:2], uint16(total))
	binary.BigEndian.PutUint16(spu[2:4], uint16(controlOffset))
	copy(spu[4:], pixelData)
	copy(spu[controlOffset:], final)
	return spu, nil
}

func demoStateCommands(rect Rect, topLen int) []byte {
	x1, x2 := rect.X, rect.X+rect.Width-1
	y1, y2 := rect.Y, rect.Y+rect.Height-1
	topOffset, botOffset := 4, 4+topLen
	return []byte{
		0x03, 0x32, 0x10,
		0x04, 0x0f, 0xf0,
		0x05,
		byte(x1 >> 4), byte(x1<<4) | byte(x2>>8), byte(x2),
		byte(y1 >> 4), byte(y1<<4) | byte(y2>>8), byte(y2),
		0x06, byte(topOffset >> 8), byte(topOffset),
		byte(botOffset >> 8), byte(botOffset),
	}
}

func encodeDemoField(r *Raster, parity int) []byte {
	var bits nibbleWriter
	rows := (r.Height + 1 - parity) / 2
	for row := 0; row < rows; row++ {
		y := row*2 + parity
		x := 0
		for x < r.Width {
			color := r.Pixels[y*r.Width+x] & 3
			run := 1
			for x+run < r.Width && (r.Pixels[y*r.Width+x+run]&3) == color {
				run++
			}
			left := run
			for left > 0 {
				n := left
				if n > 63 {
					n = 63
				}
				emitDemoRun(&bits, n, color)
				left -= n
			}
			x += run
		}
	}
	return bits.bytes()
}

func emitDemoRun(w *nibbleWriter, n int, color uint8) {
	v := uint8(n<<2) | (color & 3)
	switch {
	case n <= 3:
		w.writeNibble(v)
	case n <= 15:
		w.writeNibble(v >> 4)
		w.writeNibble(v & 0x0f)
	default:
		w.writeNibble(0)
		w.writeNibble(v >> 4)
		w.writeNibble(v & 0x0f)
	}
}

type nibbleWriter struct {
	buf []byte
	odd bool
}

func (w *nibbleWriter) writeNibble(n uint8) {
	n &= 0x0f
	if !w.odd {
		w.buf = append(w.buf, n<<4)
		w.odd = true
	} else {
		w.buf[len(w.buf)-1] |= n
		w.odd = false
	}
}

func (w *nibbleWriter) bytes() []byte {
	return append([]byte(nil), w.buf...)
}

func buildDemoBlock(spu []byte) ([]byte, error) {
	payloadLen := 3 + 1 + len(spu) // PES flags/header length, substream id, SPU
	if payloadLen > 65535 {
		return nil, fmt.Errorf("demo PES too large")
	}
	block := make([]byte, demoBlockSize)
	pack := []byte{0x00, 0x00, 0x01, 0xba, 0x44, 0x00, 0x04, 0x00, 0x04, 0x01, 0x01, 0x89, 0xc3, 0xf8}
	copy(block, pack)
	pos := len(pack)
	block[pos], block[pos+1], block[pos+2], block[pos+3] = 0x00, 0x00, 0x01, 0xbd
	binary.BigEndian.PutUint16(block[pos+4:], uint16(payloadLen))
	block[pos+6], block[pos+7], block[pos+8] = 0x80, 0x00, 0x00
	block[pos+9] = 0x20
	copy(block[pos+10:], spu)
	pos += 6 + payloadLen
	if pos+6 <= len(block) {
		block[pos], block[pos+1], block[pos+2], block[pos+3] = 0x00, 0x00, 0x01, 0xbe
		n := len(block) - pos - 6
		binary.BigEndian.PutUint16(block[pos+4:], uint16(n))
		for i := 0; i < n; i++ {
			block[pos+6+i] = 0xff
		}
	}
	return block, nil
}

func msToTicks(ms int64) int {
	return int((ms*90 + 512) / 1024)
}

func formatIDXTime(ms int64) string {
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d:%03d", h, m, s, ms)
}
