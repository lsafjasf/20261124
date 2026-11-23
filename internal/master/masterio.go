package master

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// Master is an interleaved 10-bit RGB master. Codes are stored one per
// little-endian uint16 even though only the low ten bits are significant.
type Master struct {
	Width  int
	Height int
	Codes  []uint16
}

func NewMaster(width, height int) *Master {
	return &Master{Width: width, Height: height, Codes: make([]uint16, width*height*3)}
}

func (m *Master) Validate() error {
	if m.Width <= 0 || m.Height <= 0 {
		return fmt.Errorf("invalid master dimensions %dx%d", m.Width, m.Height)
	}
	if len(m.Codes) != m.Width*m.Height*3 {
		return fmt.Errorf("master code count %d does not match %dx%d", len(m.Codes), m.Width, m.Height)
	}
	return nil
}

func (m *Master) SetPixel(x, y int, rgb [3]uint16) {
	i := (y*m.Width + x) * 3
	m.Codes[i], m.Codes[i+1], m.Codes[i+2] = rgb[0], rgb[1], rgb[2]
}

func (m *Master) Pixel(x, y int) [3]uint16 {
	i := (y*m.Width + x) * 3
	return [3]uint16{m.Codes[i], m.Codes[i+1], m.Codes[i+2]}
}

// WriteRGB10LE writes width*height*3 little-endian uint16 code words. The
// dimensions and color metadata live in the sibling JSON manifest.
func WriteRGB10LE(path string, m *Master) (bytes int64, digest string, err error) {
	if err := m.Validate(); err != nil {
		return 0, "", err
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, "", err
	}
	ok := false
	defer func() {
		out.Close()
		if !ok {
			os.Remove(tmp)
		}
	}()
	h := sha256.New()
	cw := &countingWriter{w: io.MultiWriter(out, h)}
	bw := bufio.NewWriterSize(cw, 1<<20)
	var word [2]byte
	for _, code := range m.Codes {
		binary.LittleEndian.PutUint16(word[:], code)
		if _, err := bw.Write(word[:]); err != nil {
			return 0, "", err
		}
	}
	if err := bw.Flush(); err != nil {
		return 0, "", err
	}
	if err := out.Sync(); err != nil {
		return 0, "", err
	}
	if err := out.Close(); err != nil {
		return 0, "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, "", err
	}
	ok = true
	return cw.n, hex.EncodeToString(h.Sum(nil)), nil
}

func ReadRGB10LE(path string, width, height int) (*Master, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m := NewMaster(width, height)
	br := bufio.NewReaderSize(f, 1<<20)
	var word [2]byte
	for i := range m.Codes {
		if _, err := io.ReadFull(br, word[:]); err != nil {
			return nil, fmt.Errorf("read code %d: %w", i, err)
		}
		m.Codes[i] = binary.LittleEndian.Uint16(word[:])
	}
	var extra [1]byte
	if n, _ := br.Read(extra[:]); n != 0 {
		return nil, fmt.Errorf("unexpected trailing data in %s", path)
	}
	return m, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
